package generate

import (
	"container/heap"
	"fmt"
	"math"
	"sort"
)

// townBlocked is the blockedTiles footprint of /MapObjects/RandomTown.xdb at 0 turns, relative to its
// position.
var townBlocked = [][2]int{
	{6, -1}, {6, 0}, {6, 1}, {5, -3}, {-3, -2}, {-2, -2}, {-1, -2}, {0, -2}, {1, -2}, {2, -2}, {3, -2}, {4, -2},
	{5, -2}, {-4, -1}, {-3, -1}, {-2, -1}, {-1, -1}, {0, -1}, {1, -1}, {2, -1}, {3, -1}, {4, -1}, {5, -1}, {-4, 0},
	{-3, 0}, {-2, 0}, {-1, 0}, {0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}, {5, 0}, {-4, 1}, {-3, 1}, {-2, 1}, {-1, 1},
	{0, 1}, {1, 1}, {2, 1}, {3, 1}, {4, 1}, {5, 1}, {-3, 2}, {-2, 2}, {-1, 2}, {0, 2}, {1, 2}, {2, 2}, {3, 2}, {4, 2},
	{5, 2}, {-3, 3}, {-2, 3}, {-1, 3}, {0, 3}, {1, 3}, {2, 3}, {3, 3}, {4, 3}, {5, 3}, {-3, -3}, {-2, -3}, {-1, -3},
	{0, -3}, {1, -3}, {2, -3}, {3, -3}, {4, -3}, {-2, -4}, {-1, -4}, {0, -4}, {1, -4}, {2, -4}, {3, -4}, {0, -5},
	{2, -5}, {-2, 4}, {-1, 4}, {0, 4}, {1, 4}, {2, 4}, {3, 4}, {0, 5}, {1, 5}, {2, 5}, {4, 4}, {4, -4},
}

// turn rotates a footprint offset counter-clockwise (y up) by quarter turns.
func turn(d [2]int, turns int) [2]int {
	for q := 0; q < turns&3; q++ {
		d = [2]int{-d[1], d[0]}
	}
	return d
}

// Entrance is the visit tile of a town.
func (t Town) Entrance() [2]int {
	d := turn(TownEntrance, t.Turns)
	return [2]int{t.X + d[0], t.Y + d[1]}
}

// Blocked lists the tiles a town blocks.
func (t Town) Blocked() [][2]int {
	out := make([][2]int, 0, len(townBlocked))
	for _, b := range townBlocked {
		d := turn(b, t.Turns)
		out = append(out, [2]int{t.X + d[0], t.Y + d[1]})
	}
	return out
}

// checkTowns reports towns whose footprints overlap, or whose entrance (or the tile in front of it) is
// blocked by another town.
func checkTowns(size int, towns []Town) error {
	owner := map[[2]int]int{}
	for _, t := range towns {
		for _, b := range t.Blocked() {
			if o, ok := owner[b]; ok {
				return fmt.Errorf("the towns of players %d and %d overlap: move them apart", o, t.Player)
			}
			owner[b] = t.Player
		}
	}
	for _, t := range towns {
		e := t.Entrance()
		out := turn([2]int{0, -1}, t.Turns) // the way out, in front of the entrance
		for _, tile := range [][2]int{e, {e[0] + out[0], e[1] + out[1]}} {
			if o, ok := owner[tile]; ok {
				return fmt.Errorf("the town of player %d blocks the entrance of player %d's town: move them apart", o, t.Player)
			}
		}
	}
	return nil
}

// checkTownGround reports a town that does not stand on walkable ground of one level: its footprint, its
// entrance and the tile in front of it.
func checkTownGround(size int, towns []Town, walk []bool, lvl []int) error {
	n := size + 1
	for _, t := range towns {
		e := t.Entrance()
		out := turn([2]int{0, -1}, t.Turns)
		tiles := append(t.Blocked(), e, [2]int{e[0] + out[0], e[1] + out[1]})
		for _, b := range tiles {
			if b[0] < 0 || b[1] < 0 || b[0] >= size || b[1] >= size {
				return fmt.Errorf("the town of player %d is too close to the map edge", t.Player)
			}
			if !walk[b[1]*size+b[0]] || lvl[b[1]*n+b[0]] != lvl[t.Y*n+t.X] {
				return fmt.Errorf("the town of player %d stands on a cliff edge or in water: move it", t.Player)
			}
		}
	}
	return nil
}

// Road costs: a tile step costs its length times (base + noise) plus the slope; tiles already on a road
// are cheaper, so later roads join earlier ones.
const (
	roadNoiseScale = 9.0  // tiles; size of the wiggles
	roadNoise      = 0.9  // weight of the noise in the cost
	roadSlope      = 25.0 // cost per unit of height change
	roadReuse      = 0.35 // cost factor on existing road tiles
)

// AutoLinks joins the towns with a minimum spanning tree (Prim), by straight distance between entrances.
// Links are pairs of player numbers.
func AutoLinks(towns []Town) [][2]int {
	if len(towns) < 2 {
		return nil
	}
	in := map[int]bool{0: true}
	var links [][2]int
	for len(in) < len(towns) {
		best, bi, bj := math.Inf(1), -1, -1
		for i := range in {
			for j := range towns {
				if d := entranceDistance(towns[i], towns[j]); !in[j] && d < best {
					best, bi, bj = d, i, j
				}
			}
		}
		in[bj] = true
		links = append(links, [2]int{towns[bi].Player, towns[bj].Player})
	}
	return links
}

