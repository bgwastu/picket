package hub

import (
	"errors"
	"net/http"
	"strings"

	"github.com/henrygd/beszel/internal/hub/ws"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// handleAgentConnect authenticates an agent before upgrading its outbound connection.
func (h *Hub) handleAgentConnect(e *core.RequestEvent) error {
	token, err := bearerToken(e.Request.Header.Get("Authorization"))
	if err != nil {
		return e.String(http.StatusUnauthorized, "Invalid token")
	}

	systemRecord, err := h.FindFirstRecordByFilter("systems", "token = {:token}", dbx.Params{"token": token})
	if err != nil {
		return e.String(http.StatusUnauthorized, "Invalid token")
	}

	conn, err := ws.GetUpgrader().Upgrade(e.Response, e.Request)
	if err != nil {
		return err
	}
	wsConn := ws.NewWsConnection(conn)
	conn.Session().Store("wsConn", wsConn)
	go conn.ReadLoop()

	if err := h.sm.AddWebSocketSystem(systemRecord.Id, wsConn); err != nil {
		wsConn.Close([]byte(err.Error()))
		return err
	}
	h.ssh.attachAgent(systemRecord.Id, wsConn)
	return nil
}

func bearerToken(authorization string) (string, error) {
	scheme, token, ok := strings.Cut(authorization, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.ContainsAny(token, " \t\r\n") {
		return "", errors.New("invalid authorization header")
	}
	return token, nil
}
