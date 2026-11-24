package generate

import "math"

// noise is a seeded 2D value noise.
type noise uint64

func (n noise) hash(x, y int64) float64 {
	h := uint64(n) ^ uint64(x)*0x9E3779B97F4A7C15 ^ uint64(y)*0xC2B2AE3D27D4EB4F
	h ^= h >> 31
	h *= 0xBF58476D1CE4E5B9
	h ^= h >> 29
	h *= 0x94D049BB133111EB
	h ^= h >> 32
	return float64(h>>11) / float64(1<<53)
}

// value is smooth noise in 0..1 with features about 1 unit wide.
func (n noise) value(x, y float64) float64 {
	x0, y0 := math.Floor(x), math.Floor(y)
	fx, fy := x-x0, y-y0
	ix, iy := int64(x0), int64(y0)
	sx, sy := fx*fx*(3-2*fx), fy*fy*(3-2*fy)
	a := n.hash(ix, iy) + (n.hash(ix+1, iy)-n.hash(ix, iy))*sx
	b := n.hash(ix, iy+1) + (n.hash(ix+1, iy+1)-n.hash(ix, iy+1))*sx
	return a + (b-a)*sy
}

// fbm sums octaves of value noise, result in 0..1 (mostly 0.25..0.75).
func (n noise) fbm(x, y float64, octaves int) float64 {
	sum, amp, total := 0.0, 1.0, 0.0
	for o := 0; o < octaves; o++ {
		sum += amp * noise(uint64(n)+uint64(o)*7919).value(x, y)
		total += amp
		amp *= 0.5
		x, y = x*2.03, y*2.03
	}
	return sum / total
}

func smoothstep(a, b, x float64) float64 {
	t := math.Max(0, math.Min(1, (x-a)/(b-a)))
	return t * t * (3 - 2*t)
}

// blur smooths an n x n field with a box of radius r, passes times (3 passes is close to a gaussian).
func blur(field []float64, n, r, passes int) []float64 {
	if r <= 0 {
		return field
	}
	tmp := make([]float64, len(field))
	for p := 0; p < passes; p++ {
		for pass := 0; pass < 2; pass++ { // rows into tmp, then columns back into field
			src, dst := field, tmp
			if pass == 1 {
				src, dst = tmp, field
			}
			for a := 0; a < n; a++ {
				sum := 0.0
				at := func(b int) float64 {
					b = max(0, min(n-1, b))
					if pass == 0 {
						return src[a*n+b]
					}
					return src[b*n+a]
				}
				for b := -r; b <= r; b++ {
					sum += at(b)
				}
				for b := 0; b < n; b++ {
					if pass == 0 {
						dst[a*n+b] = sum / float64(2*r+1)
					} else {
						dst[b*n+a] = sum / float64(2*r+1)
					}
					sum += at(b+r+1) - at(b-r)
				}
			}
		}
	}
	return field
}
