package vector

import (
	"fmt"
	"math"
)

func builtinCallSolve(e *env, name string, args []Value) (Value, bool, error) {
	switch name {
	case "solve1":
		// solve1(f, x0[, tol[, maxIter]]) where f is an expr in x.
		if len(args) < 2 || len(args) > 4 {
			return Value{}, true, fmt.Errorf("%w: solve1(expr, x0[, tol[, maxIter]])", ErrEval)
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

	case "solve2":
		// solve2(f, g, x0, y0[, tol[, maxIter]]) where f,g are expr in x,y.
		if len(args) < 4 || len(args) > 6 {
			return Value{}, true, fmt.Errorf("%w: solve2(f, g, x0, y0[, tol[, maxIter]])", ErrEval)
		}
		f, err := requireExpr(args[0])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		g, err := requireExpr(args[1])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		x0, err := requireFloat(args[2])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		y0, err := requireFloat(args[3])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		tol := 1e-9
		if len(args) >= 5 {
			v, err := requireFloat(args[4])
			if err != nil {
				return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
			}
			if v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v) {
				tol = v
			}
		}
		maxIter := 32
		if len(args) == 6 {
			it, err := requireInt(args[5])
			if err != nil {
				return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
			}
			if it >= 1 && it <= 512 {
				maxIter = it
			}
		}

		x, y, err := solve2Newton(e, f, g, x0, y0, tol, maxIter)
		if err != nil {
			return Value{}, true, err
		}
		return ArrayValue([]float64{x, y}), true, nil

	case "roots":
		// roots(f, xmin, xmax[, n]) where f is expr in x.
		if len(args) < 3 || len(args) > 4 {
			return Value{}, true, fmt.Errorf("%w: roots(expr, xmin, xmax[, n])", ErrEval)
		}
		f, err := requireExpr(args[0])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		xMin, err := requireFloat(args[1])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		xMax, err := requireFloat(args[2])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		if xMin >= xMax {
			return Value{}, true, fmt.Errorf("%w: roots expects xmin < xmax", ErrEval)
		}
		n := 256
		if len(args) == 4 {
			nn, err := requireInt(args[3])
			if err != nil {
				return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
			}
			if nn < 8 || nn > 4096 {
				return Value{}, true, fmt.Errorf("%w: roots n must be 8..4096", ErrEval)
			}
			n = nn
		}
		out, err := rootsScanBisection(e, f, xMin, xMax, n)
		if err != nil {
			return Value{}, true, err
		}
		return ArrayValue(out), true, nil

	case "region":
		// region(cond, xmin, xmax, ymin, ymax[, n]) where cond is expr in x,y.
		if len(args) < 5 || len(args) > 6 {
			return Value{}, true, fmt.Errorf("%w: region(cond, xmin, xmax, ymin, ymax[, n])", ErrEval)
		}
		cond, err := requireExpr(args[0])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		xMin, err := requireFloat(args[1])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		xMax, err := requireFloat(args[2])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		yMin, err := requireFloat(args[3])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		yMax, err := requireFloat(args[4])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		if xMin >= xMax || yMin >= yMax {
			return Value{}, true, fmt.Errorf("%w: region expects min < max", ErrEval)
		}
		n := 128
		if len(args) == 6 {
			nn, err := requireInt(args[5])
			if err != nil {
				return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
			}
			if nn < 8 || nn > 1024 {
				return Value{}, true, fmt.Errorf("%w: region n must be 8..1024", ErrEval)
			}
			n = nn
		}
		data, rows, err := regionScan(e, cond, xMin, xMax, yMin, yMax, n)
		if err != nil {
			return Value{}, true, err
		}
		return MatrixValue(rows, 2, data), true, nil
	}

	return Value{}, false, nil
}

func requireExpr(v Value) (node, error) {
	if v.kind != valueExpr || v.expr == nil {
		return nil, fmt.Errorf("expected expr(...)")
	}
	return v.expr, nil
}

