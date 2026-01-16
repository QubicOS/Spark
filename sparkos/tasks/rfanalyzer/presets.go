package rfanalyzer

import (
	"errors"
	"fmt"
	"strings"

	vfsclient "spark/sparkos/client/vfs"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

const (
	presetDir  = "/rf"
	presetsDir = "/rf/presets"
	presetExt  = ".cfg"
	maxPreset  = 16 * 1024
)

func (t *Task) ensurePresetsDir(ctx *kernel.Context) error {
	if t.vfs == nil {
		return errors.New("vfs unavailable")
	}
	if err := t.vfs.Mkdir(ctx, presetDir); err != nil {
		if typ, _, statErr := t.vfs.Stat(ctx, presetDir); statErr != nil || typ != proto.VFSEntryDir {
			return err
		}
	}
	if err := t.vfs.Mkdir(ctx, presetsDir); err != nil {
		if typ, _, statErr := t.vfs.Stat(ctx, presetsDir); statErr != nil || typ != proto.VFSEntryDir {
			return err
		}
	}
	return nil
}

func (t *Task) presetPath(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("empty preset name")
	}
	if strings.Contains(name, "/") {
		return "", errors.New("preset name may not contain '/'")
	}
	name = sanitizePresetName(name)
	if name == "" {
		return "", errors.New("invalid preset name")
	}
	return presetsDir + "/" + name + presetExt, nil
}

func sanitizePresetName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('_')
		default:
		}
	}
	return b.String()
}

func (t *Task) savePreset(ctx *kernel.Context, name string) error {
	if t.vfs == nil {
		return errors.New("vfs unavailable")
	}
	if err := t.ensurePresetsDir(ctx); err != nil {
		return fmt.Errorf("ensure presets dir: %w", err)
	}
	path, err := t.presetPath(name)
	if err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("# rfanalyzer preset v1\n")
	b.WriteString(fmt.Sprintf("range_lo=%d\n", t.channelRangeLo))
	b.WriteString(fmt.Sprintf("range_hi=%d\n", t.channelRangeHi))
	b.WriteString(fmt.Sprintf("dwell_ms=%d\n", t.dwellTimeMs))
	b.WriteString(fmt.Sprintf("scan_step=%d\n", clampInt(t.scanSpeedScalar, 1, 10)))
	b.WriteString(fmt.Sprintf("rate=%s\n", t.dataRate))
	b.WriteString(fmt.Sprintf("crc=%s\n", t.crcMode))
	if t.autoAck {
		b.WriteString("auto_ack=1\n")
	} else {
		b.WriteString("auto_ack=0\n")
	}
	b.WriteString(fmt.Sprintf("pwr=%s\n", t.powerLevel))
	b.WriteString(fmt.Sprintf("wf_palette=%s\n", t.wfPalette))

	data := []byte(b.String())
	if len(data) > maxPreset {
		return fmt.Errorf("preset too large (%d bytes)", len(data))
	}
	if _, err := t.vfs.Write(ctx, path, proto.VFSWriteTruncate, data); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	t.activePreset = sanitizePresetName(name)
	t.presetDirty = false
	return nil
}

func (t *Task) loadPreset(ctx *kernel.Context, name string) error {
	if t.vfs == nil {
		return errors.New("vfs unavailable")
	}
	path, err := t.presetPath(name)
	if err != nil {
		return err
	}
	data, err := readAll(ctx, t.vfs, path, maxPreset)
	if err != nil {
		return err
	}

	// Apply into a copy first.
	cfg := t.snapshotConfig()

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		cfg.applyKV(key, val)
	}

	t.applyConfig(cfg)
	t.activePreset = sanitizePresetName(name)
	t.presetDirty = false
	t.invalidate(dirtyAll)
	return nil
}

