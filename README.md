## Spark (RP2350 / Pico 2) TinyGo microkernel v0

Minimal firmware to confirm the basics needed for a microkernel:

- working toolchain/flash
- UART logging
- 1ms timebase (`ticks`)
- 4 concurrent tasks (goroutines)
- IPC prototype: mailbox (copy) + shared buffer + notify

Project direction: `docs/idea.md`.

### Wiring

- UART0 TX: `GP0` -> USB-UART RX
- UART0 RX: `GP1` -> USB-UART TX (optional for now)
- Baud: `115200 8N1`
- GND: common ground between Pico 2 and USB-UART adapter

### Build (host)

```bash
make run
```

Dev (race detector):

```bash
make dev
```

Host framebuffer output:

- opens a `320x320` window
- keyboard controls: `W/A/S/D` (or arrows) move a white pixel, `Enter` clears

Note: `tinygo run .` does not start a GUI window (use `go run .` for desktop testing).

Headless host run:

```bash
make headless
```

Flags: `-headless -hz=60 -ticks=0` (e.g. `go run . -headless -ticks=600`).

### Build (Pico 2 / UF2)

```bash
make tinygo-uf2
```

### Flash (UF2)

1. Hold `BOOTSEL`, plug the Pico 2 over USB, release `BOOTSEL`.
2. Copy `dist/spark.uf2` to the mounted drive.

### Flash (openocd, optional)

If you use a debug probe:

```bash
make tinygo-flash
```

### Expected output

On the UART console you should see:

- (no demo output by default)