func requireFloat(v Value) (float64, error) {
	if !v.IsNumber() {
		return 0, fmt.Errorf("expected number")
	}
	return v.num.Float64(), nil
}

func requireInt(v Value) (int, error) {
	f, err := requireFloat(v)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) {
		return 0, fmt.Errorf("expected integer")
	}
	return int(f), nil
}

func solve1Newton(e *env, f node, x0, tol float64, maxIter int) (float64, error) {
	x := x0
	prevX, hadPrevX := e.vars["x"]
	defer func() {
		if hadPrevX {
			e.vars["x"] = prevX
		} else {
			delete(e.vars, "x")
		}
	}()

	for iter := 0; iter < maxIter; iter++ {
		e.vars["x"] = NumberValue(Float(x))
		fx, err := evalFloat(f, e)
		if err != nil {
			return 0, err
		}
		if math.Abs(fx) <= tol {
			return x, nil
		}
		h := 1e-6 * (1 + math.Abs(x))
		e.vars["x"] = NumberValue(Float(x + h))
		fph, err := evalFloat(f, e)
		if err != nil {
			return 0, err
		}
		df := (fph - fx) / h
		if df == 0 || math.IsNaN(df) || math.IsInf(df, 0) {
			return 0, fmt.Errorf("%w: solve1 derivative is zero/invalid", ErrEval)
		}
		dx := -fx / df
		xNext := x + dx
		if math.Abs(dx) <= tol*(1+math.Abs(x)) {
			return xNext, nil
		}
		x = xNext
	}
	return x, fmt.Errorf("%w: solve1 did not converge", ErrEval)
}

func solve2Newton(e *env, f, g node, x0, y0, tol float64, maxIter int) (float64, float64, error) {
	x := x0
	y := y0
	prevX, hadPrevX := e.vars["x"]
	prevY, hadPrevY := e.vars["y"]
	defer func() {
		if hadPrevX {
			e.vars["x"] = prevX
		} else {
			delete(e.vars, "x")
		}
		if hadPrevY {
			e.vars["y"] = prevY
		} else {
			delete(e.vars, "y")
		}
	}()

	for iter := 0; iter < maxIter; iter++ {
		e.vars["x"] = NumberValue(Float(x))
		e.vars["y"] = NumberValue(Float(y))
		fv, err := evalFloat(f, e)
		if err != nil {
			return 0, 0, err
		}
		gv, err := evalFloat(g, e)
		if err != nil {
			return 0, 0, err
		}
		if math.Abs(fv) <= tol && math.Abs(gv) <= tol {
			return x, y, nil
		}

		hx := 1e-6 * (1 + math.Abs(x))
		hy := 1e-6 * (1 + math.Abs(y))

		e.vars["x"] = NumberValue(Float(x + hx))
		e.vars["y"] = NumberValue(Float(y))
		fxph, err := evalFloat(f, e)
		if err != nil {
			return 0, 0, err
		}
		gxph, err := evalFloat(g, e)
		if err != nil {
			return 0, 0, err
		}
		dfdx := (fxph - fv) / hx
		dgdx := (gxph - gv) / hx

		e.vars["x"] = NumberValue(Float(x))
		e.vars["y"] = NumberValue(Float(y + hy))
		fyph, err := evalFloat(f, e)
		if err != nil {
			return 0, 0, err
		}
		gyph, err := evalFloat(g, e)
		if err != nil {
			return 0, 0, err
		}
		dfdy := (fyph - fv) / hy
		dgdy := (gyph - gv) / hy

		det := dfdx*dgdy - dfdy*dgdx
		if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) {
			return 0, 0, fmt.Errorf("%w: solve2 singular Jacobian", ErrEval)
		}
		// Solve J*delta = -F.
		dx := (-fv*dgdy - (-gv)*dfdy) / det
		dy := (dfdx*(-gv) - dgdx*(-fv)) / det

		xNext := x + dx
		yNext := y + dy
		if math.Abs(dx) <= tol*(1+math.Abs(x)) && math.Abs(dy) <= tol*(1+math.Abs(y)) {
			return xNext, yNext, nil
		}
		x, y = xNext, yNext
	}
	return x, y, fmt.Errorf("%w: solve2 did not converge", ErrEval)
}

