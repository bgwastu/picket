//go:build testing

package ws

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewWsConnection(t *testing.T) {
	conn := NewWsConnection(nil)
	assert.NotNil(t, conn)
	assert.NotNil(t, conn.requestManager)
	assert.NotNil(t, conn.DownChan)
	assert.False(t, conn.IsConnected())
}
