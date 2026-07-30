// Package hub handles updating systems and serving the web UI.
package hub

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/henrygd/beszel/internal/alerts"
	"github.com/henrygd/beszel/internal/hub/systems"
	"github.com/henrygd/beszel/internal/hub/utils"
	"github.com/henrygd/beszel/internal/records"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// Hub is the application. It embeds the PocketBase app and keeps references to subcomponents.
type Hub struct {
	core.App
	*alerts.AlertManager
	rm                    *records.RecordManager
	sm                    *systems.SystemManager
	appURL                string
	dashboardPasswordHash string
	ssh                   *sshManager
}

// NewHub creates a new Hub instance with default configuration
func NewHub(app core.App) *Hub {
	hub := &Hub{App: app}
	hub.AlertManager = alerts.NewAlertManager(hub)
	hub.rm = records.NewRecordManager(hub)
	hub.sm = systems.NewSystemManager(hub)
	hub.ssh = newSSHManager()
	_ = onAfterBootstrapAndMigrations(app, hub.initialize)
	return hub
}

// onAfterBootstrapAndMigrations ensures the provided function runs after the database is set up and migrations are applied.
// This is a workaround for behavior in PocketBase where onBootstrap runs before migrations, forcing use of onServe for this purpose.
// However, PB's tests.TestApp is already bootstrapped, generally doesn't serve, but does handle migrations.
// So this ensures that the provided function runs at the right time either way, after DB is ready and migrations are done.
func onAfterBootstrapAndMigrations(app core.App, fn func(app core.App) error) error {
	// pb tests.TestApp is already bootstrapped and doesn't serve
	if app.IsBootstrapped() {
		return fn(app)
	}
	// Must use OnServe because OnBootstrap appears to run before migrations, even if calling e.Next() before anything else
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		if err := fn(e.App); err != nil {
			return err
		}
		return e.Next()
	})
	return nil
}

// StartHub sets up event handlers and starts the PocketBase server
func (h *Hub) StartHub() error {
	h.App.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// register middlewares
		h.registerMiddlewares(e)
		// register api routes
		if err := h.registerApiRoutes(e); err != nil {
			return err
		}
		// register cron jobs
		if err := h.registerCronJobs(e); err != nil {
			return err
		}
		// start server
		if err := h.startServer(e); err != nil {
			return err
		}
		// start system updates
		if err := h.sm.Initialize(); err != nil {
			return err
		}
		return e.Next()
	})

	pb, ok := h.App.(*pocketbase.PocketBase)
	if !ok {
		return errors.New("not a pocketbase app")
	}
	return pb.Start()
}

// initialize sets up initial configuration (collections, settings, etc.)
func (h *Hub) initialize(app core.App) error {
	// set general settings
	settings := app.Settings()
	// batch requests (for alerts)
	settings.Batch.Enabled = true
	// set URL if APP_URL env is set
	if appURL, isSet := utils.GetEnv("APP_URL"); isSet {
		h.appURL = appURL
		settings.Meta.AppURL = appURL
	}
	passwordFile := filepath.Join(app.DataDir(), ".hub-password-hash")
	if password, isSet := utils.GetEnv("PASSWORD"); isSet && password != "" {
		h.dashboardPasswordHash = hashDashboardPassword(password)
		if err := os.WriteFile(passwordFile, []byte(h.dashboardPasswordHash), 0600); err != nil {
			return err
		}
	} else if hash, err := os.ReadFile(passwordFile); err == nil {
		h.dashboardPasswordHash = strings.TrimSpace(string(hash))
	}
	if err := app.Save(settings); err != nil {
		return err
	}
	return setCollectionAccessSettings(app)
}

func hashDashboardPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

// registerCronJobs sets up scheduled tasks
func (h *Hub) registerCronJobs(_ *core.ServeEvent) error {
	// delete old system_stats and alerts_history records once every hour
	h.Cron().MustAdd("delete old records", "8 * * * *", h.rm.DeleteOldRecords)
	// create longer records every 10 minutes
	h.Cron().MustAdd("create longer records", "*/10 * * * *", h.rm.CreateLongerRecords)
	return nil
}

// MakeLink formats a link with the app URL and path segments.
// Only path segments should be provided.
func (h *Hub) MakeLink(parts ...string) string {
	base := strings.TrimSuffix(h.Settings().Meta.AppURL, "/")
	for _, part := range parts {
		if part == "" {
			continue
		}
		base = fmt.Sprintf("%s/%s", base, url.PathEscape(part))
	}
	return base
}
