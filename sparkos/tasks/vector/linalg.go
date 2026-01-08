package vector

import (
	"fmt"
	"math"
)

// solveLinearSystem solves A*x=b for x with A as row-major NxN.
func solveLinearSystem(a []float64, b []float64, n int) ([]float64, error) {
	if len(a) != n*n || len(b) != n {
		return nil, fmt.Errorf("bad dimensions")
	}
	aa := make([]float64, len(a))
	copy(aa, a)
	bb := make([]float64, len(b))
	copy(bb, b)

	for k := 0; k < n; k++ {
		piv := k
		maxAbs := math.Abs(aa[k*n+k])
		for i := k + 1; i < n; i++ {
			v := math.Abs(aa[i*n+k])
			if v > maxAbs {
				maxAbs = v
				piv = i
			}
		}
		if maxAbs == 0 || math.IsNaN(maxAbs) || math.IsInf(maxAbs, 0) {
			return nil, fmt.Errorf("singular system")
		}
		if piv != k {
			for j := k; j < n; j++ {
				aa[k*n+j], aa[piv*n+j] = aa[piv*n+j], aa[k*n+j]
			}
			bb[k], bb[piv] = bb[piv], bb[k]
		}

		pivot := aa[k*n+k]
		for i := k + 1; i < n; i++ {
			f := aa[i*n+k] / pivot
			if f == 0 {
				continue
			}
			aa[i*n+k] = 0
			for j := k + 1; j < n; j++ {
				aa[i*n+j] -= f * aa[k*n+j]
			}
			bb[i] -= f * bb[k]
		}
	}

	x := make([]float64, n)
	for i := n - 1; i >= 0; i-- {
		sum := bb[i]
		for j := i + 1; j < n; j++ {
			sum -= aa[i*n+j] * x[j]
		}
		pivot := aa[i*n+i]
		if pivot == 0 {
			return nil, fmt.Errorf("singular system")
		}
		x[i] = sum / pivot
	}
	return x, nil
}

// solveLinearSystemMulti solves A*X=B for X with A as NxN, B as NxM (row-major).
func solveLinearSystemMulti(a []float64, b []float64, n, m int) ([]float64, error) {
	if len(a) != n*n || len(b) != n*m {
		return nil, fmt.Errorf("bad dimensions")
	}
	aa := make([]float64, len(a))
	copy(aa, a)
	bb := make([]float64, len(b))
	copy(bb, b)

	for k := 0; k < n; k++ {
		piv := k
		maxAbs := math.Abs(aa[k*n+k])
		for i := k + 1; i < n; i++ {
			v := math.Abs(aa[i*n+k])
			if v > maxAbs {
				maxAbs = v
				piv = i
			}
		}
		if maxAbs == 0 || math.IsNaN(maxAbs) || math.IsInf(maxAbs, 0) {
			return nil, fmt.Errorf("singular system")
		}
		if piv != k {
			for j := k; j < n; j++ {
				aa[k*n+j], aa[piv*n+j] = aa[piv*n+j], aa[k*n+j]
			}
			for j := 0; j < m; j++ {
				bb[k*m+j], bb[piv*m+j] = bb[piv*m+j], bb[k*m+j]
			}
		}

		pivot := aa[k*n+k]
		for i := k + 1; i < n; i++ {
			f := aa[i*n+k] / pivot
			if f == 0 {
				continue
			}
			aa[i*n+k] = 0
			for j := k + 1; j < n; j++ {
				aa[i*n+j] -= f * aa[k*n+j]
			}
			for j := 0; j < m; j++ {
				bb[i*m+j] -= f * bb[k*m+j]
			}
		}
	}

	x := make([]float64, n*m)
	for i := n - 1; i >= 0; i-- {
		pivot := aa[i*n+i]
		if pivot == 0 {
			return nil, fmt.Errorf("singular system")
		}
		for j := 0; j < m; j++ {
			sum := bb[i*m+j]
			for k := i + 1; k < n; k++ {
				sum -= aa[i*n+k] * x[k*m+j]
			}
			x[i*m+j] = sum / pivot
		}
	}
	return x, nil
}

