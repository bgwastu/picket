// Package agent implements the Beszel monitoring agent that collects and serves system metrics.
//
// The agent runs on monitored systems and communicates collected data
// to the Beszel hub for centralized monitoring and alerting.
package agent

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/agent/deltatracker"
	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/system"
)

const defaultDataCacheTimeMs uint16 = 60_000

type Agent struct {
	sync.Mutex                                                                      // Used to lock agent while collecting data
	debug                     bool                                                  // true if LOG_LEVEL is set to debug
	zfs                       bool                                                  // true if system has arcstats
	memCalc                   string                                                // Memory calculation formula
	fsNames                   []string                                              // List of filesystem device names being monitored
	fsStats                   map[string]*system.FsStats                            // Keeps track of disk stats for each filesystem
	diskPrev                  map[uint16]map[string]prevDisk                        // Previous disk I/O counters per cache interval
	diskUsageCacheDuration    time.Duration                                         // How long to cache disk usage (to avoid waking sleeping disks)
	lastDiskUsageUpdate       time.Time                                             // Last time disk usage was collected
	netInterfaces             map[string]struct{}                                   // Stores all valid network interfaces
	netIoStats                map[uint16]system.NetIoStats                          // Keeps track of bandwidth usage per cache interval
	netInterfaceDeltaTrackers map[uint16]*deltatracker.DeltaTracker[string, uint64] // Per-cache-time NIC delta trackers
	dockerManager             *dockerManager                                        // Manages Docker API requests
	systemInfo                system.Info                                           // Host system info (dynamic)
	systemDetails             system.Details                                        // Host system details (static, once-per-connection)
	detailsDirty              bool                                                  // Whether system details have changed and need to be resent
	gpuManager                *GPUManager                                           // Manages GPU data
	cache                     *systemDataCache                                      // Cache for system stats based on cache time
	connectionManager         *ConnectionManager                                    // Channel to signal connection events
	handlerRegistry           *HandlerRegistry                                      // Registry for routing incoming messages
	systemdManager            *systemdManager                                       // Manages systemd services
}

// NewAgent creates a stateless monitoring agent.
func NewAgent() (agent *Agent, err error) {
	agent = &Agent{
		fsStats: make(map[string]*system.FsStats),
		cache:   NewSystemDataCache(),
	}

	// Initialize disk I/O previous counters storage
	agent.diskPrev = make(map[uint16]map[string]prevDisk)
	// Initialize per-cache-time network tracking structures
	agent.netIoStats = make(map[uint16]system.NetIoStats)
	agent.netInterfaceDeltaTrackers = make(map[uint16]*deltatracker.DeltaTracker[string, uint64])

	agent.memCalc, _ = utils.GetEnv("MEM_CALC")

	// Parse disk usage cache duration (e.g., "15m", "1h") to avoid waking sleeping disks
	if diskUsageCache, exists := utils.GetEnv("DISK_USAGE_CACHE"); exists {
		if duration, err := time.ParseDuration(diskUsageCache); err == nil {
			agent.diskUsageCacheDuration = duration
			slog.Info("DISK_USAGE_CACHE", "duration", duration)
		} else {
			slog.Warn("Invalid DISK_USAGE_CACHE", "err", err)
		}
	}

	// Set up slog with a log level determined by the LOG_LEVEL env var
	if logLevelStr, exists := utils.GetEnv("LOG_LEVEL"); exists {
		switch strings.ToLower(logLevelStr) {
		case "debug":
			agent.debug = true
			slog.SetLogLoggerLevel(slog.LevelDebug)
		case "warn":
			slog.SetLogLoggerLevel(slog.LevelWarn)
		case "error":
			slog.SetLogLoggerLevel(slog.LevelError)
		}
	}

	slog.Debug(beszel.Version)

	// initialize docker manager
	agent.dockerManager = newDockerManager(agent)

	// initialize system info
	agent.refreshSystemDetails()

	// initialize connection manager
	agent.connectionManager = newConnectionManager(agent)

	// initialize handler registry
	agent.handlerRegistry = NewHandlerRegistry()

	// initialize disk info
	agent.initializeDiskInfo()

	// initialize net io stats
	agent.initializeNetIoStats()

	agent.systemdManager, err = newSystemdManager()
	if err != nil {
		slog.Debug("Systemd", "err", err)
	}

	// initialize GPU manager
	agent.gpuManager, err = NewGPUManager()
	if err != nil {
		slog.Debug("GPU", "err", err)
	}

	// if debugging, print stats
	if agent.debug {
		slog.Debug("Stats", "data", agent.gatherStats(common.DataRequestOptions{CacheTimeMs: defaultDataCacheTimeMs, IncludeDetails: true}))
	}

	return agent, nil
}

func (a *Agent) gatherStats(options common.DataRequestOptions) *system.CombinedData {
	a.Lock()
	defer a.Unlock()

	cacheTimeMs := options.CacheTimeMs
	data, isCached := a.cache.Get(cacheTimeMs)
	if isCached {
		slog.Debug("Cached data", "cacheTimeMs", cacheTimeMs)
		return data
	}

	*data = system.CombinedData{
		Stats: a.getSystemStats(cacheTimeMs),
		Info:  a.systemInfo,
	}

	// slog.Info("System data", "data", data, "cacheTimeMs", cacheTimeMs)

	if a.dockerManager != nil {
		if containerStats, err := a.dockerManager.getDockerStats(cacheTimeMs); err == nil {
			data.Containers = containerStats
			slog.Debug("Containers", "data", data.Containers)
		} else {
			slog.Debug("Containers", "err", err)
		}
	}

	// skip updating systemd services if cache time is not the default 60sec interval
	if a.systemdManager != nil && cacheTimeMs == defaultDataCacheTimeMs {
		totalCount := uint16(a.systemdManager.getServiceStatsCount())
		if totalCount > 0 {
			numFailed := a.systemdManager.getFailedServiceCount()
			data.Info.Services = []uint16{totalCount, numFailed}
		}
		if a.systemdManager.hasFreshStats {
			data.SystemdServices = a.systemdManager.getServiceStats(nil, false)
		}
	}

	data.Stats.ExtraFs = make(map[string]*system.FsStats)
	data.Info.ExtraFsPct = make(map[string]float64)
	for name, stats := range a.fsStats {
		if !stats.Root && stats.DiskTotal > 0 {
			// Use custom name if available, otherwise use device name
			key := name
			if stats.Name != "" {
				key = stats.Name
			}
			data.Stats.ExtraFs[key] = stats
			// Add percentages to Info struct for dashboard
			if stats.DiskTotal > 0 {
				pct := utils.TwoDecimals((stats.DiskUsed / stats.DiskTotal) * 100)
				data.Info.ExtraFsPct[key] = pct
			}
		}
	}
	slog.Debug("Extra FS", "data", data.Stats.ExtraFs)

	a.cache.Set(data, cacheTimeMs)

	return a.attachSystemDetails(data, cacheTimeMs, options.IncludeDetails)
}

// Start connects to the hub and blocks until shutdown.
func (a *Agent) Start() error {
	return a.connectionManager.Start()
}
