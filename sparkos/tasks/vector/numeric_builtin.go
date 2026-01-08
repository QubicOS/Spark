package vector

import (
	"fmt"
	"math"
)

// builtinCallNumeric implements numeric analysis helpers over expr(...) values.
func builtinCallNumeric(e *env, name string, args []Value) (Value, bool, error) {
	switch name {
	case "newton":
		// newton(f, x0[, tol[, maxIter]]) where f is an expr in x.
		if len(args) < 2 || len(args) > 4 {
			return Value{}, true, fmt.Errorf("%w: newton(expr, x0[, tol[, maxIter]])", ErrEval)
		}
		f, err := requireExpr(args[0])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		x0, err := requireFloat(args[1])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		tol := 1e-9
		if len(args) >= 3 {
			v, err := requireFloat(args[2])
			if err != nil {
				return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
			}
			if v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v) {
				tol = v
			}
		}
		maxIter := 32
		if len(args) == 4 {
			it, err := requireInt(args[3])
			if err != nil {
				return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
			}
			if it >= 1 && it <= 512 {
				maxIter = it
			}
		}
		x, err := solve1Newton(e, f, x0, tol, maxIter)
		if err != nil {
			return Value{}, true, err
		}
		return NumberValue(Float(x)), true, nil

	case "bisection":
		// bisection(f, a, b[, tol[, maxIter]]) where f is an expr in x.
		if len(args) < 3 || len(args) > 5 {
			return Value{}, true, fmt.Errorf("%w: bisection(expr, a, b[, tol[, maxIter]])", ErrEval)
		}
		f, err := requireExpr(args[0])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		a, err := requireFloat(args[1])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		b, err := requireFloat(args[2])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		if a >= b {
			return Value{}, true, fmt.Errorf("%w: bisection expects a < b", ErrEval)
		}
		tol := 1e-9
		if len(args) >= 4 {
			v, err := requireFloat(args[3])
			if err != nil {
				return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
			}
			if v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v) {
				tol = v
			}
		}
		maxIter := 64
		if len(args) == 5 {
			it, err := requireInt(args[4])
			if err != nil {
				return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
			}
			if it >= 1 && it <= 2048 {
				maxIter = it
			}
		}

		root, err := bisectionRoot(e, f, a, b, tol, maxIter)
		if err != nil {
			return Value{}, true, err
		}
		return NumberValue(Float(root)), true, nil

	case "secant":
		// secant(f, x0, x1[, tol[, maxIter]]) where f is an expr in x.
		if len(args) < 3 || len(args) > 5 {
			return Value{}, true, fmt.Errorf("%w: secant(expr, x0, x1[, tol[, maxIter]])", ErrEval)
		}
		f, err := requireExpr(args[0])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		x0, err := requireFloat(args[1])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		x1, err := requireFloat(args[2])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		tol := 1e-9
		if len(args) >= 4 {
			v, err := requireFloat(args[3])
			if err != nil {
				return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
			}
			if v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v) {
				tol = v
			}
		}
		maxIter := 64
		if len(args) == 5 {
			it, err := requireInt(args[4])
			if err != nil {
				return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
			}
			if it >= 1 && it <= 2048 {
				maxIter = it
			}
		}
		root, err := secantRoot(e, f, x0, x1, tol, maxIter)
		if err != nil {
			return Value{}, true, err
		}
		return NumberValue(Float(root)), true, nil

	case "diff_num":
		// diff_num(f, x[, h]) where f is an expr in x.
		if len(args) < 2 || len(args) > 3 {
			return Value{}, true, fmt.Errorf("%w: diff_num(expr, x[, h])", ErrEval)
		}
		f, err := requireExpr(args[0])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		x, err := requireFloat(args[1])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		h := 1e-6 * (1 + math.Abs(x))
		if len(args) == 3 {
			v, err := requireFloat(args[2])
			if err != nil {
				return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
			}
			if v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v) {
				h = v
			}
		}
		d, err := diffCentral(e, f, x, h)
		if err != nil {
			return Value{}, true, err
		}
		return NumberValue(Float(d)), true, nil

	case "integrate_num":
		// integrate_num(f, a, b[, method[, n]]).
		if len(args) < 3 || len(args) > 5 {
			return Value{}, true, fmt.Errorf("%w: integrate_num(expr, a, b[, method[, n]])", ErrEval)
		}
		f, err := requireExpr(args[0])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		a, err := requireFloat(args[1])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		b, err := requireFloat(args[2])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		if a == b {
			return NumberValue(Float(0)), true, nil
		}

		method := 0
		n := 1024
		if len(args) >= 4 {
			v, err := requireInt(args[3])
			if err != nil {
				return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
			}
			// Convenience: integrate_num(f,a,b,n) if n is clearly not a method.
			if v > 1 {
				n = v
			} else {
				method = v
			}
		}
		if len(args) == 5 {
			v, err := requireInt(args[4])
			if err != nil {
				return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
			}
			n = v
		}
		if method < 0 || method > 1 {
			return Value{}, true, fmt.Errorf("%w: integrate_num method must be 0..1", ErrEval)
		}
		if n < 2 || n > 1_000_000 {
			return Value{}, true, fmt.Errorf("%w: integrate_num n must be 2..1000000", ErrEval)
		}

		var out float64
		switch method {
		case 0:
			out, err = integrateTrapezoid(e, f, a, b, n)
		case 1:
			out, err = integrateSimpson(e, f, a, b, n)
		default:
			return Value{}, true, fmt.Errorf("%w: integrate_num method must be 0..1", ErrEval)
		}
		if err != nil {
			return Value{}, true, err
		}
		return NumberValue(Float(out)), true, nil

	case "interp":
		// interp(data, x) for data as [y0,y1,...] or Nx2 matrix [x,y].
		if len(args) != 2 {
			return Value{}, true, fmt.Errorf("%w: interp(data, x)", ErrEval)
		}
		x, err := requireFloat(args[1])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		y, err := interp1(args[0], x)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: interp: %w", ErrEval, err)
		}
		return NumberValue(Float(y)), true, nil
	}

	return Value{}, false, nil
}

