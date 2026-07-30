package hub

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/alerts"
	"github.com/henrygd/beszel/internal/hub/systems"
	"github.com/henrygd/beszel/internal/hub/utils"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

var containerIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{12,64}$`)

// registerMiddlewares registers custom middlewares
func (h *Hub) registerMiddlewares(se *core.ServeEvent) {
	se.Router.BindFunc(func(e *core.RequestEvent) error {
		path := e.Request.URL.Path
		if h.getAppURL() == "" {
			h.setAppURL(requestBaseURL(e))
			settings := h.Settings()
			settings.Meta.AppURL = h.getAppURL()
			_ = h.Save(settings)
		}
		if h.dashboardPasswordHash != "" && !h.isPublicRequest(e) && !hasDashboardSession(e.Request.Header.Get("Cookie"), h.dashboardPasswordHash) {
			return e.UnauthorizedError("Dashboard password required", nil)
		}
		if path == "/_" || strings.HasPrefix(path, "/_/") || isBlockedPocketBaseAPI(path) {
			return e.NotFoundError("", nil)
		}
		return e.Next()
	})
}

func (h *Hub) isPublicRequest(e *core.RequestEvent) bool {
	path := e.Request.URL.Path
	if !strings.HasPrefix(path, "/api/") {
		return true
	}
	return path == "/api/health" || path == "/api/picket/auth" || path == "/api/picket/agent-connect" || path == "/api/picket/agent-binary" || path == "/api/picket/ssh-connect" || strings.HasPrefix(path, "/api/picket/ssh-launch/") || path == "/api/picket/ssh-connector" || strings.HasPrefix(path, "/api/picket/agent-install/")
}

func hasDashboardSession(cookieHeader, passwordHash string) bool {
	for _, cookie := range strings.Split(cookieHeader, ";") {
		if strings.TrimSpace(cookie) == "picket_session="+passwordHash {
			return true
		}
	}
	return false
}

func isBlockedPocketBaseAPI(path string) bool {
	for _, prefix := range []string{"/api/settings", "/api/logs", "/api/backups", "/api/crons"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	const collectionsPrefix = "/api/collections/"
	if path == "/api/collections" {
		return true
	}
	if !strings.HasPrefix(path, collectionsPrefix) {
		return false
	}
	name := strings.SplitN(strings.TrimPrefix(path, collectionsPrefix), "/", 2)[0]
	_, public := applicationCollections[name]
	return !public
}

// registerApiRoutes registers custom API routes
func (h *Hub) registerApiRoutes(se *core.ServeEvent) error {
	api := se.Router.Group("/api/picket")
	// get version
	api.GET("/info", h.getInfo)
	api.POST("/auth", h.authenticateDashboard)
	api.POST("/systems", h.createSystem)
	api.GET("/systems/{id}/install-script", h.getAgentInstallScript)
	api.GET("/systems/{id}/install-command", h.getAgentInstallCommand)
	api.GET("/agent-install/{token}", h.getAgentInstallByToken)
	api.POST("/systems/{id}/ssh-launch", h.createSSHLaunch)
	api.POST("/systems/{id}/uninstall-agent", h.uninstallAgent)
	api.GET("/ssh-connect", h.handleSSHConnect)
	api.GET("/ssh-launch/{token}", h.getSSHLauncher)
	api.DELETE("/ssh-launch/{token}", h.revokeSSHLaunch)
	api.GET("/ssh-connector", h.serveSSHConnector)
	api.GET("/agent-binary", h.serveAgentBinary)
	// send test notification
	api.POST("/test-notification", h.SendTestNotification)
	// handle agent websocket connection
	api.GET("/agent-connect", h.handleAgentConnect)
	api.POST("/alerts", alerts.UpsertAlerts)
	api.DELETE("/alerts", alerts.DeleteAlerts)
	api.GET("/notification-settings", h.getNotificationSettings)
	api.PUT("/notification-settings", h.updateNotificationSettings)
	// /containers routes
	if enabled, _ := utils.GetEnv("CONTAINER_DETAILS"); enabled != "false" {
		// get container logs
		api.GET("/containers/logs", h.getContainerLogs)
		// get container info
		api.GET("/containers/info", h.getContainerInfo)
	}
	return nil
}

func (h *Hub) authenticateDashboard(e *core.RequestEvent) error {
	var input struct {
		Password string `json:"password"`
	}
	if err := e.BindBody(&input); err != nil || h.dashboardPasswordHash == "" || subtle.ConstantTimeCompare([]byte(hashDashboardPassword(input.Password)), []byte(h.dashboardPasswordHash)) != 1 {
		return e.UnauthorizedError("Invalid password", nil)
	}
	e.Response.Header().Set("Set-Cookie", "picket_session="+h.dashboardPasswordHash+"; Path=/; HttpOnly; SameSite=Strict")
	return e.JSON(http.StatusOK, map[string]bool{"authenticated": true})
}

func (h *Hub) createSystem(e *core.RequestEvent) error {
	var input struct {
		Name string `json:"name"`
	}
	if err := e.BindBody(&input); err != nil || strings.TrimSpace(input.Name) == "" {
		return e.BadRequestError("A system name is required", err)
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return e.InternalServerError("Unable to generate agent token", err)
	}
	collection, err := e.App.FindCachedCollectionByNameOrId("systems")
	if err != nil {
		return e.InternalServerError("Systems collection is unavailable", err)
	}
	record := core.NewRecord(collection)
	record.Set("name", strings.TrimSpace(input.Name))
	record.Set("token", hex.EncodeToString(tokenBytes))
	record.Set("status", "pending")
	record.Set("info", map[string]any{})
	if err := e.App.Save(record); err != nil {
		return e.InternalServerError("Unable to create system", err)
	}
	return e.JSON(http.StatusOK, map[string]any{
		"system": record,
		"token":  record.GetString("token"),
	})
}

func (h *Hub) getAgentInstallScript(e *core.RequestEvent) error {
	record, err := e.App.FindRecordById("systems", e.Request.PathValue("id"))
	if err != nil {
		return e.NotFoundError("System not found", err)
	}
	return h.agentInstallScript(e, record)
}

func (h *Hub) getAgentInstallCommand(e *core.RequestEvent) error {
	record, err := e.App.FindRecordById("systems", e.Request.PathValue("id"))
	if err != nil {
		return e.NotFoundError("System not found", err)
	}
	hubURL := strings.TrimSuffix(h.getAppURL(), "/")
	if hubURL == "" {
		hubURL = requestBaseURL(e)
	}
	command := fmt.Sprintf("curl -fsSL %q | sudo sh", hubURL+"/api/picket/agent-install/"+record.GetString("token"))
	e.Response.Header().Set("Content-Type", "text/plain; charset=utf-8")
	return e.String(http.StatusOK, command)
}

func (h *Hub) getAgentInstallByToken(e *core.RequestEvent) error {
	record, err := e.App.FindFirstRecordByFilter("systems", "token = {:token}", dbx.Params{"token": e.Request.PathValue("token")})
	if err != nil {
		return e.UnauthorizedError("Invalid agent token", nil)
	}
	return h.agentInstallScript(e, record)
}

func (h *Hub) agentInstallScript(e *core.RequestEvent, record *core.Record) error {
	hubURL := strings.TrimSuffix(h.getAppURL(), "/")
	if hubURL == "" {
		hubURL = requestBaseURL(e)
	}
	script := fmt.Sprintf(`#!/bin/sh
