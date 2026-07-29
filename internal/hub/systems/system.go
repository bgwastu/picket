package systems

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"

	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/hub/transport"
	"github.com/henrygd/beszel/internal/hub/ws"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/entities/systemd"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type System struct {
	Id             string               `db:"id"`
	Host           string               `db:"host"`
	Port           string               `db:"port"`
	Status         string               `db:"status"`
	manager        *SystemManager       // Manager that this system belongs to
	data           *system.CombinedData // system data from agent
	ctx            context.Context      // Context for stopping the updater
	cancel         context.CancelFunc   // Stops and removes system from updater
	WsConn         *ws.WsConn           // Handler for agent WebSocket connection
	updateTicker   *time.Ticker         // Ticker for updating the system
	detailsFetched atomic.Bool          // True if static system details have been fetched and saved
}

func (sm *SystemManager) NewSystem(systemId string) *System {
	system := &System{
		Id:   systemId,
		data: &system.CombinedData{},
	}
	system.ctx, system.cancel = system.getContext()
	return system
}

// StartUpdater starts the system updater.
// It first fetches the data from the agent then updates the records.
// If the data is not found or the system is down, it sets the system down.
func (sys *System) StartUpdater() {
	// Channel that can be used to set the system down. Currently only used to
	// allow a short delay for reconnection after websocket connection is closed.
	var downChan chan struct{}

	// Add random jitter to first WebSocket connection to prevent
	// clustering if all agents are started at the same time.
	var jitter <-chan time.Time
	if sys.WsConn != nil {
		jitter = getJitter()
		// use the websocket connection's down channel to set the system down
		downChan = sys.WsConn.DownChan
	}

	// update immediately if system is not paused and connected
	if sys.Status != paused && sys.ctx.Err() == nil {
		if err := sys.update(); err != nil {
			_ = sys.setDown(err)
		}
	}

	sys.updateTicker = time.NewTicker(time.Duration(interval) * time.Millisecond)
	// Go 1.23+ will automatically stop the ticker when the system is garbage collected, however we seem to need this or testing/synctest will block even if calling runtime.GC()
	defer sys.updateTicker.Stop()

	for {
		select {
		case <-sys.ctx.Done():
			return
		case <-sys.updateTicker.C:
			if err := sys.update(); err != nil {
				_ = sys.setDown(err)
			}
		case <-downChan:
			sys.WsConn = nil
			downChan = nil
			_ = sys.setDown(nil)
		case <-jitter:
			sys.updateTicker.Reset(time.Duration(interval) * time.Millisecond)
			if err := sys.update(); err != nil {
				_ = sys.setDown(err)
			}
		}
	}
}

// update updates the system data and records.
func (sys *System) update() error {
	if sys.Status == paused {
		sys.handlePaused()
		return nil
	}
	options := common.DataRequestOptions{
		CacheTimeMs: uint16(interval),
	}
	// fetch system details if not already fetched
	if !sys.detailsFetched.Load() {
		options.IncludeDetails = true
	}

	data, err := sys.fetchDataFromAgent(options)
	if err != nil {
		return err
	}

	// ensure deprecated fields from older agents are migrated to current fields
	migrateDeprecatedFields(data, !sys.detailsFetched.Load())

	// create system records
	_, err = sys.createRecords(data)

	// If details were included and fetched successfully, mark them as fetched.
	if err == nil && data.Details != nil {
		sys.detailsFetched.Store(true)
	}

	return err
}

func (sys *System) handlePaused() {
	if sys.WsConn == nil {
		// if the system is paused and there's no websocket connection, remove the system
		_ = sys.manager.RemoveSystem(sys.Id)
	} else {
		// Send a ping to the agent to keep the connection alive if the system is paused
		if err := sys.WsConn.Ping(); err != nil {
			sys.manager.hub.Logger().Warn("Failed to ping agent", "system", sys.Id, "err", err)
			_ = sys.manager.RemoveSystem(sys.Id)
		}
	}
}

