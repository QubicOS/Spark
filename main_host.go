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
	var termDemo bool
	var shell bool
	flag.BoolVar(&cfg.Enabled, "headless", false, "Run without a window.")
	flag.IntVar(&cfg.Hz, "hz", 60, "Tick rate in headless mode.")
	flag.Uint64Var(&cfg.Ticks, "ticks", 0, "Stop after N ticks in headless mode (0 = run forever).")
	flag.BoolVar(&termDemo, "term-demo", false, "Run VT100 terminal demo.")
	flag.BoolVar(&shell, "shell", false, "Run interactive shell.")
	flag.Parse()

	if cfg.Enabled {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		if err := hal.RunHeadless(ctx, func(h hal.HAL) func() error {
			return app.NewWithConfig(h, app.Config{TermDemo: termDemo, Shell: shell})
		}, cfg); err != nil {
			if err == context.Canceled {
				return
			}
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if err := hal.RunWindow(func(h hal.HAL) func() error {
		return app.NewWithConfig(h, app.Config{TermDemo: termDemo, Shell: shell})
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
