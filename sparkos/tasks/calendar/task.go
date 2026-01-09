package calendar

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"spark/hal"
	vfsclient "spark/sparkos/client/vfs"
	"spark/sparkos/fonts/font6x8cp1251"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"

	"tinygo.org/x/tinyfont"
)

type viewMode uint8

const (
	viewMonth viewMode = iota
	viewDay
	viewAddEvent
)

type event struct {
	startMin int
	title    string
}

type Task struct {
	disp   hal.Display
	ep     kernel.Capability
	vfsCap kernel.Capability

	fb hal.Framebuffer

	font       tinyfont.Fonter
	fontWidth  int16
	fontHeight int16

	active bool
	muxCap kernel.Capability

	w int
	h int

	nowTick uint64

	year  int
	month int
	day   int

	todayKey uint32

	mode viewMode

	events map[uint32][]event
	vfs    *vfsclient.Client

	selectedEvent int

	inbuf []byte

	inputPrompt string
	input       []rune
	inputCursor int
	status      string
}

const (
	statePath  = "/calendar/state.txt"
	eventsPath = "/calendar/events.txt"
)

func New(disp hal.Display, ep kernel.Capability, vfsCap kernel.Capability) *Task {
	return &Task{disp: disp, ep: ep, vfsCap: vfsCap}
}

func (t *Task) Run(ctx *kernel.Context) {
	ch, ok := ctx.RecvChan(t.ep)
	if !ok {
		return
	}
	if t.disp == nil {
		return
	}

	t.fb = t.disp.Framebuffer()
	if t.fb == nil {
		return
	}
	if !t.initFont() {
		return
	}

	done := make(chan struct{})
	defer close(done)

	tickCh := make(chan uint64, 8)
	go func() {
		last := ctx.NowTick()
		for {
			select {
			case <-done:
				return
			default:
			}
			last = ctx.WaitTick(last)
			select {
			case tickCh <- last:
			default:
			}
		}
	}()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			switch proto.Kind(msg.Kind) {
			case proto.MsgAppShutdown:
				t.unload()
				return

			case proto.MsgAppControl:
				if msg.Cap.Valid() {
					t.muxCap = msg.Cap
				}
				active, ok := proto.DecodeAppControlPayload(msg.Payload())
				if !ok {
					continue
				}
				t.setActive(ctx, active)

			case proto.MsgAppSelect:
				appID, arg, ok := proto.DecodeAppSelectPayload(msg.Payload())
				if !ok || appID != proto.AppCalendar {
					continue
				}
				if arg != "" {
					t.handleSelectArg(arg)
				}
				if t.active {
					t.render()
				}

			case proto.MsgTermInput:
				if !t.active {
					continue
				}
				t.handleInput(ctx, msg.Payload())
				if t.active {
					t.render()
				}
			}

		case now := <-tickCh:
			if !t.active {
				continue
			}
			t.nowTick = now
			if t.mode == viewAddEvent && (now/350)%2 == 0 {
				t.render()
			}
		}
	}
}

func (t *Task) initFont() bool {
	t.font = font6x8cp1251.Font
	t.fontHeight = 8
	_, outboxWidth := tinyfont.LineWidth(t.font, "0")
	t.fontWidth = int16(outboxWidth)
	return t.fontWidth > 0 && t.fontHeight > 0
}

func (t *Task) setActive(ctx *kernel.Context, active bool) {
	if active == t.active {
		return
	}
	t.active = active
	if !t.active {
		return
	}
	t.initApp(ctx)
	t.render()
}

func (t *Task) initApp(ctx *kernel.Context) {
	t.w = t.fb.Width()
	t.h = t.fb.Height()
	if t.w <= 0 || t.h <= 0 {
		t.active = false
		return
	}

	if t.vfs == nil && t.vfsCap.Valid() {
		t.vfs = vfsclient.New(t.vfsCap)
	}
	if t.events == nil {
		t.events = make(map[uint32][]event)
	}

	t.mode = viewMonth
	t.selectedEvent = 0
	t.status = ""
	t.inputPrompt = ""
	t.input = t.input[:0]
	t.inputCursor = 0

	t.year, t.month, t.day = 2026, 1, 1
	t.loadState(ctx)
	t.todayKey = dateKey(t.year, t.month, t.day)

	t.loadEvents(ctx)
}

func (t *Task) unload() {
	t.active = false
	t.events = nil
	t.inbuf = nil
	t.input = nil
	t.vfs = nil
}

func (t *Task) requestExit(ctx *kernel.Context) {
	t.saveState(ctx)
	t.saveEvents(ctx)

	t.active = false
	if !t.muxCap.Valid() {
		return
	}
	for {
		res := ctx.SendToCapResult(t.muxCap, uint16(proto.MsgAppControl), proto.AppControlPayload(false), kernel.Capability{})
		switch res {
		case kernel.SendOK:
			return
		case kernel.SendErrQueueFull:
			ctx.BlockOnTick()
		default:
			return
		}
	}
}

