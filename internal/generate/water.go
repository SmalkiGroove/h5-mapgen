package generate

import (
	"container/heap"
	"fmt"
	"math"
	"sort"

	"h5mapgen/internal/catalog"
	"h5mapgen/internal/layout"
	"h5mapgen/internal/terrain"
)

// Water travel. Checked on the vanilla maps: boats, their shipyard tiles and the objects of the sea stand on
// sea tiles (four corners in water). Between the sea and the ground the coast is a cliff, except on the
// ramps down into the water (coastRamps in levels.go): heroes board and land there, so a boat tile (of a
// shipyard or a town) is a berth, a sea tile beside a ramp tile.
const (
	boardCost     = 12  // a crossing by boat counts as its length plus this, in walking tiles
	seaNeeded     = 40  // tiles of a sea where a needed shipyard may put its boat
	seaWorth      = 400 // tiles of a sea worth a shipyard of the Water setting (not the ponds)
	seaWide       = 5   // and how far from the shore it reaches somewhere (not the narrow channels)
	shipReach     = 4   // the boat tile of a shipyard lies at most this far (Chebyshev) from its footprint
	townShipReach = 18  // a town's ship tile (for its own shipyard) at most this far from its position (vanilla: up to 21)
	shipyardTown  = 4.0 // a shipyard keeps this far from a town's footprint (other buildings: buildingClear)
	seaSpace      = 6   // tiles between two objects of the sea
	openSea       = 20  // tiles from the nearest land where the open sea starts (the user's rule): its objects stand there, those of the coasts nearer
	coastDensity  = 1.0 // the Water setting's rate times this along the coasts
	openDensity   = 0.6 // and times this in the open sea
)

// waterTiles marks the sea tiles and the berths, labels the seas (8-connected sea tiles) and measures how
// far each sea tile is from the land.
func (c *content) waterTiles(lvl []int) {
	n := c.size + 1
	c.sea, c.berth = make([]bool, c.size*c.size), make([]bool, c.size*c.size)
	for y := 0; y < c.size; y++ {
		for x := 0; x < c.size; x++ {
			water := 0
			for _, v := range [4]int{y*n + x, y*n + x + 1, (y+1)*n + x, (y+1)*n + x + 1} {
				if lvl[v] == terrain.LevelWater {
					water++
				}
			}
			c.sea[y*c.size+x] = water == 4
		}
	}
	for i, r := range c.rampTile {
		if !r {
			continue
		}
		for _, d := range neighbours {
			if x, y := i%c.size+d[0], i/c.size+d[1]; c.inside(x, y) && c.sea[y*c.size+x] {
				c.berth[y*c.size+x] = true
			}
		}
	}
	c.seaOf = components(c.sea, c.size)
	c.seaSize, c.seaWide, c.seaRiver = map[int]int{}, map[int]bool{}, map[int]bool{}
	land := make([]bool, len(c.sea))
	for i, s := range c.sea {
		land[i] = !s
	}
	depth := chebyshevDistance(land, c.size)
	c.coastDist = depth
	for i, k := range c.seaOf {
		if k >= 0 {
			c.seaSize[k]++
			c.seaWide[k] = c.seaWide[k] || depth[i] >= seaWide
			c.seaRiver[k] = c.seaRiver[k] || c.l.Relief[c.cellOf[(i/c.size)*n+i%c.size]] == layout.River
		}
	}
	c.navigable = map[int]bool{}
}

// worth tells whether sea k may take the boat of a shipyard asking for minSea tiles: the shipyards of the
// Water setting (seaWorth) also want a wide sea that is no river.
func (c *content) worth(k, minSea int) bool {
	return c.seaSize[k] >= minSea && (minSea < seaWorth || c.seaWide[k] && !c.seaRiver[k])
}

// components labels the 8-connected groups of marked tiles (-1 elsewhere).
func components(marked []bool, size int) []int {
	comp := make([]int, len(marked))
	for i := range comp {
		comp[i] = -1
	}
	next := 0
	for s, m := range marked {
		if !m || comp[s] >= 0 {
			continue
		}
		comp[s] = next
		queue := []int{s}
		for q := 0; q < len(queue); q++ {
			cx, cy := queue[q]%size, queue[q]/size
			for _, d := range neighbours {
				x, y := cx+d[0], cy+d[1]
				if x >= 0 && y >= 0 && x < size && y < size && marked[y*size+x] && comp[y*size+x] < 0 {
					comp[y*size+x] = next
					queue = append(queue, y*size+x)
				}
			}
		}
		next++
	}
	return comp
}

// neighbours are the 8 tiles around one.
var neighbours = [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}

