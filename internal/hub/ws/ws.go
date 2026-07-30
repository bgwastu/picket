package ws

import (
	"context"
	"log/slog"
	"sync"
	"time"
	"weak"

	"github.com/henrygd/beszel/internal/common"

	"github.com/fxamacker/cbor/v2"
	"github.com/lxzan/gws"
)

const (
	deadline = 70 * time.Second
)

// Handler implements the WebSocket event handler for agent connections.
type Handler struct {
	gws.BuiltinEventHandler
}

// WsConn represents a WebSocket connection to an agent.
type WsConn struct {
	conn           *gws.Conn
	requestManager *RequestManager
	DownChan       chan struct{}
	streamHandler  func(*gws.Message) bool
	writeMu        sync.Mutex
}

var upgrader *gws.Upgrader

// GetUpgrader returns a singleton WebSocket upgrader instance.
func GetUpgrader() *gws.Upgrader {
	if upgrader != nil {
		return upgrader
	}
	handler := &Handler{}
	upgrader = gws.NewUpgrader(handler, &gws.ServerOption{})
	return upgrader
}

// NewWsConnection creates a new WebSocket connection wrapper.
func NewWsConnection(conn *gws.Conn) *WsConn {
	ws := &WsConn{
		conn:     conn,
		DownChan: make(chan struct{}, 1),
	}
	ws.requestManager = NewRequestManager(conn, func(data []byte) error {
		ws.writeMu.Lock()
		defer ws.writeMu.Unlock()
		return conn.WriteMessage(gws.OpcodeBinary, data)
	})
	return ws
}

// SetStreamHandler installs a handler for non-request stream messages.
func (ws *WsConn) SetStreamHandler(handler func(*gws.Message) bool) {
	ws.streamHandler = handler
}

// SendStream sends a binary stream frame to the agent.
func (ws *WsConn) SendStream(data any) error {
	if ws.conn == nil {
		return gws.ErrConnClosed
	}
	streamData, err := cbor.Marshal(data)
	if err != nil {
		return err
	}
	bytes, err := cbor.Marshal(common.HubRequest[cbor.RawMessage]{Action: common.SSHStream, Data: streamData})
	if err != nil {
		return err
	}
	ws.writeMu.Lock()
	err = ws.conn.WriteMessage(gws.OpcodeBinary, bytes)
	ws.writeMu.Unlock()
	if err != nil {
		slog.Warn("Failed to send WebSocket stream", "err", err)
	}
	return err
}

// OnOpen sets a deadline for the WebSocket connection and extracts agent version.
func (h *Handler) OnOpen(conn *gws.Conn) {
	conn.SetDeadline(time.Now().Add(deadline))
}

// OnMessage routes incoming WebSocket messages to the request manager.
func (h *Handler) OnMessage(conn *gws.Conn, message *gws.Message) {
	conn.SetDeadline(time.Now().Add(deadline))
	if message.Opcode != gws.OpcodeBinary || message.Data.Len() == 0 {
		return
	}
	wsConn, ok := conn.Session().Load("wsConn")
	if !ok {
		_ = conn.WriteClose(1000, nil)
		return
	}
	ws := wsConn.(*WsConn)
	if ws.streamHandler != nil && ws.streamHandler(message) {
		return
	}
	ws.requestManager.handleResponse(message)
}

// OnClose handles WebSocket connection closures and triggers system down status after delay.
func (h *Handler) OnClose(conn *gws.Conn, err error) {
	wsConn, ok := conn.Session().Load("wsConn")
	if !ok {
		return
	}
	wsConn.(*WsConn).conn = nil
	// wait 5 seconds to allow reconnection before setting system down
	// use a weak pointer to avoid keeping references if the system is removed
	go func(downChan weak.Pointer[chan struct{}]) {
		time.Sleep(5 * time.Second)
		downChanValue := downChan.Value()
		if downChanValue != nil {
			*downChanValue <- struct{}{}
		}
	}(weak.Make(&wsConn.(*WsConn).DownChan))
}

// Close terminates the WebSocket connection gracefully.
func (ws *WsConn) Close(msg []byte) {
	if ws.IsConnected() {
		ws.conn.WriteClose(1000, msg)
	}
	if ws.requestManager != nil {
		ws.requestManager.Close()
	}
}

// Ping sends a ping frame to keep the connection alive.
func (ws *WsConn) Ping() error {
	if ws.conn == nil {
		return gws.ErrConnClosed
	}
	ws.conn.SetDeadline(time.Now().Add(deadline))
	return ws.conn.WritePing(nil)
}

// sendMessage encodes data to CBOR and sends it as a binary message to the agent.
// This is kept for backwards compatibility but new actions should use RequestManager.
func (ws *WsConn) sendMessage(data common.HubRequest[any]) error {
	if ws.conn == nil {
		return gws.ErrConnClosed
	}
	bytes, err := cbor.Marshal(data)
	if err != nil {
		return err
	}
	return ws.conn.WriteMessage(gws.OpcodeBinary, bytes)
}

// IsConnected returns true if the WebSocket connection is active.
func (ws *WsConn) IsConnected() bool {
	return ws.conn != nil
}

// SendRequest sends a request to the agent and returns a pending request handle.
// This is used by the transport layer to send requests.
func (ws *WsConn) SendRequest(ctx context.Context, action common.WebSocketAction, data any) (*PendingRequest, error) {
	return ws.requestManager.SendRequest(ctx, action, data)
}
