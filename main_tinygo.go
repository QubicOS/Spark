//go:build tinygo && !shell

package main

import (
	"spark/app"
	"spark/hal"
)

func main() {
	app.RunWithConfig(hal.New(), app.Config{TermDemo: true})
}
