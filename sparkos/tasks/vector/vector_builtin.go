package vector

import (
	"fmt"
	"math"
)

func builtinCallVector(_ *env, name string, args []Value) (Value, bool, error) {
	switch name {
	case "get":
		if len(args) != 2 {
			return Value{}, false, nil
		}
		if args[0].kind != valueArray || !args[1].IsNumber() {
			return Value{}, false, nil
		}
		i, err := vectorIndex(args[1].num.Float64(), len(args[0].arr))
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		return NumberValue(Float(args[0].arr[i])), true, nil

	case "set":
		if len(args) != 3 {
			return Value{}, false, nil
		}
		if args[0].kind != valueArray || !args[1].IsNumber() || !args[2].IsNumber() {
			return Value{}, false, nil
		}
		i, err := vectorIndex(args[1].num.Float64(), len(args[0].arr))
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		out := make([]float64, len(args[0].arr))
		copy(out, args[0].arr)
		out[i] = args[2].num.Float64()
		return ArrayValue(out), true, nil

	case "x", "y", "z", "w":
		if len(args) != 1 {
			return Value{}, true, fmt.Errorf("%w: %s(v)", ErrEval, name)
		}
		if args[0].kind != valueArray {
			return Value{}, false, nil
		}
		idx := 0
		switch name {
		case "x":
			idx = 0
		case "y":
			idx = 1
		case "z":
			idx = 2
		case "w":
			idx = 3
		}
		if idx < 0 || idx >= len(args[0].arr) {
			return Value{}, true, fmt.Errorf("%w: %s expects vector length >= %d", ErrEval, name, idx+1)
		}
		return NumberValue(Float(args[0].arr[idx])), true, nil

	case "vec2":
		if len(args) != 2 || !args[0].IsNumber() || !args[1].IsNumber() {
			return Value{}, true, fmt.Errorf("%w: vec2(x, y)", ErrEval)
		}
		return ArrayValue([]float64{args[0].num.Float64(), args[1].num.Float64()}), true, nil

	case "vec3":
		if len(args) != 3 || !args[0].IsNumber() || !args[1].IsNumber() || !args[2].IsNumber() {
			return Value{}, true, fmt.Errorf("%w: vec3(x, y, z)", ErrEval)
		}
		return ArrayValue([]float64{args[0].num.Float64(), args[1].num.Float64(), args[2].num.Float64()}), true, nil

	case "vec4":
		if len(args) != 4 || !args[0].IsNumber() || !args[1].IsNumber() || !args[2].IsNumber() || !args[3].IsNumber() {
			return Value{}, true, fmt.Errorf("%w: vec4(x, y, z, w)", ErrEval)
		}
		return ArrayValue([]float64{
			args[0].num.Float64(),
			args[1].num.Float64(),
			args[2].num.Float64(),
			args[3].num.Float64(),
		}), true, nil

	case "dot":
		if len(args) != 2 {
			return Value{}, true, fmt.Errorf("%w: dot(a, b)", ErrEval)
		}
		if args[0].kind != valueArray || args[1].kind != valueArray {
			return Value{}, false, nil
		}
		a, b := args[0].arr, args[1].arr
		if len(a) != len(b) {
			return Value{}, true, fmt.Errorf("%w: array length mismatch", ErrEval)
		}
		var s float64
		for i := range a {
			s += a[i] * b[i]
		}
		return NumberValue(Float(s)), true, nil

	case "cross":
		if len(args) != 2 {
			return Value{}, true, fmt.Errorf("%w: cross(a, b)", ErrEval)
		}
		if args[0].kind != valueArray || args[1].kind != valueArray {
			return Value{}, false, nil
		}
		a, b := args[0].arr, args[1].arr
		if len(a) != 3 || len(b) != 3 {
			return Value{}, true, fmt.Errorf("%w: cross expects 3D vectors", ErrEval)
		}
		return ArrayValue([]float64{
			a[1]*b[2] - a[2]*b[1],
			a[2]*b[0] - a[0]*b[2],
			a[0]*b[1] - a[1]*b[0],
		}), true, nil

	case "norm", "mag":
		if len(args) != 1 {
			return Value{}, true, fmt.Errorf("%w: norm(v)", ErrEval)
		}
		if args[0].kind != valueArray {
			return Value{}, false, nil
		}
		v := args[0].arr
		var ss float64
		for _, x := range v {
			ss += x * x
		}
		return NumberValue(Float(math.Sqrt(ss))), true, nil

	case "unit", "normalize":
		if len(args) != 1 {
			return Value{}, true, fmt.Errorf("%w: unit(v)", ErrEval)
		}
		if args[0].kind != valueArray {
			return Value{}, false, nil
		}
		v := args[0].arr
		var ss float64
		for _, x := range v {
			ss += x * x
		}
		if ss == 0 {
			return Value{}, true, fmt.Errorf("%w: zero-length vector", ErrEval)
		}
		inv := 1 / math.Sqrt(ss)
		out := make([]float64, len(v))
		for i, x := range v {
			out[i] = x * inv
		}
		return ArrayValue(out), true, nil

	case "dist":
		if len(args) != 2 {
			return Value{}, true, fmt.Errorf("%w: dist(a, b)", ErrEval)
		}
		if args[0].kind != valueArray || args[1].kind != valueArray {
			return Value{}, false, nil
		}
		a, b := args[0].arr, args[1].arr
		if len(a) != len(b) {
			return Value{}, true, fmt.Errorf("%w: array length mismatch", ErrEval)
		}
		var ss float64
		for i := range a {
			d := a[i] - b[i]
			ss += d * d
		}
		return NumberValue(Float(math.Sqrt(ss))), true, nil

	case "angle":
		if len(args) != 2 {
			return Value{}, true, fmt.Errorf("%w: angle(a, b)", ErrEval)
		}
		if args[0].kind != valueArray || args[1].kind != valueArray {
			return Value{}, false, nil
		}
		a, b := args[0].arr, args[1].arr
		if len(a) != len(b) {
			return Value{}, true, fmt.Errorf("%w: array length mismatch", ErrEval)
		}
		var dot float64
		var aa float64
		var bb float64
		for i := range a {
			dot += a[i] * b[i]
			aa += a[i] * a[i]
			bb += b[i] * b[i]
		}
		if aa == 0 || bb == 0 {
			return Value{}, true, fmt.Errorf("%w: angle undefined for zero vector", ErrEval)
		}
		c := dot / math.Sqrt(aa*bb)
		if c < -1 {
			c = -1
		}
		if c > 1 {
			c = 1
		}
		return NumberValue(Float(math.Acos(c))), true, nil

	case "proj":
		if len(args) != 2 {
			return Value{}, true, fmt.Errorf("%w: proj(a, b)", ErrEval)
		}
		if args[0].kind != valueArray || args[1].kind != valueArray {
			return Value{}, false, nil
		}
		a, b := args[0].arr, args[1].arr
		if len(a) != len(b) {
			return Value{}, true, fmt.Errorf("%w: array length mismatch", ErrEval)
		}
		var dot float64
		var bb float64
		for i := range a {
			dot += a[i] * b[i]
			bb += b[i] * b[i]
		}
		if bb == 0 {
			return Value{}, true, fmt.Errorf("%w: projection onto zero vector", ErrEval)
		}
		k := dot / bb
		out := make([]float64, len(b))
		for i := range b {
			out[i] = b[i] * k
		}
		return ArrayValue(out), true, nil

	case "outer":
		if len(args) != 2 {
			return Value{}, true, fmt.Errorf("%w: outer(a, b)", ErrEval)
		}
		if args[0].kind != valueArray || args[1].kind != valueArray {
			return Value{}, false, nil
		}
		a, b := args[0].arr, args[1].arr
		r := len(a)
		c := len(b)
		if r == 0 || c == 0 {
			return Value{}, true, fmt.Errorf("%w: outer expects non-empty vectors", ErrEval)
		}
		data := make([]float64, r*c)
		for i := 0; i < r; i++ {
			for j := 0; j < c; j++ {
				data[i*c+j] = a[i] * b[j]
			}
		}
		return MatrixValue(r, c, data), true, nil

	case "lerp":
		if len(args) != 3 {
			return Value{}, true, fmt.Errorf("%w: lerp(a, b, t)", ErrEval)
		}
		if args[0].kind != valueArray || args[1].kind != valueArray {
			return Value{}, false, nil
		}
		if !args[2].IsNumber() {
			return Value{}, true, fmt.Errorf("%w: lerp(a, b, t)", ErrEval)
		}
		a, b := args[0].arr, args[1].arr
		if len(a) != len(b) {
			return Value{}, true, fmt.Errorf("%w: array length mismatch", ErrEval)
		}
		t := args[2].num.Float64()
		out := make([]float64, len(a))
		for i := range out {
			out[i] = a[i] + (b[i]-a[i])*t
		}
		return ArrayValue(out), true, nil
	}

	return Value{}, false, nil
}

func vectorIndex(x float64, size int) (int, error) {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return 0, fmt.Errorf("invalid index %v", x)
	}
	if x != math.Trunc(x) {
		return 0, fmt.Errorf("index must be an integer: %v", x)
	}
	i := int(x)
	if i < 1 || i > size {
		return 0, fmt.Errorf("index out of range: %d", i)
	}
	return i - 1, nil
}
