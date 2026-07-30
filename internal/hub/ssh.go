package hub

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/user"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/hub/ws"
	"github.com/lxzan/gws"
	"github.com/pocketbase/pocketbase/core"
)

const (
	sshLaunchTTL = 5 * time.Minute
	sshIdleTTL   = 15 * time.Minute
	sshMaxTTL    = 12 * time.Hour
)

var sshUsernamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,31}$`)

var sshConnectorPlatforms = map[string]struct{}{
	"linux-amd64":  {},
	"linux-arm64":  {},
	"darwin-amd64": {},
	"darwin-arm64": {},
}

type sshLaunch struct {
	SystemID string
	Username string
	Expires  time.Time
	Used     bool
}

type sshTunnel struct {
	mu       sync.Mutex
	launch   *sshLaunch
	userConn *gws.Conn
	agent    *ws.WsConn
	last     time.Time
	done     chan struct{}
}

type sshManager struct {
	mu      sync.Mutex
	launch  map[string]*sshLaunch
	tunnels map[string]*sshTunnel
	agents  map[string]*ws.WsConn
}

func newSSHManager() *sshManager {
	return &sshManager{launch: make(map[string]*sshLaunch), tunnels: make(map[string]*sshTunnel), agents: make(map[string]*ws.WsConn)}
}

func (m *sshManager) create(systemID, username string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	m.mu.Lock()
	m.launch[token] = &sshLaunch{SystemID: systemID, Username: username, Expires: time.Now().Add(sshLaunchTTL)}
	m.mu.Unlock()
	return token, nil
}

func (m *sshManager) take(token string) (*sshLaunch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	launch, ok := m.launch[token]
	if !ok || launch.Used || time.Now().After(launch.Expires) {
		return nil, errors.New("invalid or expired SSH launch")
	}
	launch.Used = true
	delete(m.launch, token)
	return launch, nil
}

func (m *sshManager) connectUser(e *core.RequestEvent, token string) error {
	launch, err := m.take(token)
	if err != nil {
		return e.UnauthorizedError("Invalid or expired SSH launch", nil)
	}
	conn, err := sshUpgrader().Upgrade(e.Response, e.Request)
	if err != nil {
		return err
	}
	tunnel := &sshTunnel{launch: launch, userConn: conn, last: time.Now(), done: make(chan struct{})}
	conn.Session().Store("sshManager", m)
	conn.Session().Store("sshToken", token)
	m.mu.Lock()
	m.tunnels[token] = tunnel
	if agent := m.agents[launch.SystemID]; agent != nil {
		tunnel.agent = agent
	}
	m.mu.Unlock()
	go m.expire(token, tunnel)
	go conn.ReadLoop()
	return nil
}

type sshHandler struct{ gws.BuiltinEventHandler }

func sshUpgrader() *gws.Upgrader {
	return gws.NewUpgrader(&sshHandler{}, &gws.ServerOption{})
}

func (h *sshHandler) OnMessage(conn *gws.Conn, message *gws.Message) {
	defer message.Close()
	managerValue, ok := conn.Session().Load("sshManager")
	tokenValue, tokenOK := conn.Session().Load("sshToken")
	if !ok || !tokenOK {
		return
	}
	var request common.HubRequest[cbor.RawMessage]
	if err := cbor.Unmarshal(message.Data.Bytes(), &request); err != nil || request.Action != common.SSHStream {
		return
	}
	var msg common.SSHStreamMessage
	if err := cbor.Unmarshal(request.Data, &msg); err != nil {
		return
	}
	managerValue.(*sshManager).forwardUserMessage(tokenValue.(string), &msg)
}

func (h *sshHandler) OnClose(conn *gws.Conn, _ error) {
	managerValue, ok := conn.Session().Load("sshManager")
	tokenValue, tokenOK := conn.Session().Load("sshToken")
	if ok && tokenOK {
		managerValue.(*sshManager).remove(tokenValue.(string))
	}
}

func (m *sshManager) attachAgent(systemID string, agent *ws.WsConn) {
	m.mu.Lock()
	m.agents[systemID] = agent
	m.mu.Unlock()
	agent.SetStreamHandler(func(message *gws.Message) bool {
		var request common.HubRequest[cbor.RawMessage]
		if err := cbor.Unmarshal(message.Data.Bytes(), &request); err != nil || request.Action != common.SSHStream {
			return false
		}
		var msg common.SSHStreamMessage
		if err := cbor.Unmarshal(request.Data, &msg); err != nil {
			return true
		}
		m.mu.Lock()
		var tunnel *sshTunnel
		for _, candidate := range m.tunnels {
			if candidate.launch.SystemID == systemID && candidate.agent == agent {
				tunnel = candidate
				break
			}
		}
		m.mu.Unlock()
		if tunnel == nil {
			return true
		}
		m.forwardAgentMessage(tunnel, &msg)
		return true
	})
	m.mu.Lock()
	for _, tunnel := range m.tunnels {
		if tunnel.launch.SystemID == systemID && tunnel.agent == nil {
			tunnel.agent = agent
			break
		}
	}
	m.mu.Unlock()
}

func (m *sshManager) forwardUserMessage(token string, msg *common.SSHStreamMessage) {
	m.mu.Lock()
	tunnel := m.tunnels[token]
	m.mu.Unlock()
	if tunnel == nil {
		return
	}
	tunnel.mu.Lock()
	tunnel.last = time.Now()
	agent := tunnel.agent
	tunnel.mu.Unlock()
	if agent != nil {
		_ = agent.SendStream(msg)
	}
}

func (m *sshManager) forwardAgentMessage(tunnel *sshTunnel, msg *common.SSHStreamMessage) {
	tunnel.mu.Lock()
	tunnel.last = time.Now()
	conn := tunnel.userConn
	tunnel.mu.Unlock()
	if conn == nil {
		return
	}
	streamData, _ := cbor.Marshal(msg)
	data, _ := cbor.Marshal(common.HubRequest[cbor.RawMessage]{Action: common.SSHStream, Data: streamData})
	if err := conn.WriteMessage(gws.OpcodeBinary, data); err != nil {
		_ = conn.WriteClose(1000, nil)
	}
}

func (m *sshManager) expire(token string, tunnel *sshTunnel) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			tunnel.mu.Lock()
			expired := time.Since(tunnel.last) >= sshIdleTTL || time.Since(tunnel.launch.Expires.Add(-sshLaunchTTL)) >= sshMaxTTL
			tunnel.mu.Unlock()
			if expired {
				_ = tunnel.userConn.WriteClose(1000, []byte("SSH idle timeout"))
				m.remove(token)
				return
			}
		case <-tunnel.done:
			return
		}
	}
}

func (m *sshManager) remove(token string) {
	m.mu.Lock()
	if tunnel := m.tunnels[token]; tunnel != nil {
		close(tunnel.done)
		delete(m.tunnels, token)
	}
	m.mu.Unlock()
}

func (h *Hub) createSSHLaunch(e *core.RequestEvent) error {
	systemID := e.Request.PathValue("id")
	if _, err := h.FindRecordById("systems", systemID); err != nil {
		return e.NotFoundError("System not found", err)
	}
	username := "root"
	if current, err := user.Current(); err == nil && current.Username != "" {
		username = current.Username
	}
	token, err := h.ssh.create(systemID, username)
	if err != nil {
		return e.InternalServerError("Unable to create SSH launch", err)
	}
	h.ssh.mu.Lock()
	launch := h.ssh.launch[token]
	h.ssh.mu.Unlock()
	return e.JSON(http.StatusOK, map[string]any{"token": token, "expiresAt": launch.Expires, "expiresIn": int(sshLaunchTTL.Seconds()), "idleTimeout": int(sshIdleTTL.Seconds()), "username": username})
}

func (h *Hub) revokeSSHLaunch(e *core.RequestEvent) error {
	token := e.Request.PathValue("token")
	h.ssh.mu.Lock()
	delete(h.ssh.launch, token)
	tunnel := h.ssh.tunnels[token]
	if tunnel != nil {
		delete(h.ssh.tunnels, token)
	}
	h.ssh.mu.Unlock()
	if tunnel != nil {
		select {
		case <-tunnel.done:
		default:
			close(tunnel.done)
		}
		_ = tunnel.userConn.WriteClose(1000, []byte("SSH access revoked"))
	}
	return e.JSON(http.StatusOK, map[string]bool{"revoked": true})
}

func (h *Hub) uninstallAgent(e *core.RequestEvent) error {
	system, err := h.sm.GetSystem(e.Request.PathValue("id"))
	if err != nil {
		return e.NotFoundError("System is not connected", err)
	}
	if err := system.UninstallAgent(); err != nil {
		return e.InternalServerError("Unable to uninstall agent", err)
	}
	return e.JSON(http.StatusOK, map[string]any{"uninstalled": true})
}

func (h *Hub) getSSHLauncher(e *core.RequestEvent) error {
	token := e.Request.PathValue("token")
	h.ssh.mu.Lock()
	launch, ok := h.ssh.launch[token]
	h.ssh.mu.Unlock()
	if !ok || time.Now().After(launch.Expires) {
		return e.UnauthorizedError("Invalid or expired SSH launch", nil)
	}
	hubURL := strings.TrimSuffix(h.getAppURL(), "/")
	if hubURL == "" {
		hubURL = requestBaseURL(e)
	}
	script := fmt.Sprintf(`#!/bin/sh