// isWater tells whether a catalog object stands on the sea.
func isWater(key string) bool { return catalog.Get(key).Water }

// apart tells whether town k has no land way to some other town: it needs a shipyard.
func (c *content) apart(k int) bool {
	e := c.towns[k].Entrance()
	for j, t := range c.towns {
		f := t.Entrance()
		if j != k && c.land[e[1]*c.size+e[0]] != c.land[f[1]*c.size+f[0]] {
			return true
		}
	}
	return false
}

// boatTile returns where a shipyard at (x, y) turned by turns puts its boats: a free berth on a sea of
// minSea tiles or more, within shipReach of its footprint, the nearest to the front of its slipway (its
// local +x side), or -1.
func (c *content) boatTile(x, y, turns, minSea int) int {
	blocked, active := footprint(catalog.Get("shipyard"), x, y, turns)
	f := turn([2]int{3, 0}, turns)
	fx, fy := x+f[0], y+f[1]
	best, bestScore := -1, math.Inf(1)
	for _, t := range append(blocked, active...) {
		for dy := -shipReach; dy <= shipReach; dy++ {
			for dx := -shipReach; dx <= shipReach; dx++ {
				bx, by := t[0]+dx, t[1]+dy
				if !c.inside(bx, by) {
					continue
				}
				i := by*c.size + bx
				if !c.berth[i] || c.occ[i] != tFree || c.keep[i] || !c.worth(c.seaOf[i], minSea) {
					continue
				}
				if s := math.Hypot(float64(bx-fx), float64(by-fy)); s < bestScore {
					best, bestScore = i, s
				}
			}
		}
	}
	return best
}

// putShipyard places a shipyard with its boat tile: the boat tile stays free and its sea becomes
// navigable. It returns false when the shipyard has no boat tile or cuts an object off.
func (c *content) putShipyard(x, y, turns, minSea int) bool {
	boat := c.boatTile(x, y, turns, minSea)
	if boat < 0 || !c.putReachable("shipyard", x, y, turns) {
		return false
	}
	k := len(c.objects) - 1
	c.objects[k].Ship = &[2]int{boat%c.size - x, boat/c.size - y}
	c.keep[boat] = true
	c.navigable[c.seaOf[boat]] = true
	c.changes++ // the towns reach that sea now
	return true
}

// shipyardTurns returns the rotation of a shipyard at (x, y) whose slipway faces its boat tile best (or -1).
func (c *content) shipyardTurns(x, y, minSea int) int {
	best, bestScore := -1, math.Inf(1)
	for turns := 0; turns < 4; turns++ {
		if !c.fits("shipyard", x, y, turns, 1) {
			continue
		}
		boat := c.boatTile(x, y, turns, minSea)
		if boat < 0 {
			continue
		}
		f := turn([2]int{3, 0}, turns)
		if s := math.Hypot(float64(boat%c.size-x-f[0]), float64(boat/c.size-y-f[1])); s < bestScore {
			best, bestScore = turns, s
		}
	}
	return best
}

// shipyards gives a shipyard to every town that can't walk to all the others (always: islands need one),
// and with the Water setting to every town that has a large sea nearby.
func (c *content) shipyards() {
	water := layout.Amount(c.l.Water)
	for k := range c.towns {
		needed := c.apart(k)
		if !needed && water == 0 {
			continue
		}
		minSea, reach := seaWorth, 30
		if needed {
			minSea, reach = seaNeeded, 80
		}
		var tiles []int
		for i, d := range c.dist[k] {
			if d >= 4 && d <= reach && c.near[i] == k && c.walk[i] {
				tiles = append(tiles, i)
			}
		}
		c.rnd.Shuffle(len(tiles), func(a, b int) { tiles[a], tiles[b] = tiles[b], tiles[a] })
		sort.SliceStable(tiles, func(a, b int) bool { return c.dist[k][tiles[a]]/4 < c.dist[k][tiles[b]]/4 })
		placed := false
		for _, i := range tiles {
			x, y := i%c.size, i/c.size
			if turns := c.shipyardTurns(x, y, minSea); turns >= 0 && c.putShipyard(x, y, turns, minSea) {
				placed = true
				break
			}
		}
		if !placed && needed {
			c.warnings = append(c.warnings, fmt.Sprintf("player %d: no room for a shipyard (its town can't reach the others by land)", c.towns[k].Player))
		}
	}
}

