package agent

import (
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/common"

	"github.com/fxamacker/cbor/v2"
	"github.com/lxzan/gws"
	"golang.org/x/net/proxy"
)

const (
	wsDeadline = 70 * time.Second
)

// WebSocketClient manages the WebSocket connection between the agent and hub.
// It handles authentication, message routing, and connection lifecycle management.
type WebSocketClient struct {
	gws.BuiltinEventHandler
	options            *gws.ClientOption                   // WebSocket client configuration options
	agent              *Agent                              // Reference to the parent agent
	Conn               *gws.Conn                           // Active WebSocket connection
	hubURL             *url.URL                            // Parsed hub URL for connection
	token              string                              // Authentication token for hub registration
	hubRequest         *common.HubRequest[cbor.RawMessage] // Reusable request structure for message parsing
	lastConnectAttempt time.Time                           // Timestamp of last connection attempt
	sshStreams         map[uint32]net.Conn                 // Active SSH tunnel sockets
	sshStreamsMu       sync.Mutex
	writeMu            sync.Mutex
}

// newWebSocketClient creates a new WebSocket client for the given agent.
// It reads configuration from environment variables and validates the hub URL.
func newWebSocketClient(agent *Agent) (client *WebSocketClient, err error) {
	hubURLStr, exists := utils.GetEnv("HUB_URL")
	if !exists {
		return nil, errors.New("HUB_URL environment variable not set")
	}

	client = &WebSocketClient{}
	client.sshStreams = make(map[uint32]net.Conn)

	client.hubURL, err = url.Parse(hubURLStr)
	if err != nil {
		return nil, errors.New("invalid hub URL")
	}
	// get registration token
	client.token, err = getToken()
	if err != nil {
		return nil, err
	}

	client.agent = agent
	client.hubRequest = &common.HubRequest[cbor.RawMessage]{}
	return client, nil
}

// getToken returns the token for the WebSocket client.
// It first checks the TOKEN environment variable, then the TOKEN_FILE environment variable.
// If neither is set, it returns an error.
func getToken() (string, error) {
	// get token from env var
	token, _ := utils.GetEnv("TOKEN")
	if token != "" {
		return token, nil
	}
	// get token from file
	tokenFile, _ := utils.GetEnv("TOKEN_FILE")
	if tokenFile == "" {
		return "", errors.New("must set TOKEN or TOKEN_FILE")
	}
	tokenBytes, err := os.ReadFile(tokenFile)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(tokenBytes)), nil
}

// getOptions returns the WebSocket client options, creating them if necessary.
// It configures the connection URL, TLS settings, and authentication headers.
func (client *WebSocketClient) getOptions() *gws.ClientOption {
	if client.options != nil {
		return client.options
	}

	// update the hub url to use websocket scheme and api path
	if client.hubURL.Scheme == "https" {
		client.hubURL.Scheme = "wss"
	} else {
		client.hubURL.Scheme = "ws"
	}
	client.hubURL.Path = path.Join(client.hubURL.Path, "api/picket/agent-connect")

	client.options = &gws.ClientOption{
		Addr:      client.hubURL.String(),
		TlsConfig: &tls.Config{InsecureSkipVerify: true},
		RequestHeader: http.Header{
			"Authorization": []string{"Bearer " + client.token},
		},
		NewDialer: func() (gws.Dialer, error) {
			return proxy.FromEnvironment(), nil
		},
	}
	return client.options
}

// Connect establishes a WebSocket connection to the hub.
// It closes any existing connection before attempting to reconnect.
func (client *WebSocketClient) Connect() (err error) {
	client.lastConnectAttempt = time.Now()

	// make sure previous connection is closed
	client.Close()

	client.Conn, _, err = gws.NewClient(client, client.getOptions())
	if err != nil {
		return err
	}

	go client.Conn.ReadLoop()

	return nil
}

// OnOpen handles WebSocket connection establishment.
// It sets a deadline for the connection to prevent hanging.
func (client *WebSocketClient) OnOpen(conn *gws.Conn) {
	conn.SetDeadline(time.Now().Add(wsDeadline))
	client.agent.connectionManager.eventChan <- WebSocketConnect
}

// OnClose handles WebSocket connection closure.
// It logs the closure reason and notifies the connection manager.
func (client *WebSocketClient) OnClose(conn *gws.Conn, err error) {
	if err != nil {
		slog.Warn("Connection closed", "err", strings.TrimPrefix(err.Error(), "gws: "))
	}
	client.agent.connectionManager.eventChan <- WebSocketDisconnect
}

// OnMessage handles incoming WebSocket messages from the hub.
// It decodes CBOR messages and routes them to appropriate handlers.
func (client *WebSocketClient) OnMessage(conn *gws.Conn, message *gws.Message) {
	defer message.Close()
	conn.SetDeadline(time.Now().Add(wsDeadline))

	if message.Opcode != gws.OpcodeBinary {
		return
	}

	var streamRequest common.HubRequest[cbor.RawMessage]
	if err := cbor.Unmarshal(message.Data.Bytes(), &streamRequest); err == nil && streamRequest.Action == common.SSHStream {
		var stream common.SSHStreamMessage
		if err := cbor.Unmarshal(streamRequest.Data, &stream); err != nil {
			return
		}
		if stream.Magic != common.SSHStreamMagic {
			return
		}
		if err := client.handleSSHStream(&stream); err != nil {
			slog.Error("Error handling SSH stream", "err", err)
		}
		return
	}

	var HubRequest common.HubRequest[cbor.RawMessage]

	err := cbor.Unmarshal(message.Data.Bytes(), &HubRequest)
	if err != nil {
		slog.Error("Error parsing message", "err", err)
		return
	}

	if err := client.handleHubRequest(&HubRequest, HubRequest.Id); err != nil {
		slog.Error("Error handling message", "err", err)
	}
}