func (t *Task) handleSelectArg(arg string) {
	// Supported:
	// - YYYY-MM
	// - YYYY-MM-DD
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return
	}

	parts := strings.Split(arg, "-")
	if len(parts) < 2 || len(parts) > 3 {
		return
	}
	yy, err := strconv.Atoi(parts[0])
	if err != nil {
		return
	}
	mm, err := strconv.Atoi(parts[1])
	if err != nil || mm < 1 || mm > 12 {
		return
	}
	dd := 1
	if len(parts) == 3 {
		v, err := strconv.Atoi(parts[2])
		if err != nil || v < 1 || v > 31 {
			return
		}
		dd = v
	}
	t.year, t.month, t.day = yy, mm, clampDay(yy, mm, dd)
	t.mode = viewMonth
	t.selectedEvent = 0
	t.status = ""
}

func (t *Task) handleInput(ctx *kernel.Context, b []byte) {
	t.nowTick = ctx.NowTick()
	t.inbuf = append(t.inbuf, b...)
	buf := t.inbuf
	for len(buf) > 0 {
		n, k, ok := nextKey(buf)
		if !ok {
			break
		}
		buf = buf[n:]
		t.handleKey(ctx, k)
		if !t.active {
			t.inbuf = t.inbuf[:0]
			return
		}
	}
	t.inbuf = append(t.inbuf[:0], buf...)
}

func (t *Task) handleKey(ctx *kernel.Context, k key) {
	if t.mode == viewAddEvent {
		t.handleInputKey(ctx, k)
		return
	}

	switch k.kind {
	case keyEsc:
		t.requestExit(ctx)
	case keyEnter:
		if t.mode == viewMonth {
			t.mode = viewDay
			t.selectedEvent = 0
		} else {
			t.mode = viewMonth
		}
	case keyLeft:
		t.moveSelection(-1)
	case keyRight:
		t.moveSelection(1)
	case keyUp:
		if t.mode == viewDay {
			if t.selectedEvent > 0 {
				t.selectedEvent--
			}
			return
		}
		t.moveSelection(-7)
	case keyDown:
		if t.mode == viewDay {
			evs := t.events[dateKey(t.year, t.month, t.day)]
			if t.selectedEvent < len(evs)-1 {
				t.selectedEvent++
			}
			return
		}
		t.moveSelection(7)
	case keyRune:
		t.handleRune(ctx, k.r)
	}
}

func (t *Task) handleRune(ctx *kernel.Context, r rune) {
	switch r {
	case 'q':
		t.requestExit(ctx)
	case 'm':
		t.mode = viewMonth
		t.selectedEvent = 0
	case 'a':
		t.beginAddEvent()
	case 'd':
		t.deleteSelectedEvent(ctx)
	case 't':
		t.todayKey = dateKey(t.year, t.month, t.day)
		t.status = "Set TODAY to selected date."
	case 'n':
		t.shiftMonth(1)
	case 'b':
		t.shiftMonth(-1)
	case 'N':
		t.year++
		t.day = clampDay(t.year, t.month, t.day)
	case 'B':
		t.year--
		if t.year < 1 {
			t.year = 1
		}
		t.day = clampDay(t.year, t.month, t.day)
	case 'g':
		t.beginGotoDate()
	}
}

func (t *Task) beginAddEvent() {
	t.mode = viewAddEvent
	t.inputPrompt = "Add (HH:MM Title): "
	t.input = t.input[:0]
	t.inputCursor = 0
	t.status = "Enter time (optional) and title, then Enter."
}

func (t *Task) beginGotoDate() {
	t.mode = viewAddEvent
	t.inputPrompt = "Go to (YYYY-MM-DD): "
	t.input = t.input[:0]
	t.inputCursor = 0
	t.status = "Enter date and press Enter."
}

func (t *Task) handleInputKey(ctx *kernel.Context, k key) {
	switch k.kind {
	case keyEsc:
		t.mode = viewMonth
		t.inputPrompt = ""
		t.input = t.input[:0]
		t.inputCursor = 0
		t.status = "Canceled."
	case keyEnter:
		s := strings.TrimSpace(string(t.input))
		prompt := t.inputPrompt
		t.mode = viewMonth
		t.inputPrompt = ""
		t.input = t.input[:0]
		t.inputCursor = 0
		if strings.HasPrefix(prompt, "Go to") {
			t.handleSelectArg(s)
			return
		}
		t.addEvent(ctx, s)
	case keyBackspace:
		if t.inputCursor <= 0 || len(t.input) == 0 {
			return
		}
		copy(t.input[t.inputCursor-1:], t.input[t.inputCursor:])
		t.input = t.input[:len(t.input)-1]
		t.inputCursor--
	case keyLeft:
		if t.inputCursor > 0 {
			t.inputCursor--
		}
	case keyRight:
		if t.inputCursor < len(t.input) {
			t.inputCursor++
		}
	case keyRune:
		if len(t.input) >= 120 {
			return
		}
		t.input = append(t.input, 0)
		copy(t.input[t.inputCursor+1:], t.input[t.inputCursor:])
		t.input[t.inputCursor] = k.r
		t.inputCursor++
	}
}