set -eu
[ "$(id -u)" -eq 0 ] || { echo "Run this installer as root or with sudo." >&2; exit 1; }
HUB_URL=${HUB_URL:-%q}
TOKEN=${TOKEN:-%q}
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$os/$arch" in
  linux/x86_64|linux/amd64) asset=linux-amd64 ;;
  linux/aarch64|linux/arm64) asset=linux-arm64 ;;
  linux/armv7l|linux/armv7) asset=linux-armv7 ;;
  *) echo "unsupported platform: $os/$arch" >&2; exit 1 ;;
esac
AGENT_BIN=${AGENT_BIN:-/usr/local/bin/picket-agent}
RUNNER=/usr/local/libexec/picket-agent-runner
SERVICE_FILE=/etc/systemd/system/picket-agent.service
tmp="$AGENT_BIN.tmp"
cleanup() { rm -f "$tmp"; }
trap cleanup EXIT INT TERM
mkdir -p /etc/picket /usr/local/libexec
cat > /etc/picket/agent.env <<EOF
HUB_URL=$HUB_URL
TOKEN=$TOKEN
EOF
if ! command -v curl >/dev/null 2>&1; then echo "ERROR: curl is required" >&2; exit 1; fi
echo "Downloading Picket agent ($asset) from $HUB_URL..." >&2
if ! curl -fsSL "$HUB_URL/api/picket/agent-binary?token=$TOKEN&os=$asset" -o "$tmp"; then
  echo "ERROR: unable to download the Picket agent binary for $asset from $HUB_URL" >&2
  exit 1