func entranceDistance(a, b Town) float64 {
	ea, eb := a.Entrance(), b.Entrance()
	return math.Hypot(float64(ea[0]-eb[0]), float64(ea[1]-eb[1]))
}

// cleanLinks drops duplicate and self links and orders each pair.
func cleanLinks(links [][2]int) [][2]int {
	seen := map[[2]int]bool{}
	var out [][2]int
	for _, k := range links {
		if k[0] > k[1] {
			k[0], k[1] = k[1], k[0]
		}
		if k[0] != k[1] && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

type node struct {
	tile int
	f    float64
}
type queue []node

func (q queue) Len() int            { return len(q) }
func (q queue) Less(i, j int) bool  { return q[i].f < q[j].f }
func (q queue) Swap(i, j int)       { q[i], q[j] = q[j], q[i] }
func (q *queue) Push(x interface{}) { *q = append(*q, x.(node)) }
func (q *queue) Pop() interface{} {
	old := *q
	n := old[len(old)-1]
	*q = old[:len(old)-1]
	return n
}

// routeRoads returns the road tiles (size x size, row = y) of the links (pairs of player numbers), shortest
// first so that longer roads can join them. height is the vertex height grid ((size + 1)^2), walk the tiles
// a road may cross.
func routeRoads(size int, towns []Town, links [][2]int, height []float32, walk []bool, field noise) ([]bool, error) {
	road := make([]bool, size*size)
	blocked := make([]bool, size*size)
	for i, w := range walk {
		blocked[i] = !w
	}
	for _, t := range towns {
		for _, b := range t.Blocked() {
			if b[0] >= 0 && b[1] >= 0 && b[0] < size && b[1] < size {
				blocked[b[1]*size+b[0]] = true
			}
		}
	}
	n := size + 1
	tileHeight := func(x, y int) float64 {
		return float64(height[y*n+x]+height[y*n+x+1]+height[(y+1)*n+x]+height[(y+1)*n+x+1]) / 4
	}
	base := make([]float64, size*size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			base[y*size+x] = 1 - roadNoise/2 + roadNoise*field.fbm(float64(x)/roadNoiseScale, float64(y)/roadNoiseScale, 2)
		}
	}
	steps := [8][3]int{{1, 0, 10}, {-1, 0, 10}, {0, 1, 10}, {0, -1, 10}, {1, 1, 14}, {1, -1, 14}, {-1, 1, 14}, {-1, -1, 14}}
	inside := func(x, y int) bool { return x >= 1 && y >= 1 && x < size-1 && y < size-1 } // keep off the map border

	byPlayer := map[int]Town{}
	for _, t := range towns {
		byPlayer[t.Player] = t
	}
	links = append([][2]int(nil), links...)
	length := func(k [2]int) float64 { return entranceDistance(byPlayer[k[0]], byPlayer[k[1]]) }
	sort.SliceStable(links, func(a, b int) bool { return length(links[a]) < length(links[b]) })
	for _, e := range links {
		from, to := byPlayer[e[0]].Entrance(), byPlayer[e[1]].Entrance()
		if !inside(from[0], from[1]) || !inside(to[0], to[1]) {
			continue
		}
		start, goal := from[1]*size+from[0], to[1]*size+to[0]
		cost := map[int]float64{start: 0}
		came := map[int]int{}
		q := &queue{{start, 0}}
		for q.Len() > 0 {
			cur := heap.Pop(q).(node)
			if cur.tile == goal {
				break
			}
			cx, cy := cur.tile%size, cur.tile/size
			for _, s := range steps {
				x, y := cx+s[0], cy+s[1]
				next := y*size + x
				if !inside(x, y) || (blocked[next] && next != goal) {
					continue
				}
				// diagonal steps need a free orthogonal neighbour, filled later to keep the road 4-connected
				if s[0] != 0 && s[1] != 0 && blocked[cy*size+x] && blocked[y*size+cx] {
					continue
				}
				c := base[next]
				if road[next] {
					c *= roadReuse
				}
				step := float64(s[2])/10*c + roadSlope*math.Abs(tileHeight(x, y)-tileHeight(cx, cy))
				g := cost[cur.tile] + step
				if old, ok := cost[next]; ok && old <= g {
					continue
				}
				cost[next], came[next] = g, cur.tile
				gx, gy := float64(to[0]-x), float64(to[1]-y)
				heap.Push(q, node{next, g + math.Hypot(gx, gy)*(1-roadNoise/2)*roadReuse})
			}
		}
		if _, ok := cost[goal]; !ok {
			return nil, fmt.Errorf("no road can join players %d and %d: water or cliffs are in the way", e[0], e[1])
		}
		for t := goal; ; t = came[t] {
			road[t] = true
			if t == start {
				break
			}
			p := came[t]
			px, py, tx, ty := p%size, p/size, t%size, t/size
			if px != tx && py != ty { // fill the corner of a diagonal step
				if c := py*size + tx; !blocked[c] {
					road[c] = true
				} else {
					road[ty*size+px] = true
				}
			}
		}
	}
	return road, nil
}
