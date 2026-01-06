package vector

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

var (
	ErrParse = errors.New("parse error")
	ErrEval  = errors.New("eval error")
	// ErrUnknownVar is returned when evaluating an expression with an undefined variable.
	ErrUnknownVar = errors.New("unknown variable")
)

type evalMode uint8

const (
	modeFloat evalMode = iota
	modeExact
)

type env struct {
	mode evalMode
	prec int

	vars  map[string]Value
	funcs map[string]userFunc
}

type userFunc struct {
	param string
	body  node
}

func newEnv() *env {
	return &env{
		mode: modeFloat,
		prec: 12,
		vars: map[string]Value{
			"pi":    NumberValue(Float(math.Pi)),
			"tau":   NumberValue(Float(2 * math.Pi)),
			"e":     NumberValue(Float(math.E)),
			"phi":   NumberValue(Float((1 + math.Sqrt(5)) / 2)),
			"sqrt2": NumberValue(Float(math.Sqrt2)),
			"ln2":   NumberValue(Float(math.Ln2)),
			"ln10":  NumberValue(Float(math.Ln10)),
		},
		funcs: make(map[string]userFunc),
	}
}

type scalarBuiltin struct {
	minArgs int
	maxArgs int
	fn      func(*env, []Number) (Number, error)
}

var scalarBuiltins = map[string]scalarBuiltin{
	// Trigonometry.
	"sin":  {minArgs: 1, maxArgs: 1, fn: scalarUnary(math.Sin)},
	"cos":  {minArgs: 1, maxArgs: 1, fn: scalarUnary(math.Cos)},
	"tan":  {minArgs: 1, maxArgs: 1, fn: scalarUnary(math.Tan)},
	"asin": {minArgs: 1, maxArgs: 1, fn: scalarUnary(math.Asin)},
	"acos": {minArgs: 1, maxArgs: 1, fn: scalarUnary(math.Acos)},
	"atan": {minArgs: 1, maxArgs: 1, fn: scalarUnary(math.Atan)},
	"atan2": {minArgs: 2, maxArgs: 2, fn: func(_ *env, args []Number) (Number, error) {
		return Float(math.Atan2(args[0].Float64(), args[1].Float64())), nil
	}},
	"cot": {minArgs: 1, maxArgs: 1, fn: func(_ *env, args []Number) (Number, error) {
		return Float(1 / math.Tan(args[0].Float64())), nil
	}},
	"sec": {minArgs: 1, maxArgs: 1, fn: func(_ *env, args []Number) (Number, error) {
		return Float(1 / math.Cos(args[0].Float64())), nil
	}},
	"csc": {minArgs: 1, maxArgs: 1, fn: func(_ *env, args []Number) (Number, error) {
		return Float(1 / math.Sin(args[0].Float64())), nil
	}},

	// Hyperbolic (implemented via exp/log/sqrt to keep TinyGo compatibility).
	"sinh":  {minArgs: 1, maxArgs: 1, fn: scalarSinh},
	"cosh":  {minArgs: 1, maxArgs: 1, fn: scalarCosh},
	"tanh":  {minArgs: 1, maxArgs: 1, fn: scalarTanh},
	"asinh": {minArgs: 1, maxArgs: 1, fn: scalarAsinh},
	"acosh": {minArgs: 1, maxArgs: 1, fn: scalarAcosh},
	"atanh": {minArgs: 1, maxArgs: 1, fn: scalarAtanh},

	// Exponentials and logs.
	"exp":   {minArgs: 1, maxArgs: 1, fn: scalarUnary(math.Exp)},
	"expm1": {minArgs: 1, maxArgs: 1, fn: scalarExpm1},
	"exp2":  {minArgs: 1, maxArgs: 1, fn: scalarExp2},
	"ln":    {minArgs: 1, maxArgs: 1, fn: scalarUnary(math.Log)},
	"log":   {minArgs: 1, maxArgs: 1, fn: scalarUnary(math.Log)},
	"log10": {minArgs: 1, maxArgs: 1, fn: scalarLog10},
	"log2":  {minArgs: 1, maxArgs: 1, fn: scalarLog2},
	"log1p": {minArgs: 1, maxArgs: 1, fn: scalarLog1p},

	// Powers and roots.
	"pow": {minArgs: 2, maxArgs: 2, fn: func(_ *env, args []Number) (Number, error) {
		return Float(math.Pow(args[0].Float64(), args[1].Float64())), nil
	}},
	"sqrt": {minArgs: 1, maxArgs: 1, fn: scalarUnary(math.Sqrt)},
	"cbrt": {minArgs: 1, maxArgs: 1, fn: scalarCbrt},
	"hypot": {minArgs: 2, maxArgs: 2, fn: func(_ *env, args []Number) (Number, error) {
		a := args[0].Float64()
		b := args[1].Float64()
		return Float(math.Sqrt(a*a + b*b)), nil
	}},

	// Rounding.
	"floor": {minArgs: 1, maxArgs: 1, fn: scalarUnary(math.Floor)},
	"ceil":  {minArgs: 1, maxArgs: 1, fn: scalarUnary(math.Ceil)},
	"trunc": {minArgs: 1, maxArgs: 1, fn: scalarTrunc},
	"round": {minArgs: 1, maxArgs: 1, fn: scalarRound},

	// Misc numeric.
	"abs":      {minArgs: 1, maxArgs: 1, fn: scalarAbs},
	"sign":     {minArgs: 1, maxArgs: 1, fn: scalarSign},
	"copysign": {minArgs: 2, maxArgs: 2, fn: scalarCopySign},
	"mod":      {minArgs: 2, maxArgs: 2, fn: scalarMod},

	// Helpers.
	"rad": {minArgs: 1, maxArgs: 1, fn: func(_ *env, args []Number) (Number, error) {
		return Float(args[0].Float64() * math.Pi / 180), nil
	}},
	"deg": {minArgs: 1, maxArgs: 1, fn: func(_ *env, args []Number) (Number, error) {
		return Float(args[0].Float64() * 180 / math.Pi), nil
	}},
	"clamp": {minArgs: 3, maxArgs: 3, fn: scalarClamp},
	"saturate": {minArgs: 1, maxArgs: 1, fn: func(_ *env, args []Number) (Number, error) {
		return scalarClamp(nil, []Number{args[0], Float(0), Float(1)})
	}},
	"lerp":       {minArgs: 3, maxArgs: 3, fn: scalarLerp},
	"step":       {minArgs: 2, maxArgs: 2, fn: scalarStep},
	"smoothstep": {minArgs: 3, maxArgs: 3, fn: scalarSmoothstep},
	"sq": {minArgs: 1, maxArgs: 1, fn: func(_ *env, args []Number) (Number, error) {
		x := args[0].Float64()
		return Float(x * x), nil
	}},
	"cube": {minArgs: 1, maxArgs: 1, fn: func(_ *env, args []Number) (Number, error) {
		x := args[0].Float64()
		return Float(x * x * x), nil
	}},

	// Variadic.
	"min":  {minArgs: 1, maxArgs: -1, fn: scalarMin},
	"max":  {minArgs: 1, maxArgs: -1, fn: scalarMax},
	"sum":  {minArgs: 1, maxArgs: -1, fn: scalarSum},
	"avg":  {minArgs: 1, maxArgs: -1, fn: scalarMean},
	"mean": {minArgs: 1, maxArgs: -1, fn: scalarMean},
}

