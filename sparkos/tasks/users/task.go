package users

import (
	"crypto/rand"
	"fmt"
	"image/color"
	"strings"

	"spark/hal"
	vfsclient "spark/sparkos/client/vfs"
	"spark/sparkos/fonts/font6x8cp1251"
	"spark/sparkos/internal/userdb"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"

	"tinygo.org/x/tinyfont"
)

const pbkdf2Iters = 20000

type inputMode uint8

const (
	inputNone inputMode = iota
	inputNewUserName
	inputNewUserPass
	inputNewUserPassConfirm
	inputSetPass
	inputSetPassConfirm
	inputEditHome
	inputConfirmDelete
)

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

	initialized bool

	w int
	h int

	nowTick uint64

	vfs *vfsclient.Client

	users []userdb.Record
	sel   int
	top   int

	status string

	inbuf []byte

	inputMode   inputMode
	inputPrompt string
	inputSecret bool
	input       []rune
	inputCursor int

	pendingUser  string
	pendingPass1 []byte
}

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
				t.requestExit(ctx)
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
				if t.active {
					t.render()
				}

			case proto.MsgAppSelect:
				appID, _, ok := proto.DecodeAppSelectPayload(msg.Payload())
				if !ok || appID != proto.AppUsers {
					continue
				}
				if t.active {
					t.refresh(ctx)
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
			if t.inputMode != inputNone && (now/350)%2 == 0 {
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
	if !t.initialized {
		t.initApp(ctx)
		t.initialized = true
	} else {
		t.refresh(ctx)
	}
}

func (t *Task) initApp(ctx *kernel.Context) {
	t.w = t.fb.Width()
	t.h = t.fb.Height()
	t.vfs = vfsclient.New(t.vfsCap)
	t.sel = 0
	t.top = 0
	t.inputMode = inputNone
	t.refresh(ctx)
}

func (t *Task) refresh(ctx *kernel.Context) {
	if t.vfs == nil {
		return
	}

	selected := ""
	if u, ok := t.selected(); ok {
		selected = u.Name
	}

	typ, size, err := t.vfs.Stat(ctx, userdb.UsersPath)
	if err != nil || typ != proto.VFSEntryFile || size == 0 {
		t.users = nil
		t.sel = 0
		t.top = 0
		t.status = "No users database (login as root once)."
		return
	}
	if size > userdb.MaxFileBytes {
		t.status = "Users database too large."
		return
	}

	b, err := readAll(ctx, t.vfs, userdb.UsersPath, size, userdb.MaxFileBytes)
	if err != nil {
		t.status = fmt.Sprintf("Read users: %v", err)
		return
	}
	users, err := userdb.ParseUsersFile(b)
	if err != nil {
		t.status = fmt.Sprintf("Parse users: %v", err)
		return
	}
	t.users = users
	if selected != "" {
		t.selectByName(selected)
	}
	t.ensureSelectionInRange()
}

func (t *Task) requestExit(ctx *kernel.Context) {
	t.active = false
	if !t.muxCap.Valid() {
		return
	}
	_ = ctx.SendToCapRetry(t.muxCap, uint16(proto.MsgAppControl), proto.AppControlPayload(false), kernel.Capability{}, 500)
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
	if t.inputMode != inputNone {
		t.handleInputKey(ctx, k)
		return
	}

	switch k.kind {
	case keyEsc:
		t.requestExit(ctx)
	case keyUp:
		t.moveSel(-1)
	case keyDown:
		t.moveSel(1)
	case keyRune:
		t.handleRune(ctx, k.r)
	}
}

func (t *Task) handleRune(ctx *kernel.Context, r rune) {
	switch r {
	case 'q':
		t.requestExit(ctx)
	case 'R':
		t.refresh(ctx)
		t.status = "Reloaded."
	case 'n':
		t.beginNewUser()
	case 'p':
		t.beginSetPassword()
	case 'r':
		t.toggleRole(ctx)
	case 'h':
		t.beginEditHome()
	case 'd':
		t.beginDelete()
	}
}

func (t *Task) beginNewUser() {
	t.pendingUser = ""
	t.pendingPass1 = wipeBytes(t.pendingPass1)
	t.inputMode = inputNewUserName
	t.inputPrompt = "New user: "
	t.inputSecret = false
	t.input = t.input[:0]
	t.inputCursor = 0
}

func (t *Task) beginSetPassword() {
	u, ok := t.selected()
	if !ok {
		t.status = "No selection."
		return
	}
	if u.Name == "root" {
		t.status = "Use shell login to set root password."
		return
	}

	t.pendingUser = u.Name
	t.pendingPass1 = wipeBytes(t.pendingPass1)
	t.inputMode = inputSetPass
	t.inputPrompt = "New password: "
	t.inputSecret = true
	t.input = t.input[:0]
	t.inputCursor = 0
}

func (t *Task) beginEditHome() {
	u, ok := t.selected()
	if !ok {
		t.status = "No selection."
		return
	}
	if u.Name == "root" {
		t.status = "Root home is fixed."
		return
	}

	t.pendingUser = u.Name
	t.inputMode = inputEditHome
	t.inputPrompt = "Home: "
	t.inputSecret = false
	t.input = append(t.input[:0], []rune(u.Home)...)
	t.inputCursor = len(t.input)
}

func (t *Task) beginDelete() {
	u, ok := t.selected()
	if !ok {
		t.status = "No selection."
		return
	}
	if u.Name == "root" {
		t.status = "Cannot delete root."
		return
	}

	t.pendingUser = u.Name
	t.inputMode = inputConfirmDelete
	t.inputPrompt = "Delete " + u.Name + "? y/N: "
	t.inputSecret = false
	t.input = t.input[:0]
	t.inputCursor = 0
}

func (t *Task) handleInputKey(ctx *kernel.Context, k key) {
	switch k.kind {
	case keyEsc:
		t.cancelInput()
		t.status = "Canceled."
	case keyEnter:
		text := string(t.input)
		mode := t.inputMode
		t.cancelInput()
		t.applyInput(ctx, mode, text)
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
		if len(t.input) >= 160 {
			return
		}
		t.input = append(t.input, 0)
		copy(t.input[t.inputCursor+1:], t.input[t.inputCursor:])
		t.input[t.inputCursor] = k.r
		t.inputCursor++
	}
}

func (t *Task) cancelInput() {
	t.inputMode = inputNone
	t.inputPrompt = ""
	t.inputSecret = false
	t.input = t.input[:0]
	t.inputCursor = 0
}

func (t *Task) applyInput(ctx *kernel.Context, mode inputMode, text string) {
	switch mode {
	case inputNewUserName:
		name := strings.TrimSpace(text)
		if name == "" {
			t.status = "Empty user."
			return
		}
		if _, ok := userdb.Find(t.users, name); ok {
			t.status = "User already exists."
			return
		}
		t.pendingUser = name
		t.inputMode = inputNewUserPass
		t.inputPrompt = "Password: "
		t.inputSecret = true
		t.input = t.input[:0]
		t.inputCursor = 0
		return

	case inputNewUserPass:
		if text == "" {
			t.status = "Empty password."
			t.beginNewUser()
			return
		}
		t.pendingPass1 = wipeBytes(t.pendingPass1)
		t.pendingPass1 = append(t.pendingPass1, []byte(text)...)
		t.inputMode = inputNewUserPassConfirm
		t.inputPrompt = "Confirm: "
		t.inputSecret = true
		t.input = t.input[:0]
		t.inputCursor = 0
		return

	case inputNewUserPassConfirm:
		ok := len(t.pendingPass1) == len(text) && subtleCompareBytes(t.pendingPass1, []byte(text))
		if !ok {
			t.pendingPass1 = wipeBytes(t.pendingPass1)
			t.status = "Mismatch."
			t.inputMode = inputNewUserPass
			t.inputPrompt = "Password: "
			t.inputSecret = true
			return
		}

		name := strings.TrimSpace(t.pendingUser)
		t.pendingUser = ""
		pass := t.pendingPass1
		t.pendingPass1 = nil
		defer func() { _ = wipeBytes(pass) }()

		home := "/home/" + name
		rec := userdb.Record{
			Name:   name,
			Role:   userdb.RoleUser,
			Home:   home,
			Scheme: userdb.SchemePBKDF2SHA256,
			Iter:   pbkdf2Iters,
		}
		rec.Salt = t.makeSalt(ctx)
		rec.Hash = userdb.HashPasswordPBKDF2SHA256(rec.Iter, rec.Salt, pass)

		t.users = append(t.users, rec)
		if err := t.ensureDir(ctx, "/home"); err != nil {
			t.status = fmt.Sprintf("mkdir /home: %v", err)
		} else if err := t.ensureDir(ctx, home); err != nil {
			t.status = fmt.Sprintf("mkdir %s: %v", home, err)
		}
		if err := t.saveUsers(ctx, name); err != nil {
			t.status = fmt.Sprintf("Save: %v", err)
			return
		}
		t.status = "Created."
		return

	case inputSetPass:
		if text == "" {
			t.status = "Empty password."
			return
		}
		t.pendingPass1 = wipeBytes(t.pendingPass1)
		t.pendingPass1 = append(t.pendingPass1, []byte(text)...)
		t.inputMode = inputSetPassConfirm
		t.inputPrompt = "Confirm: "
		t.inputSecret = true
		t.input = t.input[:0]
		t.inputCursor = 0
		return

	case inputSetPassConfirm:
		ok := len(t.pendingPass1) == len(text) && subtleCompareBytes(t.pendingPass1, []byte(text))
		if !ok {
			t.pendingPass1 = wipeBytes(t.pendingPass1)
			t.status = "Mismatch."
			t.inputMode = inputSetPass
			t.inputPrompt = "New password: "
			t.inputSecret = true
			return
		}

		target := strings.TrimSpace(t.pendingUser)
		pass := t.pendingPass1
		t.pendingUser = ""
		t.pendingPass1 = nil
		defer func() { _ = wipeBytes(pass) }()

		if err := t.setPassword(ctx, target, pass); err != nil {
			t.status = fmt.Sprintf("Set password: %v", err)
			return
		}
		t.status = "Password updated."
		return

	case inputEditHome:
		target := strings.TrimSpace(t.pendingUser)
		newHome := strings.TrimSpace(text)
		t.pendingUser = ""
		if newHome == "" {
			t.status = "Empty home."
			return
		}
		if err := t.setHome(ctx, target, newHome); err != nil {
			t.status = fmt.Sprintf("Set home: %v", err)
			return
		}
		t.status = "Home updated."
		return

	case inputConfirmDelete:
		target := strings.TrimSpace(t.pendingUser)
		t.pendingUser = ""
		if strings.ToLower(strings.TrimSpace(text)) != "y" {
			t.status = "Not deleted."
			return
		}
		if err := t.deleteUser(ctx, target); err != nil {
			t.status = fmt.Sprintf("Delete: %v", err)
			return
		}
		t.status = "Deleted."
		return
	}
}

func (t *Task) ensureSelectionInRange() {
	if len(t.users) == 0 {
		t.sel = 0
		t.top = 0
		return
	}
	if t.sel < 0 {
		t.sel = 0
	}
	if t.sel >= len(t.users) {
		t.sel = len(t.users) - 1
	}
	if t.top < 0 {
		t.top = 0
	}
	if t.top > t.sel {
		t.top = t.sel
	}
}

func (t *Task) moveSel(delta int) {
	if len(t.users) == 0 {
		return
	}
	t.sel += delta
	if t.sel < 0 {
		t.sel = 0
	}
	if t.sel >= len(t.users) {
		t.sel = len(t.users) - 1
	}
	t.ensureSelectionVisible()
}

func (t *Task) ensureSelectionVisible() {
	_, _, _, listH, _ := t.layout()
	lineH := int(t.fontHeight) + 2
	rows := listH / lineH
	if rows < 1 {
		rows = 1
	}
	if t.top > t.sel {
		t.top = t.sel
	}
	if t.sel >= t.top+rows {
		t.top = t.sel - rows + 1
	}
	if t.top < 0 {
		t.top = 0
	}
}

func (t *Task) selected() (userdb.Record, bool) {
	if t.sel < 0 || t.sel >= len(t.users) {
		return userdb.Record{}, false
	}
	return t.users[t.sel], true
}

func (t *Task) selectByName(name string) {
	for i, u := range t.users {
		if u.Name == name {
			t.sel = i
			t.ensureSelectionVisible()
			return
		}
	}
}

func (t *Task) toggleRole(ctx *kernel.Context) {
	u, ok := t.selected()
	if !ok {
		t.status = "No selection."
		return
	}
	if u.Name == "root" {
		t.status = "Root role is fixed."
		return
	}
	switch u.Role {
	case userdb.RoleUser:
		u.Role = userdb.RoleAdmin
	case userdb.RoleAdmin:
		u.Role = userdb.RoleUser
	default:
		u.Role = userdb.RoleUser
	}
	t.users[t.sel] = u
	if err := t.saveUsers(ctx, u.Name); err != nil {
		t.status = fmt.Sprintf("Save: %v", err)
		return
	}
	t.status = "Role updated."
}

func (t *Task) setPassword(ctx *kernel.Context, name string, pass []byte) error {
	if name == "" {
		return fmt.Errorf("empty user")
	}
	found := false
	for i := range t.users {
		if t.users[i].Name != name {
			continue
		}
		t.users[i].Scheme = userdb.SchemePBKDF2SHA256
		t.users[i].Iter = pbkdf2Iters
		t.users[i].Salt = t.makeSalt(ctx)
		t.users[i].Hash = userdb.HashPasswordPBKDF2SHA256(t.users[i].Iter, t.users[i].Salt, pass)
		found = true
		break
	}
	if !found {
		return fmt.Errorf("unknown user")
	}
	return t.saveUsers(ctx, name)
}

func (t *Task) setHome(ctx *kernel.Context, name, home string) error {
	if name == "" || home == "" {
		return fmt.Errorf("empty input")
	}
	if err := t.ensureDir(ctx, home); err != nil {
		return fmt.Errorf("ensure home dir: %w", err)
	}

	found := false
	for i := range t.users {
		if t.users[i].Name != name {
			continue
		}
		t.users[i].Home = home
		found = true
		break
	}
	if !found {
		return fmt.Errorf("unknown user")
	}
	return t.saveUsers(ctx, name)
}

func (t *Task) deleteUser(ctx *kernel.Context, name string) error {
	if name == "" {
		return fmt.Errorf("empty user")
	}
	if name == "root" {
		return fmt.Errorf("cannot delete root")
	}
	out := t.users[:0]
	for _, u := range t.users {
		if u.Name == name {
			continue
		}
		out = append(out, u)
	}
	t.users = out
	return t.saveUsers(ctx, "")
}

func (t *Task) saveUsers(ctx *kernel.Context, selectName string) error {
	if t.vfs == nil {
		return fmt.Errorf("vfs unavailable")
	}
	if err := t.vfs.Mkdir(ctx, "/etc"); err != nil {
		_ = err
	}
	b, err := userdb.FormatUsersFile(t.users)
	if err != nil {
		return err
	}
	if _, err := t.vfs.Write(ctx, userdb.UsersPath, proto.VFSWriteTruncate, b); err != nil {
		return err
	}
	t.refresh(ctx)
	if selectName != "" {
		t.selectByName(selectName)
	}
	return nil
}

func (t *Task) ensureDir(ctx *kernel.Context, dir string) error {
	if t.vfs == nil {
		return fmt.Errorf("vfs unavailable")
	}
	typ, _, err := t.vfs.Stat(ctx, dir)
	if err == nil {
		if typ == proto.VFSEntryDir {
			return nil
		}
		return fmt.Errorf("%s is not a directory", dir)
	}
	if err := t.vfs.Mkdir(ctx, dir); err == nil {
		return nil
	}
	typ, _, err = t.vfs.Stat(ctx, dir)
	if err != nil {
		return err
	}
	if typ != proto.VFSEntryDir {
		return fmt.Errorf("%s is not a directory", dir)
	}
	return nil
}

func (t *Task) makeSalt(ctx *kernel.Context) [16]byte {
	var out [16]byte
	if _, err := rand.Read(out[:]); err == nil {
		return out
	}
	seed := uint32(ctx.NowTick()) ^ (uint32(len(t.users)) * 0x9e3779b9)
	if seed == 0 {
		seed = 0x12345678
	}
	x := seed
	for i := 0; i < len(out); i++ {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		out[i] = byte(x)
	}
	return out
}

func subtleCompareBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	ok := byte(0)
	for i := 0; i < len(a); i++ {
		ok |= a[i] ^ b[i]
	}
	return ok == 0
}

