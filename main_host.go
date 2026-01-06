//go:build !tinygo

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"spark/app"
	"spark/hal"
)

func main() {
	var cfg hal.HeadlessConfig
	flag.BoolVar(&cfg.Enabled, "headless", false, "Run without a window.")
	flag.IntVar(&cfg.Hz, "hz", 60, "Tick rate in headless mode.")
	flag.Uint64Var(&cfg.Ticks, "ticks", 0, "Stop after N ticks in headless mode (0 = run forever).")
	flag.IntVar(&cfg.StepBudget, "budget", 64, "Kernel step budget per tick in headless mode.")
	flag.Parse()

	if cfg.Enabled {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		if err := hal.RunHeadless(ctx, func(h hal.HAL) func() error {
			return app.NewWithBudget(h, cfg.StepBudget)
		}, cfg); err != nil {
			if err == context.Canceled {
				return
			}
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if err := hal.RunWindow(app.New); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
