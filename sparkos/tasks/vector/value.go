package vector

import (
	"errors"
	"fmt"
	"math"
)

var ErrOverflow = errors.New("overflow")

type valueKind uint8

const (
	valueNumber valueKind = iota
	valueExpr
)

type Value struct {
	kind valueKind
	num  Number
	expr node
}

func NumberValue(n Number) Value { return Value{kind: valueNumber, num: n} }

func ExprValue(n node) Value { return Value{kind: valueExpr, expr: n} }

func (v Value) IsNumber() bool { return v.kind == valueNumber }

func (v Value) IsExpr() bool { return v.kind == valueExpr }

func (v Value) ToNode() node {
	if v.kind == valueExpr && v.expr != nil {
		return v.expr
	}
	return nodeNumber{v: v.num}
}

type NumberKind uint8

const (
	numberFloat NumberKind = iota
	numberRat
)

type Number struct {
	kind NumberKind
	f    float64
	r    Rat
}

func Float(f float64) Number { return Number{kind: numberFloat, f: f} }

func RatNumber(r Rat) Number { return Number{kind: numberRat, r: r} }

func (n Number) IsRat() bool { return n.kind == numberRat }

func (n Number) IsFloat() bool { return n.kind == numberFloat }

func (n Number) Float64() float64 {
	if n.kind == numberRat {
		return n.r.Float64()
	}
	return n.f
}

func (n Number) String(prec int) string {
	if n.kind == numberRat {
		return n.r.String()
	}
	if prec <= 0 {
		prec = 10
	}
	return formatFloat(n.f, prec)
}

func formatFloat(f float64, prec int) string {
	if math.IsNaN(f) {
		return "NaN"
	}
	if math.IsInf(f, 1) {
		return "+Inf"
	}
	if math.IsInf(f, -1) {
		return "-Inf"
	}
	return fmt.Sprintf("%.*g", prec, f)
}

type Rat struct {
	num int64
	den int64
}

func RatInt(n int64) Rat { return Rat{num: n, den: 1} }

func NewRat(num, den int64) (Rat, error) {
	if den == 0 {
		return Rat{}, errors.New("division by zero")
	}
	if den < 0 {
		num = -num
		den = -den
	}
	g := gcd64(abs64(num), den)
	return Rat{num: num / g, den: den / g}, nil
}

func (r Rat) Float64() float64 { return float64(r.num) / float64(r.den) }

func (r Rat) String() string {
	if r.den == 1 {
		return fmt.Sprintf("%d", r.num)
	}
	return fmt.Sprintf("%d/%d", r.num, r.den)
}

func (r Rat) Add(b Rat) (Rat, error) {
	n1, err := mulChecked(r.num, b.den)
	if err != nil {
		return Rat{}, err
	}
	n2, err := mulChecked(b.num, r.den)
	if err != nil {
		return Rat{}, err
	}
	n, err := addChecked(n1, n2)
	if err != nil {
		return Rat{}, err
	}
	d, err := mulChecked(r.den, b.den)
	if err != nil {
		return Rat{}, err
	}
	return NewRat(n, d)
}

func (r Rat) Sub(b Rat) (Rat, error) {
	n1, err := mulChecked(r.num, b.den)
	if err != nil {
		return Rat{}, err
	}
	n2, err := mulChecked(b.num, r.den)
	if err != nil {
		return Rat{}, err
	}
	n, err := subChecked(n1, n2)
	if err != nil {
		return Rat{}, err
	}
	d, err := mulChecked(r.den, b.den)
	if err != nil {
		return Rat{}, err
	}
	return NewRat(n, d)
}

func (r Rat) Mul(b Rat) (Rat, error) {
	n, err := mulChecked(r.num, b.num)
	if err != nil {
		return Rat{}, err
	}
	d, err := mulChecked(r.den, b.den)
	if err != nil {
		return Rat{}, err
	}
	return NewRat(n, d)
}

func (r Rat) Div(b Rat) (Rat, error) {
	if b.num == 0 {
		return Rat{}, errors.New("division by zero")
	}
	n, err := mulChecked(r.num, b.den)
	if err != nil {
		return Rat{}, err
	}
	d, err := mulChecked(r.den, abs64(b.num))
	if err != nil {
		return Rat{}, err
	}
	if b.num < 0 {
		n = -n
	}
	return NewRat(n, d)
}

func (r Rat) PowInt(exp int64) (Rat, error) {
	if exp == 0 {
		return RatInt(1), nil
	}
	if exp < 0 {
		if r.num == 0 {
			return Rat{}, errors.New("division by zero")
		}
		inv, err := NewRat(r.den, r.num)
		if err != nil {
			return Rat{}, err
		}
		return inv.PowInt(-exp)
	}

	base := r
	out := RatInt(1)
	for exp > 0 {
		if exp&1 == 1 {
			var err error
			out, err = out.Mul(base)
			if err != nil {
				return Rat{}, err
			}
		}
		exp >>= 1
		if exp == 0 {
			break
		}
		var err error
		base, err = base.Mul(base)
		if err != nil {
			return Rat{}, err
		}
	}
	return out, nil
}

func gcd64(a, b int64) int64 {
	if a < 0 {
		a = -a
	}
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func addChecked(a, b int64) (int64, error) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, ErrOverflow
	}
	return a + b, nil
}

func subChecked(a, b int64) (int64, error) {
	return addChecked(a, -b)
}

func mulChecked(a, b int64) (int64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	if a == -1 && b == math.MinInt64 {
		return 0, ErrOverflow
	}
	if b == -1 && a == math.MinInt64 {
		return 0, ErrOverflow
	}
	absA := abs64(a)
	absB := abs64(b)
	if absA > math.MaxInt64/absB {
		return 0, ErrOverflow
	}
	return a * b, nil
}
