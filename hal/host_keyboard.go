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
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		emit(KeyTab)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDelete) {
		emit(KeyDelete)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyHome) {
		emit(KeyHome)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnd) {
		emit(KeyEnd)
	}
}
