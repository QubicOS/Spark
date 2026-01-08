package vector

import (
	"fmt"
	"math"
)

func polyRatResultant(a, b polyRat) (Rat, error) {
	a = a.trim()
	b = b.trim()
	da := a.degree()
	db := b.degree()
	if da < 0 || db < 0 {
		return RatInt(0), nil
	}
	if da > 16 || db > 16 {
		return Rat{}, fmt.Errorf("degree too large")
	}

	// Build Sylvester matrix in descending-coefficient form.
	m := da
	n := db
	size := m + n

	pd := make([]Rat, m+1)
	qd := make([]Rat, n+1)
	for i := 0; i <= m; i++ {
		pd[i] = a.coeffs[m-i]
	}
	for i := 0; i <= n; i++ {
		qd[i] = b.coeffs[n-i]
	}

	syl := make([]Rat, size*size)
	for i := range syl {
		syl[i] = ratZero()
	}
	row := 0
	for i := 0; i < n; i++ {
		for j := 0; j <= m; j++ {
			syl[row*size+i+j] = pd[j]
		}
		row++
	}
	for i := 0; i < m; i++ {
		for j := 0; j <= n; j++ {
			syl[row*size+i+j] = qd[j]
		}
		row++
	}

	return detRatMatrix(syl, size)
}

func detRatMatrix(a []Rat, n int) (Rat, error) {
	if len(a) != n*n {
		return Rat{}, fmt.Errorf("bad dimensions")
	}
	m := make([]Rat, len(a))
	copy(m, a)
	sign := RatInt(1)

	for k := 0; k < n; k++ {
		piv := k
		for i := k; i < n; i++ {
			if m[i*n+k].num != 0 {
				piv = i
				break
			}
		}
		if m[piv*n+k].num == 0 {
			return RatInt(0), nil
		}
		if piv != k {
			for j := 0; j < n; j++ {
				m[k*n+j], m[piv*n+j] = m[piv*n+j], m[k*n+j]
			}
			sign = ratNeg(sign)
		}
		pivot := m[k*n+k]
		for i := k + 1; i < n; i++ {
			if m[i*n+k].num == 0 {
				continue
			}
			f, err := ratDiv(m[i*n+k], pivot)
			if err != nil {
				return Rat{}, err
			}
			m[i*n+k] = ratZero()
			for j := k + 1; j < n; j++ {
				p, err := ratMul(f, m[k*n+j])
				if err != nil {
					return Rat{}, err
				}
				m[i*n+j], err = ratSub(m[i*n+j], p)
				if err != nil {
					return Rat{}, err
				}
			}
		}
	}

	det := sign
	for i := 0; i < n; i++ {
		var err error
		det, err = ratMul(det, m[i*n+i])
		if err != nil {
			return Rat{}, err
		}
	}
	return det, nil
}

func polyRatFactorInteger(p polyRat, varName string) (node, error) {
	p = p.trim()
	d := p.degree()
	if d < 0 {
		return nodeNumber{v: RatNumber(RatInt(0))}, nil
	}
	if d == 0 {
		return nodeNumber{v: RatNumber(p.coeffs[0])}, nil
	}
	if d > 12 {
		return polyRatToExprHorner(p, varName), nil
	}

	// Require integer coefficients.
	intCoeffs := make([]int64, len(p.coeffs))
	for i, c := range p.coeffs {
		if c.den != 1 {
			return polyRatToExprHorner(p, varName), nil
		}
		intCoeffs[i] = c.num
	}

	// Factor out content (gcd of coefficients) as a constant.
	content := gcdSlice64(intCoeffs)
	if content < 0 {
		content = -content
	}
	if content == 0 {
		return nodeNumber{v: RatNumber(RatInt(0))}, nil
	}
	if content != 1 {
		for i := range intCoeffs {
			intCoeffs[i] /= content
		}
	}

	cur := polyRat{coeffs: make([]Rat, len(intCoeffs))}
	for i, v := range intCoeffs {
		cur.coeffs[i] = RatInt(v)
	}

	factors := make([]node, 0, 4)
	if content != 1 {
		factors = append(factors, nodeNumber{v: RatNumber(RatInt(content))})
	}

	for cur.degree() > 0 {
		root, ok := findRationalRoot(cur)
		if !ok {
			break
		}
		// Factor: (den*x - num).
		num := root.num
		den := root.den
		factor := nodeBinary{
			op: '-',
			left: nodeBinary{
				op:    '*',
				left:  nodeNumber{v: RatNumber(RatInt(den))},
				right: nodeIdent{name: varName},
			},
			right: nodeNumber{v: RatNumber(RatInt(num))},
		}.Simplify()
		factors = append(factors, factor)

		divisor := polyRat{coeffs: []Rat{RatInt(-num), RatInt(den)}}
		q, r, err := polyRatDivMod(cur, divisor)
		if err != nil {
			return nil, err
		}
		if r.degree() >= 0 {
			break
		}
		cur = q.trim()
	}

	rem := polyRatToExprHorner(cur, varName)
	if cur.degree() > 0 {
		factors = append(factors, rem)
	}

	out := factors[0]
	for _, f := range factors[1:] {
		out = nodeBinary{op: '*', left: out, right: f}.Simplify()
	}
	return out, nil
}

func gcdSlice64(xs []int64) int64 {
	var g int64
	for _, x := range xs {
		if x < 0 {
			x = -x
		}
		if x == 0 {
			continue
		}
		if g == 0 {
			g = x
			continue
		}
		g = gcd64(g, x)
	}
	if g == 0 {
		return 1
	}
	return g
}

func findRationalRoot(p polyRat) (Rat, bool) {
	d := p.degree()
	if d <= 0 {
		return RatInt(0), false
	}
	lead := p.coeffs[d]
	constant := p.coeffs[0]
	if lead.den != 1 || constant.den != 1 {
		return RatInt(0), false
	}
	a := lead.num
	b := constant.num
	if a == 0 {
		return RatInt(0), false
	}
	divP := divisors64(abs64(b))
	divQ := divisors64(abs64(a))
	try := func(num, den int64) bool {
		r, err := NewRat(num, den)
		if err != nil {
			return false
		}
		ok, err := evalPolyRatAt(p, r)
		return err == nil && ok
	}

	for _, pp := range divP {
		for _, qq := range divQ {
			if try(pp, qq) {
				r, _ := NewRat(pp, qq)
				return r, true
			}
			if try(-pp, qq) {
				r, _ := NewRat(-pp, qq)
				return r, true
			}
		}
	}
	return RatInt(0), false
}

func evalPolyRatAt(p polyRat, x Rat) (bool, error) {
	// Horner, exact.
	if len(p.coeffs) == 0 {
		return false, nil
	}
	v := p.coeffs[len(p.coeffs)-1]
	for i := len(p.coeffs) - 2; i >= 0; i-- {
		var err error
		v, err = ratMul(v, x)
		if err != nil {
			return false, err
		}
		v, err = ratAdd(v, p.coeffs[i])
		if err != nil {
			return false, err
		}
	}
	return v.num == 0, nil
}

func divisors64(n int64) []int64 {
	if n <= 0 {
		return []int64{1}
	}
	out := make([]int64, 0, 16)
	limit := int64(math.Sqrt(float64(n)))
	for i := int64(1); i <= limit; i++ {
		if n%i != 0 {
			continue
		}
		out = append(out, i)
		if i != n/i {
			out = append(out, n/i)
		}
	}
	return out
}