func (t *Task) addEvent(ctx *kernel.Context, s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		t.status = "Empty event."
		return
	}

	startMin := -1
	title := s
	if len(s) >= 5 && s[2] == ':' {
		hh, err1 := strconv.Atoi(s[0:2])
		mm, err2 := strconv.Atoi(s[3:5])
		if err1 == nil && err2 == nil && hh >= 0 && hh <= 23 && mm >= 0 && mm <= 59 {
			startMin = hh*60 + mm
			title = strings.TrimSpace(s[5:])
			if title == "" {
				title = "(no title)"
			}
		}
	}

	k := dateKey(t.year, t.month, t.day)
	evs := t.events[k]
	evs = append(evs, event{startMin: startMin, title: title})
	sortEvents(evs)
	t.events[k] = evs
	t.selectedEvent = len(evs) - 1
	t.status = "Event added."

	t.saveEvents(ctx)
}

func (t *Task) deleteSelectedEvent(ctx *kernel.Context) {
	k := dateKey(t.year, t.month, t.day)
	evs := t.events[k]
	if len(evs) == 0 || t.selectedEvent < 0 || t.selectedEvent >= len(evs) {
		t.status = "No event selected."
		return
	}
	evs = append(evs[:t.selectedEvent], evs[t.selectedEvent+1:]...)
	if len(evs) == 0 {
		delete(t.events, k)
		t.selectedEvent = 0
	} else {
		t.events[k] = evs
		if t.selectedEvent >= len(evs) {
			t.selectedEvent = len(evs) - 1
		}
	}
	t.status = "Event deleted."
	t.saveEvents(ctx)
}

func (t *Task) moveSelection(deltaDays int) {
	y, m, d := t.year, t.month, t.day
	for deltaDays != 0 {
		if deltaDays > 0 {
			y, m, d = addOneDay(y, m, d)
			deltaDays--
		} else {
			y, m, d = subOneDay(y, m, d)
			deltaDays++
		}
	}
	t.year, t.month, t.day = y, m, d
	t.selectedEvent = 0
}

func (t *Task) shiftMonth(delta int) {
	y := t.year
	m := t.month + delta
	for m < 1 {
		m += 12
		y--
	}
	for m > 12 {
		m -= 12
		y++
	}
	if y < 1 {
		y = 1
	}
	t.year = y
	t.month = m
	t.day = clampDay(y, m, t.day)
	t.selectedEvent = 0
}

func (t *Task) ensureCalendarDir(ctx *kernel.Context) error {
	if t.vfs == nil {
		return fmt.Errorf("calendar: vfs not available")
	}
	typ, _, err := t.vfs.Stat(ctx, "/calendar")
	if err == nil {
		if typ == proto.VFSEntryDir {
			return nil
		}
		return fmt.Errorf("calendar: /calendar is not a directory")
	}
	if err := t.vfs.Mkdir(ctx, "/calendar"); err == nil {
		return nil
	}
	typ, _, err = t.vfs.Stat(ctx, "/calendar")
	if err != nil {
		return fmt.Errorf("calendar: ensure /calendar: %w", err)
	}
	if typ != proto.VFSEntryDir {
		return fmt.Errorf("calendar: /calendar is not a directory")
	}
	return nil
}

func (t *Task) loadState(ctx *kernel.Context) {
	if t.vfs == nil {
		return
	}
	if err := t.ensureCalendarDir(ctx); err != nil {
		return
	}
	typ, size, err := t.vfs.Stat(ctx, statePath)
	if err != nil || typ != proto.VFSEntryFile || size == 0 || size > 64 {
		return
	}
	b, _, err := readAll(ctx, t.vfs, statePath, size, 128)
	if err != nil {
		return
	}
	t.handleSelectArg(strings.TrimSpace(string(b)))
}

func (t *Task) saveState(ctx *kernel.Context) {
	if t.vfs == nil {
		return
	}
	if err := t.ensureCalendarDir(ctx); err != nil {
		return
	}
	s := fmt.Sprintf("%04d-%02d-%02d\n", t.year, t.month, t.day)
	_, _ = t.vfs.Write(ctx, statePath, proto.VFSWriteTruncate, []byte(s))
}

func (t *Task) loadEvents(ctx *kernel.Context) {
	if t.vfs == nil {
		return
	}
	if err := t.ensureCalendarDir(ctx); err != nil {
		return
	}
	typ, size, err := t.vfs.Stat(ctx, eventsPath)
	if err != nil || typ != proto.VFSEntryFile || size == 0 {
		return
	}
	if size > 64*1024 {
		t.status = "Events file too large."
		return
	}
	b, _, err := readAll(ctx, t.vfs, eventsPath, size, uint16(size))
	if err != nil {
		t.status = "Failed to read events."
		return
	}
	t.parseEvents(string(b))
}

