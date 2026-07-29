//go:build !linux

package agent

import "github.com/henrygd/beszel/internal/entities/systemd"

type systemdManager struct{ hasFreshStats bool }

func newSystemdManager() (*systemdManager, error)                    { return nil, nil }
func (*systemdManager) getServiceStatsCount() int                    { return 0 }
func (*systemdManager) getFailedServiceCount() uint16                { return 0 }
func (*systemdManager) getServiceStats(any, bool) []*systemd.Service { return nil }
