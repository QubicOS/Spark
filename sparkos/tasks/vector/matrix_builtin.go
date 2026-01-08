package vector

import (
	"fmt"
	"math"
)

func builtinCallMatrix(e *env, name string, args []Value) (Value, bool, error) {
	_ = e

	if len(args) == 1 && args[0].kind == valueMatrix {
		if fn, ok := unaryArrayBuiltins[name]; ok {
			out := make([]float64, len(args[0].mat))
			for i, x := range args[0].mat {
				out[i] = fn(x)
			}
			return MatrixValue(args[0].rows, args[0].cols, out), true, nil
		}
		if agg, ok := arrayAggBuiltins[name]; ok {
			return NumberValue(Float(agg(args[0].mat))), true, nil
		}
	}

	switch name {
	case "zeros":
		if len(args) != 2 || !args[0].IsNumber() || !args[1].IsNumber() {
			return Value{}, true, fmt.Errorf("%w: zeros(rows, cols)", ErrEval)
		}
		r := int(args[0].num.Float64())
		c := int(args[1].num.Float64())
		data, err := matrixZeros(r, c)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		return MatrixValue(r, c, data), true, nil

	case "ones":
		if len(args) != 2 || !args[0].IsNumber() || !args[1].IsNumber() {
			return Value{}, true, fmt.Errorf("%w: ones(rows, cols)", ErrEval)
		}
		r := int(args[0].num.Float64())
		c := int(args[1].num.Float64())
		data, err := matrixOnes(r, c)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		return MatrixValue(r, c, data), true, nil

	case "eye":
		if len(args) != 1 || !args[0].IsNumber() {
			return Value{}, true, fmt.Errorf("%w: eye(n)", ErrEval)
		}
		n := int(args[0].num.Float64())
		data, err := matrixEye(n)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		return MatrixValue(n, n, data), true, nil

	case "reshape":
		if len(args) != 3 || (!args[0].IsArray() && !args[0].IsMatrix()) || !args[1].IsNumber() || !args[2].IsNumber() {
			return Value{}, true, fmt.Errorf("%w: reshape(xs, rows, cols)", ErrEval)
		}
		r := int(args[1].num.Float64())
		c := int(args[2].num.Float64())
		var xs []float64
		if args[0].IsArray() {
			xs = args[0].arr
		} else {
			xs = args[0].mat
		}
		if r <= 0 || c <= 0 || len(xs) != r*c {
			return Value{}, true, fmt.Errorf("%w: reshape: need len(xs)==rows*cols", ErrEval)
		}
		data := make([]float64, len(xs))
		copy(data, xs)
		return MatrixValue(r, c, data), true, nil

	case "T", "transpose":
		if len(args) != 1 || args[0].kind != valueMatrix {
			return Value{}, true, fmt.Errorf("%w: T(A)", ErrEval)
		}
		out, err := matrixTranspose(args[0].rows, args[0].cols, args[0].mat)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		return MatrixValue(args[0].cols, args[0].rows, out), true, nil

	case "det":
		if len(args) != 1 || args[0].kind != valueMatrix {
			return Value{}, true, fmt.Errorf("%w: det(A)", ErrEval)
		}
		d, err := matrixDet(args[0].rows, args[0].cols, args[0].mat)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		return NumberValue(Float(d)), true, nil

	case "inv":
		if len(args) != 1 || args[0].kind != valueMatrix {
			return Value{}, true, fmt.Errorf("%w: inv(A)", ErrEval)
		}
		out, err := matrixInv(args[0].rows, args[0].cols, args[0].mat)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		return MatrixValue(args[0].rows, args[0].cols, out), true, nil

	case "shape":
		if len(args) != 1 || args[0].kind != valueMatrix {
			return Value{}, true, fmt.Errorf("%w: shape(A)", ErrEval)
		}
		return ArrayValue([]float64{float64(args[0].rows), float64(args[0].cols)}), true, nil

	case "flatten":
		if len(args) != 1 || args[0].kind != valueMatrix {
			return Value{}, true, fmt.Errorf("%w: flatten(A)", ErrEval)
		}
		out := make([]float64, len(args[0].mat))
		copy(out, args[0].mat)
		return ArrayValue(out), true, nil

	case "get":
		if len(args) != 3 || args[0].kind != valueMatrix || !args[1].IsNumber() || !args[2].IsNumber() {
			return Value{}, true, fmt.Errorf("%w: get(A, row, col)", ErrEval)
		}
		r, err := matrixIndex(args[1].num.Float64(), args[0].rows)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		c, err := matrixIndex(args[2].num.Float64(), args[0].cols)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		return NumberValue(Float(args[0].mat[r*args[0].cols+c])), true, nil

	case "set":
		if len(args) != 4 || args[0].kind != valueMatrix || !args[1].IsNumber() || !args[2].IsNumber() || !args[3].IsNumber() {
			return Value{}, true, fmt.Errorf("%w: set(A, row, col, value)", ErrEval)
		}
		r, err := matrixIndex(args[1].num.Float64(), args[0].rows)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		c, err := matrixIndex(args[2].num.Float64(), args[0].cols)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		out := make([]float64, len(args[0].mat))
		copy(out, args[0].mat)
		out[r*args[0].cols+c] = args[3].num.Float64()
		return MatrixValue(args[0].rows, args[0].cols, out), true, nil

	case "row":
		if len(args) != 2 || args[0].kind != valueMatrix || !args[1].IsNumber() {
			return Value{}, true, fmt.Errorf("%w: row(A, row)", ErrEval)
		}
		r, err := matrixIndex(args[1].num.Float64(), args[0].rows)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		out := make([]float64, args[0].cols)
		copy(out, args[0].mat[r*args[0].cols:(r+1)*args[0].cols])
		return ArrayValue(out), true, nil

	case "col":
		if len(args) != 2 || args[0].kind != valueMatrix || !args[1].IsNumber() {
			return Value{}, true, fmt.Errorf("%w: col(A, col)", ErrEval)
		}
		c, err := matrixIndex(args[1].num.Float64(), args[0].cols)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		out := make([]float64, args[0].rows)
		for r := 0; r < args[0].rows; r++ {
			out[r] = args[0].mat[r*args[0].cols+c]
		}
		return ArrayValue(out), true, nil

	case "diag":
		if len(args) != 1 || args[0].kind != valueMatrix {
			return Value{}, true, fmt.Errorf("%w: diag(A)", ErrEval)
		}
		if args[0].rows != args[0].cols {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, ErrMatrixShape)
		}
		n := args[0].rows
		out := make([]float64, n)
		for i := 0; i < n; i++ {
			out[i] = args[0].mat[i*n+i]
		}
		return ArrayValue(out), true, nil

	case "trace":
		if len(args) != 1 || args[0].kind != valueMatrix {
			return Value{}, true, fmt.Errorf("%w: trace(A)", ErrEval)
		}
		if args[0].rows != args[0].cols {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, ErrMatrixShape)
		}
		n := args[0].rows
		var s float64
		for i := 0; i < n; i++ {
			s += args[0].mat[i*n+i]
		}
		return NumberValue(Float(s)), true, nil

	case "norm":
		if len(args) != 1 || args[0].kind != valueMatrix {
			return Value{}, true, fmt.Errorf("%w: norm(A)", ErrEval)
		}
		var ss float64
		for _, x := range args[0].mat {
			ss += x * x
		}
		return NumberValue(Float(math.Sqrt(ss))), true, nil
	}

	return Value{}, false, nil
}

func matrixIndex(x float64, size int) (int, error) {
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