var unaryArrayBuiltins = map[string]func(float64) float64{
	"sin":  math.Sin,
	"cos":  math.Cos,
	"tan":  math.Tan,
	"asin": math.Asin,
	"acos": math.Acos,
	"atan": math.Atan,
	"cot":  func(x float64) float64 { return 1 / math.Tan(x) },
	"sec":  func(x float64) float64 { return 1 / math.Cos(x) },
	"csc":  func(x float64) float64 { return 1 / math.Sin(x) },

	"sinh":  sinh,
	"cosh":  cosh,
	"tanh":  tanh,
	"asinh": asinh,
	"acosh": acosh,
	"atanh": atanh,

	"sqrt": math.Sqrt,
	"cbrt": cbrt,

	"abs":  math.Abs,
	"sign": sign,

	"exp":   math.Exp,
	"expm1": expm1,
	"exp2":  exp2,
	"ln":    math.Log,
	"log":   math.Log,
	"log10": log10,
	"log2":  log2,
	"log1p": log1p,

	"floor": math.Floor,
	"ceil":  math.Ceil,
	"trunc": trunc,
	"round": round,

	"rad": func(x float64) float64 { return x * math.Pi / 180 },
	"deg": func(x float64) float64 { return x * 180 / math.Pi },
	"saturate": func(x float64) float64 {
		if x < 0 {
			return 0
		}
		if x > 1 {
			return 1
		}
		return x
	},
	"sq":   func(x float64) float64 { return x * x },
	"cube": func(x float64) float64 { return x * x * x },
}

