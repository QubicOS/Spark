package basic

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

func (m *vm) execOpen(s *scanner) (stepResult, error) {
	if err := m.ensureVFS(); err != nil {
		return stepResult{}, err
	}

	fd, err := parseIntExpr(m, s)
	if err != nil {
		return stepResult{}, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return stepResult{}, ErrSyntax
	}
	path, err := m.parseStringExpr(s)
	if err != nil {
		return stepResult{}, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return stepResult{}, ErrSyntax
	}
	modeStr, err := m.parseStringExpr(s)
	if err != nil {
		return stepResult{}, err
	}

	if fd <= 0 || int(fd) > len(m.files) {
		return stepResult{}, ErrBadFile
	}

	mode, err := parseFileMode(modeStr)
	if err != nil {
		return stepResult{}, err
	}
	h := &m.files[int(fd)-1]
	if h.inUse {
		_ = m.closeHandle(h)
	}

	h.inUse = true
	h.mode = mode
	h.path = path
	h.pos = 0

	if mode != fileRead {
		wMode, err := vfsMode(mode)
		if err != nil {
			return stepResult{}, err
		}
		w, err := m.vfs.OpenWriter(m.ctx, path, wMode)
		if err != nil {
			*h = fileHandle{}
			return stepResult{}, err
		}
		h.w = w
	}
	return stepResult{}, nil
}

func parseFileMode(s string) (fileMode, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch s {
	case "R", "RB":
		return fileRead, nil
	case "W", "WB":
		return fileWrite, nil
	case "A", "AB":
		return fileAppend, nil
	default:
		return 0, ErrBadFile
	}
}

func (m *vm) execClose(s *scanner) (stepResult, error) {
	s.skipSpaces()
	if s.eof() {
		for i := range m.files {
			_ = m.closeHandle(&m.files[i])
		}
		return stepResult{}, nil
	}
	fd, err := parseIntExpr(m, s)
	if err != nil {
		return stepResult{}, err
	}
	h, err := m.getFile(fd)
	if err != nil {
		return stepResult{}, err
	}
	return stepResult{}, m.closeHandle(h)
}

func (m *vm) closeHandle(h *fileHandle) error {
	if h == nil || !h.inUse {
		return nil
	}
	if h.w != nil {
		if _, err := h.w.Close(); err != nil {
			*h = fileHandle{}
			return err
		}
	}
	*h = fileHandle{}
	return nil
}

func (m *vm) execGetB(s *scanner) (stepResult, error) {
	if err := m.ensureVFS(); err != nil {
		return stepResult{}, err
	}

	fd, err := parseIntExpr(m, s)
	if err != nil {
		return stepResult{}, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return stepResult{}, ErrSyntax
	}
	dst, err := parseVarRef(m, s)
	if err != nil {
		return stepResult{}, err
	}
	if dst.kind != varInt {
		return stepResult{}, ErrType
	}

	h, err := m.getFile(fd)
	if err != nil {
		return stepResult{}, err
	}
	if h.mode != fileRead {
		return stepResult{}, ErrBadFile
	}
	data, eof, err := m.vfs.ReadAt(m.ctx, h.path, h.pos, 1)
	if err != nil {
		return stepResult{}, err
	}
	if eof || len(data) == 0 {
		m.intVars[dst.index] = -1
		return stepResult{}, nil
	}
	h.pos++
	m.intVars[dst.index] = int32(data[0])
	return stepResult{}, nil
}

func (m *vm) execPutB(s *scanner) (stepResult, error) {
	fd, err := parseIntExpr(m, s)
	if err != nil {
		return stepResult{}, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return stepResult{}, ErrSyntax
	}
	val, err := parseIntExpr(m, s)
	if err != nil {
		return stepResult{}, err
	}
	h, err := m.getFile(fd)
	if err != nil {
		return stepResult{}, err
	}
	if h.w == nil {
		return stepResult{}, ErrBadFile
	}
	b := []byte{byte(val)}
	if _, err := h.w.Write(b); err != nil {
		return stepResult{}, err
	}
	h.pos++
	return stepResult{}, nil
}

func (m *vm) execGetW(s *scanner) (stepResult, error) {
	if err := m.ensureVFS(); err != nil {
		return stepResult{}, err
	}

	fd, err := parseIntExpr(m, s)
	if err != nil {
		return stepResult{}, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return stepResult{}, ErrSyntax
	}
	dst, err := parseVarRef(m, s)
	if err != nil {
		return stepResult{}, err
	}
	if dst.kind != varInt {
		return stepResult{}, ErrType
	}

	h, err := m.getFile(fd)
	if err != nil {
		return stepResult{}, err
	}
	if h.mode != fileRead {
		return stepResult{}, ErrBadFile
	}
	data, eof, err := m.vfs.ReadAt(m.ctx, h.path, h.pos, 4)
	if err != nil {
		return stepResult{}, err
	}
	if eof || len(data) < 4 {
		m.intVars[dst.index] = -1
		return stepResult{}, nil
	}
	h.pos += 4
	m.intVars[dst.index] = int32(binary.LittleEndian.Uint32(data[:4]))
	return stepResult{}, nil
}

