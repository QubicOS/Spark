package vector

import "math"

// unaryArrayBuiltins apply an element-wise function to an array value.
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

// arrayAggBuiltins reduce an array value to a scalar.
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
