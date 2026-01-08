package vector

import "fmt"

func builtinCallLinAlg(_ *env, name string, args []Value) (Value, bool, error) {
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
	}

	return Value{}, false, nil
}

