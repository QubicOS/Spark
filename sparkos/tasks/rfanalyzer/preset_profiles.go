package rfanalyzer

import (
	"fmt"
	"sort"
	"strings"

	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

const autoloadPresetPath = presetsDir + "/autoload.txt"

func (t *Task) presetsListRows() int {
	rows := t.rows - headerRows - statusRows - 10
	if rows < 6 {
		rows = 6
	}
	if rows > 14 {
		rows = 14
	}
	return rows
}

func (t *Task) refreshPresetList(ctx *kernel.Context) {
	t.presetList = nil
	t.autoloadErr = ""
	t.autoloadPreset = ""

	if ctx == nil || t.vfs == nil {
		t.autoloadErr = "vfs unavailable"
		return
	}
	if err := t.ensurePresetsDir(ctx); err != nil {
		t.autoloadErr = err.Error()
		return
	}
	entries, err := t.vfs.List(ctx, presetsDir)
	if err != nil {
		t.autoloadErr = err.Error()
		return
	}
	for _, e := range entries {
		if e.Type != proto.VFSEntryFile {
			continue
		}
		if !strings.HasSuffix(e.Name, presetExt) {
			continue
		}
		name := strings.TrimSuffix(e.Name, presetExt)
		name = sanitizePresetName(name)
		if name == "" {
			continue
		}
		t.presetList = append(t.presetList, name)
	}
	sort.Strings(t.presetList)

	t.autoloadPreset = t.readAutoloadPreset(ctx)

	if t.presetSel < 0 {
		t.presetSel = 0
	}
	if t.presetSel >= len(t.presetList) {
		t.presetSel = len(t.presetList) - 1
		if t.presetSel < 0 {
			t.presetSel = 0
		}
	}
	t.presetTop = 0
}

func (t *Task) readAutoloadPreset(ctx *kernel.Context) string {
	if ctx == nil || t.vfs == nil {
		return ""
	}
	typ, size, err := t.vfs.Stat(ctx, autoloadPresetPath)
	if err != nil || typ != proto.VFSEntryFile || size == 0 {
		return ""
	}
	if size > 64 {
		size = 64
	}
	b, err := readAtExact(ctx, t.vfs, autoloadPresetPath, 0, int(size))
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(string(b))
	name = strings.TrimSuffix(name, presetExt)
	name = sanitizePresetName(name)
	return name
}

func (t *Task) writeAutoloadPreset(ctx *kernel.Context, name string) error {
	if ctx == nil || t.vfs == nil {
		return fmt.Errorf("vfs unavailable")
	}
	if err := t.ensurePresetsDir(ctx); err != nil {
		return err
	}
	name = sanitizePresetName(strings.TrimSpace(name))
	if name == "" {
		name = ""
	}
	if _, err := t.vfs.Write(ctx, autoloadPresetPath, proto.VFSWriteTruncate, []byte(name+"\n")); err != nil {
		return err
	}
	t.autoloadPreset = name
	return nil
}

func (t *Task) maybeAutoloadPreset(ctx *kernel.Context) {
	if ctx == nil || t.vfs == nil {
		return
	}
	if t.activePreset != "" {
		return
	}
	name := t.readAutoloadPreset(ctx)
	if name == "" {
		return
	}
	if err := t.loadPreset(ctx, name); err != nil {
		return
	}
	t.recordConfig(ctx.NowTick())
}

func (t *Task) exportPresetToExports(ctx *kernel.Context, name string) error {
	if ctx == nil || t.vfs == nil {
		return fmt.Errorf("vfs unavailable")
	}
	if err := t.ensureExportDir(ctx); err != nil {
		return err
	}
	name = sanitizePresetName(strings.TrimSpace(name))
	if name == "" {
		return fmt.Errorf("invalid preset")
	}
	src, err := t.presetPath(name)
	if err != nil {
		return err
	}
	_, dst, err := t.exportPath(name, presetExt)
	if err != nil {
		return err
	}
	return t.vfs.Copy(ctx, src, dst)
}

func (t *Task) handlePresetsKey(ctx *kernel.Context, k key) {
	switch k.kind {
	case keyEsc:
		t.showPresets = false
		t.invalidate(dirtyOverlay | dirtyStatus)
		return
	case keyRune:
		switch k.r {
		case 'r', 'R':
			t.refreshPresetList(ctx)
			t.invalidate(dirtyOverlay | dirtyStatus)
			return
		case 'd', 'D':
			if t.presetSel >= 0 && t.presetSel < len(t.presetList) {
				if err := t.writeAutoloadPreset(ctx, t.presetList[t.presetSel]); err != nil {
					t.autoloadErr = err.Error()
				} else {
					t.autoloadErr = "default set: " + t.autoloadPreset
				}
				t.invalidate(dirtyOverlay | dirtyStatus)
			}
			return
		case 'c', 'C':
			if err := t.writeAutoloadPreset(ctx, ""); err != nil {
				t.autoloadErr = err.Error()
			} else {
				t.autoloadErr = "default cleared"
			}
			t.invalidate(dirtyOverlay | dirtyStatus)
			return
		case 'x', 'X':
			if t.presetSel >= 0 && t.presetSel < len(t.presetList) {
				name := t.presetList[t.presetSel]
				if err := t.exportPresetToExports(ctx, name); err != nil {
					t.autoloadErr = "export: " + err.Error()
				} else {
					t.autoloadErr = "exported: " + name
				}
				t.invalidate(dirtyOverlay | dirtyStatus)
			}
			return
		}
	case keyUp:
		if len(t.presetList) == 0 {
			return
		}
		if t.presetSel <= 0 {
			t.presetSel = len(t.presetList) - 1
		} else {
			t.presetSel--
		}
	case keyDown:
		if len(t.presetList) == 0 {
			return
		}
		if t.presetSel >= len(t.presetList)-1 {
			t.presetSel = 0
		} else {
			t.presetSel++
		}
	case keyEnter:
		if t.presetSel < 0 || t.presetSel >= len(t.presetList) {
			return
		}
		name := t.presetList[t.presetSel]
		if err := t.loadPreset(ctx, name); err != nil {
			t.autoloadErr = err.Error()
		} else {
			t.autoloadErr = ""
		}
		t.recordConfig(ctx.NowTick())
		t.invalidate(dirtyAll | dirtyOverlay | dirtyStatus)
		return
	}

	rows := t.presetsListRows()
	if t.presetSel < t.presetTop {
		t.presetTop = t.presetSel
	}
	if t.presetSel >= t.presetTop+rows {
		t.presetTop = t.presetSel - rows + 1
	}
	if t.presetTop < 0 {
		t.presetTop = 0
	}
	if t.presetTop > len(t.presetList)-1 {
		t.presetTop = len(t.presetList) - 1
		if t.presetTop < 0 {
			t.presetTop = 0
		}
	}
	t.invalidate(dirtyOverlay)
}

func (t *Task) renderPresetsOverlay(l layout) {
	boxCols := t.cols - 10
	if boxCols > 64 {
		boxCols = 64
	}
	if boxCols < 34 {
		boxCols = 34
	}
	listRows := t.presetsListRows()
	boxRows := 4 + listRows

	px := int16(5) * t.fontWidth
	py := int16(headerRows+2) * t.fontHeight
	pw := int16(boxCols) * t.fontWidth
	ph := int16(boxRows) * t.fontHeight

	_ = t.d.FillRectangle(px, py, pw, ph, colorBorder)
	_ = t.d.FillRectangle(px+1, py+1, pw-2, ph-2, colorPanelBG)
	_ = t.d.FillRectangle(px+1, py+1, pw-2, t.fontHeight+1, colorHeaderBG)
	t.drawStringClipped(px+2, py, "Preset Profiles (Esc close)", colorFG, boxCols)

	status := fmt.Sprintf("active:%s  autoload:%s", orText(t.activePreset, "(none)"), orText(t.autoloadPreset, "(off)"))
	if t.autoloadErr != "" {
		status = "msg: " + t.autoloadErr
	}
	t.drawStringClipped(px+2, py+t.fontHeight, status, colorDim, boxCols)

	y0 := py + 2*t.fontHeight
	for row := 0; row < listRows; row++ {
		i := t.presetTop + row
		if i < 0 || i >= len(t.presetList) {
			continue
		}
		name := t.presetList[i]
		active := (name != "" && name == t.activePreset)
		def := (name != "" && name == t.autoloadPreset)
		prefix := "  "
		if active {
			prefix = ">"
		} else {
			prefix = " "
		}
		if def {
			prefix += "*"
		} else {
			prefix += " "
		}
		line := prefix + " " + name

		yy := y0 + int16(row)*t.fontHeight
		fg := colorFG
		bg := colorPanelBG
		if i == t.presetSel {
			fg = colorSelFG
			bg = colorSelBG
		}
		_ = t.d.FillRectangle(px+1, yy, pw-2, t.fontHeight, bg)
		t.drawStringClipped(px+2, yy, line, fg, boxCols)
	}

	hint := "Enter load  d default  c clear  x export  r refresh"
	t.drawStringClipped(px+2, py+ph-t.fontHeight, hint, colorDim, boxCols)
}

func orText(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