var arrayAggBuiltins = map[string]func([]float64) float64{
	"len": func(xs []float64) float64 { return float64(len(xs)) },
	"sum": func(xs []float64) float64 {
		var total float64
		for _, x := range xs {
			total += x
		}
		return total
	},
	"avg":  arrayAvg,
	"mean": arrayAvg,
	"min": func(xs []float64) float64 {
		if len(xs) == 0 {
			return math.NaN()
		}
		m := xs[0]
		for _, x := range xs[1:] {
			if x < m {
				m = x
			}
		}
		return m
	},
	"max": func(xs []float64) float64 {
		if len(xs) == 0 {
			return math.NaN()
		}
		m := xs[0]
		for _, x := range xs[1:] {
			if x > m {
				m = x
			}
		}
		return m
	},
}

func arrayAvg(xs []float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	var total float64
	for _, x := range xs {
		total += x
	}
	return total / float64(len(xs))
}

func builtinKeywords() []string {
	set := make(map[string]struct{}, len(scalarBuiltins)+len(unaryArrayBuiltins)+len(arrayAggBuiltins)+3)
	set["range"] = struct{}{}
	set["simp"] = struct{}{}
	set["diff"] = struct{}{}
	for name := range scalarBuiltins {
		set[name] = struct{}{}
	}
	for name := range unaryArrayBuiltins {
		set[name] = struct{}{}
	}
	for name := range arrayAggBuiltins {
		set[name] = struct{}{}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func isBuiltinKeyword(name string) bool {
	if name == "range" || name == "simp" || name == "diff" {
		return true
	}
	if _, ok := scalarBuiltins[name]; ok {
		return true
	}
	if _, ok := unaryArrayBuiltins[name]; ok {
		return true
	}
	if _, ok := arrayAggBuiltins[name]; ok {
		return true
	}
	return false
}

func scalarUnary(fn func(float64) float64) func(*env, []Number) (Number, error) {
	return func(_ *env, args []Number) (Number, error) {
		return Float(fn(args[0].Float64())), nil
	}
}

func scalarAbs(_ *env, args []Number) (Number, error) {
	if args[0].kind == numberRat {
		r := args[0].r
		if r.num < 0 {
			r.num = -r.num
		}
		return RatNumber(r), nil
	}
	return Float(math.Abs(args[0].f)), nil
}

func scalarSign(_ *env, args []Number) (Number, error) {
	if args[0].kind == numberRat {
		switch {
		case args[0].r.num > 0:
			return RatNumber(RatInt(1)), nil
		case args[0].r.num < 0:
			return RatNumber(RatInt(-1)), nil
		default:
			return RatNumber(RatInt(0)), nil
		}
	}
	f := args[0].f
	if math.IsNaN(f) {
		return Float(f), nil
	}
	switch {
	case f > 0:
		return Float(1), nil
	case f < 0:
		return Float(-1), nil
	default:
		return Float(0), nil
	}
}

func scalarMin(_ *env, args []Number) (Number, error) {
	m := args[0].Float64()
	for _, v := range args[1:] {
		f := v.Float64()
		if f < m {
			m = f
		}
	}
	return Float(m), nil
}

func scalarMax(_ *env, args []Number) (Number, error) {
	m := args[0].Float64()
	for _, v := range args[1:] {
		f := v.Float64()
		if f > m {
			m = f
		}
	}
	return Float(m), nil
}

func scalarSum(_ *env, args []Number) (Number, error) {
	var total float64
	for _, v := range args {
		total += v.Float64()
	}
	return Float(total), nil
}

func scalarMean(_ *env, args []Number) (Number, error) {
	var total float64
	for _, v := range args {
		total += v.Float64()
	}
	return Float(total / float64(len(args))), nil
}

func scalarCopySign(_ *env, args []Number) (Number, error) {
	mag := math.Abs(args[0].Float64())
	signSource := args[1].Float64()
	if math.IsNaN(signSource) {
		return Float(math.NaN()), nil
	}
	if signSource < 0 {
		return Float(-mag), nil
	}
	return Float(mag), nil
}

func scalarMod(_ *env, args []Number) (Number, error) {
	a := args[0].Float64()
	b := args[1].Float64()
	if b == 0 {
		return Number{}, fmt.Errorf("%w: mod: division by zero", ErrEval)
	}
	return Float(mod(a, b)), nil
}

func scalarClamp(_ *env, args []Number) (Number, error) {
	x := args[0].Float64()
	lo := args[1].Float64()
	hi := args[2].Float64()
	if lo > hi {
		lo, hi = hi, lo
	}
	if x < lo {
		return Float(lo), nil
	}
	if x > hi {
		return Float(hi), nil
	}
	return Float(x), nil
}

func scalarLerp(_ *env, args []Number) (Number, error) {
	a := args[0].Float64()
	b := args[1].Float64()
	t := args[2].Float64()
	return Float(a + t*(b-a)), nil
}

func scalarStep(_ *env, args []Number) (Number, error) {
	edge := args[0].Float64()
	x := args[1].Float64()
	if x < edge {
		return Float(0), nil
	}
	return Float(1), nil
}

func scalarSmoothstep(_ *env, args []Number) (Number, error) {
	edge0 := args[0].Float64()
	edge1 := args[1].Float64()
	x := args[2].Float64()
	if edge0 == edge1 {
		if x < edge0 {
			return Float(0), nil
		}
		return Float(1), nil
	}
	t := (x - edge0) / (edge1 - edge0)
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return Float(t * t * (3 - 2*t)), nil
}

func scalarTrunc(_ *env, args []Number) (Number, error) { return Float(trunc(args[0].Float64())), nil }
func scalarRound(_ *env, args []Number) (Number, error) { return Float(round(args[0].Float64())), nil }
func scalarCbrt(_ *env, args []Number) (Number, error)  { return Float(cbrt(args[0].Float64())), nil }
func scalarExpm1(_ *env, args []Number) (Number, error) { return Float(expm1(args[0].Float64())), nil }
func scalarExp2(_ *env, args []Number) (Number, error)  { return Float(exp2(args[0].Float64())), nil }
func scalarLog2(_ *env, args []Number) (Number, error)  { return Float(log2(args[0].Float64())), nil }
func scalarLog10(_ *env, args []Number) (Number, error) { return Float(log10(args[0].Float64())), nil }
func scalarLog1p(_ *env, args []Number) (Number, error) { return Float(log1p(args[0].Float64())), nil }
func scalarSinh(_ *env, args []Number) (Number, error)  { return Float(sinh(args[0].Float64())), nil }
func scalarCosh(_ *env, args []Number) (Number, error)  { return Float(cosh(args[0].Float64())), nil }
func scalarTanh(_ *env, args []Number) (Number, error)  { return Float(tanh(args[0].Float64())), nil }
func scalarAsinh(_ *env, args []Number) (Number, error) { return Float(asinh(args[0].Float64())), nil }
func scalarAcosh(_ *env, args []Number) (Number, error) { return Float(acosh(args[0].Float64())), nil }
func scalarAtanh(_ *env, args []Number) (Number, error) { return Float(atanh(args[0].Float64())), nil }

func sign(x float64) float64 {
	if math.IsNaN(x) {
		return x
	}
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	default:
		return 0
	}
}

func expm1(x float64) float64 { return math.Exp(x) - 1 }
func exp2(x float64) float64  { return math.Pow(2, x) }
func log2(x float64) float64  { return math.Log(x) / math.Ln2 }
func log10(x float64) float64 { return math.Log(x) / math.Ln10 }
func log1p(x float64) float64 { return math.Log(1 + x) }

func mod(a, b float64) float64 { return a - b*math.Floor(a/b) }

func trunc(x float64) float64 {
	if x < 0 {
		return math.Ceil(x)
	}
	return math.Floor(x)
}

func round(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	if x < 0 {
		return math.Ceil(x - 0.5)
	}
	return math.Floor(x + 0.5)
}

func cbrt(x float64) float64 {
	if x == 0 || math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	if x < 0 {
		return -math.Pow(-x, 1.0/3.0)
	}
	return math.Pow(x, 1.0/3.0)
}

func sinh(x float64) float64 {
	ex := math.Exp(x)
	emx := math.Exp(-x)
	return (ex - emx) / 2
}

func cosh(x float64) float64 {
	ex := math.Exp(x)
	emx := math.Exp(-x)
	return (ex + emx) / 2
}

func tanh(x float64) float64 {
	ex2 := math.Exp(2 * x)
	return (ex2 - 1) / (ex2 + 1)
}

func asinh(x float64) float64 { return math.Log(x + math.Sqrt(x*x+1)) }
func acosh(x float64) float64 { return math.Log(x + math.Sqrt(x-1)*math.Sqrt(x+1)) }
func atanh(x float64) float64 { return 0.5 * math.Log((1+x)/(1-x)) }

type node interface {
	Eval(e *env) (Value, error)
	Simplify() node
	Deriv(varName string) node
}

type nodeNumber struct{ v Number }

func (n nodeNumber) Eval(_ *env) (Value, error) { return NumberValue(n.v), nil }

func (n nodeNumber) Simplify() node { return n }

func (n nodeNumber) Deriv(_ string) node { return nodeNumber{v: RatNumber(RatInt(0))} }

type nodeIdent struct{ name string }

func (n nodeIdent) Eval(e *env) (Value, error) {
	v, ok := e.vars[n.name]
	if !ok {
		return Value{}, fmt.Errorf("%w: %w %q", ErrEval, ErrUnknownVar, n.name)
	}
	return v, nil
}

func (n nodeIdent) Simplify() node { return n }

func (n nodeIdent) Deriv(varName string) node {
	if n.name == varName {
		return nodeNumber{v: RatNumber(RatInt(1))}
	}
	return nodeNumber{v: RatNumber(RatInt(0))}
}

type nodeUnary struct {
	op byte
	x  node
}

func (n nodeUnary) Eval(e *env) (Value, error) {
	v, err := n.x.Eval(e)
	if err != nil {
		return Value{}, err
	}
	if v.kind == valueArray {
		switch n.op {
		case '+':
			return v, nil
		case '-':
			out := make([]float64, len(v.arr))
			for i, x := range v.arr {
				out[i] = -x
			}
			return ArrayValue(out), nil
		default:
			return Value{}, fmt.Errorf("%w: unary %q", ErrEval, n.op)
		}
	}
	if v.kind == valueExpr {
		return ExprValue(nodeUnary{op: n.op, x: v.expr}.Simplify()), nil
	}

	switch n.op {
	case '+':
		return v, nil
	case '-':
		return NumberValue(negNumber(v.num)), nil
	default:
		return Value{}, fmt.Errorf("%w: unary %q", ErrEval, n.op)
	}
}

func (n nodeUnary) Simplify() node {
	x := n.x.Simplify()
	if num, ok := x.(nodeNumber); ok {
		switch n.op {
		case '+':
			return num
		case '-':
			return nodeNumber{v: negNumber(num.v)}
		}
	}
	if u, ok := x.(nodeUnary); ok && n.op == '+' {
		return u
	}
	return nodeUnary{op: n.op, x: x}
}

func (n nodeUnary) Deriv(varName string) node {
	switch n.op {
	case '+':
		return n.x.Deriv(varName)
	case '-':
		return nodeUnary{op: '-', x: n.x.Deriv(varName)}.Simplify()
	default:
		return nodeNumber{v: RatNumber(RatInt(0))}
	}
}

type nodeBinary struct {
	op    byte
	left  node
	right node
}

func (n nodeBinary) Eval(e *env) (Value, error) {
	a, err := n.left.Eval(e)
	if err != nil {
		return Value{}, err
	}
	b, err := n.right.Eval(e)
	if err != nil {
		return Value{}, err
	}

	if a.kind == valueArray || b.kind == valueArray {
		return evalBinaryArray(e, n.op, a, b)
	}

	if a.kind == valueExpr || b.kind == valueExpr {
		return ExprValue(nodeBinary{op: n.op, left: a.ToNode(), right: b.ToNode()}.Simplify()), nil
	}

	out, err := evalBinaryNumber(e, n.op, a.num, b.num)
	if err != nil {
		return Value{}, err
	}
	return NumberValue(out), nil
}

func evalBinaryNumber(e *env, op byte, a, b Number) (Number, error) {
	switch op {
	case '+':
		return addNumber(e, a, b)
	case '-':
		return subNumber(e, a, b)
	case '*':
		return mulNumber(e, a, b)
	case '/':
		return divNumber(e, a, b)
	case '^':
		return powNumber(e, a, b)
	default:
		return Number{}, fmt.Errorf("%w: binary %q", ErrEval, op)
	}
}

func (n nodeBinary) Simplify() node {
	left := n.left.Simplify()
	right := n.right.Simplify()

	ln, lok := left.(nodeNumber)
	rn, rok := right.(nodeNumber)
	if lok && rok {
		out, err := evalBinaryNumber(newEnv(), n.op, ln.v, rn.v)
		if err == nil {
			return nodeNumber{v: out}
		}
	}

	switch n.op {
	case '+':
		if isZeroNode(left) {
			return right
		}
		if isZeroNode(right) {
			return left
		}
	case '-':
		if isZeroNode(right) {
			return left
		}
	case '*':
		if isZeroNode(left) || isZeroNode(right) {
			return nodeNumber{v: RatNumber(RatInt(0))}
		}
		if isOneNode(left) {
			return right
		}
		if isOneNode(right) {
			return left
		}
	case '/':
		if isZeroNode(left) {
			return nodeNumber{v: RatNumber(RatInt(0))}
		}
		if isOneNode(right) {
			return left
		}
	case '^':
		if isOneNode(right) {
			return left
		}
		if isZeroNode(right) {
			return nodeNumber{v: RatNumber(RatInt(1))}
		}
	}

	return nodeBinary{op: n.op, left: left, right: right}
}

func isZeroNode(n node) bool {
	num, ok := n.(nodeNumber)
	if !ok {
		return false
	}
	if num.v.kind == numberRat {
		return num.v.r.num == 0
	}
	return num.v.f == 0
}

func isOneNode(n node) bool {
	num, ok := n.(nodeNumber)
	if !ok {
		return false
	}
	if num.v.kind == numberRat {
		return num.v.r.num == 1 && num.v.r.den == 1
	}
	return num.v.f == 1
}

func (n nodeBinary) Deriv(varName string) node {
	switch n.op {
	case '+':
		return nodeBinary{op: '+', left: n.left.Deriv(varName), right: n.right.Deriv(varName)}.Simplify()
	case '-':
		return nodeBinary{op: '-', left: n.left.Deriv(varName), right: n.right.Deriv(varName)}.Simplify()
	case '*':
		// (uv)' = u'v + uv'
		return nodeBinary{
			op: '+',
			left: nodeBinary{
				op:    '*',
				left:  n.left.Deriv(varName),
				right: n.right,
			},
			right: nodeBinary{
				op:    '*',
				left:  n.left,
				right: n.right.Deriv(varName),
			},
		}.Simplify()
	case '/':
		// (u/v)' = (u'v - uv') / v^2
		num := nodeBinary{
			op: '-',
			left: nodeBinary{
				op:    '*',
				left:  n.left.Deriv(varName),
				right: n.right,
			},
			right: nodeBinary{
				op:    '*',
				left:  n.left,
				right: n.right.Deriv(varName),
			},
		}
		den := nodeBinary{op: '^', left: n.right, right: nodeNumber{v: RatNumber(RatInt(2))}}
		return nodeBinary{op: '/', left: num, right: den}.Simplify()
	case '^':
		// Handle f(x)^c where c is constant number.
		if cn, ok := n.right.(nodeNumber); ok && cn.v.kind == numberRat && cn.v.r.den == 1 {
			c := cn.v.r.num
			if c == 0 {
				return nodeNumber{v: RatNumber(RatInt(0))}
			}
			// (u^c)' = c*u^(c-1)*u'
			return nodeBinary{
				op: '*',
				left: nodeBinary{
					op:    '*',
					left:  nodeNumber{v: RatNumber(RatInt(c))},
					right: nodeBinary{op: '^', left: n.left, right: nodeNumber{v: RatNumber(RatInt(c - 1))}},
				},
				right: n.left.Deriv(varName),
			}.Simplify()
		}
		// Fallback to numeric zero.
		return nodeNumber{v: RatNumber(RatInt(0))}
	default:
		return nodeNumber{v: RatNumber(RatInt(0))}
	}
}

func evalBinaryArray(e *env, op byte, a, b Value) (Value, error) {
	switch {
	case a.kind == valueArray && b.kind == valueArray:
		if len(a.arr) != len(b.arr) {
			return Value{}, fmt.Errorf("%w: array length mismatch", ErrEval)
		}
		out := make([]float64, len(a.arr))
		for i := range out {
			nn, err := evalBinaryNumber(e, op, Float(a.arr[i]), Float(b.arr[i]))
			if err != nil {
				return Value{}, err
			}
			out[i] = nn.Float64()
		}
		return ArrayValue(out), nil

	case a.kind == valueArray && b.kind == valueNumber:
		out := make([]float64, len(a.arr))
		bf := b.num.Float64()
		for i := range out {
			nn, err := evalBinaryNumber(e, op, Float(a.arr[i]), Float(bf))
			if err != nil {
				return Value{}, err
			}
			out[i] = nn.Float64()
		}
		return ArrayValue(out), nil

	case a.kind == valueNumber && b.kind == valueArray:
		out := make([]float64, len(b.arr))
		af := a.num.Float64()
		for i := range out {
			nn, err := evalBinaryNumber(e, op, Float(af), Float(b.arr[i]))
			if err != nil {
				return Value{}, err
			}
			out[i] = nn.Float64()
		}
		return ArrayValue(out), nil

	default:
		return Value{}, fmt.Errorf("%w: unsupported array operation", ErrEval)
	}
}

type nodeCall struct {
	name string
	args []node
}

func (n nodeCall) Eval(e *env) (Value, error) {
	if n.name == "simp" && len(n.args) == 1 {
		return ExprValue(n.args[0].Simplify()), nil
	}
	if n.name == "diff" && len(n.args) == 2 {
		varName, ok := callArgIdent(n.args[1])
		if !ok {
			return Value{}, fmt.Errorf("%w: diff expects second arg as identifier", ErrEval)
		}
		return ExprValue(n.args[0].Deriv(varName).Simplify()), nil
	}

	args := make([]Value, 0, len(n.args))
	for _, a := range n.args {
		v, err := a.Eval(e)
		if err != nil {
			return Value{}, err
		}
		args = append(args, v)
	}

	if out, ok, err := builtinCallValue(e, n.name, args); ok {
		return out, err
	}

	if fn, ok := e.funcs[n.name]; ok {
		if len(args) != 1 {
			return Value{}, fmt.Errorf("%w: %s expects 1 argument", ErrEval, n.name)
		}
		prev, hadPrev := e.vars[fn.param]
		e.vars[fn.param] = args[0]
		out, err := fn.body.Eval(e)
		if hadPrev {
			e.vars[fn.param] = prev
		} else {
			delete(e.vars, fn.param)
		}
		return out, err
	}

	out, ok, err := builtinCall(e, n.name, args)
	if !ok {
		return Value{}, fmt.Errorf("%w: unknown function %q", ErrEval, n.name)
	}
	if err != nil {
		return Value{}, err
	}
	return NumberValue(out), nil
}

func builtinCallValue(e *env, name string, args []Value) (Value, bool, error) {
	if name == "range" {
		out, err := builtinRange(args)
		return out, true, err
	}

	if name == "clamp" && len(args) == 3 && args[0].kind == valueArray && args[1].IsNumber() && args[2].IsNumber() {
		lo := args[1].num.Float64()
		hi := args[2].num.Float64()
		if lo > hi {
			lo, hi = hi, lo
		}
		out := make([]float64, len(args[0].arr))
		for i, x := range args[0].arr {
			if x < lo {
				out[i] = lo
				continue
			}
			if x > hi {
				out[i] = hi
				continue
			}
			out[i] = x
		}
		_ = e
		return ArrayValue(out), true, nil
	}

	if len(args) != 1 || args[0].kind != valueArray {
		return Value{}, false, nil
	}

	if fn, ok := unaryArrayBuiltins[name]; ok {
		out := make([]float64, len(args[0].arr))
		for i, x := range args[0].arr {
			out[i] = fn(x)
		}
		_ = e
		return ArrayValue(out), true, nil
	}

	if agg, ok := arrayAggBuiltins[name]; ok {
		_ = e
		return NumberValue(Float(agg(args[0].arr))), true, nil
	}

	return Value{}, false, nil
}

func builtinRange(args []Value) (Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return Value{}, fmt.Errorf("%w: range expects 2 or 3 arguments", ErrEval)
	}
	if !args[0].IsNumber() || !args[1].IsNumber() {
		return Value{}, fmt.Errorf("%w: range bounds must be numbers", ErrEval)
	}
	n := 256
	if len(args) == 3 {
		if !args[2].IsNumber() {
			return Value{}, fmt.Errorf("%w: range count must be a number", ErrEval)
		}
		nf := args[2].num.Float64()
		if nf < 2 || nf > 4096 {
			return Value{}, fmt.Errorf("%w: range count must be 2..4096", ErrEval)
		}
		n = int(nf)
	}
	a := args[0].num.Float64()
	b := args[1].num.Float64()
	out := make([]float64, n)
	if n == 1 {
		out[0] = a
		return ArrayValue(out), nil
	}
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n-1)
		out[i] = a + t*(b-a)
	}
	return ArrayValue(out), nil
}