func bisectionRoot(e *env, f node, a, b, tol float64, maxIter int) (float64, error) {
	prevX, hadPrevX := e.vars["x"]
	defer func() {
		if hadPrevX {
			e.vars["x"] = prevX
		} else {
			delete(e.vars, "x")
		}
	}()

	e.vars["x"] = NumberValue(Float(a))
	fa, err := evalFloat(f, e)
	if err != nil {
		return 0, err
	}
	e.vars["x"] = NumberValue(Float(b))
	fb, err := evalFloat(f, e)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(fa) || math.IsNaN(fb) || math.IsInf(fa, 0) || math.IsInf(fb, 0) {
		return 0, fmt.Errorf("%w: bisection invalid endpoint value", ErrEval)
	}
	if fa == 0 {
		return a, nil
	}
	if fb == 0 {
		return b, nil
	}
	if (fa < 0) == (fb < 0) {
		return 0, fmt.Errorf("%w: bisection requires opposite signs at endpoints", ErrEval)
	}

	for iter := 0; iter < maxIter; iter++ {
		m := (a + b) / 2
		e.vars["x"] = NumberValue(Float(m))
		fm, err := evalFloat(f, e)
		if err != nil {
			return 0, err
		}
		if math.Abs(fm) <= tol || math.Abs(b-a) <= tol*(1+math.Abs(m)) {
			return m, nil
		}
		if (fa < 0) != (fm < 0) {
			b = m
			fb = fm
		} else {
			a = m
			fa = fm
		}
		_ = fb
	}
	return (a + b) / 2, fmt.Errorf("%w: bisection did not converge", ErrEval)
}

func secantRoot(e *env, f node, x0, x1, tol float64, maxIter int) (float64, error) {
	prevX, hadPrevX := e.vars["x"]
	defer func() {
		if hadPrevX {
			e.vars["x"] = prevX
		} else {
			delete(e.vars, "x")
		}
	}()

	e.vars["x"] = NumberValue(Float(x0))
	f0, err := evalFloat(f, e)
	if err != nil {
		return 0, err
	}
	e.vars["x"] = NumberValue(Float(x1))
	f1, err := evalFloat(f, e)
	if err != nil {
		return 0, err
	}

	for iter := 0; iter < maxIter; iter++ {
		if math.Abs(f1) <= tol {
			return x1, nil
		}
		den := f1 - f0
		if den == 0 || math.IsNaN(den) || math.IsInf(den, 0) {
			return x1, fmt.Errorf("%w: secant slope is zero/invalid", ErrEval)
		}
		x2 := x1 - f1*(x1-x0)/den
		if math.Abs(x2-x1) <= tol*(1+math.Abs(x1)) {
			return x2, nil
		}
		x0, x1 = x1, x2
		f0 = f1
		e.vars["x"] = NumberValue(Float(x1))
		f1, err = evalFloat(f, e)
		if err != nil {
			return 0, err
		}
	}
	return x1, fmt.Errorf("%w: secant did not converge", ErrEval)
}