fi
if [ ! -s "$tmp" ]; then
  echo "ERROR: hub returned an empty Picket agent binary for $asset" >&2
  exit 1
fi
chmod 0755 "$tmp"
mv "$tmp" "$AGENT_BIN"
cat > "$RUNNER" <<'EOF'
#!/bin/sh
set -eu
set -a
. /etc/picket/agent.env
set +a
if [ -z "${HUB_URL:-}" ] || [ -z "${TOKEN:-}" ]; then
  echo "ERROR: /etc/picket/agent.env must define HUB_URL and TOKEN" >&2
  exit 1
fi
BIN=/usr/local/bin/picket-agent
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$os/$arch" in
  linux/x86_64|linux/amd64) asset=linux-amd64 ;;
  linux/aarch64|linux/arm64) asset=linux-arm64 ;;
  linux/armv7l|linux/armv7) asset=linux-armv7 ;;
  *) echo "unsupported platform: $os/$arch" >&2; exit 1 ;;
esac
URL="$HUB_URL/api/picket/agent-binary?token=$TOKEN&os=$asset"
while :; do
  if ! curl -fsSL "$URL" -o "$BIN.next"; then
    echo "ERROR: unable to update Picket agent binary ($asset); retrying in 30 seconds" >&2
    sleep 30
    continue
  fi
  if [ ! -s "$BIN.next" ]; then
    echo "ERROR: hub returned an empty Picket agent binary ($asset); retrying in 30 seconds" >&2
    rm -f "$BIN.next"
    sleep 30
    continue
  fi
  chmod 0755 "$BIN.next"
  mv "$BIN.next" "$BIN"
  echo "Starting Picket agent ($asset)..." >&2
  "$BIN" &
  child=$!
  sleep 21600 &
  timer=$!
  while kill -0 "$child" 2>/dev/null; do
    if ! kill -0 "$timer" 2>/dev/null; then kill "$child" 2>/dev/null || true; break; fi
    sleep 5
  done
  kill "$timer" 2>/dev/null || true
  wait "$child" || echo "ERROR: Picket agent exited with status $?; restarting" >&2