// townShips gives each town with a berth nearby a ship tile, where the boats of its own shipyard appear.
func (c *content) townShips() {
	for k := range c.towns {
		t := &c.towns[k]
		best, bestDist := -1, math.Inf(1)
		for dy := -townShipReach; dy <= townShipReach; dy++ {
			for dx := -townShipReach; dx <= townShipReach; dx++ {
				x, y := t.X+dx, t.Y+dy
				if !c.inside(x, y) {
					continue
				}
				i := y*c.size + x
				if !c.berth[i] || c.occ[i] != tFree || c.keep[i] || c.seaSize[c.seaOf[i]] < seaNeeded {
					continue
				}
				if d := math.Hypot(float64(dx), float64(dy)); d < bestDist {
					best, bestDist = i, d
				}
			}
		}
		if best >= 0 {
			t.Ship = &[2]int{best%c.size - t.X, best/c.size - t.Y}
			c.keep[best] = true
		}
	}
}

// boatable tells whether a boat can sail on tile i: a free tile of a sea that has a shipyard.
func (c *content) boatable(i int) bool {
	return c.sea[i] && c.navigable[c.seaOf[i]] && c.occ[i] == tFree
}

// seaDistances extends the walking distances of each town over the seas that have a shipyard (taking a
// boat costs boardCost), to the tiles its heroes can't walk to. A tile reached on foot keeps its walking
// distance. It also marks the landings: ground tiles a hero reaches from a boat (the coast ramps).
func (c *content) seaDistances() {
	n := c.size * c.size
	c.landing = make([]bool, n)
	if len(c.navigable) == 0 {
		return
	}
	for i := 0; i < n; i++ {
		if !c.walk[i] || c.occ[i] != tFree {
			continue
		}
		for _, d := range neighbours {
			x, y := i%c.size+d[0], i/c.size+d[1]
			if x < 0 || y < 0 || x >= c.size || y >= c.size {
				continue
			}
			if c.boatable(y*c.size + x) {
				c.landing[i] = true
			}
		}
	}
	for k := range c.towns {
		d := c.dist[k]
		ext := make([]int, n)
		q := &queue{}
		for i := range ext {
			ext[i] = d[i]
			if d[i] < unreached {
				heap.Push(q, node{i, float64(d[i])})
			}
		}
		for q.Len() > 0 {
			cur := heap.Pop(q).(node)
			if int(cur.f) > ext[cur.tile] {
				continue
			}
			cx, cy := cur.tile%c.size, cur.tile/c.size
			for _, s := range neighbours {
				x, y := cx+s[0], cy+s[1]
				if x < 0 || y < 0 || x >= c.size || y >= c.size {
					continue
				}
				j := y*c.size + x
				if d[j] < unreached {
					continue // reached on foot
				}
				step := 1
				switch {
				case c.boatable(j):
					if !c.sea[cur.tile] {
						step += boardCost
					}
				case c.walk[j] && c.occ[j] == tFree:
				default:
					continue
				}
				if g := ext[cur.tile] + step; g < ext[j] {
					ext[j] = g
					heap.Push(q, node{j, float64(g)})
				}
			}
		}
		c.dist[k] = ext
	}
	for i := range c.near {
		c.near[i] = -1
		best := unreached
		for k := range c.towns {
			if c.dist[k][i] < best {
				best, c.near[i] = c.dist[k][i], k
			}
		}
	}
}

// coastKinds are the objects of the sea near the coasts, openKinds those of the open sea (the user's lists),
// with their weights.
var (
	coastKinds = map[string]int{"water_buoy": 15, "water_floatsam": 20, "water_peasant_wreck": 15, "water_sea_chest": 30,
		"water_wrecked_junk": 8, "water_wrecked_galley": 6, "water_wrecked_galleon": 6}
	openKinds = map[string]int{"water_mermaids": 25, "water_sirens": 20, "water_sunken_temple": 8, "water_aging_junk": 22,
		"water_weatherbeaten_galleon": 18}
)

// seaKind picks an object of the sea for a tile: of the coasts nearer than openSea tiles to the land, of the
// open sea farther.
func (c *content) seaKind(i int) string {
	if c.coastDist[i] < openSea {
		return c.pick(coastKinds)
	}
	return c.pick(openKinds)
}

// pick draws a key by weight.
func (c *content) pick(weights map[string]int) string {
	keys := make([]string, 0, len(weights))
	total := 0
	for k, w := range weights {
		keys = append(keys, k)
		total += w
	}
	sort.Strings(keys)
	r := c.rnd.Intn(total)
	for _, k := range keys {
		if r -= weights[k]; r < 0 {
			return k
		}
	}
	return keys[0]
}

