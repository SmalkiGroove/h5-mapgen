package generate

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"h5mapgen/internal/layout"
	"h5mapgen/internal/terrain"
)

// Plateau levels ([7], one byte per vertex) and ramps ([8]).
//
// The game's rules, checked on every vanilla map: two neighbouring vertices outside ramps are on the same
// level or one step apart (16); water is level 0 at height 0; the height of a vertex is its level / 8, plus
// what the height brush adds. A ramp is a block of vertices with [8] = 1, its middle row half a step
// between the two levels (the editor's ramp is 5 vertices wide and 3 long).
const (
	terrace     = 2    // vertices: the narrowest band of a level between a higher and a lower one
	townLevelR  = 10.0 // tiles around a town brought to the town's level
	rampHalf    = 2    // a ramp is 2 * rampHalf + 1 vertices wide
	rampChanges = 14   // most vertices a ramp may raise or lower to fit the cliff
	rampArea    = 1200 // vertices of plateau per ramp: larger plateaus get more ramps
	rampMax     = 4    // ramps per plateau
	rampSpacing = 24.0 // tiles between two ramps of a plateau
)

// reliefLevel is the level a relief asks for.
func reliefLevel(r string) int {
	switch r {
	case layout.Water:
		return terrain.LevelWater
	case layout.Plateau:
		return terrain.LevelGround + terrain.LevelStep
	case layout.High:
		return terrain.LevelGround + 2*terrain.LevelStep
	}
	return terrain.LevelGround
}

// levels returns the level and ramp flag of each vertex ((size + 1)^2, row = y).
func levels(l *layout.Layout, size int, cellOf []int, towns []Town, rnd *rand.Rand) (lvl []int, ramp []bool, err error) {
	n := size + 1
	lvl = make([]int, n*n)
	for i, c := range cellOf {
		lvl[i] = reliefLevel(l.Relief[c])
	}
	for pass := 0; pass < 2; pass++ {
		lvl = modeFilter(lvl, n)
	}

	// each town stands on a flat disc at the level of the cell it was placed on
	inTown := make([]bool, n*n)
	for _, t := range towns {
		s := l.Starts[t.Player-1]
		gx, gy := min(l.Grid-1, int(s.X)), min(l.Grid-1, int(s.Y))
		tl := reliefLevel(l.Relief[gy*l.Grid+gx])
		if tl == terrain.LevelWater {
			return nil, nil, fmt.Errorf("the town of player %d is in water: move it or paint land under it", t.Player)
		}
		cx, cy := float64(t.X)+0.5, float64(t.Y)+0.5
		r := int(townLevelR) + 1
		for y := max(0, t.Y-r); y <= min(n-1, t.Y+r); y++ {
			for x := max(0, t.X-r); x <= min(n-1, t.X+r); x++ {
				if math.Hypot(float64(x)-cx, float64(y)-cy) <= townLevelR {
					lvl[y*n+x] = tl
					inTown[y*n+x] = true
				}
			}
		}
	}

	// raise the ground around the towns by steps, so that capping the levels never lowers a town
	floor := make([]int, n*n)
	for i, in := range inTown {
		if in {
			floor[i] = lvl[i]
		}
	}
	for changed := true; changed; {
		changed = false
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				if inTown[y*n+x] {
					continue
				}
				high := floor[y*n+x]
				for yy := max(0, y-terrace); yy <= min(n-1, y+terrace); yy++ {
					for xx := max(0, x-terrace); xx <= min(n-1, x+terrace); xx++ {
						high = max(high, floor[yy*n+xx]-terrain.LevelStep)
					}
				}
				if high > floor[y*n+x] {
					floor[y*n+x] = high
					changed = true
				}
			}
		}
	}
	for i := range lvl {
		lvl[i] = max(lvl[i], floor[i])
	}

	capLevels(lvl, n, terrace)
	ramp = make([]bool, n*n)
	placeRamps(lvl, ramp, n, towns, inTown, rnd)
	return lvl, ramp, nil
}