func evalFloat(ex node, e *env) (float64, error) {
	v, err := ex.Eval(e)
	if err != nil {
		return 0, err
	}
	if !v.IsNumber() {
		return 0, fmt.Errorf("%w: expected numeric expression", ErrEval)
	}
	return v.num.Float64(), nil
}

func rootsScanBisection(e *env, f node, xMin, xMax float64, n int) ([]float64, error) {
	prevX, hadPrevX := e.vars["x"]
	defer func() {
		if hadPrevX {
			e.vars["x"] = prevX
		} else {
			delete(e.vars, "x")
		}
	}()

	eps := 1e-10 * (1 + math.Abs(xMax-xMin))
	out := make([]float64, 0, 16)
	lastRoot := math.NaN()

	var prevVal float64
	var prevOK bool
	var prevT float64
	for i := 0; i < n; i++ {
		t := xMin + float64(i)*(xMax-xMin)/float64(n-1)
		e.vars["x"] = NumberValue(Float(t))
		val, err := evalFloat(f, e)
		if err != nil {
			return nil, err
		}
		if math.IsNaN(val) || math.IsInf(val, 0) {
			prevOK = false
			continue
		}

		if math.Abs(val) <= eps {
			if math.IsNaN(lastRoot) || math.Abs(t-lastRoot) > 1e-6 {
				out = append(out, t)
				lastRoot = t
			}
			prevOK = false
			continue
		}

		if prevOK && (prevVal < 0) != (val < 0) {
			a := prevT
			b := t
			fa := prevVal
			fb := val
			for iter := 0; iter < 64; iter++ {
				m := (a + b) / 2
				e.vars["x"] = NumberValue(Float(m))
				fm, err := evalFloat(f, e)
				if err != nil {
					return nil, err
				}
				if math.Abs(fm) <= eps || (b-a) <= eps {
					if math.IsNaN(lastRoot) || math.Abs(m-lastRoot) > 1e-6 {
						out = append(out, m)
						lastRoot = m
					}
					break
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
		}

		prevOK = true
		prevVal = val
		prevT = t
	}

	return out, nil
}

func regionScan(e *env, cond node, xMin, xMax, yMin, yMax float64, n int) ([]float64, int, error) {
	prevX, hadPrevX := e.vars["x"]
	prevY, hadPrevY := e.vars["y"]
	defer func() {
		if hadPrevX {
			e.vars["x"] = prevX
		} else {
			delete(e.vars, "x")
		}
		if hadPrevY {
			e.vars["y"] = prevY
		} else {
			delete(e.vars, "y")
		}
	}()

	data := make([]float64, 0, n*n)
	appendBreak := func() {
		if len(data) >= 2 && math.IsNaN(data[len(data)-2]) && math.IsNaN(data[len(data)-1]) {
			return
		}
		data = append(data, math.NaN(), math.NaN())
	}

	for yi := 0; yi < n; yi++ {
		y := yMin + float64(yi)*(yMax-yMin)/float64(n-1)
		run := false
		for xi := 0; xi < n; xi++ {
			x := xMin + float64(xi)*(xMax-xMin)/float64(n-1)
			e.vars["x"] = NumberValue(Float(x))
			e.vars["y"] = NumberValue(Float(y))
			v, err := cond.Eval(e)
			if err != nil {
				return nil, 0, err
			}
			ok, err := truthy(v)
			if err != nil {
				return nil, 0, err
			}
			if ok {
				data = append(data, x, y)
				run = true
				continue
			}
			if run {
				appendBreak()
				run = false
			}
		}
		appendBreak()
	}

	rows := len(data) / 2
	if rows == 0 {
		return []float64{math.NaN(), math.NaN()}, 1, nil
	}
	return data, rows, nil
}
