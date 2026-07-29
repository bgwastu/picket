package systems

import (
	"errors"
	"fmt"

	"github.com/henrygd/beszel/internal/hub/ws"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/store"
)

// System status constants
const (
	up      string = "up"      // System is online and responding
	down    string = "down"    // System is offline or not responding
	paused  string = "paused"  // System monitoring is paused
	pending string = "pending" // System is waiting on initial connection result

	// interval is the default update interval in milliseconds (60 seconds)
	interval int = 60_000
	// interval int = 10_000 // Debug interval for faster updates

)

// errSystemExists is returned when attempting to add a system that already exists
var errSystemExists = errors.New("system exists")

// SystemManager manages a collection of monitored systems and their connections.
// It handles system lifecycle, status updates, and outbound WebSocket connections.
type SystemManager struct {
	hub     hubLike                       // Hub interface for database and alert operations
	systems *store.Store[string, *System] // Thread-safe store of active systems
}

// hubLike defines the interface requirements for the hub dependency.
// It extends core.App with system-specific functionality.
type hubLike interface {
	core.App
	HandleSystemAlerts(systemRecord *core.Record, data *system.CombinedData) error
	HandleStatusAlerts(status string, systemRecord *core.Record) error
	CancelPendingStatusAlerts(systemID string)
}

// NewSystemManager creates a new SystemManager instance with the provided hub.
// The hub must implement the hubLike interface to provide database and alert functionality.
func NewSystemManager(hub hubLike) *SystemManager {
	return &SystemManager{
		systems: store.New(map[string]*System{}),
		hub:     hub,
	}
}

// GetSystem returns a system by ID from the store
func (sm *SystemManager) GetSystem(systemID string) (*System, error) {
	sys, ok := sm.systems.GetOk(systemID)
	if !ok {
		return nil, fmt.Errorf("system not found")
	}
	return sys, nil
}

// Initialize binds lifecycle hooks. Agents add themselves after authenticating.
func (sm *SystemManager) Initialize() error {
	sm.bindEventHooks()
	return nil
}

// bindEventHooks registers event handlers for system record changes.
// These hooks ensure the system manager stays synchronized with database changes.
func (sm *SystemManager) bindEventHooks() {
	sm.hub.OnRecordCreate("systems").BindFunc(sm.onRecordCreate)
	sm.hub.OnRecordAfterCreateSuccess("systems").BindFunc(sm.onRecordAfterCreateSuccess)
	sm.hub.OnRecordUpdate("systems").BindFunc(sm.onRecordUpdate)
	sm.hub.OnRecordAfterUpdateSuccess("systems").BindFunc(sm.onRecordAfterUpdateSuccess)
	sm.hub.OnRecordAfterDeleteSuccess("systems").BindFunc(sm.onRecordAfterDeleteSuccess)
	sm.hub.OnRealtimeSubscribeRequest().BindFunc(sm.onRealtimeSubscribeRequest)
	sm.hub.OnRealtimeConnectRequest().BindFunc(sm.onRealtimeConnectRequest)
}

// onRecordCreate is called before a new system record is committed to the database.
// It initializes the record with default values: empty info and pending status.
func (sm *SystemManager) onRecordCreate(e *core.RecordEvent) error {
	e.Record.Set("info", system.Info{})
	e.Record.Set("status", pending)
	return e.Next()
}

// onRecordAfterCreateSuccess leaves the system pending until its agent connects.
func (sm *SystemManager) onRecordAfterCreateSuccess(e *core.RecordEvent) error {
	return e.Next()
}

// onRecordUpdate is called before a system record is updated in the database.
// It clears system info when the status is changed to paused.
func (sm *SystemManager) onRecordUpdate(e *core.RecordEvent) error {
	if e.Record.GetString("status") == paused {
		e.Record.Set("info", system.Info{})
	}
	return e.Next()
}