// modeFilter replaces each level with the most common one of its 3 x 3 neighbourhood (the vertex's own
// level on a tie), which removes specks and one-vertex spikes.
func modeFilter(lvl []int, n int) []int {
	out := make([]int, len(lvl))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			var counts [256]int
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					xx, yy := min(n-1, max(0, x+dx)), min(n-1, max(0, y+dy))
					counts[lvl[yy*n+xx]]++
				}
			}
			best := lvl[y*n+x]
			for v := 0; v < len(counts); v += terrain.LevelStep {
				if counts[v] > counts[best] {
					best = v
				}
			}
			out[y*n+x] = best
		}
	}
	return out
}

// capLevels lowers vertices until none is more than one step above any vertex within r (Chebyshev): the
// cliffs are one step high, with bands at least r vertices wide between them.
func capLevels(lvl []int, n, r int) {
	for changed := true; changed; {
		changed = false
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				low := lvl[y*n+x]
				for yy := max(0, y-r); yy <= min(n-1, y+r); yy++ {
					for xx := max(0, x-r); xx <= min(n-1, x+r); xx++ {
						low = min(low, lvl[yy*n+xx])
					}
				}
				if lvl[y*n+x] > low+terrain.LevelStep {
					lvl[y*n+x] = low + terrain.LevelStep
					changed = true
				}
			}
		}
	}
}

// plateaus returns the components (4-connected) of vertices at or above level h.
func plateaus(lvl []int, n, h int) [][]int {
	seen := make([]bool, n*n)
	var out [][]int
	for i := range lvl {
		if seen[i] || lvl[i] < h {
			continue
		}
		comp := []int{i}
		seen[i] = true
		for k := 0; k < len(comp); k++ {
			x, y := comp[k]%n, comp[k]/n
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				xx, yy := x+d[0], y+d[1]
				if xx < 0 || yy < 0 || xx >= n || yy >= n {
					continue
				}
				if j := yy*n + xx; !seen[j] && lvl[j] >= h {
					seen[j] = true
					comp = append(comp, j)
				}
			}
		}
		out = append(out, comp)
	}
	return out
}

// rampSite is a ramp down from level h: rows k = 0..2 run from the top vertex (x, y) along (dx, dy), across
// columns -rampHalf..rampHalf.
type rampSite struct {
	x, y, dx, dy, h int
	changes         int
	score           float64
}

// stamp lists the vertices a ramp sets and their levels: two rows behind it at the top level, the ramp
// itself (top, half step, bottom; marked as ramp), and one row in front at the bottom level. The column on
// each side is the editor's (map dev2): top level beside the top row, bottom level beside the middle and
// bottom rows, so the cliff meets the ramp between its top and middle rows. A side vertex at the top level
// beside the middle row, with the bottom level after it, leaves a tile the game draws without texture.
func (s rampSite) stamp(fn func(x, y, level int, isRamp bool)) {
	px, py := -s.dy, s.dx
	for k := -2; k <= 3; k++ {
		level := s.h
		switch {
		case k == 1:
			level = s.h - terrain.LevelStep/2
		case k >= 2:
			level = s.h - terrain.LevelStep
		}
		for j := -rampHalf; j <= rampHalf; j++ {
			fn(s.x+k*s.dx+j*px, s.y+k*s.dy+j*py, level, k >= 0 && k <= 2)
		}
		if k >= 0 && k <= 2 {
			side := s.h
			if k >= 1 {
				side = s.h - terrain.LevelStep
			}
			for _, j := range []int{-rampHalf - 1, rampHalf + 1} {
				fn(s.x+k*s.dx+j*px, s.y+k*s.dy+j*py, side, false)
			}
		}
	}
}