done
EOF
chmod 0755 "$RUNNER"
cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=Picket monitoring agent
After=network-online.target
Wants=network-online.target
[Service]
EnvironmentFile=/etc/picket/agent.env
ExecStart=$RUNNER
Restart=always
RestartSec=5
User=root
[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable --now picket-agent.service
echo "Picket agent installed and started."
`, hubURL, record.GetString("token"))
	e.Response.Header().Set("Content-Type", "text/plain; charset=utf-8")
	return e.String(http.StatusOK, script)
}

func requestBaseURL(e *core.RequestEvent) string {
	scheme := e.Request.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
		if e.Request.TLS != nil {
			scheme = "https"
		}
	}
	if comma := strings.IndexByte(scheme, ','); comma >= 0 {
		scheme = scheme[:comma]
	}
	host := e.Request.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = e.Request.Host
	}
	return strings.TrimSuffix(strings.TrimSpace(scheme), "://") + "://" + host
}

func (h *Hub) serveAgentBinary(e *core.RequestEvent) error {
	token := e.Request.URL.Query().Get("token")
	if token == "" {
		return e.UnauthorizedError("Agent token required", nil)
	}
	if _, err := h.FindFirstRecordByFilter("systems", "token = {:token}", dbx.Params{"token": token}); err != nil {
		return e.UnauthorizedError("Invalid agent token", nil)
	}
	asset := e.Request.URL.Query().Get("os")
	if asset == "" {
		asset = "native"
	}
	paths := map[string]string{
		"linux-amd64": "/linux-amd64",
		"linux-arm64": "/linux-arm64",
		"linux-armv7": "/linux-armv7",
	}
	path := os.Getenv("PICKET_AGENT_BINARY")
	if binaryDir := os.Getenv("PICKET_AGENT_BINARY_DIR"); binaryDir != "" {
		if suffix, ok := paths[asset]; ok {
			path = strings.TrimSuffix(binaryDir, "/") + suffix
		}
	}
	if path == "" {
		return e.NotFoundError("Agent binary is not configured", nil)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return e.NotFoundError("Agent binary is unavailable", err)
	}
	e.Response.Header().Set("Content-Type", "application/octet-stream")
	e.Response.Header().Set("Content-Disposition", "attachment; filename=picket-agent")
	return e.Blob(http.StatusOK, "application/octet-stream", data)
}

func (h *Hub) handleSSHConnect(e *core.RequestEvent) error {
	token := e.Request.URL.Query().Get("token")
	if token == "" {
		return e.UnauthorizedError("SSH launch token required", nil)
	}
	return h.ssh.connectUser(e, token)
}

// getInfo returns public hub information.
func (h *Hub) getInfo(e *core.RequestEvent) error {
	type infoResponse struct {
		Version string `json:"v"`
	}
	info := infoResponse{
		Version: beszel.Version,
	}
	return e.JSON(http.StatusOK, info)
}

func (h *Hub) getNotificationSettings(e *core.RequestEvent) error {
	record, err := e.App.FindFirstRecordByFilter("notification_settings", "id = 'globalsettings1'")
	if err != nil {
		return e.InternalServerError("Notification settings are unavailable", err)
	}
	var settings alerts.NotificationSettings
	if err := record.UnmarshalJSONField("settings", &settings); err != nil {
		return e.InternalServerError("Invalid notification settings", err)
	}
	return e.JSON(http.StatusOK, settings)
}

func (h *Hub) updateNotificationSettings(e *core.RequestEvent) error {
	var settings alerts.NotificationSettings
	if err := e.BindBody(&settings); err != nil {
		return e.BadRequestError("Invalid notification settings", err)
	}
	if (settings.TelegramBotToken == "") != (len(settings.TelegramUserIDs) == 0) {
		return e.BadRequestError("Telegram bot token and allowed user IDs must be configured together", errors.New("incomplete Telegram settings"))
	}
	record, err := e.App.FindFirstRecordByFilter("notification_settings", "id = 'globalsettings1'")
	if err != nil {
		return e.InternalServerError("Notification settings are unavailable", err)
	}
	record.Set("settings", settings)
	if err := e.App.Save(record); err != nil {
		return e.InternalServerError("Unable to save notification settings", err)
	}
	return e.JSON(http.StatusOK, settings)
}

// containerRequestHandler handles both container logs and info requests
func (h *Hub) containerRequestHandler(e *core.RequestEvent, fetchFunc func(*systems.System, string) (string, error), responseKey string) error {
	systemID := e.Request.URL.Query().Get("system")
	containerID := e.Request.URL.Query().Get("container")

	if systemID == "" || containerID == "" || !containerIDPattern.MatchString(containerID) {
		return e.BadRequestError("Invalid system or container parameter", nil)
	}

	system, err := h.sm.GetSystem(systemID)
	if err != nil {
		return e.NotFoundError("", nil)
	}

	data, err := fetchFunc(system, containerID)
	if err != nil {
		return e.InternalServerError("", err)
	}

	return e.JSON(http.StatusOK, map[string]string{responseKey: data})
}

// getContainerLogs handles GET /api/picket/containers/logs requests
func (h *Hub) getContainerLogs(e *core.RequestEvent) error {
	return h.containerRequestHandler(e, func(system *systems.System, containerID string) (string, error) {
		return system.FetchContainerLogsFromAgent(containerID)
	}, "logs")
}

func (h *Hub) getContainerInfo(e *core.RequestEvent) error {
	return h.containerRequestHandler(e, func(system *systems.System, containerID string) (string, error) {
		return system.FetchContainerInfoFromAgent(containerID)
	}, "info")
}