func (t *Task) saveEvents(ctx *kernel.Context) {
	if t.vfs == nil || t.events == nil {
		return
	}
	if err := t.ensureCalendarDir(ctx); err != nil {
		return
	}
	data := t.serializeEvents()
	_, _ = t.vfs.Write(ctx, eventsPath, proto.VFSWriteTruncate, []byte(data))
}

func readAll(ctx *kernel.Context, c *vfsclient.Client, path string, size uint32, maxChunk uint16) ([]byte, bool, error) {
	if size == 0 {
		return nil, true, nil
	}
	out := make([]byte, 0, int(size))
	off := uint32(0)
	for {
		chunk, eof, err := c.ReadAt(ctx, path, off, maxChunk)
		if err != nil {
			return nil, false, err
		}
		out = append(out, chunk...)
		off += uint32(len(chunk))
		if eof || len(chunk) == 0 {
			return out, eof, nil
		}
		if off >= size {
			return out, true, nil
		}
	}
}

func (t *Task) parseEvents(s string) {
	clearEvents := make(map[uint32][]event)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		datePart, timePart, titlePart, ok := split3(line, "|")
		if !ok {
			continue
		}
		yy, mm, dd, ok := parseDate(datePart)
		if !ok {
			continue
		}
		startMin := -1
		if strings.TrimSpace(timePart) != "" {
			if v, ok := parseHHMM(timePart); ok {
				startMin = v
			}
		}
		title := strings.TrimSpace(titlePart)
		if title == "" {
			title = "(no title)"
		}
		k := dateKey(yy, mm, dd)
		clearEvents[k] = append(clearEvents[k], event{startMin: startMin, title: title})
	}
	for k, evs := range clearEvents {
		sortEvents(evs)
		clearEvents[k] = evs
	}
	t.events = clearEvents
}

func (t *Task) serializeEvents() string {
	var b strings.Builder
	b.WriteString("# SparkOS Calendar events.\n")
	keys := make([]uint32, 0, len(t.events))
	for k := range t.events {
		keys = append(keys, k)
	}
	sortU32(keys)
	for _, k := range keys {
		yy, mm, dd := splitDateKey(k)
		evs := t.events[k]
		sortEvents(evs)
		for _, e := range evs {
			dateStr := fmt.Sprintf("%04d-%02d-%02d", yy, mm, dd)
			timeStr := ""
			if e.startMin >= 0 {
				timeStr = fmt.Sprintf("%02d:%02d", e.startMin/60, e.startMin%60)
			}
			b.WriteString(dateStr)
			b.WriteString("|")
			b.WriteString(timeStr)
			b.WriteString("|")
			b.WriteString(strings.ReplaceAll(e.title, "\n", " "))
			b.WriteString("\n")
		}
	}
	return b.String()
}

func sortEvents(evs []event) {
	if len(evs) < 2 {
		return
	}
	for i := 0; i < len(evs); i++ {
		for j := i + 1; j < len(evs); j++ {
			a := evs[i]
			b := evs[j]
			if a.startMin == -1 && b.startMin != -1 {
				continue
			}
			if b.startMin == -1 && a.startMin != -1 {
				evs[i], evs[j] = evs[j], evs[i]
				continue
			}
			if a.startMin > b.startMin || (a.startMin == b.startMin && a.title > b.title) {
				evs[i], evs[j] = evs[j], evs[i]
			}
		}
	}
}

func sortU32(a []uint32) {
	for i := 0; i < len(a); i++ {
		for j := i + 1; j < len(a); j++ {
			if a[i] > a[j] {
				a[i], a[j] = a[j], a[i]
			}
		}
	}
}

func split3(s, sep string) (a, b, c string, ok bool) {
	i := strings.Index(s, sep)
	if i < 0 {
		return "", "", "", false
	}
	j := strings.Index(s[i+len(sep):], sep)
	if j < 0 {
		return "", "", "", false
	}
	j += i + len(sep)
	return s[:i], s[i+len(sep) : j], s[j+len(sep):], true
}

func parseDate(s string) (yy, mm, dd int, ok bool) {
	parts := strings.Split(strings.TrimSpace(s), "-")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	yy, err := strconv.Atoi(parts[0])
	if err != nil || yy < 1 {
		return 0, 0, 0, false
	}
	mm, err = strconv.Atoi(parts[1])
	if err != nil || mm < 1 || mm > 12 {
		return 0, 0, 0, false
	}
	dd, err = strconv.Atoi(parts[2])
	if err != nil || dd < 1 || dd > 31 {
		return 0, 0, 0, false
	}
	dd = clampDay(yy, mm, dd)
	return yy, mm, dd, true
}

func parseHHMM(s string) (minutes int, ok bool) {
	s = strings.TrimSpace(s)
	if len(s) != 5 || s[2] != ':' {
		return 0, false
	}
	hh, err1 := strconv.Atoi(s[0:2])
	mm, err2 := strconv.Atoi(s[3:5])
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, false
	}
	return hh*60 + mm, true
}

func dateKey(yy, mm, dd int) uint32 {
	return uint32(yy*10000 + mm*100 + dd)
}

