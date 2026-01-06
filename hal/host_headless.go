//go:build !tinygo

package hal

import (
	"context"
	"fmt"
	"time"
)

// HeadlessConfig controls the no-window host runner.
type HeadlessConfig struct {
	Enabled    bool
	Hz         int
	Ticks      uint64
	StepBudget int
}

// RunHeadless runs the OS without opening a window.
func RunHeadless(ctx context.Context, newApp func(HAL) func() error, cfg HeadlessConfig) error {
	if cfg.Hz <= 0 {
		cfg.Hz = 60
	}
	if cfg.StepBudget <= 0 {
		cfg.StepBudget = 1
	}

	h := New().(*hostHAL)
	step := newApp(h)

	d := time.Second / time.Duration(cfg.Hz)
	if d <= 0 {
		return fmt.Errorf("invalid headless hz: %d", cfg.Hz)
	}
	t := time.NewTicker(d)
	defer t.Stop()

	var tick uint64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			h.t.step(1)
			if step != nil {
				if err := step(); err != nil {
					return err
				}
			}
			tick++
			if cfg.Ticks > 0 && tick >= cfg.Ticks {
				return nil
			}
		}
	}
}
