package vector

import "math"

// scalarBuiltin describes a numeric function callable from expressions.
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