// createRecords updates the system record and adds system_stats and container_stats records
func (sys *System) createRecords(data *system.CombinedData) (*core.Record, error) {
	systemRecord, err := sys.getRecord(sys.manager.hub)
	if err != nil {
		return nil, err
	}
	hub := sys.manager.hub
	err = hub.RunInTransaction(func(txApp core.App) error {
		// add system_stats record
		systemStatsCollection, err := txApp.FindCachedCollectionByNameOrId("system_stats")
		if err != nil {
			return err
		}
		systemStatsRecord := core.NewRecord(systemStatsCollection)
		systemStatsRecord.Set("system", systemRecord.Id)
		systemStatsRecord.Set("stats", data.Stats)
		systemStatsRecord.Set("type", "1m")
		if err := txApp.SaveNoValidate(systemStatsRecord); err != nil {
			return err
		}

		// add containers and container_stats records
		if len(data.Containers) > 0 {
			if data.Containers[0].Id != "" {
				if err := createContainerRecords(txApp, data.Containers, sys.Id); err != nil {
					return err
				}
			}
			containerStatsCollection, err := txApp.FindCachedCollectionByNameOrId("container_stats")
			if err != nil {
				return err
			}
			containerStatsRecord := core.NewRecord(containerStatsCollection)
			containerStatsRecord.Set("system", systemRecord.Id)
			containerStatsRecord.Set("stats", data.Containers)
			containerStatsRecord.Set("type", "1m")
			if err := txApp.SaveNoValidate(containerStatsRecord); err != nil {
				return err
			}
		}

		// add new systemd_stats record
		if len(data.SystemdServices) > 0 {
			if err := createSystemdStatsRecords(txApp, data.SystemdServices, sys.Id); err != nil {
				return err
			}
		}

		// add system details record
		if data.Details != nil {
			if err := createSystemDetailsRecord(txApp, data.Details, sys.Id); err != nil {
				return err
			}
		}

		// update system record (do this last because it triggers alerts and we need above records to be inserted first)
		systemRecord.Set("status", up)
		systemRecord.Set("info", data.Info)
		if err := txApp.SaveNoValidate(systemRecord); err != nil {
			return err
		}
		return nil
	})

	return systemRecord, err
}

func createSystemDetailsRecord(app core.App, data *system.Details, systemId string) error {
	collectionName := "system_details"
	params := dbx.Params{
		"id":       systemId,
		"system":   systemId,
		"hostname": data.Hostname,
		"kernel":   data.Kernel,
		"cores":    data.Cores,
		"threads":  data.Threads,
		"cpu":      data.CpuModel,
		"os":       data.Os,
		"os_name":  data.OsName,
		"arch":     data.Arch,
		"memory":   data.MemoryTotal,
		"podman":   data.Podman,
		"updated":  time.Now().UTC(),
	}
	result, err := app.DB().Update(collectionName, params, dbx.HashExp{"id": systemId}).Execute()
	rowsAffected, _ := result.RowsAffected()
	if err != nil || rowsAffected == 0 {
		_, err = app.DB().Insert(collectionName, params).Execute()
	}
	return err
}

func createSystemdStatsRecords(app core.App, data []*systemd.Service, systemId string) error {
	if len(data) == 0 {
		return nil
	}
	// shared params for all records
	params := dbx.Params{
		"system":  systemId,
		"updated": time.Now().UTC().UnixMilli(),
	}

	valueStrings := make([]string, 0, len(data))
	for i, service := range data {
		suffix := fmt.Sprintf("%d", i)
		valueStrings = append(valueStrings, fmt.Sprintf("({:id%[1]s}, {:system}, {:name%[1]s}, {:state%[1]s}, {:sub%[1]s}, {:cpu%[1]s}, {:cpuPeak%[1]s}, {:memory%[1]s}, {:memPeak%[1]s}, {:updated})", suffix))
		params["id"+suffix] = makeStableHashId(systemId, service.Name)
		params["name"+suffix] = service.Name
		params["state"+suffix] = service.State
		params["sub"+suffix] = service.Sub
		params["cpu"+suffix] = service.Cpu
		params["cpuPeak"+suffix] = service.CpuPeak
		params["memory"+suffix] = service.Mem
		params["memPeak"+suffix] = service.MemPeak
	}
	queryString := fmt.Sprintf(
		"INSERT INTO systemd_services (id, system, name, state, sub, cpu, cpuPeak, memory, memPeak, updated) VALUES %s ON CONFLICT(id) DO UPDATE SET system = excluded.system, name = excluded.name, state = excluded.state, sub = excluded.sub, cpu = excluded.cpu, cpuPeak = excluded.cpuPeak, memory = excluded.memory, memPeak = excluded.memPeak, updated = excluded.updated",
		strings.Join(valueStrings, ","),
	)
	_, err := app.DB().NewQuery(queryString).Bind(params).Execute()
	return err
}

