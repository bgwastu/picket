//go:build !linux

package agent

func (gm *GPUManager) startPowermetricsCollector() {}
func (gm *GPUManager) startMacmonCollector()       {}

func (gm *GPUManager) hasAmdSysfs() bool      { return false }
func (gm *GPUManager) collectAmdStats() error { return nil }