func wipeBytes(b []byte) []byte {
	for i := range b {
		b[i] = 0
	}
	return b[:0]
}

func (t *Task) layout() (xList, yList, listW, listH, inputH int) {
	pad := 6
	titleH := int(t.fontHeight) + 10
	footerH := int(t.fontHeight) + 10
	inputH = 0
	if t.inputMode != inputNone {
		inputH = int(t.fontHeight) + 6
	}
	yList = pad + titleH
	listH = t.h - yList - footerH - pad - inputH
	if listH < 0 {
		listH = 0
	}
	xList = pad
	listW = (t.w - pad*3) / 2
	if listW < 0 {
		listW = 0
	}
	return xList, yList, listW, listH, inputH
}

func (t *Task) render() {
	buf := t.fb.Buffer()
	if buf == nil || t.fb.Format() != hal.PixelFormatRGB565 {
		return
	}

	clearRGB565(buf, rgb565From888(0x08, 0x0B, 0x10))
	stride := t.fb.StrideBytes()

	pad := 6
	title := "Users"
	t.drawText(pad, pad, title, color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})
	t.drawText(pad+textWidth(t.font, title)+8, pad, "n new  p pass  r role  h home  d del  q back", color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF})

	xList, yList, listW, listH, inputH := t.layout()
	xInfo := xList + listW + pad
	yInfo := yList
	infoW := t.w - xInfo - pad
	infoH := listH

	fillRectRGB565(buf, stride, xList, yList, listW, listH, rgb565From888(0x10, 0x14, 0x1E))
	drawRectOutlineRGB565(buf, stride, xList, yList, listW, listH, rgb565From888(0x2B, 0x33, 0x44))
	fillRectRGB565(buf, stride, xInfo, yInfo, infoW, infoH, rgb565From888(0x10, 0x14, 0x1E))
	drawRectOutlineRGB565(buf, stride, xInfo, yInfo, infoW, infoH, rgb565From888(0x2B, 0x33, 0x44))

	t.renderList(xList+2, yList+2, listW-4, listH-4)
	t.renderInfo(xInfo+2, yInfo+2, infoW-4, infoH-4)

	footerY := t.h - (int(t.fontHeight) + 10) - pad - inputH
	if footerY < 0 {
		footerY = 0
	}
	t.drawText(pad, footerY, truncateToWidth(t.font, t.status, t.w-pad*2), color.RGBA{R: 0xAA, G: 0xAA, B: 0xAA, A: 0xFF})

	if t.inputMode != inputNone {
		t.renderInputBar()
	}
}