func splitDateKey(k uint32) (yy, mm, dd int) {
	yy = int(k / 10000)
	mm = int((k / 100) % 100)
	dd = int(k % 100)
	return yy, mm, dd
}

func clampDay(yy, mm, dd int) int {
	max := daysInMonth(yy, mm)
	if dd > max {
		return max
	}
	return dd
}

func addOneDay(yy, mm, dd int) (int, int, int) {
	dd++
	if dd <= daysInMonth(yy, mm) {
		return yy, mm, dd
	}
	dd = 1
	mm++
	if mm <= 12 {
		return yy, mm, dd
	}
	return yy + 1, 1, 1
}

func subOneDay(yy, mm, dd int) (int, int, int) {
	dd--
	if dd >= 1 {
		return yy, mm, dd
	}
	mm--
	if mm < 1 {
		yy--
		mm = 12
		if yy < 1 {
			yy = 1
			mm = 1
			return yy, mm, 1
		}
	}
	dd = daysInMonth(yy, mm)
	return yy, mm, dd
}

func daysInMonth(yy, mm int) int {
	switch mm {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if isLeapYear(yy) {
			return 29
		}
		return 28
	default:
		return 30
	}
}

func isLeapYear(yy int) bool {
	if yy%400 == 0 {
		return true
	}
	if yy%100 == 0 {
		return false
	}
	return yy%4 == 0
}

// weekday returns 0=Mon..6=Sun.
func weekday(yy, mm, dd int) int {
	// Sakamoto algorithm: 0=Sun..6=Sat.
	tbl := [...]int{0, 3, 2, 5, 0, 3, 5, 1, 4, 6, 2, 4}
	y := yy
	if mm < 3 {
		y--
	}
	w := (y + y/4 - y/100 + y/400 + tbl[mm-1] + dd) % 7
	// Convert Sun=0..Sat=6 to Mon=0..Sun=6.
	if w == 0 {
		return 6
	}
	return w - 1
}

func monthName(mm int) string {
	switch mm {
	case 1:
		return "January"
	case 2:
		return "February"
	case 3:
		return "March"
	case 4:
		return "April"
	case 5:
		return "May"
	case 6:
		return "June"
	case 7:
		return "July"
	case 8:
		return "August"
	case 9:
		return "September"
	case 10:
		return "October"
	case 11:
		return "November"
	case 12:
		return "December"
	default:
		return "Month"
	}
}

func weekdayShort(wd int) string {
	switch wd {
	case 0:
		return "Mo"
	case 1:
		return "Tu"
	case 2:
		return "We"
	case 3:
		return "Th"
	case 4:
		return "Fr"
	case 5:
		return "Sa"
	case 6:
		return "Su"
	default:
		return "??"
	}
}

