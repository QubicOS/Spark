package vector

import (
	"errors"
	"fmt"
	"math"
)

// polyRat is a univariate polynomial with rational coefficients:
// p(x) = c0 + c1*x + c2*x^2 + ...
type polyRat struct {
	coeffs []Rat
}

func (p polyRat) degree() int {
	for i := len(p.coeffs) - 1; i >= 0; i-- {
		if p.coeffs[i].num != 0 {
			return i
		}
	}
	return -1
}

func (p polyRat) trim() polyRat {
	i := len(p.coeffs)
	for i > 0 && p.coeffs[i-1].num == 0 {
		i--
	}
	out := make([]Rat, i)
	copy(out, p.coeffs[:i])
	return polyRat{coeffs: out}
}

func ratZero() Rat { return RatInt(0) }
func ratOne() Rat  { return RatInt(1) }

func ratNeg(a Rat) Rat { return Rat{num: -a.num, den: a.den} }

func ratIsZero(a Rat) bool { return a.num == 0 }

func ratFromNumber(n Number) (Rat, bool) {
	if n.kind == numberRat {
		return n.r, true
	}
	f := n.f
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return Rat{}, false
	}
	if f != math.Trunc(f) {
		return Rat{}, false
	}
	if f < math.MinInt64 || f > math.MaxInt64 {
		return Rat{}, false
	}
	return RatInt(int64(f)), true
}

func ratAdd(a, b Rat) (Rat, error) { return a.Add(b) }
func ratSub(a, b Rat) (Rat, error) { return a.Sub(b) }
func ratMul(a, b Rat) (Rat, error) { return a.Mul(b) }
func ratDiv(a, b Rat) (Rat, error) { return a.Div(b) }

func polyRatFromExpr(e *env, ex node, varName string) (polyRat, error) {
	if e == nil {
		return polyRat{}, errors.New("nil env")
	}
	p, ok, err := polyRatFromNode(e, ex, varName)
	if err != nil {
		return polyRat{}, err
	}
	if !ok {
		return polyRat{}, errors.New("not a rational polynomial")
	}
	return p.trim(), nil
}

func polyRatFromNode(e *env, ex node, varName string) (polyRat, bool, error) {
	switch n := ex.(type) {
	case nodeNumber:
		r, ok := ratFromNumber(n.v)
		if !ok {
			return polyRat{}, false, nil
		}
		return polyRat{coeffs: []Rat{r}}, true, nil
	case nodeIdent:
		if n.name == varName {
			return polyRat{coeffs: []Rat{ratZero(), ratOne()}}, true, nil
		}
		v, ok := e.vars[n.name]
		if !ok || !v.IsNumber() {
			return polyRat{}, false, nil
		}
		r, ok := ratFromNumber(v.num)
		if !ok {
			return polyRat{}, false, nil
		}
		return polyRat{coeffs: []Rat{r}}, true, nil
	case nodeUnary:
		p, ok, err := polyRatFromNode(e, n.x, varName)
		if err != nil || !ok {
			return polyRat{}, ok, err
		}
		switch n.op {
		case '+':
			return p, true, nil
		case '-':
			out := make([]Rat, len(p.coeffs))
			for i, c := range p.coeffs {
				out[i] = ratNeg(c)
			}
			return polyRat{coeffs: out}, true, nil
		default:
			return polyRat{}, false, nil
		}
	case nodeBinary:
		a, okA, err := polyRatFromNode(e, n.left, varName)
		if err != nil {
			return polyRat{}, false, err
		}
		b, okB, err := polyRatFromNode(e, n.right, varName)
		if err != nil {
			return polyRat{}, false, err
		}
		if !okA || !okB {
			return polyRat{}, false, nil
		}
		switch n.op {
		case '+':
			out, err := polyRatAdd(a, b)
			return out, err == nil, err
		case '-':
			out, err := polyRatSub(a, b)
			return out, err == nil, err
		case '*':
			out, err := polyRatMul(a, b)
			return out, err == nil, err
		case '^':
			expNode, ok := n.right.(nodeNumber)
			if !ok {
				return polyRat{}, false, nil
			}
			expF := expNode.v.Float64()
			if math.IsNaN(expF) || math.IsInf(expF, 0) || expF != math.Trunc(expF) {
				return polyRat{}, false, nil
			}
			exp := int(expF)
			if exp < 0 || exp > 64 {
				return polyRat{}, false, nil
			}
			out, err := polyRatPow(a, exp)
			return out, err == nil, err
		default:
			return polyRat{}, false, nil
		}
	default:
		return polyRat{}, false, nil
	}
}

func polyRatAdd(a, b polyRat) (polyRat, error) {
	n := len(a.coeffs)
	if len(b.coeffs) > n {
		n = len(b.coeffs)
	}
	out := make([]Rat, n)
	for i := 0; i < n; i++ {
		av := ratZero()
		bv := ratZero()
		if i < len(a.coeffs) {
			av = a.coeffs[i]
		}
		if i < len(b.coeffs) {
			bv = b.coeffs[i]
		}
		s, err := ratAdd(av, bv)
		if err != nil {
			return polyRat{}, err
		}
		out[i] = s
	}
	return polyRat{coeffs: out}.trim(), nil
}