func readAll(ctx *kernel.Context, vfs *vfsclient.Client, path string, maxBytes int) ([]byte, error) {
	if vfs == nil {
		return nil, errors.New("vfs unavailable")
	}
	typ, size, err := vfs.Stat(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if typ != proto.VFSEntryFile {
		return nil, fmt.Errorf("not a file: %s", path)
	}
	if size > uint32(maxBytes) {
		return nil, fmt.Errorf("too large: %d bytes", size)
	}

	var out []byte
	var off uint32
	for {
		if maxBytes > 0 && len(out) >= maxBytes {
			return out[:maxBytes], nil
		}
		limit := uint16(kernel.MaxMessageBytes)
		if maxBytes > 0 {
			rem := maxBytes - len(out)
			if rem <= 0 {
				rem = 1
			}
			if rem < int(limit) {
				limit = uint16(rem)
			}
		}

		chunk, eof, err := vfs.ReadAt(ctx, path, off, limit)
		if err != nil {
			return nil, fmt.Errorf("read %s at %d: %w", path, off, err)
		}
		if len(chunk) == 0 && eof {
			return out, nil
		}
		if maxBytes > 0 && len(out)+len(chunk) > maxBytes {
			chunk = chunk[:maxBytes-len(out)]
		}
		out = append(out, chunk...)
		off += uint32(len(chunk))
		if eof || off >= size {
			return out, nil
		}
	}
}

type cfgSnapshot struct {
	channelRangeLo int
	channelRangeHi int
	dwellTimeMs    int
	scanStep       int
	dataRate       rfDataRate
	crcMode        rfCRCMode
	autoAck        bool
	powerLevel     rfPowerLevel
	wfPalette      wfPalette
}

func (t *Task) snapshotConfig() cfgSnapshot {
	return cfgSnapshot{
		channelRangeLo: t.channelRangeLo,
		channelRangeHi: t.channelRangeHi,
		dwellTimeMs:    t.dwellTimeMs,
		scanStep:       clampInt(t.scanSpeedScalar, 1, 10),
		dataRate:       t.dataRate,
		crcMode:        t.crcMode,
		autoAck:        t.autoAck,
		powerLevel:     t.powerLevel,
		wfPalette:      t.wfPalette,
	}
}

func (t *Task) applyConfig(cfg cfgSnapshot) {
	t.channelRangeLo = clampInt(cfg.channelRangeLo, 0, maxChannel)
	t.channelRangeHi = clampInt(cfg.channelRangeHi, 0, maxChannel)
	if t.channelRangeLo > t.channelRangeHi {
		t.channelRangeLo, t.channelRangeHi = t.channelRangeHi, t.channelRangeLo
	}
	t.dwellTimeMs = clampInt(cfg.dwellTimeMs, 1, 50)
	t.scanSpeedScalar = clampInt(cfg.scanStep, 1, 10)
	t.dataRate = cfg.dataRate
	t.crcMode = cfg.crcMode
	t.autoAck = cfg.autoAck
	t.powerLevel = cfg.powerLevel
	t.wfPalette = cfg.wfPalette
	t.rebuildWaterfallPalette()
	t.scanNextTick = 0
}

func (c *cfgSnapshot) applyKV(key, val string) {
	switch key {
	case "range_lo":
		c.channelRangeLo = parseIntDef(val, c.channelRangeLo)
	case "range_hi":
		c.channelRangeHi = parseIntDef(val, c.channelRangeHi)
	case "dwell_ms":
		c.dwellTimeMs = parseIntDef(val, c.dwellTimeMs)
	case "scan_step":
		c.scanStep = parseIntDef(val, c.scanStep)
	case "rate":
		if r, ok := parseRate(val); ok {
			c.dataRate = r
		}
	case "crc":
		if m, ok := parseCRC(val); ok {
			c.crcMode = m
		}
	case "auto_ack":
		c.autoAck = val == "1" || strings.EqualFold(val, "true") || strings.EqualFold(val, "on")
	case "pwr":
		if p, ok := parsePower(val); ok {
			c.powerLevel = p
		}
	case "wf_palette":
		if p, ok := parsePalette(val); ok {
			c.wfPalette = p
		}
	}
}

func parseIntDef(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n := 0
	sign := 1
	for i, r := range s {
		if i == 0 && r == '-' {
			sign = -1
			continue
		}
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
	}
	return sign * n
}

func parseRate(s string) (rfDataRate, bool) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "250K":
		return rfRate250K, true
	case "1M":
		return rfRate1M, true
	case "2M":
		return rfRate2M, true
	default:
		return 0, false
	}
}

func parseCRC(s string) (rfCRCMode, bool) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "OFF":
		return rfCRCOff, true
	case "1B":
		return rfCRC1B, true
	case "2B":
		return rfCRC2B, true
	default:
		return 0, false
	}
}

func parsePower(s string) (rfPowerLevel, bool) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "MIN":
		return rfPwrMin, true
	case "LOW":
		return rfPwrLow, true
	case "HIGH":
		return rfPwrHigh, true
	case "MAX":
		return rfPwrMax, true
	default:
		return 0, false
	}
}

func parsePalette(s string) (wfPalette, bool) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "CYAN":
		return wfPaletteCyan, true
	case "FIRE":
		return wfPaletteFire, true
	case "GRAY":
		return wfPaletteGray, true
	default:
		return 0, false
	}
}
