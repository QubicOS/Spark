//go:build !tinygo && cgo

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
	emit := func(code KeyCode, press bool) {
		select {
		case k.ch <- KeyEvent{Code: code, Press: press}:
		default:
		}
	}

	ctrl := ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight)
	alt := ebiten.IsKeyPressed(ebiten.KeyAltLeft) || ebiten.IsKeyPressed(ebiten.KeyAltRight)
	shift := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)

	_ = alt
	_ = shift

	if ctrl {
		emitCtrl := func(key ebiten.Key, r rune) {
			if !inpututil.IsKeyJustPressed(key) {
				return
			}
			select {
			case k.ch <- KeyEvent{Press: true, Rune: r}:
			default:
			}
		}
		emitCtrl(ebiten.KeyA, 0x01)
		emitCtrl(ebiten.KeyE, 0x05)
		emitCtrl(ebiten.KeyG, 0x07)
		emitCtrl(ebiten.KeyU, 0x15)
		emitCtrl(ebiten.KeyW, 0x17)
		emitCtrl(ebiten.KeyC, 0x03)
	}

	for _, r := range ebiten.AppendInputChars(nil) {
		select {
		case k.ch <- KeyEvent{Press: true, Rune: r}:
		default:
		}
	}

	// Use only arrow keys for navigation. Letter keys are treated as text input.
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		emit(KeyUp, true)
	}
	if inpututil.IsKeyJustReleased(ebiten.KeyArrowUp) {
		emit(KeyUp, false)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		emit(KeyDown, true)
	}
	if inpututil.IsKeyJustReleased(ebiten.KeyArrowDown) {
		emit(KeyDown, false)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) {
		emit(KeyLeft, true)
	}
	if inpututil.IsKeyJustReleased(ebiten.KeyArrowLeft) {
		emit(KeyLeft, false)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) {
		emit(KeyRight, true)
	}
	if inpututil.IsKeyJustReleased(ebiten.KeyArrowRight) {
		emit(KeyRight, false)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		emit(KeyEnter, true)
	}
	if inpututil.IsKeyJustReleased(ebiten.KeyEnter) {
		emit(KeyEnter, false)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		emit(KeyEscape, true)
	}
	if inpututil.IsKeyJustReleased(ebiten.KeyEscape) {
		emit(KeyEscape, false)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		emit(KeyBackspace, true)
	}
	if inpututil.IsKeyJustReleased(ebiten.KeyBackspace) {
		emit(KeyBackspace, false)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		emit(KeyTab, true)
	}
	if inpututil.IsKeyJustReleased(ebiten.KeyTab) {
		emit(KeyTab, false)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDelete) {
		emit(KeyDelete, true)
	}
	if inpututil.IsKeyJustReleased(ebiten.KeyDelete) {
		emit(KeyDelete, false)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyHome) {
		emit(KeyHome, true)
	}
	if inpututil.IsKeyJustReleased(ebiten.KeyHome) {
		emit(KeyHome, false)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnd) {
		emit(KeyEnd, true)
	}
	if inpututil.IsKeyJustReleased(ebiten.KeyEnd) {
		emit(KeyEnd, false)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF1) {
		emit(KeyF1, true)
	}
	if inpututil.IsKeyJustReleased(ebiten.KeyF1) {
		emit(KeyF1, false)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF2) {
		emit(KeyF2, true)
	}
	if inpututil.IsKeyJustReleased(ebiten.KeyF2) {
		emit(KeyF2, false)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF3) {
		emit(KeyF3, true)
	}
	if inpututil.IsKeyJustReleased(ebiten.KeyF3) {
		emit(KeyF3, false)
	}
}