func (n nodeCall) Simplify() node {
	args := make([]node, 0, len(n.args))
	for _, a := range n.args {
		args = append(args, a.Simplify())
	}
	return nodeCall{name: n.name, args: args}
}

func (n nodeCall) Deriv(varName string) node {
	// Very small set: sin/cos/exp/ln with 1 arg.
	if len(n.args) != 1 {
		return nodeNumber{v: RatNumber(RatInt(0))}
	}
	u := n.args[0]
	du := u.Deriv(varName)
	switch n.name {
	case "sin":
		return nodeBinary{op: '*', left: nodeCall{name: "cos", args: []node{u}}, right: du}.Simplify()
	case "cos":
		return nodeBinary{op: '*', left: nodeUnary{op: '-', x: nodeCall{name: "sin", args: []node{u}}}, right: du}.Simplify()
	case "exp":
		return nodeBinary{op: '*', left: nodeCall{name: "exp", args: []node{u}}, right: du}.Simplify()
	case "ln":
		return nodeBinary{op: '*', left: nodeBinary{op: '/', left: nodeNumber{v: RatNumber(RatInt(1))}, right: u}, right: du}.Simplify()
	default:
		return nodeNumber{v: RatNumber(RatInt(0))}
	}
}

func callArgIdent(n node) (string, bool) {
	id, ok := n.(nodeIdent)
	if ok {
		return id.name, true
	}
	return "", false
}