func diffCentral(e *env, f node, x, h float64) (float64, error) {
	prevX, hadPrevX := e.vars["x"]
	defer func() {
		if hadPrevX {
			e.vars["x"] = prevX
		} else {
			delete(e.vars, "x")
		}
	}()

	e.vars["x"] = NumberValue(Float(x - h))
	fm, err := evalFloat(f, e)
	if err != nil {
		return 0, err
	}
	e.vars["x"] = NumberValue(Float(x + h))
	fp, err := evalFloat(f, e)
	if err != nil {
		return 0, err
	}
	return (fp - fm) / (2 * h), nil
}

func integrateTrapezoid(e *env, f node, a, b float64, n int) (float64, error) {
	prevX, hadPrevX := e.vars["x"]
	defer func() {
		if hadPrevX {
			e.vars["x"] = prevX
		} else {
			delete(e.vars, "x")
		}
	}()

	h := (b - a) / float64(n)
	e.vars["x"] = NumberValue(Float(a))
	fa, err := evalFloat(f, e)
	if err != nil {
		return 0, err
	}
	e.vars["x"] = NumberValue(Float(b))
	fb, err := evalFloat(f, e)
	if err != nil {
		return 0, err
	}
	sum := 0.5 * (fa + fb)
	for i := 1; i < n; i++ {
		x := a + float64(i)*h
		e.vars["x"] = NumberValue(Float(x))
		fx, err := evalFloat(f, e)
		if err != nil {
			return 0, err
		}
		sum += fx
	}
	return sum * h, nil
}

func integrateSimpson(e *env, f node, a, b float64, n int) (float64, error) {
	if n%2 == 1 {
		n++
	}
	prevX, hadPrevX := e.vars["x"]
	defer func() {
		if hadPrevX {
			e.vars["x"] = prevX
		} else {
			delete(e.vars, "x")
		}
	}()

	h := (b - a) / float64(n)
	sumOdd := 0.0
	sumEven := 0.0

	for i := 1; i < n; i++ {
		x := a + float64(i)*h
		e.vars["x"] = NumberValue(Float(x))
		fx, err := evalFloat(f, e)
		if err != nil {
			return 0, err
		}
		if i%2 == 1 {
			sumOdd += fx
		} else {
			sumEven += fx
		}
	}

	e.vars["x"] = NumberValue(Float(a))
	fa, err := evalFloat(f, e)
	if err != nil {
		return 0, err
	}
	e.vars["x"] = NumberValue(Float(b))
	fb, err := evalFloat(f, e)
	if err != nil {
		return 0, err
	}
	return (h / 3) * (fa + fb + 4*sumOdd + 2*sumEven), nil
}

func interp1(v Value, x float64) (float64, error) {
	switch v.kind {
	case valueArray:
		if len(v.arr) == 0 {
			return math.NaN(), nil
		}
		if x <= 0 {
			return v.arr[0], nil
		}
		last := float64(len(v.arr) - 1)
		if x >= last {
			return v.arr[len(v.arr)-1], nil
		}
		i0 := int(math.Floor(x))
		i1 := i0 + 1
		t := x - float64(i0)
		return v.arr[i0] + t*(v.arr[i1]-v.arr[i0]), nil

	case valueMatrix:
		if v.cols != 2 || v.rows == 0 {
			return 0, fmt.Errorf("expected Nx2 matrix")
		}
		if x <= v.mat[0] {
			return v.mat[1], nil
		}
		last := v.rows - 1
		if x >= v.mat[last*2] {
			return v.mat[last*2+1], nil
		}
		for i := 0; i < last; i++ {
			x0 := v.mat[i*2]
			y0 := v.mat[i*2+1]
			x1 := v.mat[(i+1)*2]
			y1 := v.mat[(i+1)*2+1]
			if (x0 <= x && x <= x1) || (x1 <= x && x <= x0) {
				if x0 == x1 {
					return y0, nil
				}
				t := (x - x0) / (x1 - x0)
				return y0 + t*(y1-y0), nil
			}
		}
		return math.NaN(), nil
	default:
		return 0, fmt.Errorf("expected array or matrix")
	}
}