func (t *Task) renderList(x, y, w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	lineH := int(t.fontHeight) + 2
	rows := h / lineH
	if rows < 1 {
		rows = 1
	}
	t.ensureSelectionVisible()

	buf := t.fb.Buffer()
	if buf == nil {
		return
	}
	stride := t.fb.StrideBytes()

	yy := y
	for i := 0; i < rows; i++ {
		idx := t.top + i
		if idx >= len(t.users) {
			break
		}
		u := t.users[idx]
		line := u.Name
		if u.Role == userdb.RoleAdmin {
			line += "  (admin)"
		}

		if idx == t.sel {
			fillRectRGB565(buf, stride, x, yy, w, lineH, rgb565From888(0x20, 0x35, 0x60))
			t.drawText(x+4, yy, truncateToWidth(t.font, line, w-8), color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF})
		} else {
			t.drawText(x+4, yy, truncateToWidth(t.font, line, w-8), color.RGBA{R: 0xD6, G: 0xD6, B: 0xD6, A: 0xFF})
		}
		yy += lineH
	}
}

func (t *Task) renderInfo(x, y, w, h int) {
	if w <= 0 || h <= 0 {
		return
	}

	u, ok := t.selected()
	if !ok {
		t.drawText(x, y, "No users.", color.RGBA{R: 0xD6, G: 0xD6, B: 0xD6, A: 0xFF})
		return
	}
	yy := y
	lineH := int(t.fontHeight) + 2

	t.drawText(x, yy, "User: "+u.Name, color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})
	yy += lineH
	t.drawText(x, yy, "Role: "+u.Role.String(), color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})
	yy += lineH
	t.drawText(x, yy, "Home: "+truncateToWidth(t.font, u.Home, w), color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})
	yy += lineH * 2

	t.drawText(x, yy, "Keys:", color.RGBA{R: 0xAA, G: 0xAA, B: 0xAA, A: 0xFF})
	yy += lineH
	for _, ln := range []string{
		"n  create user",
		"p  set password",
		"r  toggle role",
		"h  edit home",
		"d  delete user",
		"R  reload",
		"q  back to shell",
	} {
		if yy+lineH > y+h {
			break
		}
		t.drawText(x, yy, ln, color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF})
		yy += lineH
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

	value := string(t.input)
	if t.inputSecret {
		value = strings.Repeat("*", len(t.input))
	}
	prompt := t.inputPrompt + value
	t.drawText(6, y0+3, truncateToWidth(t.font, prompt, t.w-12), color.RGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})

	if (t.nowTick/350)%2 == 0 {
		cursorX := 6 + textWidth(t.font, t.inputPrompt) + textWidthRunes(t.font, t.input[:t.inputCursor])
		fillRectRGB565(buf, t.fb.StrideBytes(), cursorX, y0+3, 2, int(t.fontHeight), rgb565From888(0xFF, 0xFF, 0xFF))
	}
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
	if x0 < 0 {
		w += x0
		x0 = 0
	}
	if y0 < 0 {
		h += y0
		y0 = 0
	}
	if w <= 0 || h <= 0 {
		return
	}
	lo := byte(pixel)
	hi := byte(pixel >> 8)
	for y := 0; y < h; y++ {
		row := (y0+y)*stride + x0*2
		for x := 0; x < w; x++ {
			off := row + x*2
			if off+1 >= len(buf) {
				break
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
	fillRectRGB565(buf, stride, x0, y0, w, 1, pixel)
	fillRectRGB565(buf, stride, x0, y0+h-1, w, 1, pixel)
	fillRectRGB565(buf, stride, x0, y0, 1, h, pixel)
	fillRectRGB565(buf, stride, x0+w-1, y0, 1, h, pixel)
}

func rgb565From888(r, g, b uint8) uint16 {
	r5 := uint16(r>>3) & 0x1F
	g6 := uint16(g>>2) & 0x3F
	b5 := uint16(b>>3) & 0x1F
	return (r5 << 11) | (g6 << 5) | b5
}

func readAll(ctx *kernel.Context, vfs *vfsclient.Client, path string, size, max uint32) ([]byte, error) {
	var out []byte
	var off uint32
	for {
		if max > 0 && uint32(len(out)) >= max {
			return nil, fmt.Errorf("file too large")
		}
		want := uint16(size - off)
		if want == 0 {
			break
		}
		maxPayload := uint16(kernel.MaxMessageBytes - 11)
		if want > maxPayload {
			want = maxPayload
		}
		b, eof, err := vfs.ReadAt(ctx, path, off, want)
		if err != nil {
			return nil, err
		}
		if len(b) == 0 {
			break
		}
		out = append(out, b...)
		off += uint32(len(b))
		if eof {
			break
		}
	}
	return out, nil
}
