package vector

import (
	"fmt"
)

func builtinCallLinAlg(e *env, name string, args []Value) (Value, bool, error) {
	switch name {
	case "solve":
		// solve(A, b) where A is NxN matrix and b is N array or NxM matrix.
		if len(args) != 2 || args[0].kind != valueMatrix {
			return Value{}, true, fmt.Errorf("%w: solve(A, b)", ErrEval)
		}
		A := args[0]
		if A.rows != A.cols {
			return Value{}, true, fmt.Errorf("%w: solve expects square matrix", ErrEval)
		}
		n := A.rows

		switch args[1].kind {
		case valueArray:
			if len(args[1].arr) != n {
				return Value{}, true, fmt.Errorf("%w: solve expects len(b)==rows(A)", ErrEval)
			}
			x, err := solveLinearSystem(A.mat, args[1].arr, n)
			if err != nil {
				return Value{}, true, fmt.Errorf("%w: solve: %w", ErrEval, err)
			}
			return ArrayValue(x), true, nil

		case valueMatrix:
			B := args[1]
			if B.rows != n {
				return Value{}, true, fmt.Errorf("%w: solve expects rows(b)==rows(A)", ErrEval)
			}
			X, err := solveLinearSystemMulti(A.mat, B.mat, n, B.cols)
			if err != nil {
				return Value{}, true, fmt.Errorf("%w: solve: %w", ErrEval, err)
			}
			return MatrixValue(n, B.cols, X), true, nil

		default:
			return Value{}, true, fmt.Errorf("%w: solve expects b as array or matrix", ErrEval)
		}

	case "qr":
		// qr(A) returns Q and sets _R.
		if len(args) != 1 || args[0].kind != valueMatrix {
			return Value{}, true, fmt.Errorf("%w: qr(A)", ErrEval)
		}
		A := args[0]
		q, r, err := qrDecompose(A.rows, A.cols, A.mat)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: qr: %w", ErrEval, err)
		}
		e.vars["_R"] = MatrixValue(A.cols, A.cols, r)
		e.vars["_Q"] = MatrixValue(A.rows, A.cols, q)
		return MatrixValue(A.rows, A.cols, q), true, nil

	case "svd":
		// svd(A) returns s (singular values) and sets _U, _V.
		if len(args) != 1 || args[0].kind != valueMatrix {
			return Value{}, true, fmt.Errorf("%w: svd(A)", ErrEval)
		}
		A := args[0]
		u, s, v, err := svdThin(A.rows, A.cols, A.mat)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: svd: %w", ErrEval, err)
		}
		e.vars["_U"] = MatrixValue(A.rows, A.cols, u)
		e.vars["_V"] = MatrixValue(A.cols, A.cols, v)
		e.vars["_S"] = ArrayValue(append([]float64(nil), s...))
		return ArrayValue(s), true, nil
	}

	return Value{}, false, nil
}
