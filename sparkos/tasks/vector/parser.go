package vector

import (
	"fmt"
	"strconv"
	"unicode"
)

type tokenKind uint8

const (
	tokEOF tokenKind = iota
	tokNumber
	tokIdent
	tokSemi
	tokPlus
	tokMinus
	tokStar
	tokSlash
	tokCaret
	tokLParen
	tokRParen
	tokComma
	tokAssign
)

type token struct {
	kind tokenKind
	text string
	num  Number
}

type lexer struct {
	s string
	i int
}

func (l *lexer) next() token {
	for l.i < len(l.s) {
		r := rune(l.s[l.i])
		if r == '\n' || r == ';' {
			l.i++
			return token{kind: tokSemi, text: ";"}
		}
		if !unicode.IsSpace(r) {
			break
		}
		l.i++
	}
	if l.i >= len(l.s) {
		return token{kind: tokEOF}
	}

	switch l.s[l.i] {
	case ';':
		l.i++
		return token{kind: tokSemi, text: ";"}
	case '+':
		l.i++
		return token{kind: tokPlus, text: "+"}
	case '-':
		l.i++
		return token{kind: tokMinus, text: "-"}
	case '*':
		l.i++
		return token{kind: tokStar, text: "*"}
	case '/':
		l.i++
		return token{kind: tokSlash, text: "/"}
	case '^':
		l.i++
		return token{kind: tokCaret, text: "^"}
	case '(':
		l.i++
		return token{kind: tokLParen, text: "("}
	case '[':
		l.i++
		return token{kind: tokLParen, text: "("}
	case ')':
		l.i++
		return token{kind: tokRParen, text: ")"}
	case ']':
		l.i++
		return token{kind: tokRParen, text: ")"}
	case ',':
		l.i++
		return token{kind: tokComma, text: ","}
	case '=':
		l.i++
		return token{kind: tokAssign, text: "="}
	}

	ch := rune(l.s[l.i])
	if isIdentStart(ch) {
		start := l.i
		l.i++
		for l.i < len(l.s) && isIdentContinue(rune(l.s[l.i])) {
			l.i++
		}
		return token{kind: tokIdent, text: l.s[start:l.i]}
	}
	if ch == '.' || unicode.IsDigit(ch) {
		start := l.i
		l.i = scanNumber(l.s, l.i)
		txt := l.s[start:l.i]
		n, ok := parseNumber(txt)
		if !ok {
			return token{kind: tokEOF, text: txt}
		}
		return token{kind: tokNumber, text: txt, num: n}
	}

	l.i++
	return token{kind: tokEOF, text: string(ch)}
}

