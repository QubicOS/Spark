//go:build !tinygo

package hal

import "github.com/hajimehoshi/ebiten/v2"

type hostKeyboard struct {
	ch chan KeyEvent
}

func newHostKeyboard() *hostKeyboard {
	return &hostKeyboard{ch: make(chan KeyEvent, 64)}
}

func (k *hostKeyboard) Events() <-chan KeyEvent { return k.ch }

func (k *hostKeyboard) poll() {
	emit := func(code KeyCode) {
		select {
		case k.ch <- KeyEvent{Code: code, Press: true}:
		default:
		}
	}

	if ebiten.IsKeyPressed(ebiten.KeyW) || ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
		emit(KeyUp)
	}
	if ebiten.IsKeyPressed(ebiten.KeyS) || ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
		emit(KeyDown)
	}
	if ebiten.IsKeyPressed(ebiten.KeyA) || ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		emit(KeyLeft)
	}
	if ebiten.IsKeyPressed(ebiten.KeyD) || ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		emit(KeyRight)
	}
	if ebiten.IsKeyPressed(ebiten.KeyEnter) {
		emit(KeyEnter)
	}
	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		emit(KeyEscape)
	}
}

