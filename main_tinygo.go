//go:build tinygo

package main

import (
	"spark/app"
	"spark/hal"
)

func main() {
	app.Run(hal.New())
}