func scanNumber(s string, i int) int {
	start := i
	if i < len(s) && s[i] == '.' {
		i++
	}
	for i < len(s) && unicode.IsDigit(rune(s[i])) {
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && unicode.IsDigit(rune(s[i])) {
			i++
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		k := j
		for k < len(s) && unicode.IsDigit(rune(s[k])) {
			k++
		}
		if k > j {
			i = k
		}
	}
	if i == start {
		return start
	}
	return i
}

func parseNumber(txt string) (Number, bool) {
	isFloat := false
	for i := 0; i < len(txt); i++ {
		switch txt[i] {
		case '.', 'e', 'E':
			isFloat = true
		}
	}
	if !isFloat {
		u, err := strconv.ParseInt(txt, 10, 64)
		if err == nil {
			return RatNumber(RatInt(u)), true
		}
	}
	f, err := strconv.ParseFloat(txt, 64)
	if err != nil {
		return Number{}, false
	}
	return Float(f), true
}

func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isIdentContinue(r rune) bool {
	return isIdentStart(r) || unicode.IsDigit(r)
}

type parser struct {
	l   lexer
	cur token
}

type actionKind uint8

const (
	actionEval actionKind = iota
	actionAssignVar
	actionAssignFunc
)

type action struct {
	kind      actionKind
	expr      node
	varName   string
	funcName  string
	funcParam string
}

func parseInput(s string) ([]action, error) {
	p := &parser{l: lexer{s: s}}
	p.cur = p.l.next()
	var out []action
	for {
		for p.cur.kind == tokSemi {
			p.next()
		}
		if p.cur.kind == tokEOF {
			return out, nil
		}
		act, err := p.parseTop()
		if err != nil {
			return nil, err
		}
		out = append(out, act)
		for p.cur.kind == tokSemi {
			p.next()
		}
		if p.cur.kind == tokEOF {
			return out, nil
		}
	}
}

func (p *parser) parseTop() (action, error) {
	if p.cur.kind == tokIdent {
		identTok := p.cur
		afterIdentPos := p.l.i
		name := identTok.text
		p.next()

		if p.cur.kind == tokAssign {
			p.next()
			ex, err := p.parseExpr()
			if err != nil {
				return action{}, err
			}
			return action{kind: actionAssignVar, varName: name, expr: ex}, nil
		}

		if p.cur.kind == tokLParen {
			// This can be either a function definition `f(x)=...` or a regular call `f(...)`.
			// Only treat it as a definition if it matches the full `ident ( ident ) =` shape.
			if act, ok, err := p.tryParseFuncDef(name, identTok, afterIdentPos); err != nil {
				return action{}, err
			} else if ok {
				return act, nil
			}
		}

		p.l.i = afterIdentPos
		p.cur = identTok
	}

	ex, err := p.parseExpr()
	if err != nil {
		return action{}, err
	}
	return action{kind: actionEval, expr: ex}, nil
}

func (p *parser) next() { p.cur = p.l.next() }

func (p *parser) tryParseFuncDef(name string, identTok token, afterIdentPos int) (action, bool, error) {
	restore := func() {
		p.cur = identTok
		p.l.i = afterIdentPos
	}

	if p.cur.kind != tokLParen {
		return action{}, false, nil
	}
	p.next()
	if p.cur.kind != tokIdent {
		restore()
		return action{}, false, nil
	}
	param := p.cur.text
	p.next()
	if p.cur.kind != tokRParen {
		restore()
		return action{}, false, nil
	}
	p.next()
	if p.cur.kind != tokAssign {
		restore()
		return action{}, false, nil
	}
	p.next()
	ex, err := p.parseExpr()
	if err != nil {
		return action{}, false, err
	}
	return action{kind: actionAssignFunc, funcName: name, funcParam: param, expr: ex}, true, nil
}

func (p *parser) parseExpr() (node, error) {
	return p.parseSum()
}

func (p *parser) parseSum() (node, error) {
	left, err := p.parseProduct()
	if err != nil {
		return nil, err
	}
	for p.cur.kind == tokPlus || p.cur.kind == tokMinus {
		op := p.cur.text[0]
		p.next()
		right, err := p.parseProduct()
		if err != nil {
			return nil, err
		}
		left = nodeBinary{op: op, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseProduct() (node, error) {
	left, err := p.parsePower()
	if err != nil {
		return nil, err
	}
	for p.cur.kind == tokStar || p.cur.kind == tokSlash {
		op := p.cur.text[0]
		p.next()
		right, err := p.parsePower()
		if err != nil {
			return nil, err
		}
		left = nodeBinary{op: op, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parsePower() (node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	if p.cur.kind == tokCaret {
		p.next()
		right, err := p.parsePower()
		if err != nil {
			return nil, err
		}
		return nodeBinary{op: '^', left: left, right: right}, nil
	}
	return left, nil
}

func (p *parser) parseUnary() (node, error) {
	if p.cur.kind == tokPlus || p.cur.kind == tokMinus {
		op := p.cur.text[0]
		p.next()
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return nodeUnary{op: op, x: x}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (node, error) {
	switch p.cur.kind {
	case tokNumber:
		v := p.cur.num
		p.next()
		return nodeNumber{v: v}, nil
	case tokIdent:
		name := p.cur.text
		p.next()
		if p.cur.kind == tokLParen {
			p.next()
			var args []node
			if p.cur.kind != tokRParen {
				for {
					ex, err := p.parseExpr()
					if err != nil {
						return nil, err
					}
					args = append(args, ex)
					if p.cur.kind == tokComma {
						p.next()
						continue
					}
					break
				}
			}
			if p.cur.kind != tokRParen {
				return nil, fmt.Errorf("%w: expected ')'", ErrParse)
			}
			p.next()
			return nodeCall{name: name, args: args}, nil
		}
		return nodeIdent{name: name}, nil
	case tokLParen:
		p.next()
		ex, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.cur.kind != tokRParen {
			return nil, fmt.Errorf("%w: expected ')'", ErrParse)
		}
		p.next()
		return ex, nil
	default:
		return nil, fmt.Errorf("%w: unexpected %q", ErrParse, p.cur.text)
	}
}