// seaContent spreads the objects of the sea over the seas that have a shipyard, those of the coasts near
// the land and those of the open sea farther; none on the rivers. Whirlpools only come from the brush (the
// user's rule).
func (c *content) seaContent() {
	amount := layout.Amount(c.l.Water)
	rate := []float64{0, 1.0 / 420, 1.0 / 240, 1.0 / 140}[amount]
	if rate == 0 || len(c.navigable) == 0 {
		return
	}
	var tiles []int
	for i, k := range c.near {
		if k >= 0 && c.boatable(i) && !c.keep[i] && !c.seaRiver[c.seaOf[i]] {
			tiles = append(tiles, i)
		}
	}
	c.rnd.Shuffle(len(tiles), func(a, b int) { tiles[a], tiles[b] = tiles[b], tiles[a] })
	var placed [][2]int
	spaced := func(x, y int) bool {
		for _, p := range placed {
			if abs(p[0]-x) < seaSpace && abs(p[1]-y) < seaSpace {
				return false
			}
		}
		return true
	}
	// denser along the coasts than in the open sea
	var want [2]int // coasts, open sea
	for _, i := range tiles {
		if c.coastDist[i] < openSea {
			want[0]++
		} else {
			want[1]++
		}
	}
	want[0], want[1] = int(float64(want[0])*rate*coastDensity), int(float64(want[1])*rate*openDensity)
	for _, i := range tiles {
		x, y := i%c.size, i/c.size
		zone := 0
		if c.coastDist[i] >= openSea {
			zone = 1
		}
		if want[zone] == 0 || !spaced(x, y) {
			continue
		}
		key := c.seaKind(i)
		turns := c.rnd.Intn(4)
		if !c.fits(key, x, y, turns, 1) || !c.putReachable(key, x, y, turns) {
			continue
		}
		placed = append(placed, [2]int{x, y})
		want[zone]--
	}

}

// seaDecor puts rocks in the sea. Each stands in open sea with a free ring of sea around it, so rocks never
// touch one another or the shore and boats can always sail around them; none on a boat's way to a visit
// tile or a ship tile.
func (c *content) seaDecor(field noise) []Object {
	amount := layout.Amount(c.l.Obstacles)
	rocks := catalog.All.Decor["water"]
	if amount == 0 || len(rocks) == 0 {
		return nil
	}
	threshold := []float64{1, 0.66, 0.6, 0.54}[amount]
	scatter := []float64{0, 0.002, 0.004, 0.007}[amount]
	var large, small []catalog.Decor
	for _, d := range rocks {
		if len(d.Blocked) > 4 {
			large = append(large, d)
		} else if len(d.Blocked) > 0 {
			small = append(small, d)
		}
	}
	pick := func(list []catalog.Decor) *catalog.Decor {
		total := 0
		for _, d := range list {
			total += d.Uses
		}
		if total == 0 {
			return nil
		}
		r := c.rnd.Intn(total)
		for i := range list {
			if r -= list[i].Uses; r < 0 {
				return &list[i]
			}
		}
		return nil
	}
	open := func(x, y int) bool { // a sea tile with nothing on or next to it
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				u, v := x+dx, y+dy
				if !c.inside(u, v) {
					return false
				}
				j := v*c.size + u
				if !c.sea[j] || c.occ[j] != tFree || c.keep[j] {
					return false
				}
			}
		}
		for dy := -2; dy <= 2; dy++ { // and off the visit tiles of the sea
			for dx := -2; dx <= 2; dx++ {
				if u, v := x+dx, y+dy; c.inside(u, v) && c.occ[v*c.size+u] == tVisit {
					return false
				}
			}
		}
		return true
	}
	var out []Object
	for _, i := range c.rnd.Perm(c.size * c.size) {
		x, y := i%c.size, i/c.size
		if !c.sea[i] || !open(x, y) {
			continue
		}
		v := field.fbm(float64(x)/decorScale, float64(y)/decorScale, 3)
		r := c.rnd.Float64()
		var d *catalog.Decor
		switch {
		case v > threshold && r < 0.05:
			d = pick(large)
		case v > threshold && r < 0.12:
			d = pick(small)
		case r < scatter:
			d = pick(small)
		}
		if d == nil {
			continue
		}
		turns := c.rnd.Intn(4)
		var tiles [][2]int
		ok := true
		for _, b := range d.Blocked {
			t := turn(b, turns)
			if !open(x+t[0], y+t[1]) {
				ok = false
				break
			}
			tiles = append(tiles, [2]int{x + t[0], y + t[1]})
		}
		if !ok {
			continue
		}
		for _, t := range tiles {
			c.occ[t[1]*c.size+t[0]] = tBlocked
		}
		out = append(out, Object{Href: d.Href, X: float64(x), Y: float64(y), Turns: turns, Guards: -1})
	}
	return out
}

// landComponents labels the land masses: 8-connected walkable tiles.
func landComponents(walk []bool, size int) []int { return components(walk, size) }
