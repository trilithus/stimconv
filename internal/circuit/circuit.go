// Package circuit models the original stimulation hardware so the analysis
// can work on estimated electrode current instead of raw audio voltage:
//
//	audio -> class-D amp (V = AmpVPeak * x) -> SeriesR + WindingR -> transformer
//	primary (magnetising inductance MagL) -> 1:Turns -> skin
//	skin = SkinR + (SkinParR || SkinParC)
//
// Everything is referred to the primary side (R' = R/n^2, C' = C*n^2) and
// integrated with the trapezoidal rule, which is stable for any step size.
package circuit

import (
	"math"

	"github.com/trilithus/stimconv/internal/config"
)

// Model is a per-channel streaming filter from normalised audio sample to
// estimated electrode current in amperes.
type Model struct {
	vpk, n float64
	a, b   float64 // i_load = a*(v_s - v_c - r1*i_m) with a = 1/(r1+rs')
	r1     float64
	// discrete trapezoidal update s' = P s + Q (v_prev + v_now)
	p11, p12, p21, p22 float64
	q1, q2             float64
	im, vc             float64 // states: magnetising current, skin capacitor voltage (primary-referred)
	vPrev              float64
	rs                 float64 // skin series resistance, primary-referred
}

// New builds a model for the given sample rate.
func New(c config.Circuit, sampleRate int) *Model {
	n := c.Turns
	r1 := c.SeriesR + c.WindingR
	rs := c.SkinR / (n * n)
	rp := c.SkinParR / (n * n)
	cp := c.SkinParC * n * n
	lm := c.MagL
	a := 1 / (r1 + rs)
	// continuous-time: ds/dt = A s + B v_s, s = [i_m, v_c]
	A11 := -rs * a * r1 / lm
	A12 := (1 - rs*a) / lm
	B1 := rs * a / lm
	A21 := -a * r1 / cp
	A22 := (-a - 1/rp) / cp
	B2 := a / cp
	h := 1 / float64(sampleRate) / 2
	// M = I - h A ; N = I + h A
	m11, m12, m21, m22 := 1-h*A11, -h*A12, -h*A21, 1-h*A22
	det := m11*m22 - m12*m21
	i11, i12, i21, i22 := m22/det, -m12/det, -m21/det, m11/det
	n11, n12, n21, n22 := 1+h*A11, h*A12, h*A21, 1+h*A22
	return &Model{
		vpk: c.AmpVPeak, n: n, a: a, r1: r1, rs: rs,
		p11: i11*n11 + i12*n21, p12: i11*n12 + i12*n22,
		p21: i21*n11 + i22*n21, p22: i21*n12 + i22*n22,
		q1: (i11*B1 + i12*B2) * h, q2: (i21*B1 + i22*B2) * h,
	}
}

// Step feeds one audio sample (-1..1) and returns the electrode current (A).
func (m *Model) Step(x float64) float64 {
	v := x * m.vpk
	im := m.p11*m.im + m.p12*m.vc + m.q1*(m.vPrev+v)
	vc := m.p21*m.im + m.p22*m.vc + m.q2*(m.vPrev+v)
	m.im, m.vc, m.vPrev = im, vc, v
	iLoad := m.a * (v - vc - m.r1*im)
	return iLoad / m.n
}

// Gain returns the steady-state current amplitude per unit audio amplitude
// at frequency f (A), computed analytically in the frequency domain.
func Gain(c config.Circuit, f float64) float64 {
	w := 2 * math.Pi * f
	n := c.Turns
	r1 := complex(c.SeriesR+c.WindingR, 0)
	zl := complex(c.SkinR/(n*n), 0) + parallel(complex(c.SkinParR/(n*n), 0), 1/complex(0, w*c.SkinParC*n*n))
	zm := complex(0, w*c.MagL)
	zp := parallel(zl, zm)
	vp := complex(c.AmpVPeak, 0) * zp / (r1 + zp)
	iLoad := vp / zl
	return abs(iLoad) / n
}

func parallel(a, b complex128) complex128 { return a * b / (a + b) }
func abs(z complex128) float64            { return math.Hypot(real(z), imag(z)) }
