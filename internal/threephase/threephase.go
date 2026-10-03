// Package threephase inverts restim's continuous three-phase (stereostim)
// signal model: given the covariance of the L/R channel currents, it returns
// the (alpha, beta) position that restim would need to produce a signal with
// the same shape.
//
// restim (stim_math/threephase.py) generates
//
//	[L, R] = T @ Q @ [cos wt, sin wt]
//	T = (potential_to_channel @ ab_transform)[:2,:2] / sqrt(3) = [[√3/2, -1/2], [√3/2, 1/2]]
//	Q = 0.5 * [[2-r+α, -β], [-β, 2-r-α]],  r = |(α,β)|
//
// The electrode wired to both channel commons corresponds to restim's
// neutral (N) electrode, which is exactly the "joined commons" wiring.
package threephase

import "math"

var (
	t11, t12 = math.Sqrt(3) / 2, -0.5
	t21, t22 = math.Sqrt(3) / 2, 0.5
)

// Forward returns the L/R covariance (per unit carrier power, averaged over
// a carrier cycle) produced by restim for position (alpha, beta).
func Forward(alpha, beta float64) (cLL, cLR, cRR float64) {
	r := math.Hypot(alpha, beta)
	if r > 1 {
		alpha, beta, r = alpha/r, beta/r, 1
	}
	q11, q12, q22 := (2-r+alpha)/2, -beta/2, (2-r-alpha)/2
	// S = T @ Q
	s11 := t11*q11 + t12*q12
	s12 := t11*q12 + t12*q22
	s21 := t21*q11 + t22*q12
	s22 := t21*q12 + t22*q22
	// cov = S S^T / 2 (mean of cos^2 = sin^2 = 1/2, cross term 0)
	cLL = (s11*s11 + s12*s12) / 2
	cLR = (s11*s21 + s12*s22) / 2
	cRR = (s21*s21 + s22*s22) / 2
	return
}

// Inverse returns (alpha, beta) whose Forward covariance is proportional to
// the given one. ok is false when the covariance carries no signal.
func Inverse(cLL, cLR, cRR float64) (alpha, beta float64, ok bool) {
	if cLL+cRR <= 0 {
		return 0, 0, false
	}
	// M = T^-1 C T^-T  (proportional to Q Q^T = Q^2 since Q is symmetric)
	det := t11*t22 - t12*t21
	i11, i12 := t22/det, -t12/det
	i21, i22 := -t21/det, t11/det
	// A = Tinv @ C
	a11 := i11*cLL + i12*cLR
	a12 := i11*cLR + i12*cRR
	a21 := i21*cLL + i22*cLR
	a22 := i21*cLR + i22*cRR
	// M = A @ Tinv^T
	m11 := a11*i11 + a12*i12
	m12 := a11*i21 + a12*i22
	m22 := a21*i21 + a22*i22
	q11, q12, q22 := sqrtm2(m11, m12, m22)
	tr := q11 + q22
	if tr <= 0 {
		return 0, 0, false
	}
	u := (q11 - q22) / tr
	v := -2 * q12 / tr
	rho := math.Hypot(u, v)
	r := 2 * rho / (1 + rho)
	alpha = u * (2 - r)
	beta = v * (2 - r)
	if rr := math.Hypot(alpha, beta); rr > 1 {
		alpha, beta = alpha/rr, beta/rr
	}
	return alpha, beta, true
}

// sqrtm2 returns the principal square root of the symmetric PSD matrix [[a,b],[b,c]].
func sqrtm2(a, b, c float64) (float64, float64, float64) {
	det := a*c - b*b
	if det < 0 {
		det = 0
	}
	s := math.Sqrt(det)
	t := math.Sqrt(math.Max(a+c+2*s, 0))
	if t == 0 {
		return 0, 0, 0
	}
	return (a + s) / t, b / t, (c + s) / t
}
