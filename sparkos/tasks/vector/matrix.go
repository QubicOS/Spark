package vector

import (
	"errors"
	"fmt"
	"math"
)

var ErrMatrixShape = errors.New("matrix shape mismatch")

func matrixCheck(rows, cols int, data []float64) error {
	if rows <= 0 || cols <= 0 {
		return errors.New("invalid matrix shape")
	}
	if len(data) != rows*cols {
		return fmt.Errorf("invalid matrix data length: %d != %d", len(data), rows*cols)
	}
	return nil
}

func matrixZeros(rows, cols int) ([]float64, error) {
	if rows <= 0 || cols <= 0 {
		return nil, errors.New("invalid matrix shape")
	}
	return make([]float64, rows*cols), nil
}

func matrixOnes(rows, cols int) ([]float64, error) {
	data, err := matrixZeros(rows, cols)
	if err != nil {
		return nil, err
	}
	for i := range data {
		data[i] = 1
	}
	return data, nil
}

func matrixEye(n int) ([]float64, error) {
	data, err := matrixZeros(n, n)
	if err != nil {
		return nil, err
	}
	for i := 0; i < n; i++ {
		data[i*n+i] = 1
	}
	return data, nil
}

func matrixTranspose(rows, cols int, a []float64) ([]float64, error) {
	if err := matrixCheck(rows, cols, a); err != nil {
		return nil, err
	}
	out := make([]float64, len(a))
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			out[c*rows+r] = a[r*cols+c]
		}
	}
	return out, nil
}

func matrixAdd(rows, cols int, a, b []float64) ([]float64, error) {
	if err := matrixCheck(rows, cols, a); err != nil {
		return nil, err
	}
	if err := matrixCheck(rows, cols, b); err != nil {
		return nil, err
	}
	out := make([]float64, len(a))
	for i := range out {
		out[i] = a[i] + b[i]
	}
	return out, nil
}

func matrixSub(rows, cols int, a, b []float64) ([]float64, error) {
	if err := matrixCheck(rows, cols, a); err != nil {
		return nil, err
	}
	if err := matrixCheck(rows, cols, b); err != nil {
		return nil, err
	}
	out := make([]float64, len(a))
	for i := range out {
		out[i] = a[i] - b[i]
	}
	return out, nil
}

func matrixScale(rows, cols int, a []float64, k float64) ([]float64, error) {
	if err := matrixCheck(rows, cols, a); err != nil {
		return nil, err
	}
	out := make([]float64, len(a))
	for i := range out {
		out[i] = a[i] * k
	}
	return out, nil
}

func matrixMul(aRows, aCols int, a []float64, bRows, bCols int, b []float64) ([]float64, error) {
	if err := matrixCheck(aRows, aCols, a); err != nil {
		return nil, err
	}
	if err := matrixCheck(bRows, bCols, b); err != nil {
		return nil, err
	}
	if aCols != bRows {
		return nil, ErrMatrixShape
	}
	out := make([]float64, aRows*bCols)
	for i := 0; i < aRows; i++ {
		for k := 0; k < aCols; k++ {
			aik := a[i*aCols+k]
			if aik == 0 {
				continue
			}
			for j := 0; j < bCols; j++ {
				out[i*bCols+j] += aik * b[k*bCols+j]
			}
		}
	}
	return out, nil
}

func matrixMulVec(aRows, aCols int, a []float64, x []float64) ([]float64, error) {
	if err := matrixCheck(aRows, aCols, a); err != nil {
		return nil, err
	}
	if len(x) != aCols {
		return nil, ErrMatrixShape
	}
	out := make([]float64, aRows)
	for i := 0; i < aRows; i++ {
		var s float64
		row := a[i*aCols : (i+1)*aCols]
		for j, v := range row {
			s += v * x[j]
		}
		out[i] = s
	}
	return out, nil
}

func matrixDet(rows, cols int, a []float64) (float64, error) {
	if err := matrixCheck(rows, cols, a); err != nil {
		return 0, err
	}
	if rows != cols {
		return 0, ErrMatrixShape
	}

	// LU decomposition with partial pivoting, in-place on a copy.
	n := rows
	m := make([]float64, len(a))
	copy(m, a)
	sign := 1.0

	for k := 0; k < n; k++ {
		pivot := k
		maxAbs := math.Abs(m[k*n+k])
		for i := k + 1; i < n; i++ {
			v := math.Abs(m[i*n+k])
			if v > maxAbs {
				maxAbs = v
				pivot = i
			}
		}
		if maxAbs == 0 {
			return 0, nil
		}
		if pivot != k {
			for j := 0; j < n; j++ {
				m[k*n+j], m[pivot*n+j] = m[pivot*n+j], m[k*n+j]
			}
			sign = -sign
		}
		p := m[k*n+k]
		for i := k + 1; i < n; i++ {
			f := m[i*n+k] / p
			m[i*n+k] = f
			for j := k + 1; j < n; j++ {
				m[i*n+j] -= f * m[k*n+j]
			}
		}
	}

	det := sign
	for i := 0; i < n; i++ {
		det *= m[i*n+i]
	}
	return det, nil
}

func matrixInv(rows, cols int, a []float64) ([]float64, error) {
	if err := matrixCheck(rows, cols, a); err != nil {
		return nil, err
	}
	if rows != cols {
		return nil, ErrMatrixShape
	}
	n := rows

	// Augment [A | I] and Gauss-Jordan.
	aug := make([]float64, n*(2*n))
	for r := 0; r < n; r++ {
		copy(aug[r*(2*n):r*(2*n)+n], a[r*n:(r+1)*n])
		aug[r*(2*n)+n+r] = 1
	}

	for col := 0; col < n; col++ {
		pivot := col
		maxAbs := math.Abs(aug[col*(2*n)+col])
		for r := col + 1; r < n; r++ {
			v := math.Abs(aug[r*(2*n)+col])
			if v > maxAbs {
				maxAbs = v
				pivot = r
			}
		}
		if maxAbs == 0 {
			return nil, errors.New("singular matrix")
		}
		if pivot != col {
			for j := 0; j < 2*n; j++ {
				aug[col*(2*n)+j], aug[pivot*(2*n)+j] = aug[pivot*(2*n)+j], aug[col*(2*n)+j]
			}
		}

		p := aug[col*(2*n)+col]
		invP := 1 / p
		for j := 0; j < 2*n; j++ {
			aug[col*(2*n)+j] *= invP
		}
		for r := 0; r < n; r++ {
			if r == col {
				continue
			}
			f := aug[r*(2*n)+col]
			if f == 0 {
				continue
			}
			for j := 0; j < 2*n; j++ {
				aug[r*(2*n)+j] -= f * aug[col*(2*n)+j]
			}
		}
	}

	out := make([]float64, n*n)
	for r := 0; r < n; r++ {
		copy(out[r*n:(r+1)*n], aug[r*(2*n)+n:r*(2*n)+2*n])
	}
	return out, nil
}
