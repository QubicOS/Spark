//go:build !tinygo

package hal

import "github.com/hajimehoshi/ebiten/v2"

import "github.com/hajimehoshi/ebiten/v2/inpututil"

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

	for _, r := range ebiten.AppendInputChars(nil) {
		select {
		case k.ch <- KeyEvent{Press: true, Rune: r}:
		default:
		}
	}

	// Use only arrow keys for navigation. Letter keys are treated as text input.
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		emit(KeyUp)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		emit(KeyDown)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) {
		emit(KeyLeft)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) {
		emit(KeyRight)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		emit(KeyEnter)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		emit(KeyEscape)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		emit(KeyBackspace)
	}
}