func (t *Task) render() {
	if !t.active || t.fb == nil || t.fb.Format() != hal.PixelFormatRGB565 {
		return
	}
	buf := t.fb.Buffer()
	if buf == nil {
		return
	}

	bg := rgb565From888(0x08, 0x0B, 0x10)
	clearRGB565(buf, bg)

	title := fmt.Sprintf("%s %04d", monthName(t.month), t.year)
	t.drawText(6, 0, "CALENDAR", color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})
	t.drawText(6, int(t.fontHeight)+1, title, color.RGBA{R: 0x9A, G: 0xC6, B: 0xFF, A: 0xFF})

	statusH := int(t.fontHeight) + 4
	inputBarH := 0
	if t.mode == viewAddEvent {
		inputBarH = int(t.fontHeight) + 6
	}
	if t.status != "" {
		y := t.h - inputBarH - int(t.fontHeight) - 1
		if y >= int(t.fontHeight)*2+2 {
			t.drawText(6, y, truncateToWidth(t.font, t.status, t.w-12), color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF})
		}
	}

	weekY := int(t.fontHeight)*2 + 8
	top := weekY + int(t.fontHeight) + 8
	margin := 6
	sideMin := 120
	if t.w < 7*14+sideMin+3*margin {
		sideMin = 0
	}

	cellWMax := (t.w - sideMin - 3*margin) / 7
	cellHMax := (t.h - top - margin - statusH - inputBarH) / 6
	cell := cellWMax
	if cellHMax < cell {
		cell = cellHMax
	}
	if cell < 14 {
		cell = 14
	}
	if cell > 34 {
		cell = 34
	}

	gridW := cell * 7
	gridH := cell * 6
	left := margin
	if sideMin > 0 {
		left = (t.w - sideMin - 2*margin - gridW) / 2
		if left < margin {
			left = margin
		}
	}

	for i := 0; i < 7; i++ {
		c := color.RGBA{R: 0xAA, G: 0xAA, B: 0xAA, A: 0xFF}
		if i >= 5 {
			c = color.RGBA{R: 0xFF, G: 0xB3, B: 0xA1, A: 0xFF}
		}
		t.drawText(left+i*cell+2, weekY, weekdayShort(i), c)
	}

	border := rgb565From888(0x2B, 0x33, 0x44)
	drawRectOutlineRGB565(buf, t.fb.StrideBytes(), left-1, top-1, gridW+2, gridH+2, border)

	firstWD := weekday(t.year, t.month, 1)
	days := daysInMonth(t.year, t.month)

	prevY, prevM := t.year, t.month-1
	if prevM < 1 {
		prevM = 12
		prevY--
	}
	prevDays := daysInMonth(prevY, prevM)

	nextY, nextM := t.year, t.month+1
	if nextM > 12 {
		nextM = 1
		nextY++
	}

	selKey := dateKey(t.year, t.month, t.day)

	for row := 0; row < 6; row++ {
		for col := 0; col < 7; col++ {
			idx := row*7 + col
			x0 := left + col*cell
			y0 := top + row*cell

			dayNum := idx - firstWD + 1
			yy, mm, dd := t.year, t.month, dayNum
			inMonth := true
			if dayNum < 1 {
				inMonth = false
				yy, mm = prevY, prevM
				dd = prevDays + dayNum
			} else if dayNum > days {
				inMonth = false
				yy, mm = nextY, nextM
				dd = dayNum - days
			}

			k := dateKey(yy, mm, dd)
			isSelected := k == selKey && t.mode == viewMonth
			isToday := k == t.todayKey

			if isSelected {
				fillRectRGB565(buf, t.fb.StrideBytes(), x0, y0, cell, cell, rgb565From888(0x1A, 0x2D, 0x44))
			}
			if isToday {
				drawRectOutlineRGB565(buf, t.fb.StrideBytes(), x0+1, y0+1, cell-2, cell-2, rgb565From888(0x4A, 0xD1, 0xFF))
			}

			c := color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF}
			if !inMonth {
				c = color.RGBA{R: 0x55, G: 0x5D, B: 0x6A, A: 0xFF}
			} else if col >= 5 {
				c = color.RGBA{R: 0xFF, G: 0xD1, B: 0x4A, A: 0xFF}
			}
			t.drawText(x0+3, y0+2, fmt.Sprintf("%d", dd), c)

			if len(t.events[k]) > 0 {
				dot := rgb565From888(0x9A, 0xC6, 0xFF)
				fillRectRGB565(buf, t.fb.StrideBytes(), x0+cell-5, y0+cell-5, 3, 3, dot)
			}
		}
	}

	if sideMin > 0 {
		panelX := left + gridW + margin
		panelY := top
		panelW := t.w - panelX - margin
		if panelW > 0 {
			drawRectOutlineRGB565(buf, t.fb.StrideBytes(), panelX-1, panelY-1, panelW, gridH+2, border)
			t.renderSidePanel(panelX, panelY, panelW, gridH)
		}
	}

	bottomY := top + gridH + margin
	bottomH := t.h - bottomY - statusH - inputBarH - margin
	if bottomH > int(t.fontHeight)*2 {
		t.renderAgendaPanel(margin, bottomY, t.w-2*margin, bottomH)
	}

	if t.mode == viewAddEvent {
		t.renderInputBar()
	}

	_ = t.fb.Present()
}

func (t *Task) renderAgendaPanel(x, y, w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	buf := t.fb.Buffer()
	if buf == nil {
		return
	}

	border := rgb565From888(0x2B, 0x33, 0x44)
	drawRectOutlineRGB565(buf, t.fb.StrideBytes(), x, y, w, h, border)

	maxTextW := w - 10
	if maxTextW < 0 {
		maxTextW = 0
	}
	lineH := int(t.fontHeight) + 2

	header := fmt.Sprintf("Agenda (selected %04d-%02d-%02d)", t.year, t.month, t.day)
	t.drawText(x+4, y+2, truncateToWidth(t.font, header, maxTextW), color.RGBA{R: 0x9A, G: 0xC6, B: 0xFF, A: 0xFF})

	yy, mm, dd := t.year, t.month, t.day
	lineY := y + 2 + lineH
	maxLines := (h - (lineY - y) - 2) / lineH
	if maxLines < 1 {
		return
	}
	if maxLines > 7 {
		maxLines = 7
	}

	for i := 0; i < maxLines; i++ {
		k := dateKey(yy, mm, dd)
		evs := t.events[k]
		prefix := fmt.Sprintf("%s %02d", weekdayShort(weekday(yy, mm, dd)), dd)
		text := prefix + "  (no events)"
		if len(evs) == 1 {
			text = prefix + "  " + formatEvent(evs[0])
		} else if len(evs) > 1 {
			text = fmt.Sprintf("%s  %s  +%d", prefix, formatEvent(evs[0]), len(evs)-1)
		}
		t.drawText(x+4, lineY, truncateToWidth(t.font, text, maxTextW), color.RGBA{R: 0xD6, G: 0xD6, B: 0xD6, A: 0xFF})
		lineY += lineH

		yy, mm, dd = addOneDay(yy, mm, dd)
	}
}