func builtinCall(e *env, name string, args []Value) (Number, bool, error) {
	// If any arg is symbolic, keep it symbolic by returning "unknown" and letting caller wrap.
	for _, a := range args {
		if a.kind == valueExpr {
			return Number{}, false, nil
		}
		if a.kind == valueArray {
			return Number{}, false, nil
		}
	}
	nums := make([]Number, 0, len(args))
	for _, a := range args {
		nums = append(nums, a.num)
	}

	spec, ok := scalarBuiltins[name]
	if !ok {
		return Number{}, false, nil
	}
	if len(nums) < spec.minArgs || (spec.maxArgs >= 0 && len(nums) > spec.maxArgs) {
		if spec.minArgs == spec.maxArgs {
			return Number{}, true, fmt.Errorf("%w: %s expects %d argument(s)", ErrEval, name, spec.minArgs)
		}
		if spec.maxArgs < 0 {
			return Number{}, true, fmt.Errorf("%w: %s expects >= %d argument(s)", ErrEval, name, spec.minArgs)
		}
		return Number{}, true, fmt.Errorf("%w: %s expects %d..%d argument(s)", ErrEval, name, spec.minArgs, spec.maxArgs)
	}
	out, err := spec.fn(e, nums)
	if err != nil {
		return Number{}, true, err
	}
	return out, true, nil
}