func (m *vm) execPutW(s *scanner) (stepResult, error) {
	fd, err := parseIntExpr(m, s)
	if err != nil {
		return stepResult{}, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return stepResult{}, ErrSyntax
	}
	val, err := parseIntExpr(m, s)
	if err != nil {
		return stepResult{}, err
	}
	h, err := m.getFile(fd)
	if err != nil {
		return stepResult{}, err
	}
	if h.w == nil {
		return stepResult{}, ErrBadFile
	}
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(val))
	if _, err := h.w.Write(buf[:]); err != nil {
		return stepResult{}, err
	}
	h.pos += 4
	return stepResult{}, nil
}

func (m *vm) execSeek(s *scanner) (stepResult, error) {
	fd, err := parseIntExpr(m, s)
	if err != nil {
		return stepResult{}, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return stepResult{}, ErrSyntax
	}
	pos, err := parseIntExpr(m, s)
	if err != nil {
		return stepResult{}, err
	}
	h, err := m.getFile(fd)
	if err != nil {
		return stepResult{}, err
	}
	if pos < 0 {
		pos = 0
	}
	h.pos = uint32(pos)
	return stepResult{}, nil
}

func (m *vm) execDir(s *scanner) (stepResult, error) {
	if err := m.ensureVFS(); err != nil {
		return stepResult{}, err
	}
	path, err := m.parseStringExpr(s)
	if err != nil {
		return stepResult{}, err
	}
	ents, err := m.vfs.List(m.ctx, path)
	if err != nil {
		return stepResult{}, err
	}
	out := make([]string, 0, len(ents))
	for _, e := range ents {
		out = append(out, fmt.Sprintf("%s %d", e.Name, e.Size))
	}
	return stepResult{output: out}, nil
}

func (m *vm) execMkdir(s *scanner) (stepResult, error) {
	if err := m.ensureVFS(); err != nil {
		return stepResult{}, err
	}
	path, err := m.parseStringExpr(s)
	if err != nil {
		return stepResult{}, err
	}
	return stepResult{}, m.vfs.Mkdir(m.ctx, path)
}

func (m *vm) execRmdir(s *scanner) (stepResult, error) {
	return m.execDel(s)
}

func (m *vm) execStat(s *scanner) (stepResult, error) {
	if err := m.ensureVFS(); err != nil {
		return stepResult{}, err
	}
	path, err := m.parseStringExpr(s)
	if err != nil {
		return stepResult{}, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return stepResult{}, ErrSyntax
	}
	typVar, err := parseVarRef(m, s)
	if err != nil {
		return stepResult{}, err
	}
	if typVar.kind != varInt {
		return stepResult{}, ErrType
	}
	s.skipSpaces()
	if !s.accept(',') {
		return stepResult{}, ErrSyntax
	}
	sizeVar, err := parseVarRef(m, s)
	if err != nil {
		return stepResult{}, err
	}
	if sizeVar.kind != varInt {
		return stepResult{}, ErrType
	}
	typ, size, err := m.vfs.Stat(m.ctx, path)
	if err != nil {
		return stepResult{}, err
	}
	m.intVars[typVar.index] = int32(typ)
	m.intVars[sizeVar.index] = int32(size)
	return stepResult{}, nil
}

func (m *vm) execDel(s *scanner) (stepResult, error) {
	if err := m.ensureVFS(); err != nil {
		return stepResult{}, err
	}
	path, err := m.parseStringExpr(s)
	if err != nil {
		return stepResult{}, err
	}
	return stepResult{}, m.vfs.Remove(m.ctx, path)
}

func (m *vm) execRen(s *scanner) (stepResult, error) {
	if err := m.ensureVFS(); err != nil {
		return stepResult{}, err
	}
	oldPath, err := m.parseStringExpr(s)
	if err != nil {
		return stepResult{}, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return stepResult{}, ErrSyntax
	}
	newPath, err := m.parseStringExpr(s)
	if err != nil {
		return stepResult{}, err
	}
	return stepResult{}, m.vfs.Rename(m.ctx, oldPath, newPath)
}

func (m *vm) execCopy(s *scanner) (stepResult, error) {
	if err := m.ensureVFS(); err != nil {
		return stepResult{}, err
	}
	srcPath, err := m.parseStringExpr(s)
	if err != nil {
		return stepResult{}, err
	}
	s.skipSpaces()
	if !s.accept(',') {
		return stepResult{}, ErrSyntax
	}
	dstPath, err := m.parseStringExpr(s)
	if err != nil {
		return stepResult{}, err
	}
	return stepResult{}, m.vfs.Copy(m.ctx, srcPath, dstPath)
}

func (m *vm) execSpawn(s *scanner) (stepResult, error) {
	if m.spawnFn == nil {
		return stepResult{}, errors.New("spawn not available")
	}
	path, err := m.parseStringExpr(s)
	if err != nil {
		return stepResult{}, err
	}
	if err := m.spawnFn(path); err != nil {
		return stepResult{}, err
	}
	return stepResult{}, nil
}