func (t *Task) renderSidePanel(x, y, w, h int) {
	maxTextW := w - 8
	if maxTextW < 0 {
		maxTextW = 0
	}
	lineH := int(t.fontHeight) + 2

	dateStr := fmt.Sprintf("%04d-%02d-%02d", t.year, t.month, t.day)
	t.drawText(x+4, y+2, truncateToWidth(t.font, dateStr, maxTextW), color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})

	k := dateKey(t.year, t.month, t.day)
	evs := t.events[k]
	lineY := y + int(t.fontHeight) + 10

	var footerLines []string
	for _, s := range []string{
		"Enter: day view  a:add  d:del",
		"n/b month  N/B year  g goto",
		"q/ESC quit  m month  t set-today",
	} {
		footerLines = append(footerLines, wrapText(t.font, s, maxTextW)...)
	}
	maxFooterLines := (h - 6) / lineH
	if maxFooterLines < 1 {
		maxFooterLines = 1
	}
	if len(footerLines) > maxFooterLines {
		footerLines = footerLines[len(footerLines)-maxFooterLines:]
	}
	footerH := len(footerLines)*lineH + 2
	mainBottom := y + h - footerH
	if mainBottom < lineY {
		mainBottom = lineY
	}

	if t.mode == viewDay {
		t.drawText(x+4, lineY, "Events:", color.RGBA{R: 0x9A, G: 0xC6, B: 0xFF, A: 0xFF})
		lineY += lineH
		for i := 0; i < len(evs); i++ {
			if lineY+int(t.fontHeight) >= mainBottom {
				break
			}
			prefix := "  "
			if i == t.selectedEvent {
				prefix = "> "
			}
			s := prefix + formatEvent(evs[i])
			c := color.RGBA{R: 0xD6, G: 0xD6, B: 0xD6, A: 0xFF}
			if i == t.selectedEvent {
				c = color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
			}
			t.drawText(x+4, lineY, truncateToWidth(t.font, s, maxTextW), c)
			lineY += lineH
		}
		if len(evs) == 0 {
			for _, line := range wrapText(t.font, "(no events)  press 'a' to add", maxTextW) {
				if lineY+int(t.fontHeight) >= mainBottom {
					break
				}
				t.drawText(x+4, lineY, line, color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF})
				lineY += lineH
			}
		}
	} else {
		t.drawText(x+4, lineY, "Events:", color.RGBA{R: 0x9A, G: 0xC6, B: 0xFF, A: 0xFF})
		lineY += lineH
		for i := 0; i < len(evs); i++ {
			if lineY+int(t.fontHeight) >= mainBottom {
				break
			}
			s := "  " + formatEvent(evs[i])
			t.drawText(x+4, lineY, truncateToWidth(t.font, s, maxTextW), color.RGBA{R: 0xD6, G: 0xD6, B: 0xD6, A: 0xFF})
			lineY += lineH
		}
		if len(evs) == 0 && lineY+int(t.fontHeight) < mainBottom {
			t.drawText(x+4, lineY, "(no events)", color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF})
			lineY += lineH
		}
	}

	footerY := y + h - footerH + 2
	for i := 0; i < len(footerLines); i++ {
		yy := footerY + i*lineH
		if yy+int(t.fontHeight) >= y+h {
			break
		}
		t.drawText(x+4, yy, truncateToWidth(t.font, footerLines[i], maxTextW), color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF})
	}
}

func (t *Task) renderInputBar() {
	buf := t.fb.Buffer()
	if buf == nil {
		return
	}
	barH := int(t.fontHeight) + 6
	y0 := t.h - barH
	fillRectRGB565(buf, t.fb.StrideBytes(), 0, y0, t.w, barH, rgb565From888(0x10, 0x14, 0x1E))
	drawRectOutlineRGB565(buf, t.fb.StrideBytes(), 0, y0, t.w, barH, rgb565From888(0x2B, 0x33, 0x44))

	prompt := t.inputPrompt + string(t.input)
	t.drawText(6, y0+3, truncateToWidth(t.font, prompt, t.w-12), color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})

	if (t.nowTick/350)%2 == 0 {
		cursorX := 6 + textWidth(t.font, t.inputPrompt) + textWidthRunes(t.font, t.input[:t.inputCursor])
		fillRectRGB565(buf, t.fb.StrideBytes(), cursorX, y0+3, 2, int(t.fontHeight), rgb565From888(0xFF, 0xFF, 0xFF))
	}
}

func formatEvent(e event) string {
	if e.startMin < 0 {
		return e.title
	}
	return fmt.Sprintf("%02d:%02d %s", e.startMin/60, e.startMin%60, e.title)
}

func truncateToWidth(f tinyfont.Fonter, s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	w, _ := tinyfont.LineWidth(f, s)
	if int(w) <= maxW {
		return s
	}
	r := []rune(s)
	for len(r) > 0 {
		r = r[:len(r)-1]
		w, _ = tinyfont.LineWidth(f, string(r)+"…")
		if int(w) <= maxW {
			return string(r) + "…"
		}
	}
	return ""
}