set -eu
tmp="$(mktemp -d)"
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT INT TERM
hub_url=%q
token=%q
ssh_user=%q
system_id=%q
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$os/$arch" in
  linux/x86_64|linux/amd64) asset=linux-amd64 ;;
  linux/aarch64|linux/arm64) asset=linux-arm64 ;;
  darwin/x86_64) asset=darwin-amd64 ;;
  darwin/arm64) asset=darwin-arm64 ;;
  *) echo "unsupported platform: $os/$arch" >&2; exit 1 ;;
esac
curl -fsSL "$hub_url/api/picket/ssh-connector?os=$asset" -o "$tmp/picket-connect"
chmod 700 "$tmp/picket-connect"
non_interactive=false
identity=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --non-interactive) non_interactive=true; shift ;;
    --identity)
      [ "$#" -ge 2 ] || { echo "--identity requires a path" >&2; exit 2; }
      identity=$2
      shift 2
      ;;
    --) shift; break ;;
    *) break ;;
  esac
done
if [ -n "$identity" ]; then
  set -- -i "$identity" "$@"
fi
if [ "$non_interactive" = true ]; then
  known_hosts="${SSH_KNOWN_HOSTS:-$HOME/.ssh/known_hosts}"
  set -- -o BatchMode=yes -o StrictHostKeyChecking=yes -o "UserKnownHostsFile=$known_hosts" "$@"