// fit counts the vertices a ramp changes, or returns -1 when it cannot go there: off the map, over a town
// or another ramp, or leaving a cliff higher than one step.
func (s rampSite) fit(lvl []int, ramp []bool, n int, inTown []bool) int {
	changes, ok := 0, true
	set := map[int]int{}
	s.stamp(func(x, y, level int, isRamp bool) {
		if x < 1 || y < 1 || x >= n-1 || y >= n-1 {
			ok = false
			return
		}
		i := y*n + x
		set[i] = level
		if ramp[i] || (inTown[i] && lvl[i] != level) {
			ok = false
		}
		if lvl[i] != level && level%terrain.LevelStep == 0 {
			changes++
		}
	})
	if !ok || changes > rampChanges {
		return -1
	}
	at := func(i int) int {
		if v, ok := set[i]; ok {
			return v
		}
		return lvl[i]
	}
	for i, v := range set {
		x, y := i%n, i/n
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				j := (y+dy)*n + x + dx
				if ramp[j] && !(dx == 0 && dy == 0) {
					return -1 // keep ramps apart
				}
				if d := at(j) - v; d > terrain.LevelStep || d < -terrain.LevelStep {
					return -1
				}
			}
		}
	}
	return changes
}

// placeRamps gives every plateau one ramp or more down to the level below it, the first one facing the
// nearest town off the plateau (or the map centre).
func placeRamps(lvl []int, ramp []bool, n int, towns []Town, inTown []bool, rnd *rand.Rand) {
	top := 0
	for _, v := range lvl {
		top = max(top, v)
	}
	for h := terrain.LevelGround + terrain.LevelStep; h <= top; h += terrain.LevelStep {
		for _, comp := range plateaus(lvl, n, h) {
			member := map[int]bool{}
			for _, i := range comp {
				member[i] = true
			}
			var targets [][2]float64
			for _, t := range towns {
				if !member[t.Y*n+t.X] {
					targets = append(targets, [2]float64{float64(t.X), float64(t.Y)})
				}
			}
			if len(targets) == 0 {
				targets = [][2]float64{{float64(n) / 2, float64(n) / 2}}
			}
			var sites []rampSite
			for _, i := range comp {
				if lvl[i] != h {
					continue
				}
				x, y := i%n, i/n
				for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
					// the cliff is right in front: the vertex 2 rows down is a step lower
					xx, yy := x+2*d[0], y+2*d[1]
					if xx < 0 || yy < 0 || xx >= n || yy >= n || lvl[yy*n+xx] != h-terrain.LevelStep {
						continue
					}
					s := rampSite{x: x, y: y, dx: d[0], dy: d[1], h: h}
					if s.changes = s.fit(lvl, ramp, n, inTown); s.changes < 0 {
						continue
					}
					near := math.Inf(1)
					for _, t := range targets {
						near = math.Min(near, math.Hypot(float64(xx)-t[0], float64(yy)-t[1]))
					}
					s.score = float64(s.changes)*3 + near/4 + rnd.Float64()
					sites = append(sites, s)
				}
			}
			sort.SliceStable(sites, func(a, b int) bool { return sites[a].score < sites[b].score })
			want := min(rampMax, 1+len(comp)/rampArea)
			var placed []rampSite
			for _, s := range sites {
				if len(placed) == want {
					break
				}
				far := true
				for _, p := range placed {
					far = far && math.Hypot(float64(s.x-p.x), float64(s.y-p.y)) >= rampSpacing
				}
				if !far || s.fit(lvl, ramp, n, inTown) < 0 { // an earlier ramp may have changed the ground
					continue
				}
				s.stamp(func(x, y, level int, isRamp bool) {
					lvl[y*n+x] = level
					ramp[y*n+x] = ramp[y*n+x] || isRamp
				})
				placed = append(placed, s)
			}
		}
	}
}

// walkableTiles marks the tiles (size x size, row = y) a hero can stand on: four corners on one level above
// water, or four corners on a ramp.
func walkableTiles(size int, lvl []int, ramp []bool) []bool {
	n := size + 1
	out := make([]bool, size*size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			a, b, c, d := y*n+x, y*n+x+1, (y+1)*n+x, (y+1)*n+x+1
			flat := lvl[a] == lvl[b] && lvl[a] == lvl[c] && lvl[a] == lvl[d] && lvl[a] > terrain.LevelWater
			out[y*size+x] = flat || (ramp[a] && ramp[b] && ramp[c] && ramp[d])
		}
	}
	return out
}
