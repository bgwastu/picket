//go:build testing

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetToken(t *testing.T) {
	t.Run("environment", func(t *testing.T) {
		t.Setenv("TOKEN", "system-token")
		token, err := getToken()
		require.NoError(t, err)
		assert.Equal(t, "system-token", token)
	})

	t.Run("file", func(t *testing.T) {
		t.Setenv("TOKEN", "")
		path := filepath.Join(t.TempDir(), "token")
		require.NoError(t, os.WriteFile(path, []byte("system-token\n"), 0o600))
		t.Setenv("TOKEN_FILE", path)
		token, err := getToken()
		require.NoError(t, err)
		assert.Equal(t, "system-token", token)
	})
}

func TestWebSocketOptionsUseBearerToken(t *testing.T) {
	t.Setenv("HUB_URL", "https://hub.example/base")
	t.Setenv("TOKEN", "system-token")
	client, err := newWebSocketClient(&Agent{})
	require.NoError(t, err)
	options := client.getOptions()
	assert.Equal(t, "wss://hub.example/base/api/picket/agent-connect", options.Addr)
	assert.Equal(t, "Bearer system-token", options.RequestHeader.Get("Authorization"))
}
