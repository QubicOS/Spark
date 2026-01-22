.PHONY: help dev run headless build build-release test fmt tidy make-vfs vfs clean tinygo-uf2 tinygo-flash check vet test-debug

GO ?= go
TINYGO ?= tinygo

BIN_DIR ?= bin
DIST_DIR ?= dist
ROOTFS_DIR ?= rootfs
FLASH_IMG ?= flash.bin
FLASH_SIZE ?= 16777216

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

HOST_BIN := $(BIN_DIR)/spark-host
UF2 := $(DIST_DIR)/spark.uf2

LDFLAGS_COMMON := -X 'spark/internal/buildinfo.Version=$(VERSION)' -X 'spark/internal/buildinfo.Commit=$(COMMIT)' -X 'spark/internal/buildinfo.Date=$(DATE)'
LDFLAGS_RELEASE := $(LDFLAGS_COMMON) -s -w

help:
	@printf "%s\n" \
	"Targets:" \
	"  dev            Host run (race detector)." \
	"  run            Host run (window)." \
	"  headless       Host run (no window)." \
	"  build          Host build (debug)." \
	"  build-release  Host build (production flags)." \
	"  test           Unit tests." \
	"  fmt            gofmt." \
	"  tidy           go mod tidy." \
	"  make-vfs       Build VFS image from $(ROOTFS_DIR) into $(FLASH_IMG)." \
	"  tinygo-uf2     Build UF2 for RP2350 (Pico 2)." \
	"  tinygo-flash   Flash RP2350 (debug probe / UF2-capable target)." \
	"" \
	"Variables:" \
	"  VERSION=$(VERSION)" \
	"  COMMIT=$(COMMIT)" \
	"  DATE=$(DATE)" \
	"  ROOTFS_DIR=$(ROOTFS_DIR)" \
	"  FLASH_IMG=$(FLASH_IMG)"

dev:
	$(GO) run -race .

run:
	$(GO) run .

headless:
	$(GO) run . -headless

build:
	mkdir -p $(BIN_DIR)
	$(GO) build -ldflags "$(LDFLAGS_COMMON)" -o $(HOST_BIN) .

build-release:
	mkdir -p $(DIST_DIR)
	$(GO) build -trimpath -ldflags "$(LDFLAGS_RELEASE)" -o $(DIST_DIR)/spark-host .

test:
	$(GO) test ./...

test-debug:
	$(GO) test -tags=debug ./...

vet:
	$(GO) vet ./...

check: fmt vet test test-debug build

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

make-vfs:
	@test -d "$(ROOTFS_DIR)" || (echo "error: $(ROOTFS_DIR) not found (create it with files to import)"; exit 2)
	$(GO) run ./cmd/mkflash -src "$(ROOTFS_DIR)" -out "$(FLASH_IMG)" -size "$(FLASH_SIZE)"
	@if [ "$(FLASH_IMG)" != "Flash.bin" ]; then cp -f "$(FLASH_IMG)" Flash.bin; fi

vfs: make-vfs

tinygo-uf2:
	mkdir -p $(DIST_DIR)
	$(TINYGO) build -target=pico2 -size=full -tags=shell -o $(UF2) .

tinygo-flash:
	$(TINYGO) flash -target=pico2 .

tinygo-uf2-picocalc:
	mkdir -p $(DIST_DIR)
	$(TINYGO) build -target=pico2 -size=full -tags=picocalc,shell,spark_vi -o dist/spark-picocalc.uf2 .

tinygo-flash-picocalc:
	$(TINYGO) flash -target=pico -tags=picocalc .

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR)
