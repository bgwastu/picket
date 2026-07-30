package sshstream

import (
	"context"
	"io"
	"sync"
	"time"
)

const (
	DefaultIdleTimeout = 15 * time.Minute
	MaxLifetime        = 12 * time.Hour
)

// Bridge copies bytes in both directions and closes both ends after inactivity.
// The activity clock is intentionally transport-level: any bytes in either
// direction count as activity, while no bytes can keep a session alive.
func Bridge(ctx context.Context, left, right io.ReadWriteCloser, idle, lifetime time.Duration) error {
	if idle <= 0 {
		idle = DefaultIdleTimeout
	}
	if lifetime <= 0 {
		lifetime = MaxLifetime
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer left.Close()
	defer right.Close()

	var once sync.Once
	closeBoth := func() {
		once.Do(func() {
			_ = left.Close()
			_ = right.Close()
		})
	}
	errs := make(chan error, 2)
	copySide := func(dst io.Writer, src io.Reader) {
		_, err := io.Copy(dst, src)
		errs <- err
		closeBoth()
	}
	go copySide(left, right)
	go copySide(right, left)

	idleTimer := time.NewTimer(idle)
	defer idleTimer.Stop()
	lifetimeTimer := time.NewTimer(lifetime)
	defer lifetimeTimer.Stop()
	for {
		select {
		case <-ctx.Done():
			closeBoth()
			return ctx.Err()
		case <-idleTimer.C:
			closeBoth()
			return context.DeadlineExceeded
		case <-lifetimeTimer.C:
			closeBoth()
			return context.DeadlineExceeded
		case err := <-errs:
			closeBoth()
			if err == nil || err == io.EOF {
				return nil
			}
			return err
		}
	}
}
