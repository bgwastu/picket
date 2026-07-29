package hub

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBearerToken(t *testing.T) {
	token, err := bearerToken("Bearer unique-system-token")
	require.NoError(t, err)
	assert.Equal(t, "unique-system-token", token)

	for _, value := range []string{"", "unique-system-token", "Basic value", "Bearer", "Bearer two tokens"} {
		_, err := bearerToken(value)
		assert.Error(t, err, value)
	}
}
