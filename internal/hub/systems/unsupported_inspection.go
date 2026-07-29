package systems

import (
	"errors"

	"github.com/henrygd/beszel/internal/entities/systemd"
)

// These methods remain only as boundaries for existing hub routes. The agent
// protocol no longer exposes SMART or detailed systemd inspection.
func (sys *System) FetchAndSaveSmartDevices() error {
	return errors.ErrUnsupported
}

func (sys *System) FetchSystemdInfoFromAgent(string) (systemd.ServiceDetails, error) {
	return nil, errors.ErrUnsupported
}
