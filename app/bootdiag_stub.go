//go:build !bootdebug

package app

import "spark/hal"

func bootDiagSetStep(msg string) {
	_ = msg
}

func bootDiagStart(h hal.HAL) {
	_ = h
}
