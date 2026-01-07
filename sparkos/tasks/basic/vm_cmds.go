package basic

import (
	"fmt"
	"strings"
)

func (m *vm) execLet(s *scanner) (stepResult, error) {
	return m.execAssignment(s)
}

func (m *vm) execAssignment(s *scanner) (stepResult, error) {
	v, err := parseVarRef(m, s)
	if err != nil {
		return stepResult{}, err
	}
	s.skipSpaces()
	if !s.accept('=') {
		return stepResult{}, ErrSyntax
	}

	switch v.kind {
	case varString:
		val, err := m.parseStringExpr(s)
		if err != nil {
			return stepResult{}, err
		}
		return stepResult{}, m.setVar(v, val)
	case varInt, varArray:
		n, err := parseIntExpr(m, s)
		if err != nil {
			return stepResult{}, err
		}
		return stepResult{}, m.setVar(v, n)
	default:
		return stepResult{}, ErrType
	}
}

func (m *vm) parseStringExpr(s *scanner) (string, error) {
	if lit, ok := s.readQuotedString(); ok {
		return lit, nil
	}
	v, err := parseVarRef(m, s)
	if err != nil {
		return "", err
	}
	if v.kind != varString {
		return "", ErrType
	}
	val, err := m.getVar(v)
	if err != nil {
		return "", err
	}
	str, ok := val.(string)
	if !ok {
		return "", ErrType
	}
	return str, nil
}

func (m *vm) execPrint(s *scanner) (stepResult, error) {
	var out strings.Builder
	skipNL := false

	for {
		s.skipSpaces()
		if s.eof() {
			break
		}
		if s.peek() == ';' {
			s.pos++
			skipNL = true
			continue
		}
		if s.peek() == ',' {
			s.pos++
			out.WriteByte(' ')
			continue
		}

		if lit, ok := s.readQuotedString(); ok {
			out.WriteString(lit)
		} else if looksLikeStringVar(s) {
			str, err := m.parseStringExpr(s)
			if err != nil {
				return stepResult{}, err
			}
			out.WriteString(str)
		} else {
			n, err := parseIntExpr(m, s)
			if err != nil {
				return stepResult{}, err
			}
			out.WriteString(fmt.Sprintf("%d", n))
		}
	}

	if skipNL {
		return stepResult{output: []string{out.String()}}, nil
	}
	return stepResult{output: []string{out.String()}}, nil
}

func looksLikeStringVar(s *scanner) bool {
	s.skipSpaces()
	if s.eof() {
		return false
	}
	c := s.peek()
	if c >= 'a' && c <= 'z' {
		c = c - 'a' + 'A'
	}
	if c < 'A' || c > 'Z' {
		return false
	}
	if s.pos+1 >= len(s.buf) {
		return false
	}
	return s.buf[s.pos+1] == '$'
}

func (m *vm) execInput(s *scanner) (stepResult, error) {
	v, err := parseVarRef(m, s)
	if err != nil {
		return stepResult{}, err
	}
	if v.kind == varArray {
		return stepResult{}, ErrType
	}
	m.pendingInputs = []varRef{v}
	return stepResult{awaitInput: true, awaitVar: v}, nil
}

func (m *vm) execIf(s *scanner) (stepResult, error) {
	left, err := parseIntExpr(m, s)
	if err != nil {
		return stepResult{}, err
	}
	s.skipSpaces()
	op := readRelOp(s)
	if op == "" {
		return stepResult{}, ErrSyntax
	}
	right, err := parseIntExpr(m, s)
	if err != nil {
		return stepResult{}, err
	}
	s.skipSpaces()
	if strings.ToUpper(s.readWord()) != "THEN" {
		return stepResult{}, ErrSyntax
	}
	cond := evalRel(left, op, right)
	if !cond {
		return stepResult{}, nil
	}
	thenStmt := s.rest()
	if thenStmt == "" {
		return stepResult{}, ErrSyntax
	}
	return m.execLine(thenStmt)
}