func negNumber(n Number) Number {
	if n.kind == numberRat {
		r := n.r
		r.num = -r.num
		return RatNumber(r)
	}
	return Float(-n.f)
}

func addNumber(e *env, a, b Number) (Number, error) {
	if e.mode == modeExact && a.kind == numberRat && b.kind == numberRat {
		r, err := a.r.Add(b.r)
		if err == nil {
			return RatNumber(r), nil
		}
	}
	return Float(a.Float64() + b.Float64()), nil
}

func subNumber(e *env, a, b Number) (Number, error) {
	if e.mode == modeExact && a.kind == numberRat && b.kind == numberRat {
		r, err := a.r.Sub(b.r)
		if err == nil {
			return RatNumber(r), nil
		}
	}
	return Float(a.Float64() - b.Float64()), nil
}

func mulNumber(e *env, a, b Number) (Number, error) {
	if e.mode == modeExact && a.kind == numberRat && b.kind == numberRat {
		r, err := a.r.Mul(b.r)
		if err == nil {
			return RatNumber(r), nil
		}
	}
	return Float(a.Float64() * b.Float64()), nil
}

func divNumber(e *env, a, b Number) (Number, error) {
	if b.Float64() == 0 {
		return Number{}, fmt.Errorf("%w: division by zero", ErrEval)
	}
	if e.mode == modeExact && a.kind == numberRat && b.kind == numberRat {
		r, err := a.r.Div(b.r)
		if err == nil {
			return RatNumber(r), nil
		}
	}
	return Float(a.Float64() / b.Float64()), nil
}

func powNumber(e *env, a, b Number) (Number, error) {
	if e.mode == modeExact && a.kind == numberRat && b.kind == numberRat && b.r.den == 1 {
		r, err := a.r.PowInt(b.r.num)
		if err == nil {
			return RatNumber(r), nil
		}
	}
	return Float(math.Pow(a.Float64(), b.Float64())), nil
}
