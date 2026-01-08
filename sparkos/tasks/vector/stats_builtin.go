package vector

import (
	"fmt"
	"math"
)

func builtinCallStats(_ *env, name string, args []Value) (Value, bool, error) {
	switch name {
	case "cov":
		if len(args) != 2 || args[0].kind != valueArray || args[1].kind != valueArray {
			return Value{}, true, fmt.Errorf("%w: cov(x, y)", ErrEval)
		}
		c, err := cov(args[0].arr, args[1].arr)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: cov: %w", ErrEval, err)
		}
		return NumberValue(Float(c)), true, nil

	case "corr":
		if len(args) != 2 || args[0].kind != valueArray || args[1].kind != valueArray {
			return Value{}, true, fmt.Errorf("%w: corr(x, y)", ErrEval)
		}
		c, err := corr(args[0].arr, args[1].arr)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: corr: %w", ErrEval, err)
		}
		return NumberValue(Float(c)), true, nil

	case "hist":
		// hist(data, bins) -> matrix (bins x 2) of [center,count].
		if len(args) != 2 || args[0].kind != valueArray || !args[1].IsNumber() {
			return Value{}, true, fmt.Errorf("%w: hist(data, bins)", ErrEval)
		}
		bins, err := requireInt(args[1])
		if err != nil || bins < 1 || bins > 8192 {
			return Value{}, true, fmt.Errorf("%w: hist bins must be 1..8192", ErrEval)
		}
		out, err := hist(args[0].arr, bins)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: hist: %w", ErrEval, err)
		}
		return MatrixValue(bins, 2, out), true, nil

	case "convolve":
		// convolve(a, b) -> discrete convolution.
		if len(args) != 2 || args[0].kind != valueArray || args[1].kind != valueArray {
			return Value{}, true, fmt.Errorf("%w: convolve(a, b)", ErrEval)
		}
		out := convolve(args[0].arr, args[1].arr)
		return ArrayValue(out), true, nil
	}

	return Value{}, false, nil
}

func cov(x, y []float64) (float64, error) {
	if len(x) != len(y) {
		return 0, fmt.Errorf("length mismatch")
	}
	if len(x) == 0 {
		return math.NaN(), nil
	}
	mx := arrayAvg(x)
	my := arrayAvg(y)
	var sum float64
	for i := range x {
		sum += (x[i] - mx) * (y[i] - my)
	}
	return sum / float64(len(x)), nil
}

func corr(x, y []float64) (float64, error) {
	c, err := cov(x, y)
	if err != nil {
		return 0, err
	}
	sx := arrayStd(x)
	sy := arrayStd(y)
	if sx == 0 || sy == 0 || math.IsNaN(sx) || math.IsNaN(sy) {
		return math.NaN(), nil
	}
	return c / (sx * sy), nil
}

func hist(data []float64, bins int) ([]float64, error) {
	if len(data) == 0 {
		out := make([]float64, bins*2)
		for i := 0; i < bins; i++ {
			out[i*2+0] = float64(i)
			out[i*2+1] = 0
		}
		return out, nil
	}
	min := data[0]
	max := data[0]
	for _, x := range data[1:] {
		if x < min {
			min = x
		}
		if x > max {
			max = x
		}
	}
	if math.IsNaN(min) || math.IsNaN(max) || math.IsInf(min, 0) || math.IsInf(max, 0) {
		return nil, fmt.Errorf("invalid data range")
	}
	if min == max {
		out := make([]float64, bins*2)
		for i := 0; i < bins; i++ {
			out[i*2+0] = min
			out[i*2+1] = 0
		}
		out[0*2+1] = float64(len(data))
		return out, nil
	}

	width := (max - min) / float64(bins)
	counts := make([]float64, bins)
	for _, x := range data {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			continue
		}
		i := int((x - min) / width)
		if i < 0 {
			i = 0
		} else if i >= bins {
			i = bins - 1
		}
		counts[i]++
	}

	out := make([]float64, bins*2)
	for i := 0; i < bins; i++ {
		center := min + (float64(i)+0.5)*width
		out[i*2+0] = center
		out[i*2+1] = counts[i]
	}
	return out, nil
}

func convolve(a, b []float64) []float64 {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	out := make([]float64, len(a)+len(b)-1)
	for i, av := range a {
		for j, bv := range b {
			out[i+j] += av * bv
		}
	}
	return out
}
