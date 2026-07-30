package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
)

// HandlerContext provides context for request handlers
type HandlerContext struct {
	Client       *WebSocketClient
	Agent        *Agent
	Request      *common.HubRequest[cbor.RawMessage]
	RequestID    *uint32
	SendResponse func(data any, requestID *uint32) error
}

// RequestHandler defines the interface for handling specific websocket request types
type RequestHandler interface {
	// Handle processes the request and returns an error if unsuccessful
	Handle(hctx *HandlerContext) error
}

// Responder sends handler responses back to the hub.
type Responder interface {
	SendResponse(data any, requestID *uint32) error
}

// HandlerRegistry manages the mapping between actions and their handlers
type HandlerRegistry struct {
	handlers map[common.WebSocketAction]RequestHandler
}

// NewHandlerRegistry creates a new handler registry with default handlers
func NewHandlerRegistry() *HandlerRegistry {
	registry := &HandlerRegistry{
		handlers: make(map[common.WebSocketAction]RequestHandler),
	}

	registry.Register(common.GetData, &GetDataHandler{})
	registry.Register(common.GetContainerLogs, &GetContainerLogsHandler{})
	registry.Register(common.GetContainerInfo, &GetContainerInfoHandler{})
	registry.Register(common.UninstallAgent, &UninstallAgentHandler{})

	return registry
}

// UninstallAgentHandler removes the files created by Picket's systemd installer.
// It responds first, then schedules cleanup so the acknowledgement can cross
// the WebSocket before the service is stopped.
type UninstallAgentHandler struct{}

func (h *UninstallAgentHandler) Handle(hctx *HandlerContext) error {
	if os.Geteuid() != 0 {
		return hctx.SendResponse(common.UninstallAgentResponse{Message: "agent is not running as root"}, hctx.RequestID)
	}
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		return hctx.SendResponse(common.UninstallAgentResponse{Message: "systemd is not available; remove this installation manually"}, hctx.RequestID)
	}
	if err := hctx.SendResponse(common.UninstallAgentResponse{Uninstalled: true, Message: "uninstall scheduled"}, hctx.RequestID); err != nil {
		return err
	}
	go func() {
		time.Sleep(500 * time.Millisecond)
		_ = exec.Command("systemctl", "disable", "--now", "picket-agent.service").Run()
		for _, file := range []string{
			"/usr/local/bin/picket-agent",
			"/usr/local/libexec/picket-agent-runner",
			"/etc/systemd/system/picket-agent.service",
			"/etc/picket/agent.env",
		} {
			_ = os.Remove(file)
		}
		_ = exec.Command("systemctl", "daemon-reload").Run()
	}()
	return nil
}

// Register registers a handler for a specific action type
func (hr *HandlerRegistry) Register(action common.WebSocketAction, handler RequestHandler) {
	hr.handlers[action] = handler
}

// Handle routes the request to the appropriate handler
func (hr *HandlerRegistry) Handle(hctx *HandlerContext) error {
	handler, exists := hr.handlers[hctx.Request.Action]
	if !exists {
		return fmt.Errorf("unknown action: %d", hctx.Request.Action)
	}

	// Log handler execution for debugging
	// slog.Debug("Executing handler", "action", hctx.Request.Action)

	return handler.Handle(hctx)
}

// GetHandler returns the handler for a specific action
func (hr *HandlerRegistry) GetHandler(action common.WebSocketAction) (RequestHandler, bool) {
	handler, exists := hr.handlers[action]
	return handler, exists
}

////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////

// GetDataHandler handles system data requests
type GetDataHandler struct{}

func (h *GetDataHandler) Handle(hctx *HandlerContext) error {
	var options common.DataRequestOptions
	_ = cbor.Unmarshal(hctx.Request.Data, &options)

	sysStats := hctx.Agent.gatherStats(options)
	return hctx.SendResponse(sysStats, hctx.RequestID)
}

////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////

// GetContainerLogsHandler handles container log requests
type GetContainerLogsHandler struct{}

func (h *GetContainerLogsHandler) Handle(hctx *HandlerContext) error {
	if hctx.Agent.dockerManager == nil {
		return hctx.SendResponse("", hctx.RequestID)
	}

	var req common.ContainerLogsRequest
	if err := cbor.Unmarshal(hctx.Request.Data, &req); err != nil {
		return err
	}

	ctx := context.Background()
	logContent, err := hctx.Agent.dockerManager.getLogs(ctx, req.ContainerID)
	if err != nil {
		return err
	}

	return hctx.SendResponse(logContent, hctx.RequestID)
}

////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////

// GetContainerInfoHandler handles container info requests
type GetContainerInfoHandler struct{}

func (h *GetContainerInfoHandler) Handle(hctx *HandlerContext) error {
	if hctx.Agent.dockerManager == nil {
		return hctx.SendResponse("", hctx.RequestID)
	}

	var req common.ContainerInfoRequest
	if err := cbor.Unmarshal(hctx.Request.Data, &req); err != nil {
		return err
	}

	ctx := context.Background()
	info, err := hctx.Agent.dockerManager.getContainerInfo(ctx, req.ContainerID)
	if err != nil {
		return err
	}

	return hctx.SendResponse(string(info), hctx.RequestID)
}
