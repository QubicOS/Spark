package vector

import (
	"errors"
	"fmt"
	"math"
)

// poly is a univariate polynomial with float64 coefficients:
// p(x) = c0 + c1*x + c2*x^2 + ...
type poly struct {
	coeffs []float64
}

func (p poly) degree() int {
	for i := len(p.coeffs) - 1; i >= 0; i-- {
		if p.coeffs[i] != 0 {
			return i
		}
	}
	return -1
}

func (p poly) trim() poly {
	i := len(p.coeffs)
	for i > 0 && p.coeffs[i-1] == 0 {
		i--
	}
	out := make([]float64, i)
	copy(out, p.coeffs[:i])
	return poly{coeffs: out}
}

func (p poly) eval(x float64) float64 {
	// Horner.
	if len(p.coeffs) == 0 {
		return 0
	}
	v := p.coeffs[len(p.coeffs)-1]
	for i := len(p.coeffs) - 2; i >= 0; i-- {
		v = v*x + p.coeffs[i]
	}
	return v
}

func (p poly) deriv() poly {
	if len(p.coeffs) <= 1 {
		return poly{}
	}
	out := make([]float64, len(p.coeffs)-1)
	for i := 1; i < len(p.coeffs); i++ {
		out[i-1] = float64(i) * p.coeffs[i]
	}
	return poly{coeffs: out}.trim()
}

func polyFromExpr(e *env, ex node, varName string) (poly, error) {
	if e == nil {
		return poly{}, errors.New("nil env")
	}
	p, ok, err := polyFromNode(e, ex, varName)
	if err != nil {
		return poly{}, err
	}
	if !ok {
		return poly{}, errors.New("not a polynomial")
	}
	return p.trim(), nil
}

func polyFromNode(e *env, ex node, varName string) (poly, bool, error) {
	switch n := ex.(type) {
	case nodeNumber:
		return poly{coeffs: []float64{n.v.Float64()}}, true, nil
	case nodeIdent:
		if n.name == varName {
			return poly{coeffs: []float64{0, 1}}, true, nil
		}
		// Allow other idents if they are defined as constants.
		v, ok := e.vars[n.name]
		if !ok || !v.IsNumber() {
			return poly{}, false, nil
		}
		return poly{coeffs: []float64{v.num.Float64()}}, true, nil
	case nodeUnary:
		p, ok, err := polyFromNode(e, n.x, varName)
		if err != nil || !ok {
			return poly{}, ok, err
		}
		switch n.op {
		case '+':
			return p, true, nil
		case '-':
			out := make([]float64, len(p.coeffs))
			for i, c := range p.coeffs {
				out[i] = -c
			}
			return poly{coeffs: out}, true, nil
		default:
			return poly{}, false, nil
		}
	case nodeBinary:
		a, okA, err := polyFromNode(e, n.left, varName)
		if err != nil {
			return poly{}, false, err
		}
		b, okB, err := polyFromNode(e, n.right, varName)
		if err != nil {
			return poly{}, false, err
		}
		if !okA || !okB {
			return poly{}, false, nil
		}
		switch n.op {
		case '+':
			return polyAdd(a, b), true, nil
		case '-':
			return polySub(a, b), true, nil
		case '*':
			return polyMul(a, b), true, nil
		case '^':
			// Only integer exponents.
			expNode, ok := n.right.(nodeNumber)
			if !ok {
				return poly{}, false, nil
			}
			expF := expNode.v.Float64()
			if math.IsNaN(expF) || math.IsInf(expF, 0) || expF != math.Trunc(expF) {
				return poly{}, false, nil
			}
			exp := int(expF)
			if exp < 0 || exp > 64 {
				return poly{}, false, nil
			}
			return polyPow(a, exp), true, nil
		default:
			return poly{}, false, nil
		}
	default:
		return poly{}, false, nil
	}
}

func polyAdd(a, b poly) poly {
	n := len(a.coeffs)
	if len(b.coeffs) > n {
		n = len(b.coeffs)
	}
	out := make([]float64, n)
	for i := range out {
		if i < len(a.coeffs) {
			out[i] += a.coeffs[i]
		}
		if i < len(b.coeffs) {
			out[i] += b.coeffs[i]
		}
	}
	return poly{coeffs: out}.trim()
}

func polySub(a, b poly) poly {
	n := len(a.coeffs)
	if len(b.coeffs) > n {
		n = len(b.coeffs)
	}
	out := make([]float64, n)
	for i := range out {
		if i < len(a.coeffs) {
			out[i] += a.coeffs[i]
		}
		if i < len(b.coeffs) {
			out[i] -= b.coeffs[i]
		}
	}
	return poly{coeffs: out}.trim()
}

func polyMul(a, b poly) poly {
	if len(a.coeffs) == 0 || len(b.coeffs) == 0 {
		return poly{}
	}
	out := make([]float64, len(a.coeffs)+len(b.coeffs)-1)
	for i, ca := range a.coeffs {
		if ca == 0 {
			continue
		}
		for j, cb := range b.coeffs {
			out[i+j] += ca * cb
		}
	}
	return poly{coeffs: out}.trim()
}

func polyPow(p poly, exp int) poly {
	if exp == 0 {
		return poly{coeffs: []float64{1}}
	}
	if exp == 1 {
		return p.trim()
	}
	base := p.trim()
	out := poly{coeffs: []float64{1}}
	for exp > 0 {
		if exp&1 == 1 {
			out = polyMul(out, base)
		}
		exp >>= 1
		if exp == 0 {
			break
		}
		base = polyMul(base, base)
	}
	return out.trim()
}

func polyToExprHorner(p poly, varName string) node {
	if len(p.coeffs) == 0 {
		return nodeNumber{v: Float(0)}
	}
	x := nodeIdent{name: varName}
	ex := node(nodeNumber{v: Float(p.coeffs[len(p.coeffs)-1])})
	for i := len(p.coeffs) - 2; i >= 0; i-- {
		ex = nodeBinary{
			op: '+',
			left: nodeBinary{
				op:    '*',
				left:  ex,
				right: x,
			},
			right: nodeNumber{v: Float(p.coeffs[i])},
		}.Simplify()
	}
	return ex
}

func polyFromCoeffsValue(v Value) (poly, error) {
	if v.kind != valueArray {
		return poly{}, fmt.Errorf("expected coefficient array")
	}
	out := make([]float64, len(v.arr))
	copy(out, v.arr)
	return poly{coeffs: out}.trim(), nil
}
