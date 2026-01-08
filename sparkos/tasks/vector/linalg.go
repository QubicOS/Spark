package vector

import (
	"fmt"
	"math"
	"sort"
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

func matrixDot(a []float64, aOff int, b []float64, bOff int, n int) float64 {
	var s float64
	for i := 0; i < n; i++ {
		s += a[aOff+i] * b[bOff+i]
	}
	return s
}

// qrDecompose computes a thin QR decomposition of A (m x n): A = Q*R.
// Q is m x n with orthonormal columns, R is n x n upper-triangular.
func qrDecompose(m, n int, a []float64) (q []float64, r []float64, err error) {
	if err := matrixCheck(m, n, a); err != nil {
		return nil, nil, err
	}
	if m < 1 || n < 1 {
		return nil, nil, fmt.Errorf("invalid shape")
	}
	q = make([]float64, m*n)
	copy(q, a)
	r = make([]float64, n*n)

	// Modified Gram-Schmidt.
	for k := 0; k < n; k++ {
		// r[k,k] = ||q[:,k]||.
		var norm float64
		for i := 0; i < m; i++ {
			v := q[i*n+k]
			norm += v * v
		}
		norm = math.Sqrt(norm)
		if norm == 0 || math.IsNaN(norm) || math.IsInf(norm, 0) {
			return nil, nil, fmt.Errorf("rank deficient")
		}
		r[k*n+k] = norm
		inv := 1 / norm
		for i := 0; i < m; i++ {
			q[i*n+k] *= inv
		}
		for j := k + 1; j < n; j++ {
			// r[k,j] = q[:,k]^T * q[:,j].
			var dot float64
			for i := 0; i < m; i++ {
				dot += q[i*n+k] * q[i*n+j]
			}
			r[k*n+j] = dot
			for i := 0; i < m; i++ {
				q[i*n+j] -= dot * q[i*n+k]
			}
		}
	}
	return q, r, nil
}

// jacobiEigenSym computes eigenvalues/vectors for a real symmetric matrix A (n x n).
// It returns eigenvalues (ascending) and eigenvectors as columns in V (n x n) such that A = V*diag(w)*V^T.
func jacobiEigenSym(a []float64, n int) ([]float64, []float64, error) {
	if err := matrixCheck(n, n, a); err != nil {
		return nil, nil, err
	}
	aa := make([]float64, len(a))
	copy(aa, a)
	v, err := matrixEye(n)
	if err != nil {
		return nil, nil, err
	}

	const (
		maxIter = 64
		eps     = 1e-12
	)

	for iter := 0; iter < maxIter; iter++ {
		// Find largest off-diagonal element.
		p, q := 0, 1
		maxAbs := 0.0
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				val := math.Abs(aa[i*n+j])
				if val > maxAbs {
					maxAbs = val
					p, q = i, j
				}
			}
		}
		if maxAbs < eps {
			break
		}

		app := aa[p*n+p]
		aqq := aa[q*n+q]
		apq := aa[p*n+q]
		if apq == 0 {
			continue
		}

		tau := (aqq - app) / (2 * apq)
		t := 1 / (math.Abs(tau) + math.Sqrt(1+tau*tau))
		if tau < 0 {
			t = -t
		}
		c := 1 / math.Sqrt(1+t*t)
		s := t * c

		// Rotate rows/cols p,q in aa.
		for k := 0; k < n; k++ {
			if k == p || k == q {
				continue
			}
			akp := aa[k*n+p]
			akq := aa[k*n+q]
			aa[k*n+p] = c*akp - s*akq
			aa[k*n+q] = s*akp + c*akq
			aa[p*n+k] = aa[k*n+p]
			aa[q*n+k] = aa[k*n+q]
		}
		aa[p*n+p] = c*c*app - 2*s*c*apq + s*s*aqq
		aa[q*n+q] = s*s*app + 2*s*c*apq + c*c*aqq
		aa[p*n+q] = 0
		aa[q*n+p] = 0

		// Update eigenvectors.
		for k := 0; k < n; k++ {
			vkp := v[k*n+p]
			vkq := v[k*n+q]
			v[k*n+p] = c*vkp - s*vkq
			v[k*n+q] = s*vkp + c*vkq
		}
	}

	w := make([]float64, n)
	for i := 0; i < n; i++ {
		w[i] = aa[i*n+i]
	}

	// Sort ascending.
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(i, j int) bool { return w[idx[i]] < w[idx[j]] })
	ws := make([]float64, n)
	vs := make([]float64, n*n)
	for col, src := range idx {
		ws[col] = w[src]
		for r := 0; r < n; r++ {
			vs[r*n+col] = v[r*n+src]
		}
	}
	return ws, vs, nil
}

// svdThin computes a thin SVD A = U*diag(s)*V^T for A (m x n).
// It returns U (m x n), s (n), and V (n x n) with singular values sorted descending.
func svdThin(m, n int, a []float64) (u []float64, s []float64, v []float64, err error) {
	if err := matrixCheck(m, n, a); err != nil {
		return nil, nil, nil, err
	}
	if n > 64 {
		return nil, nil, nil, fmt.Errorf("svd supports n<=64")
	}

	// Compute ATA = A^T A (n x n).
	at, err := matrixTranspose(m, n, a)
	if err != nil {
		return nil, nil, nil, err
	}
	ata, err := matrixMul(n, m, at, m, n, a)
	if err != nil {
		return nil, nil, nil, err
	}

	evals, vecs, err := jacobiEigenSym(ata, n)
	if err != nil {
		return nil, nil, nil, err
	}

	// Singular values = sqrt(max(eigen,0)), sort descending.
	type pair struct {
		s float64
		i int
	}
	ps := make([]pair, n)
	for i := 0; i < n; i++ {
		ev := evals[i]
		if ev < 0 && ev > -1e-9 {
			ev = 0
		}
		if ev < 0 {
			ev = 0
		}
		ps[i] = pair{s: math.Sqrt(ev), i: i}
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].s > ps[j].s })

	v = make([]float64, n*n)
	s = make([]float64, n)
	for col, p := range ps {
		s[col] = p.s
		for r := 0; r < n; r++ {
			v[r*n+col] = vecs[r*n+p.i]
		}
	}

	// U = A*V*diag(1/s).
	av, err := matrixMul(m, n, a, n, n, v)
	if err != nil {
		return nil, nil, nil, err
	}
	u = make([]float64, m*n)
	for col := 0; col < n; col++ {
		sigma := s[col]
		if sigma == 0 {
			continue
		}
		inv := 1 / sigma
		for r := 0; r < m; r++ {
			u[r*n+col] = av[r*n+col] * inv
		}
	}

	// Re-orthonormalize U via QR to improve stability.
	qq, _, err := qrDecompose(m, n, u)
	if err == nil {
		u = qq
	}

	return u, s, v, nil
}
