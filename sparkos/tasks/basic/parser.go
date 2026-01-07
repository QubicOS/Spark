package basic

import (
	"strings"
)

type scanner struct {
	buf []byte
	pos int
}

func newScanner(s string) scanner {
	return scanner{buf: []byte(s)}
}

func (s *scanner) eof() bool { return s.pos >= len(s.buf) }

func (s *scanner) peek() byte {
	if s.eof() {
		return 0
	}
	return s.buf[s.pos]
}

func (s *scanner) accept(b byte) bool {
	if s.eof() || s.buf[s.pos] != b {
		return false
	}
	s.pos++
	return true
}

func (s *scanner) skipSpaces() {
	for !s.eof() {
		switch s.buf[s.pos] {
		case ' ', '\t':
			s.pos++
		default:
			return
		}
	}
}

func (s *scanner) readWord() string {
	s.skipSpaces()
	start := s.pos
	for !s.eof() {
		c := s.buf[s.pos]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			s.pos++
			continue
		}
		break
	}
	return string(s.buf[start:s.pos])
}

func (s *scanner) readQuotedString() (string, bool) {
	s.skipSpaces()
	if !s.accept('"') {
		return "", false
	}
	start := s.pos
	for !s.eof() {
		if s.accept('"') {
			return string(s.buf[start : s.pos-1]), true
		}
		s.pos++
	}
	return "", false
}

func (s *scanner) rest() string {
	if s.eof() {
		return ""
	}
	return strings.TrimSpace(string(s.buf[s.pos:]))
}

func parseIntExpr(m *vm, s *scanner) (int32, error) {
	return parseAddSub(m, s)
}

func parseAddSub(m *vm, s *scanner) (int32, error) {
	v, err := parseMulDiv(m, s)
	if err != nil {
		return 0, err
	}
	for {
		s.skipSpaces()
		switch s.peek() {
		case '+':
			s.pos++
			rhs, err := parseMulDiv(m, s)
			if err != nil {
				return 0, err
			}
			v += rhs
		case '-':
			s.pos++
			rhs, err := parseMulDiv(m, s)
			if err != nil {
				return 0, err
			}
			v -= rhs
		default:
			return v, nil
		}
	}
}

func parseMulDiv(m *vm, s *scanner) (int32, error) {
	v, err := parseFactor(m, s)
	if err != nil {
		return 0, err
	}
	for {
		s.skipSpaces()
		switch s.peek() {
		case '*':
			s.pos++
			rhs, err := parseFactor(m, s)
			if err != nil {
				return 0, err
			}
			v *= rhs
		case '/':
			s.pos++
			rhs, err := parseFactor(m, s)
			if err != nil {
				return 0, err
			}
			if rhs == 0 {
				return 0, ErrType
			}
			v /= rhs
		default:
			return v, nil
		}
	}
}

func parseFactor(m *vm, s *scanner) (int32, error) {
	s.skipSpaces()
	if s.eof() {
		return 0, ErrSyntax
	}
	if s.accept('(') {
		v, err := parseIntExpr(m, s)
		if err != nil {
			return 0, err
		}
		s.skipSpaces()
		if !s.accept(')') {
			return 0, ErrSyntax
		}
		return v, nil
	}
	if s.accept('-') {
		v, err := parseFactor(m, s)
		if err != nil {
			return 0, err
		}
		return -v, nil
	}

	if isDigit(s.peek()) {
		return s.readInt()
	}

	word := strings.ToUpper(s.readWord())
	switch word {
	case "ABS":
		return parseFunc1(m, s, func(x int32) int32 {
			if x < 0 {
				return -x
			}
			return x
		})
	case "SGN":
		return parseFunc1(m, s, func(x int32) int32 {
			switch {
			case x < 0:
				return -1
			case x > 0:
				return 1
			default:
				return 0
			}
		})
	case "INT":
		return parseFunc1(m, s, func(x int32) int32 { return x })
	case "RND":
		return parseFunc1(m, s, func(x int32) int32 {
			if x <= 0 {
				return 0
			}
			return int32((uint32(x) * lcg()) >> 16)
		})
	case "EOF":
		return parseEOF(m, s)
	}

	s.pos -= len(word)
	vref, err := parseVarRef(m, s)
	if err != nil {
		return 0, err
	}
	if vref.kind == varString {
		return 0, ErrType
	}
	val, err := m.getVar(vref)
	if err != nil {
		return 0, err
	}
	n, ok := val.(int32)
	if !ok {
		return 0, ErrType
	}
	return n, nil
}

func (s *scanner) readInt() (int32, error) {
	s.skipSpaces()
	start := s.pos
	for !s.eof() && isDigit(s.peek()) {
		s.pos++
	}
	if start == s.pos {
		return 0, ErrSyntax
	}
	var n int32
	for i := start; i < s.pos; i++ {
		n = n*10 + int32(s.buf[i]-'0')
	}
	return n, nil
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func parseFunc1(m *vm, s *scanner, f func(int32) int32) (int32, error) {
	s.skipSpaces()
	if !s.accept('(') {
		return 0, ErrSyntax
	}
	v, err := parseIntExpr(m, s)
	if err != nil {
		return 0, err
	}
	s.skipSpaces()
	if !s.accept(')') {
		return 0, ErrSyntax
	}
	return f(v), nil
}

func parseEOF(m *vm, s *scanner) (int32, error) {
	s.skipSpaces()
	if !s.accept('(') {
		return 0, ErrSyntax
	}
	fd, err := parseIntExpr(m, s)
	if err != nil {
		return 0, err
	}
	s.skipSpaces()
	if !s.accept(')') {
		return 0, ErrSyntax
	}
	if err := m.ensureVFS(); err != nil {
		return 0, err
	}
	h, err := m.getFile(fd)
	if err != nil {
		return 0, err
	}
	_, size, err := m.vfs.Stat(m.ctx, h.path)
	if err != nil {
		return 0, err
	}
	if h.pos >= size {
		return 1, nil
	}
	return 0, nil
}

func parseVarRef(m *vm, s *scanner) (varRef, error) {
	s.skipSpaces()
	if s.eof() {
		return varRef{}, ErrSyntax
	}
	ch := s.peek()
	if ch >= 'a' && ch <= 'z' {
		ch = ch - 'a' + 'A'
	}
	if ch < 'A' || ch > 'Z' {
		return varRef{}, ErrSyntax
	}
	s.pos++
	idx := int(ch - 'A')

	if s.accept('$') {
		return varRef{kind: varString, index: idx}, nil
	}
	if s.accept('(') {
		sub, err := parseIntExpr(m, s)
		if err != nil {
			return varRef{}, err
		}
		s.skipSpaces()
		if !s.accept(')') {
			return varRef{}, ErrSyntax
		}
		return varRef{kind: varArray, index: idx, sub: int(sub)}, nil
	}
	return varRef{kind: varInt, index: idx}, nil
}

var rngState uint32 = 1

func lcg() uint32 {
	rngState = rngState*1103515245 + 12345
	return rngState
}