fi
exec ssh -o "ProxyCommand=$tmp/picket-connect proxy --hub $hub_url --token $token" -o "HostKeyAlias=picket-system-$system_id" -o ForwardAgent=no -o ClearAllForwardings=yes -o ServerAliveInterval=0 -o ServerAliveCountMax=0 "$ssh_user@picket-system-$system_id" "$@"
`, hubURL, token, launch.Username, launch.SystemID)
	e.Response.Header().Set("Content-Type", "text/plain; charset=utf-8")
	return e.String(http.StatusOK, script)
}

func (h *Hub) serveSSHConnector(e *core.RequestEvent) error {
	asset := e.Request.URL.Query().Get("os")
	if _, ok := sshConnectorPlatforms[asset]; !ok {
		return e.BadRequestError("Unsupported connector platform", nil)
	}
	base := os.Getenv("PICKET_SSH_CONNECTOR_DIR")
	if base == "" {
		return e.NotFoundError("SSH connector is not configured", nil)
	}
	data, err := os.ReadFile(base + "/picket-connect_" + asset)
	if err != nil {
		return e.NotFoundError("SSH connector is unavailable", err)
	}
	e.Response.Header().Set("Content-Type", "application/octet-stream")
	return e.Blob(http.StatusOK, "application/octet-stream", data)
}
