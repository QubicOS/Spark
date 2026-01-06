package termdemo

import (
	"fmt"

	termclient "spark/sparkos/client/term"
	timeclient "spark/sparkos/client/time"
	"spark/sparkos/kernel"
)

type Task struct {
	timeCap kernel.Capability
	termCap kernel.Capability
}

func New(timeCap, termCap kernel.Capability) *Task {
	return &Task{timeCap: timeCap, termCap: termCap}
}

func (t *Task) Run(ctx *kernel.Context) {
	_ = writeAll(ctx, t.termCap, []byte("\x1b[0m"))
	_ = clearWithRetry(ctx, t.termCap)

	_ = writeAll(ctx, t.termCap, []byte(
		"\x1b[1mSparkOS VT100 demo (tinyterm)\x1b[0m\n"+
			"Привет, мир!\n"+
			"SGR colors, scroll, and cursor-back.\n\n"+
			"\x1b[38;5;39m256-color:\x1b[0m "+
			"\x1b[38;5;196mRED\x1b[0m "+
			"\x1b[38;5;46mGREEN\x1b[0m "+
			"\x1b[38;5;226mYELLOW\x1b[0m "+
			"\x1b[38;5;21mBLUE\x1b[0m\n\n",
	))

	spin := []byte{'-', '\\', '|', '/'}
	i := 0

	_ = writeAll(ctx, t.termCap, []byte("spinner: -"))
	for {
		if err := timeclient.Sleep(ctx, t.timeCap, 30); err != nil {
			_ = writeAll(ctx, t.termCap, []byte(fmt.Sprintf("\n\x1b[31mtime error: %v\x1b[0m\n", err)))
		}

		_ = writeAll(ctx, t.termCap, []byte("\x1b[D"))
		_ = writeAll(ctx, t.termCap, []byte{spin[i%len(spin)]})
		i++

		if i%20 == 0 {
			_ = writeAll(ctx, t.termCap, []byte(fmt.Sprintf("\nnow tick: %d\n", ctx.NowTick())))
		}
	}
}

func clearWithRetry(ctx *kernel.Context, termCap kernel.Capability) error {
	for {
		res := termclient.Clear(ctx, termCap)
		switch res {
		case kernel.SendOK:
			return nil
		case kernel.SendErrQueueFull:
			ctx.BlockOnTick()
			continue
		default:
			return fmt.Errorf("term clear: %s", res)
		}
	}
}

func writeAll(ctx *kernel.Context, termCap kernel.Capability, payload []byte) error {
	for len(payload) > 0 {
		chunk := payload
		if len(chunk) > kernel.MaxMessageBytes {
			chunk = chunk[:kernel.MaxMessageBytes]
		}
		for {
			res := termclient.Write(ctx, termCap, chunk)
			switch res {
			case kernel.SendOK:
				break
			case kernel.SendErrQueueFull:
				ctx.BlockOnTick()
				continue
			default:
				return fmt.Errorf("term write: %s", res)
			}
			break
		}
		payload = payload[len(chunk):]
	}
	return nil
}