// createContainerRecords creates container records
func createContainerRecords(app core.App, data []*container.Stats, systemId string) error {
	if len(data) == 0 {
		return nil
	}
	// shared params for all records
	params := dbx.Params{
		"system":  systemId,
		"updated": time.Now().UTC().UnixMilli(),
	}
	valueStrings := make([]string, 0, len(data))
	for i, container := range data {
		suffix := fmt.Sprintf("%d", i)
		valueStrings = append(valueStrings, fmt.Sprintf("({:id%[1]s}, {:system}, {:name%[1]s}, {:image%[1]s}, {:ports%[1]s}, {:status%[1]s}, {:health%[1]s}, {:cpu%[1]s}, {:memory%[1]s}, {:net%[1]s}, {:updated})", suffix))
		params["id"+suffix] = container.Id
		params["name"+suffix] = container.Name
		params["image"+suffix] = container.Image
		params["ports"+suffix] = container.Ports
		params["status"+suffix] = container.Status
		params["health"+suffix] = container.Health
		params["cpu"+suffix] = container.Cpu
		params["memory"+suffix] = container.Mem
		netBytes := container.Bandwidth[0] + container.Bandwidth[1]
		if netBytes == 0 {
			netBytes = uint64((container.NetworkSent + container.NetworkRecv) * 1024 * 1024)
		}
		params["net"+suffix] = netBytes
	}
	queryString := fmt.Sprintf(
		"INSERT INTO containers (id, system, name, image, ports, status, health, cpu, memory, net, updated) VALUES %s ON CONFLICT(id) DO UPDATE SET system = excluded.system, name = excluded.name, image = excluded.image, ports = excluded.ports, status = excluded.status, health = excluded.health, cpu = excluded.cpu, memory = excluded.memory, net = excluded.net, updated = excluded.updated",
		strings.Join(valueStrings, ","),
	)
	_, err := app.DB().NewQuery(queryString).Bind(params).Execute()
	return err
}

// getRecord retrieves the system record from the database.
// If the record is not found, it removes the system from the manager.
func (sys *System) getRecord(app core.App) (*core.Record, error) {
	record, err := app.FindRecordById("systems", sys.Id)
	if err != nil || record == nil {
		_ = sys.manager.RemoveSystem(sys.Id)
		return nil, err
	}
	return record, nil
}

// setDown marks a system as down in the database.
// It takes the original error that caused the system to go down and returns any error
// encountered during the process of updating the system status.
func (sys *System) setDown(originalError error) error {
	if sys.Status == down || sys.Status == paused {
		return nil
	}
	record, err := sys.getRecord(sys.manager.hub)
	if err != nil {
		return err
	}
	if originalError != nil {
		sys.manager.hub.Logger().Error("System down", "system", record.GetString("name"), "err", originalError)
	}
	record.Set("status", down)
	return sys.manager.hub.SaveNoValidate(record)
}

func (sys *System) getContext() (context.Context, context.CancelFunc) {
	if sys.ctx == nil {
		sys.ctx, sys.cancel = context.WithCancel(context.Background())
	}
	return sys.ctx, sys.cancel
}

// request sends a request to the agent over its outbound WebSocket.
func (sys *System) request(ctx context.Context, action common.WebSocketAction, req any, dest any) error {
	if sys.WsConn == nil || !sys.WsConn.IsConnected() {
		return transport.ErrWebSocketNotConnected
	}
	return transport.NewWebSocketTransport(sys.WsConn).Request(ctx, action, req, dest)
}

// fetchDataFromAgent attempts to fetch data from the agent, prioritizing WebSocket if available.
func (sys *System) fetchDataFromAgent(options common.DataRequestOptions) (*system.CombinedData, error) {
	if sys.data == nil {
		sys.data = &system.CombinedData{}
	}

	return sys.fetchDataViaWebSocket(options)
}

