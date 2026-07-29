package migrations

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"
)

func TestFreshInstallSchema(t *testing.T) {
	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	expected := []string{
		"systems",
		"system_stats",
		"containers",
		"container_stats",
		"system_details",
		"systemd_services",
		"alerts",
		"alerts_history",
		"notification_settings",
	}
	for _, name := range expected {
		collection, err := app.FindCollectionByNameOrId(name)
		require.NoError(t, err, "missing collection %s", name)
		require.Nil(t, collection.Fields.GetByName("user"), "collection %s has a user field", name)
		require.Nil(t, collection.Fields.GetByName("users"), "collection %s has a users field", name)
	}

	for _, name := range []string{"user_settings", "quiet_hours", "fingerprints", "smart_devices", "universal_tokens"} {
		_, err := app.FindCollectionByNameOrId(name)
		require.Error(t, err, "unexpected collection %s", name)
	}

	systems, err := app.FindCollectionByNameOrId("systems")
	require.NoError(t, err)
	token := systems.Fields.GetByName("token")
	require.NotNil(t, token)
	require.True(t, token.GetHidden())

	settings, err := app.FindAllRecords("notification_settings")
	require.NoError(t, err)
	require.Len(t, settings, 1)
	require.Equal(t, "globalsettings1", settings[0].Id)
}
