package vector

import (
	"fmt"
	"math"
	"math/cmplx"
)

func builtinCallPoly(e *env, name string, args []Value) (Value, bool, error) {
	_ = e
	switch name {
	case "polyval":
		// polyval(coeffs, x).
		if len(args) != 2 || args[0].kind != valueArray {
			return Value{}, true, fmt.Errorf("%w: polyval(coeffs, x)", ErrEval)
		}
		p, err := polyFromCoeffsValue(args[0])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: polyval: %w", ErrEval, err)
		}
		switch args[1].kind {
		case valueNumber:
			return NumberValue(Float(p.eval(args[1].num.Float64()))), true, nil
		case valueComplex:
			return ComplexValueC(evalPolyComplex(p, args[1].c)), true, nil
		default:
			return Value{}, true, fmt.Errorf("%w: polyval expects numeric x", ErrEval)
		}

	case "polyfit":
		// polyfit(data, n) or polyfit(x, y, n).
		if len(args) != 2 && len(args) != 3 {
			return Value{}, true, fmt.Errorf("%w: polyfit(data, n) or polyfit(x, y, n)", ErrEval)
		}
		var xs, ys []float64
		var degV Value
		if len(args) == 2 {
			if args[0].kind != valueMatrix || args[0].cols != 2 {
				return Value{}, true, fmt.Errorf("%w: polyfit expects Nx2 matrix", ErrEval)
			}
			xs = make([]float64, args[0].rows)
			ys = make([]float64, args[0].rows)
			for i := 0; i < args[0].rows; i++ {
				xs[i] = args[0].mat[i*2]
				ys[i] = args[0].mat[i*2+1]
			}
			degV = args[1]
		} else {
			if args[0].kind != valueArray || args[1].kind != valueArray {
				return Value{}, true, fmt.Errorf("%w: polyfit expects x and y as arrays", ErrEval)
			}
			if len(args[0].arr) != len(args[1].arr) {
				return Value{}, true, fmt.Errorf("%w: polyfit x/y length mismatch", ErrEval)
			}
			xs = args[0].arr
			ys = args[1].arr
			degV = args[2]
		}
		deg, err := requireInt(degV)
		if err != nil || deg < 0 || deg > 32 {
			return Value{}, true, fmt.Errorf("%w: polyfit degree must be 0..32", ErrEval)
		}
		coeffs, err := polyFitLeastSquares(xs, ys, deg)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: polyfit: %w", ErrEval, err)
		}
		return ArrayValue(coeffs), true, nil

	case "roots":
		// roots(coeffs) where coeffs are [c0..cn] (ascending degree).
		if len(args) != 1 || args[0].kind != valueArray {
			return Value{}, false, nil
		}
		p, err := polyFromCoeffsValue(args[0])
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: roots: %w", ErrEval, err)
		}
		roots, err := polyRootsDurandKerner(p)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: roots: %w", ErrEval, err)
		}
		out := make([]float64, len(roots)*2)
		for i, z := range roots {
			out[i*2+0] = real(z)
			out[i*2+1] = imag(z)
		}
		return MatrixValue(len(roots), 2, out), true, nil
	}

	return Value{}, false, nil
}

func evalPolyComplex(p poly, z complex128) complex128 {
	var out complex128
	for i := len(p.coeffs) - 1; i >= 0; i-- {
		out = out*z + complex(p.coeffs[i], 0)
	}
	return out
}

func polyFitLeastSquares(xs, ys []float64, deg int) ([]float64, error) {
	if len(xs) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	m := deg + 1
	ata := make([]float64, m*m)
	atb := make([]float64, m)

	// Build normal equations: A = V^T V, b = V^T y.
	for i := range xs {
		x := xs[i]
		y := ys[i]
		if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
			continue
		}
		pows := make([]float64, m)
		pows[0] = 1
		for j := 1; j < m; j++ {
			pows[j] = pows[j-1] * x
		}
		for r := 0; r < m; r++ {
			atb[r] += pows[r] * y
			for c := 0; c < m; c++ {
				ata[r*m+c] += pows[r] * pows[c]
			}
		}
	}

	coeffs, err := solveLinearSystem(ata, atb, m)
	if err != nil {
		return nil, err
	}
	return coeffs, nil
}

func polyRootsDurandKerner(p poly) ([]complex128, error) {
	p = p.trim()
	n := p.degree()
	if n <= 0 {
		return nil, fmt.Errorf("degree must be >= 1")
	}
	cn := p.coeffs[n]
	if cn == 0 {
		return nil, fmt.Errorf("leading coefficient is zero")
	}
	// Normalize to monic.
	coeffs := make([]float64, n+1)
	for i := 0; i <= n; i++ {
		coeffs[i] = p.coeffs[i] / cn
	}
	monic := poly{coeffs: coeffs}

	if n == 1 {
		return []complex128{complex(-coeffs[0], 0)}, nil
	}

	// Cauchy bound.
	var maxAbs float64
	for i := 0; i < n; i++ {
		v := math.Abs(coeffs[i])
		if v > maxAbs {
			maxAbs = v
		}
	}
	radius := 1 + maxAbs

	roots := make([]complex128, n)
	for k := 0; k < n; k++ {
		theta := 2 * math.Pi * float64(k) / float64(n)
		roots[k] = complex(radius*math.Cos(theta), radius*math.Sin(theta))
	}

	const (
		tol     = 1e-12
		maxIter = 256
	)

	for iter := 0; iter < maxIter; iter++ {
		maxDelta := 0.0
		for i := 0; i < n; i++ {
			zi := roots[i]
			den := complex(1, 0)
			for j := 0; j < n; j++ {
				if i == j {
					continue
				}
				d := zi - roots[j]
				if d == 0 {
					d = complex(1e-9, 1e-9)
				}
				den *= d
			}
			if den == 0 {
				continue
			}
			pz := evalPolyComplex(monic, zi)
			dz := pz / den
			roots[i] = zi - dz
			if d := cmplx.Abs(dz); d > maxDelta {
				maxDelta = d
			}
		}
		if maxDelta <= tol {
			break
		}
	}

	return roots, nil
}
