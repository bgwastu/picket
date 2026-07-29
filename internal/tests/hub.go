//go:build testing

// Package tests provides helpers for testing the application.
package tests

import (
	"fmt"
	"testing"

	"github.com/henrygd/beszel/internal/hub"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"

	_ "github.com/pocketbase/pocketbase/migrations"
)

// TestHub is a wrapper hub instance used for testing.
type TestHub struct {
	core.App
	*tests.TestApp
	*hub.Hub
}

// NewTestHub creates and initializes a test application instance.
//
// It is the caller's responsibility to call app.Cleanup() when the app is no longer needed.
func NewTestHub(optTestDataDir ...string) (*TestHub, error) {
	var testDataDir string
	if len(optTestDataDir) > 0 {
		testDataDir = optTestDataDir[0]
	}

	return NewTestHubWithConfig(core.BaseAppConfig{
		DataDir:       testDataDir,
		EncryptionEnv: "pb_test_env",
	})
}

// NewTestHubWithConfig creates and initializes a test application instance
// from the provided config.
//
// If config.DataDir is not set it fallbacks to the default internal test data directory.
//
// config.DataDir is cloned for each new test application instance.
//
// It is the caller's responsibility to call app.Cleanup() when the app is no longer needed.
func NewTestHubWithConfig(config core.BaseAppConfig) (*TestHub, error) {
	testApp, err := tests.NewTestAppWithConfig(config)
	if err != nil {
		return nil, err
	}

	hub := hub.NewHub(testApp)

	t := &TestHub{
		App:     testApp,
		TestApp: testApp,
		Hub:     hub,
	}

	return t, nil
}

// Helper function to create a test record
func CreateRecord(app core.App, collectionName string, fields map[string]any) (*core.Record, error) {
	collection, err := app.FindCachedCollectionByNameOrId(collectionName)
	if err != nil {
		return nil, err
	}

	record := core.NewRecord(collection)
	record.Load(fields)

	return record, app.Save(record)
}

func ClearCollection(t testing.TB, app core.App, collectionName string) error {
	_, err := app.DB().NewQuery(fmt.Sprintf("DELETE from %s", collectionName)).Execute()
	recordCount, err := app.CountRecords(collectionName)
	assert.EqualValues(t, recordCount, 0, "should have 0 records after clearing")
	return err
}

func (h *TestHub) Cleanup() {
	h.GetAlertManager().Stop()
	h.GetSystemManager().RemoveAllSystems()
	h.TestApp.Cleanup()
}

func CreateSystems(app core.App, count int, _ string, status string) ([]*core.Record, error) {
	systems := make([]*core.Record, 0, count)
	for i := range count {
		system, err := CreateRecord(app, "systems", map[string]any{
			"name":  fmt.Sprintf("test-system-%d", i),
			"token": fmt.Sprintf("test-token-%d-012345678901234567890123456789", i),
		})
		if err != nil {
			return nil, err
		}
		system.Set("status", status)
		err = app.SaveNoValidate(system)
		if err != nil {
			return nil, err
		}
		systems = append(systems, system)
	}
	return systems, nil
}

// GetHubWithUser retains the old helper shape while creating a global notification record.
func GetHubWithUser(t *testing.T) (*TestHub, *core.Record) {
	hub, err := NewTestHub(t.TempDir())
	assert.NoError(t, err)
	hub.StartHub()

	// Manually initialize the system manager to bind event hooks
	err = hub.GetSystemManager().Initialize()
	assert.NoError(t, err)

	settingsData := map[string]any{
		"id":       "globalsettings1",
		"settings": `{"emails":["test@example.com"],"webhooks":[]}`,
	}
	_, err = CreateRecord(hub, "notification_settings", settingsData)
	assert.NoError(t, err)

	return hub, core.NewRecord(core.NewBaseCollection("test", "test"))
}