func (client *WebSocketClient) handleSSHStream(msg *common.SSHStreamMessage) error {
	switch msg.Type {
	case common.SSHStreamOpen:
		client.sshStreamsMu.Lock()
		if len(client.sshStreams) >= 2 {
			client.sshStreamsMu.Unlock()
			return client.sendSSHStream(&common.SSHStreamMessage{StreamID: msg.StreamID, Type: common.SSHStreamOpenError, Error: "SSH session limit reached"})
		}
		client.sshStreamsMu.Unlock()
		conn, err := net.DialTimeout("tcp", "127.0.0.1:22", 5*time.Second)
		if err != nil {
			return client.sendSSHStream(&common.SSHStreamMessage{StreamID: msg.StreamID, Type: common.SSHStreamOpenError, Error: "unable to connect to local SSH server"})
		}
		client.sshStreamsMu.Lock()
		client.sshStreams[msg.StreamID] = conn
		client.sshStreamsMu.Unlock()
		if err := client.sendSSHStream(&common.SSHStreamMessage{StreamID: msg.StreamID, Type: common.SSHStreamOpenOK}); err != nil {
			_ = conn.Close()
			client.sshStreamsMu.Lock()
			delete(client.sshStreams, msg.StreamID)
			client.sshStreamsMu.Unlock()
			return err
		}
		go client.readSSHStream(msg.StreamID, conn)
	case common.SSHStreamData:
		client.sshStreamsMu.Lock()
		conn := client.sshStreams[msg.StreamID]
		client.sshStreamsMu.Unlock()
		if conn == nil {
			return nil
		}
		_, err := conn.Write(msg.Data)
		if err != nil {
		}
		return err
	case common.SSHStreamEOF, common.SSHStreamClose:
		client.sshStreamsMu.Lock()
		conn := client.sshStreams[msg.StreamID]
		delete(client.sshStreams, msg.StreamID)
		client.sshStreamsMu.Unlock()
		if conn != nil {
			_ = conn.Close()
		}
	}
	return nil
}

func (client *WebSocketClient) readSSHStream(streamID uint32, conn net.Conn) {
	buf := make([]byte, 32*1024)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			data := append([]byte(nil), buf[:n]...)
			if sendErr := client.sendSSHStream(&common.SSHStreamMessage{StreamID: streamID, Type: common.SSHStreamData, Data: data}); sendErr != nil {
				break
			}
		}
		if err != nil {
			_ = client.sendSSHStream(&common.SSHStreamMessage{StreamID: streamID, Type: common.SSHStreamEOF})
			break
		}
	}
	_ = conn.Close()
	client.sshStreamsMu.Lock()
	delete(client.sshStreams, streamID)
	client.sshStreamsMu.Unlock()
}

func (client *WebSocketClient) sendSSHStream(msg *common.SSHStreamMessage) error {
	streamData, err := cbor.Marshal(msg)
	if err != nil {
		return err
	}
	data, err := cbor.Marshal(common.HubRequest[cbor.RawMessage]{Action: common.SSHStream, Data: streamData})
	if err != nil {
		return err
	}
	client.writeMu.Lock()
	err = client.Conn.WriteMessage(gws.OpcodeBinary, data)
	client.writeMu.Unlock()
	return err
}

// OnPing handles WebSocket ping frames.
// It responds with a pong and updates the connection deadline.
func (client *WebSocketClient) OnPing(conn *gws.Conn, message []byte) {
	conn.SetDeadline(time.Now().Add(wsDeadline))
	conn.WritePong(message)
}

// Close closes the WebSocket connection gracefully.
// This method is safe to call multiple times.
func (client *WebSocketClient) Close() {
	if client.Conn != nil {
		_ = client.Conn.WriteClose(1000, nil)
	}
	client.sshStreamsMu.Lock()
	for id, conn := range client.sshStreams {
		_ = conn.Close()
		delete(client.sshStreams, id)
	}
	client.sshStreamsMu.Unlock()
}

// handleHubRequest routes the request to the appropriate handler using the handler registry.
func (client *WebSocketClient) handleHubRequest(msg *common.HubRequest[cbor.RawMessage], requestID *uint32) error {
	ctx := &HandlerContext{
		Client:       client,
		Agent:        client.agent,
		Request:      msg,
		RequestID:    requestID,
		SendResponse: client.sendResponse,
	}
	return client.agent.handlerRegistry.Handle(ctx)
}

// sendMessage encodes the given data to CBOR and sends it as a binary message over the WebSocket connection to the hub.
func (client *WebSocketClient) sendMessage(data any) error {
	bytes, err := cbor.Marshal(data)
	if err != nil {
		return err
	}
	client.writeMu.Lock()
	err = client.Conn.WriteMessage(gws.OpcodeBinary, bytes)
	client.writeMu.Unlock()
	if err != nil {
		// If writing fails (e.g., broken pipe due to network issues),
		// close the connection to trigger reconnection logic (#1263)
		client.Close()
	}
	return err
}

// sendResponse sends a response with optional request ID.
func (client *WebSocketClient) sendResponse(data any, requestID *uint32) error {
	return client.sendMessage(newAgentResponse(data, requestID))
}
