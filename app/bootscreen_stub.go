//go:build !bootdebug

package app

import "spark/hal"

func bootScreen(h hal.HAL, msg string) {
	_ = h
	_ = msg
}
