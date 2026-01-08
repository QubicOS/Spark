package vector

import (
	"fmt"
	"math"
)

// builtinCallPlot implements plotting helpers that generate XY series as Nx2 matrices.
//
// Returned matrices may include NaN pairs to break line segments.
func builtinCallPlot(e *env, name string, args []Value) (Value, bool, error) {
	switch name {
	case "implicit":
		// implicit(f, xmin, xmax, ymin, ymax[, n]) where f is expr in x,y.
		if len(args) < 5 || len(args) > 6 {
			return Value{}, true, fmt.Errorf("%w: implicit(expr, xmin, xmax, ymin, ymax[, n])", ErrEval)
		}
		f, err := requireExpr(args[0])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		xmin, err := requireFloat(args[1])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		xmax, err := requireFloat(args[2])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		ymin, err := requireFloat(args[3])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		ymax, err := requireFloat(args[4])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		n := 96
		if len(args) == 6 {
			v, err := requireInt(args[5])
			if err != nil || v < 8 || v > 512 {
				return Value{}, true, fmt.Errorf("%w: implicit n must be 8..512", ErrEval)
			}
			n = v
		}
		data, rows, err := contourMarchingSquares(e, f, []float64{0}, xmin, xmax, ymin, ymax, n)
		if err != nil {
			return Value{}, true, err
		}
		return MatrixValue(rows, 2, data), true, nil

	case "contour":
		// contour(f, levels, xmin, xmax, ymin, ymax[, n]) where f is expr in x,y.
		if len(args) < 6 || len(args) > 7 {
			return Value{}, true, fmt.Errorf("%w: contour(expr, levels, xmin, xmax, ymin, ymax[, n])", ErrEval)
		}
		f, err := requireExpr(args[0])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		levels, err := levelsArg(args[1])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: contour: %w", ErrEval, err)
		}
		xmin, err := requireFloat(args[2])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		xmax, err := requireFloat(args[3])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		ymin, err := requireFloat(args[4])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		ymax, err := requireFloat(args[5])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		n := 96
		if len(args) == 7 {
			v, err := requireInt(args[6])
			if err != nil || v < 8 || v > 512 {
				return Value{}, true, fmt.Errorf("%w: contour n must be 8..512", ErrEval)
			}
			n = v
		}
		data, rows, err := contourMarchingSquares(e, f, levels, xmin, xmax, ymin, ymax, n)
		if err != nil {
			return Value{}, true, err
		}
		return MatrixValue(rows, 2, data), true, nil

	case "vectorfield":
		// vectorfield(f, g, xmin, xmax, ymin, ymax[, n]) where f,g are expr in x,y.
		if len(args) < 6 || len(args) > 7 {
			return Value{}, true, fmt.Errorf("%w: vectorfield(f, g, xmin, xmax, ymin, ymax[, n])", ErrEval)
		}
		f, err := requireExpr(args[0])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		g, err := requireExpr(args[1])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		xmin, err := requireFloat(args[2])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		xmax, err := requireFloat(args[3])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		ymin, err := requireFloat(args[4])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		ymax, err := requireFloat(args[5])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: %w", ErrEval, err)
		}
		n := 16
		if len(args) == 7 {
			v, err := requireInt(args[6])
			if err != nil || v < 4 || v > 128 {
				return Value{}, true, fmt.Errorf("%w: vectorfield n must be 4..128", ErrEval)
			}
			n = v
		}
		data, rows, err := vectorFieldSegments(e, f, g, xmin, xmax, ymin, ymax, n)
		if err != nil {
			return Value{}, true, err
		}
		return MatrixValue(rows, 2, data), true, nil
	}

	return Value{}, false, nil
}

func levelsArg(v Value) ([]float64, error) {
	switch v.kind {
	case valueNumber:
		return []float64{v.num.Float64()}, nil
	case valueArray:
		if len(v.arr) == 0 {
			return nil, fmt.Errorf("empty levels")
		}
		out := make([]float64, len(v.arr))
		copy(out, v.arr)
		return out, nil
	default:
		return nil, fmt.Errorf("expected number or array")
	}
}