func readRelOp(s *scanner) string {
	if s.accept('<') {
		if s.accept('=') {
			return "<="
		}
		if s.accept('>') {
			return "<>"
		}
		return "<"
	}
	if s.accept('>') {
		if s.accept('=') {
			return ">="
		}
		return ">"
	}
	if s.accept('=') {
		return "="
	}
	return ""
}

func evalRel(a int32, op string, b int32) bool {
	switch op {
	case "=":
		return a == b
	case "<>":
		return a != b
	case "<":
		return a < b
	case ">":
		return a > b
	case "<=":
		return a <= b
	case ">=":
		return a >= b
	default:
		return false
	}
}

func (m *vm) execGoto(s *scanner) (stepResult, error) {
	lineNo, err := parseIntExpr(m, s)
	if err != nil {
		return stepResult{}, err
	}
	if err := m.gotoLine(lineNo); err != nil {
		return stepResult{}, err
	}
	return stepResult{}, nil
}

func (m *vm) execGosub(s *scanner) (stepResult, error) {
	lineNo, err := parseIntExpr(m, s)
	if err != nil {
		return stepResult{}, err
	}
	m.callStack = append(m.callStack, m.pc)
	if err := m.gotoLine(lineNo); err != nil {
		return stepResult{}, err
	}
	return stepResult{}, nil
}

func (m *vm) execReturn(_ *scanner) (stepResult, error) {
	if len(m.callStack) == 0 {
		return stepResult{}, ErrBadSub
	}
	ret := m.callStack[len(m.callStack)-1]
	m.callStack = m.callStack[:len(m.callStack)-1]
	m.pc = ret
	return stepResult{}, nil
}

func (m *vm) execFor(s *scanner) (stepResult, error) {
	v, err := parseVarRef(m, s)
	if err != nil {
		return stepResult{}, err
	}
	if v.kind != varInt {
		return stepResult{}, ErrType
	}
	s.skipSpaces()
	if !s.accept('=') {
		return stepResult{}, ErrSyntax
	}
	start, err := parseIntExpr(m, s)
	if err != nil {
		return stepResult{}, err
	}
	s.skipSpaces()
	if strings.ToUpper(s.readWord()) != "TO" {
		return stepResult{}, ErrSyntax
	}
	end, err := parseIntExpr(m, s)
	if err != nil {
		return stepResult{}, err
	}
	m.intVars[v.index] = start
	m.forStack = append(m.forStack, forFrame{varIndex: v.index, end: end, pc: m.pc})
	return stepResult{}, nil
}

func (m *vm) execNext(s *scanner) (stepResult, error) {
	v, err := parseVarRef(m, s)
	if err != nil {
		return stepResult{}, err
	}
	if v.kind != varInt {
		return stepResult{}, ErrType
	}
	if len(m.forStack) == 0 {
		return stepResult{}, ErrBadNext
	}
	top := m.forStack[len(m.forStack)-1]
	if top.varIndex != v.index {
		return stepResult{}, ErrBadNext
	}
	m.intVars[v.index]++
	if m.intVars[v.index] <= top.end {
		m.pc = top.pc
		return stepResult{}, nil
	}
	m.forStack = m.forStack[:len(m.forStack)-1]
	return stepResult{}, nil
}

func (m *vm) execSleep(s *scanner) (stepResult, error) {
	n, err := parseIntExpr(m, s)
	if err != nil {
		return stepResult{}, err
	}
	if n <= 0 {
		return stepResult{}, nil
	}
	start := m.ctx.NowTick()
	for m.ctx.NowTick()-start < uint64(n) {
		m.ctx.BlockOnTick()
	}
	return stepResult{}, nil
}

func (m *vm) execDim(s *scanner) (stepResult, error) {
	v, err := parseVarRef(m, s)
	if err != nil {
		return stepResult{}, err
	}
	if v.kind != varArray {
		return stepResult{}, ErrSyntax
	}
	if v.sub < 0 {
		return stepResult{}, ErrBadDim
	}
	m.arrVars[v.index] = make([]int32, v.sub+1)
	return stepResult{}, nil
}