func polyRatSub(a, b polyRat) (polyRat, error) {
	n := len(a.coeffs)
	if len(b.coeffs) > n {
		n = len(b.coeffs)
	}
	out := make([]Rat, n)
	for i := 0; i < n; i++ {
		av := ratZero()
		bv := ratZero()
		if i < len(a.coeffs) {
			av = a.coeffs[i]
		}
		if i < len(b.coeffs) {
			bv = b.coeffs[i]
		}
		s, err := ratSub(av, bv)
		if err != nil {
			return polyRat{}, err
		}
		out[i] = s
	}
	return polyRat{coeffs: out}.trim(), nil
}

func polyRatMul(a, b polyRat) (polyRat, error) {
	if len(a.coeffs) == 0 || len(b.coeffs) == 0 {
		return polyRat{}, nil
	}
	out := make([]Rat, len(a.coeffs)+len(b.coeffs)-1)
	for i := range out {
		out[i] = ratZero()
	}
	for i, ca := range a.coeffs {
		if ratIsZero(ca) {
			continue
		}
		for j, cb := range b.coeffs {
			if ratIsZero(cb) {
				continue
			}
			p, err := ratMul(ca, cb)
			if err != nil {
				return polyRat{}, err
			}
			s, err := ratAdd(out[i+j], p)
			if err != nil {
				return polyRat{}, err
			}
			out[i+j] = s
		}
	}
	return polyRat{coeffs: out}.trim(), nil
}

func polyRatPow(p polyRat, exp int) (polyRat, error) {
	if exp == 0 {
		return polyRat{coeffs: []Rat{ratOne()}}, nil
	}
	if exp == 1 {
		return p.trim(), nil
	}
	base := p.trim()
	out := polyRat{coeffs: []Rat{ratOne()}}
	for exp > 0 {
		if exp&1 == 1 {
			var err error
			out, err = polyRatMul(out, base)
			if err != nil {
				return polyRat{}, err
			}
		}
		exp >>= 1
		if exp == 0 {
			break
		}
		var err error
		base, err = polyRatMul(base, base)
		if err != nil {
			return polyRat{}, err
		}
	}
	return out.trim(), nil
}

func polyRatMonic(p polyRat) (polyRat, error) {
	p = p.trim()
	d := p.degree()
	if d < 0 {
		return polyRat{}, nil
	}
	lead := p.coeffs[d]
	if ratIsZero(lead) {
		return polyRat{}, fmt.Errorf("leading coefficient is zero")
	}
	out := make([]Rat, len(p.coeffs))
	for i, c := range p.coeffs {
		v, err := ratDiv(c, lead)
		if err != nil {
			return polyRat{}, err
		}
		out[i] = v
	}
	return polyRat{coeffs: out}.trim(), nil
}

func polyRatDivMod(a, b polyRat) (q polyRat, r polyRat, err error) {
	a = a.trim()
	b = b.trim()
	da := a.degree()
	db := b.degree()
	if db < 0 {
		return polyRat{}, polyRat{}, fmt.Errorf("division by zero polynomial")
	}
	if da < db {
		return polyRat{}, a, nil
	}

	lead := b.coeffs[db]
	if ratIsZero(lead) {
		return polyRat{}, polyRat{}, fmt.Errorf("zero leading coefficient")
	}

	rem := make([]Rat, len(a.coeffs))
	copy(rem, a.coeffs)
	quo := make([]Rat, da-db+1)
	for i := range quo {
		quo[i] = ratZero()
	}

	for k := da; k >= db; k-- {
		if ratIsZero(rem[k]) {
			continue
		}
		t, err := ratDiv(rem[k], lead)
		if err != nil {
			return polyRat{}, polyRat{}, err
		}
		quo[k-db] = t
		for j := 0; j <= db; j++ {
			p, err := ratMul(t, b.coeffs[j])
			if err != nil {
				return polyRat{}, polyRat{}, err
			}
			rem[k-db+j], err = ratSub(rem[k-db+j], p)
			if err != nil {
				return polyRat{}, polyRat{}, err
			}
		}
	}

	return polyRat{coeffs: quo}.trim(), polyRat{coeffs: rem}.trim(), nil
}

func polyRatGCD(a, b polyRat) (polyRat, error) {
	a = a.trim()
	b = b.trim()
	if a.degree() < 0 {
		return polyRatMonic(b)
	}
	if b.degree() < 0 {
		return polyRatMonic(a)
	}

	for b.degree() >= 0 {
		_, r, err := polyRatDivMod(a, b)
		if err != nil {
			return polyRat{}, err
		}
		a, b = b, r
	}
	return polyRatMonic(a)
}

func polyRatToExprHorner(p polyRat, varName string) node {
	if len(p.coeffs) == 0 {
		return nodeNumber{v: RatNumber(RatInt(0))}
	}
	x := nodeIdent{name: varName}
	ex := node(nodeNumber{v: RatNumber(p.coeffs[len(p.coeffs)-1])})
	for i := len(p.coeffs) - 2; i >= 0; i-- {
		ex = nodeBinary{
			op: '+',
			left: nodeBinary{
				op:    '*',
				left:  ex,
				right: x,
			},
			right: nodeNumber{v: RatNumber(p.coeffs[i])},
		}.Simplify()
	}
	return ex
}