func (sys *System) fetchDataViaWebSocket(options common.DataRequestOptions) (*system.CombinedData, error) {
	if sys.WsConn == nil || !sys.WsConn.IsConnected() {
		return nil, errors.New("no websocket connection")
	}
	wsTransport := transport.NewWebSocketTransport(sys.WsConn)
	err := wsTransport.Request(context.Background(), common.GetData, options, sys.data)
	if err != nil {
		return nil, err
	}
	return sys.data, nil
}

// FetchContainerInfoFromAgent fetches container info from the agent
func (sys *System) FetchContainerInfoFromAgent(containerID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var result string
	err := sys.request(ctx, common.GetContainerInfo, common.ContainerInfoRequest{ContainerID: containerID}, &result)
	return result, err
}

// FetchContainerLogsFromAgent fetches container logs from the agent
func (sys *System) FetchContainerLogsFromAgent(containerID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var result string
	err := sys.request(ctx, common.GetContainerLogs, common.ContainerLogsRequest{ContainerID: containerID}, &result)
	return result, err
}

func makeStableHashId(strings ...string) string {
	hash := fnv.New32a()
	for _, str := range strings {
		hash.Write([]byte(str))
	}
	return fmt.Sprintf("%x", hash.Sum32())
}

// closeWebSocketConnection closes the outbound agent connection.
func (sys *System) closeWebSocketConnection() {
	if sys.WsConn != nil {
		sys.WsConn.Close(nil)
	}
}

// getJitter returns a channel that will be triggered after a random delay
// between 51% and 95% of the interval.
// This is used to stagger the initial WebSocket connections to prevent clustering.
func getJitter() <-chan time.Time {
	minPercent := 51
	maxPercent := 95
	jitterRange := maxPercent - minPercent
	msDelay := (interval * minPercent / 100) + rand.Intn(interval*jitterRange/100)
	return time.After(time.Duration(msDelay) * time.Millisecond)
}

// migrateDeprecatedFields moves values from deprecated fields to their new locations if the new
// fields are not already populated. Deprecated fields and refs may be removed at least 30 days
// and one minor version release after the release that includes the migration.
//
// This is run when processing incoming system data from agents, which may be on older versions.
func migrateDeprecatedFields(cd *system.CombinedData, createDetails bool) {
	// migration added 0.19.0
	if cd.Stats.Bandwidth[0] == 0 && cd.Stats.Bandwidth[1] == 0 {
		cd.Stats.Bandwidth[0] = uint64(cd.Stats.NetworkSent * 1024 * 1024)
		cd.Stats.Bandwidth[1] = uint64(cd.Stats.NetworkRecv * 1024 * 1024)
		cd.Stats.NetworkSent, cd.Stats.NetworkRecv = 0, 0
	}
	// migration added 0.19.0
	if cd.Info.BandwidthBytes == 0 {
		cd.Info.BandwidthBytes = uint64(cd.Info.Bandwidth * 1024 * 1024)
		cd.Info.Bandwidth = 0
	}
	// migration added 0.19.0
	if cd.Stats.DiskIO[0] == 0 && cd.Stats.DiskIO[1] == 0 {
		cd.Stats.DiskIO[0] = uint64(cd.Stats.DiskReadPs * 1024 * 1024)
		cd.Stats.DiskIO[1] = uint64(cd.Stats.DiskWritePs * 1024 * 1024)
		cd.Stats.DiskReadPs, cd.Stats.DiskWritePs = 0, 0
	}
	// migration added 0.19.0 - Move deprecated Info fields to Details struct
	if cd.Details == nil && cd.Info.Hostname != "" {
		if createDetails {
			cd.Details = &system.Details{
				Hostname:    cd.Info.Hostname,
				Kernel:      cd.Info.KernelVersion,
				Cores:       cd.Info.Cores,
				Threads:     cd.Info.Threads,
				CpuModel:    cd.Info.CpuModel,
				Podman:      cd.Info.Podman,
				Os:          cd.Info.Os,
				MemoryTotal: uint64(cd.Stats.Mem * 1024 * 1024 * 1024),
			}
		}
		// zero the deprecated fields to prevent saving them in systems.info DB json payload
		cd.Info.Hostname = ""
		cd.Info.KernelVersion = ""
		cd.Info.Cores = 0
		cd.Info.CpuModel = ""
		cd.Info.Podman = false
		cd.Info.Os = 0
	}
}