func vectorFieldSegments(e *env, fx, fy node, xmin, xmax, ymin, ymax float64, n int) ([]float64, int, error) {
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

	if xmin >= xmax || ymin >= ymax {
		return nil, 0, fmt.Errorf("%w: vectorfield expects min < max", ErrEval)
	}
	if n < 2 {
		n = 2
	}

	dx := (xmax - xmin) / float64(n-1)
	dy := (ymax - ymin) / float64(n-1)
	scale := 0.35 * math.Min(dx, dy)

	data := make([]float64, 0, n*n*6)
	appendBreak := func() {
		data = append(data, math.NaN(), math.NaN())
	}

	for j := 0; j < n; j++ {
		y := ymin + float64(j)*dy
		for i := 0; i < n; i++ {
			x := xmin + float64(i)*dx
			e.vars["x"] = NumberValue(Float(x))
			e.vars["y"] = NumberValue(Float(y))
			vx, err := evalFloat(fx, e)
			if err != nil {
				return nil, 0, err
			}
			vy, err := evalFloat(fy, e)
			if err != nil {
				return nil, 0, err
			}
			if math.IsNaN(vx) || math.IsNaN(vy) || math.IsInf(vx, 0) || math.IsInf(vy, 0) {
				continue
			}
			mag := math.Hypot(vx, vy)
			if mag == 0 || math.IsNaN(mag) || math.IsInf(mag, 0) {
				continue
			}
			ux := vx / mag
			uy := vy / mag
			x1 := x + ux*scale
			y1 := y + uy*scale
			data = append(data, x, y, x1, y1)
			appendBreak()
		}
	}

	rows := len(data) / 2
	if rows == 0 {
		return []float64{math.NaN(), math.NaN()}, 1, nil
	}
	return data, rows, nil
}

func contourMarchingSquares(e *env, f node, levels []float64, xmin, xmax, ymin, ymax float64, n int) ([]float64, int, error) {
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

	if xmin >= xmax || ymin >= ymax {
		return nil, 0, fmt.Errorf("%w: contour expects min < max", ErrEval)
	}
	if n < 8 {
		n = 8
	}

	// Sample grid.
	xs := make([]float64, n)
	ys := make([]float64, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n-1)
		xs[i] = xmin + t*(xmax-xmin)
		ys[i] = ymin + t*(ymax-ymin)
	}
	val := make([]float64, n*n)
	for j := 0; j < n; j++ {
		for i := 0; i < n; i++ {
			e.vars["x"] = NumberValue(Float(xs[i]))
			e.vars["y"] = NumberValue(Float(ys[j]))
			v, err := evalFloat(f, e)
			if err != nil {
				return nil, 0, err
			}
			val[j*n+i] = v
		}
	}

	data := make([]float64, 0, n*n*4)
	appendSeg := func(x0, y0, x1, y1 float64) {
		data = append(data, x0, y0, x1, y1, math.NaN(), math.NaN())
	}

	interp := func(x0, y0, z0, x1, y1, z1, level float64) (float64, float64, bool) {
		if math.IsNaN(z0) || math.IsNaN(z1) || math.IsInf(z0, 0) || math.IsInf(z1, 0) {
			return 0, 0, false
		}
		dz := z1 - z0
		if dz == 0 {
			return (x0 + x1) / 2, (y0 + y1) / 2, true
		}
		t := (level - z0) / dz
		if t < 0 {
			t = 0
		} else if t > 1 {
			t = 1
		}
		return x0 + t*(x1-x0), y0 + t*(y1-y0), true
	}

	for _, level := range levels {
		for j := 0; j < n-1; j++ {
			y0 := ys[j]
			y1 := ys[j+1]
			for i := 0; i < n-1; i++ {
				x0 := xs[i]
				x1 := xs[i+1]
				z00 := val[j*n+i]
				z10 := val[j*n+i+1]
				z01 := val[(j+1)*n+i]
				z11 := val[(j+1)*n+i+1]

				c0 := z00 > level
				c1 := z10 > level
				c2 := z11 > level
				c3 := z01 > level
				idx := 0
				if c0 {
					idx |= 1
				}
				if c1 {
					idx |= 2
				}
				if c2 {
					idx |= 4
				}
				if c3 {
					idx |= 8
				}
				if idx == 0 || idx == 15 {
					continue
				}

				// Edge intersections: e0 bottom (00-10), e1 right (10-11), e2 top (01-11), e3 left (00-01).
				var ex [4]float64
				var ey [4]float64
				var ok [4]bool
				ex[0], ey[0], ok[0] = interp(x0, y0, z00, x1, y0, z10, level)
				ex[1], ey[1], ok[1] = interp(x1, y0, z10, x1, y1, z11, level)
				ex[2], ey[2], ok[2] = interp(x0, y1, z01, x1, y1, z11, level)
				ex[3], ey[3], ok[3] = interp(x0, y0, z00, x0, y1, z01, level)

				emit := func(a, b int) {
					if ok[a] && ok[b] {
						appendSeg(ex[a], ey[a], ex[b], ey[b])
					}
				}

				switch idx {
				case 1, 14:
					emit(3, 0)
				case 2, 13:
					emit(0, 1)
				case 3, 12:
					emit(3, 1)
				case 4, 11:
					emit(1, 2)
				case 5:
					emit(3, 2)
					emit(0, 1)
				case 6, 9:
					emit(0, 2)
				case 7, 8:
					emit(3, 2)
				case 10:
					emit(3, 0)
					emit(1, 2)
				}
			}
		}
	}

	rows := len(data) / 2
	if rows == 0 {
		return []float64{math.NaN(), math.NaN()}, 1, nil
	}
	return data, rows, nil
}