// onRecordAfterUpdateSuccess handles system record updates after they're committed to the database.
// It manages system lifecycle based on status changes and triggers appropriate alerts.
// Status transitions are handled as follows:
// - paused: Closes the WebSocket connection and deactivates alerts
// - pending: Starts monitoring (reuses WebSocket if available)
// - up: Triggers system alerts
// - down: Triggers status change alerts
func (sm *SystemManager) onRecordAfterUpdateSuccess(e *core.RecordEvent) error {
	newStatus := e.Record.GetString("status")
	prevStatus := pending
	system, ok := sm.systems.GetOk(e.Record.Id)
	if ok {
		prevStatus = system.Status
		system.Status = newStatus
		if e.Record.GetString("token") != e.Record.Original().GetString("token") {
			_ = system.setDown(nil)
			_ = sm.RemoveSystem(e.Record.Id)
			return e.Next()
		}
	}

	switch newStatus {
	case paused:
		if ok {
			system.closeWebSocketConnection()
		}
		_ = deactivateAlerts(e.App, e.Record.Id)
		sm.hub.CancelPendingStatusAlerts(e.Record.Id)
		return e.Next()
	case pending:
		// Resume monitoring, preferring existing WebSocket connection
		if ok && system.WsConn != nil {
			go system.update()
			return e.Next()
		}
		_ = deactivateAlerts(e.App, e.Record.Id)
		return e.Next()
	}

	// Handle systems not in manager
	if !ok {
		return sm.AddRecord(e.Record, nil)
	}

	// Trigger system alerts when system comes online
	if newStatus == up {
		if err := sm.hub.HandleSystemAlerts(e.Record, system.data); err != nil {
			e.App.Logger().Error("Error handling system alerts", "err", err)
		}
	}

	// Trigger status change alerts for up/down transitions
	if (newStatus == down && prevStatus == up) || (newStatus == up && prevStatus == down) {
		if err := sm.hub.HandleStatusAlerts(newStatus, e.Record); err != nil {
			e.App.Logger().Error("Error handling status alerts", "err", err)
		}
	}
	return e.Next()
}

// onRecordAfterDeleteSuccess is called after a system record is successfully deleted.
// It removes the system from the manager and cleans up all associated resources.
func (sm *SystemManager) onRecordAfterDeleteSuccess(e *core.RecordEvent) error {
	sm.RemoveSystem(e.Record.Id)
	return e.Next()
}

// AddSystem adds a system to the manager and starts monitoring it.
// It validates required fields, initializes the system context, and starts the update goroutine.
// Returns error if a system with the same ID already exists.
func (sm *SystemManager) AddSystem(sys *System) error {
	if sm.systems.Has(sys.Id) {
		return errSystemExists
	}
	if sys.Id == "" || sys.WsConn == nil {
		return errors.New("system missing required fields")
	}

	// Initialize system for monitoring
	sys.manager = sm
	sys.ctx, sys.cancel = sys.getContext()
	sys.data = &system.CombinedData{}
	sm.systems.Set(sys.Id, sys)

	// Start monitoring in background
	go sys.StartUpdater()
	return nil
}

// RemoveSystem removes a system from the manager and cleans up all associated resources.
// It cancels the system's context, closes all connections, and removes it from the store.
// Returns an error if the system is not found.
func (sm *SystemManager) RemoveSystem(systemID string) error {
	system, ok := sm.systems.GetOk(systemID)
	if !ok {
		return errors.New("system not found")
	}

	// Stop the update goroutine
	if system.cancel != nil {
		system.cancel()
	}

	system.closeWebSocketConnection()
	sm.systems.Remove(systemID)
	return nil
}

// AddRecord creates a System instance from a database record and adds it to the manager.
// If a system with the same ID already exists, it's removed first to ensure clean state.
// If no system instance is provided, a new one is created.
// This method is typically called when systems are created or their status changes to pending.
func (sm *SystemManager) AddRecord(record *core.Record, system *System) (err error) {
	// Remove existing system to ensure clean state
	if sm.systems.Has(record.Id) {
		_ = sm.RemoveSystem(record.Id)
	}

	// Create new system if none provided
	if system == nil {
		system = sm.NewSystem(record.Id)
	}

	// Populate system from record
	system.Status = record.GetString("status")
	system.Host = record.GetString("host")
	system.Port = record.GetString("port")

	return sm.AddSystem(system)
}

// AddWebSocketSystem creates and adds a system with an established WebSocket connection.
// This method is called when an agent connects via WebSocket with valid authentication.
// The system is immediately added to monitoring with the provided connection and version info.
func (sm *SystemManager) AddWebSocketSystem(systemId string, wsConn *ws.WsConn) error {
	systemRecord, err := sm.hub.FindRecordById("systems", systemId)
	if err != nil {
		return err
	}
	system := sm.NewSystem(systemId)
	system.WsConn = wsConn

	if err := sm.AddRecord(systemRecord, system); err != nil {
		return err
	}
	return nil
}

// deactivateAlerts finds all triggered alerts for a system and sets them to inactive.
// This is called when a system is paused or goes offline to prevent continued alerts.
func deactivateAlerts(app core.App, systemID string) error {
	// Note: Direct SQL updates don't trigger SSE, so we use the PocketBase API
	// _, err := app.DB().NewQuery(fmt.Sprintf("UPDATE alerts SET triggered = false WHERE system = '%s'", systemID)).Execute()

	alerts, err := app.FindRecordsByFilter("alerts", fmt.Sprintf("system = '%s' && triggered = 1", systemID), "", -1, 0)
	if err != nil {
		return err
	}

	for _, alert := range alerts {
		alert.Set("triggered", false)
		if err := app.SaveNoValidate(alert); err != nil {
			return err
		}
	}
	return nil
}