func wrapText(f tinyfont.Fonter, s string, maxW int) []string {
	s = strings.TrimSpace(s)
	if s == "" || maxW <= 0 {
		return nil
	}
	if int(textWidth(f, s)) <= maxW {
		return []string{s}
	}

	words := strings.Fields(s)
	var out []string

	cur := ""
	for _, word := range words {
		next := word
		if cur != "" {
			next = cur + " " + word
		}
		if int(textWidth(f, next)) <= maxW {
			cur = next
			continue
		}

		if cur != "" {
			out = append(out, cur)
			cur = ""
		}

		if int(textWidth(f, word)) <= maxW {
			cur = word
			continue
		}

		// Hard-wrap long words.
		r := []rune(word)
		for len(r) > 0 {
			n := len(r)
			for n > 1 && int(textWidthRunes(f, r[:n])) > maxW {
				n--
			}
			if n <= 0 {
				break
			}
			out = append(out, string(r[:n]))
			r = r[n:]
		}
	}

	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func textWidth(f tinyfont.Fonter, s string) int {
	w, _ := tinyfont.LineWidth(f, s)
	return int(w)
}

func textWidthRunes(f tinyfont.Fonter, r []rune) int {
	if len(r) == 0 {
		return 0
	}
	w, _ := tinyfont.LineWidth(f, string(r))
	return int(w)
}

func (t *Task) drawText(x, y int, s string, c color.RGBA) {
	d := &fbDisplayer{fb: t.fb}
	tinyfont.WriteLine(d, t.font, int16(x), int16(y)+t.fontHeight, s, c)
}

type fbDisplayer struct {
	fb hal.Framebuffer
}

func (d *fbDisplayer) Size() (x, y int16) {
	if d.fb == nil {
		return 0, 0
	}
	return int16(d.fb.Width()), int16(d.fb.Height())
}

func (d *fbDisplayer) SetPixel(x, y int16, c color.RGBA) {
	if d.fb == nil || d.fb.Format() != hal.PixelFormatRGB565 {
		return
	}
	buf := d.fb.Buffer()
	if buf == nil {
		return
	}
	w := d.fb.Width()
	h := d.fb.Height()
	ix := int(x)
	iy := int(y)
	if ix < 0 || ix >= w || iy < 0 || iy >= h {
		return
	}
	pixel := rgb565From888(c.R, c.G, c.B)
	off := iy*d.fb.StrideBytes() + ix*2
	if off < 0 || off+1 >= len(buf) {
		return
	}
	buf[off] = byte(pixel)
	buf[off+1] = byte(pixel >> 8)
}

func (d *fbDisplayer) Display() error { return nil }

func clearRGB565(buf []byte, pixel uint16) {
	lo := byte(pixel)
	hi := byte(pixel >> 8)
	for i := 0; i+1 < len(buf); i += 2 {
		buf[i] = lo
		buf[i+1] = hi
	}
}

func fillRectRGB565(buf []byte, stride, x0, y0, w, h int, pixel uint16) {
	if w <= 0 || h <= 0 {
		return
	}
	lo := byte(pixel)
	hi := byte(pixel >> 8)
	for y := 0; y < h; y++ {
		row := (y0+y)*stride + x0*2
		for x := 0; x < w; x++ {
			off := row + x*2
			if off < 0 || off+1 >= len(buf) {
				continue
			}
			buf[off] = lo
			buf[off+1] = hi
		}
	}
}

func drawRectOutlineRGB565(buf []byte, stride, x0, y0, w, h int, pixel uint16) {
	if w <= 0 || h <= 0 {
		return
	}
	drawHLineRGB565(buf, stride, x0, x0+w-1, y0, pixel)
	drawHLineRGB565(buf, stride, x0, x0+w-1, y0+h-1, pixel)
	drawVLineRGB565(buf, stride, x0, y0, y0+h-1, pixel)
	drawVLineRGB565(buf, stride, x0+w-1, y0, y0+h-1, pixel)
}

func drawHLineRGB565(buf []byte, stride, x0, x1, y int, pixel uint16) {
	if y < 0 || stride <= 0 {
		return
	}
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	lo := byte(pixel)
	hi := byte(pixel >> 8)
	row := y * stride
	for x := x0; x <= x1; x++ {
		off := row + x*2
		if off < 0 || off+1 >= len(buf) {
			continue
		}
		buf[off] = lo
		buf[off+1] = hi
	}
}

func drawVLineRGB565(buf []byte, stride, x, y0, y1 int, pixel uint16) {
	if x < 0 || stride <= 0 {
		return
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	lo := byte(pixel)
	hi := byte(pixel >> 8)
	for y := y0; y <= y1; y++ {
		off := y*stride + x*2
		if off < 0 || off+1 >= len(buf) {
			continue
		}
		buf[off] = lo
		buf[off+1] = hi
	}
}

func rgb565From888(r, g, b uint8) uint16 {
	return uint16((uint16(r>>3)&0x1F)<<11 | (uint16(g>>2)&0x3F)<<5 | (uint16(b>>3) & 0x1F))
}
