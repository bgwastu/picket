//go:build testing

package hub

import (
	"github.com/henrygd/beszel/internal/hub/systems"
)

// TESTING ONLY: GetSystemManager returns the system manager
func (h *Hub) GetSystemManager() *systems.SystemManager {
	return h.sm
}

func (h *Hub) SetCollectionAuthSettings() error {
	return setCollectionAccessSettings(h)
}
