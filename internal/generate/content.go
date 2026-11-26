package generate

import (
	"fmt"
	"math"
	"math/rand"
	"slices"
	"sort"
	"strings"

	"h5mapgen/internal/catalog"
	"h5mapgen/internal/layout"
)

// Object is an object of the generated map: a catalog object (Key) or a decoration (Href).
type Object struct {
	Key    string  `json:"key"` // catalog key, "" for a decoration
	Href   string  `json:"-"`   // decoration shared href
	X      float64 `json:"x"`   // tile (fractional for some decorations)
	Y      float64 `json:"y"`
	Turns  int     `json:"turns"` // quarter turns counter-clockwise (objects and blocking decorations)
	Rot    float64 `json:"-"`     // radians, for decorations that block nothing
	Guards int     `json:"-"`     // index in Objects of the monster guarding it, -1 when unguarded
	Player int     `json:"-"`     // a random dwelling's player, whose town's faction it takes (0: random)
	Ship   *[2]int `json:"-"`     // a shipyard's boat tile, relative to its position
	Group  int     `json:"-"`     // whirlpools of a group lead to one another
}

// Content placement (distances in tiles, walking along walkable ground).
const (
	guardMargin   = 3    // guarded objects stay this far from roads and entrances (Chebyshev), so the guard's zone doesn't touch them
	treasureSpace = 4    // tiles between two treasures
	objectSpace   = 2    // free tiles between any two objects
	townClear     = 8.0  // tiles around a town centre kept free of blocking decorations
	buildingClear = 10.0 // no building within this distance of a town's footprint
	decorScale    = 9.0  // size of obstacle clusters, tiles
	userSearch    = 1.5  // a brush object may move this many cells to fit
	pocket        = 6    // tiles a building may cut off the towns besides its own footprint
	unreached     = 1 << 30
)

// mineBands are the walking distances from the owner's town of each mine: wood, ore and abandoned mines
// near, rare mines farther, gold the farthest (the vanilla RMG's bands, RMG/Params/Default.xdb 7..20,
// 15..40, 25..45, put rare and gold mines too close).
var mineBands = map[string][2]int{"near": {7, 20}, "mid": {25, 45}, "far": {35, 60}}

// tile states
const (
	tFree = iota
	tBlocked
	tVisit
)

type content struct {
	l           *layout.Layout
	size        int
	cellOf      []int // vertex -> layout cell
	walk        []bool
	occ         []byte
	keep        []bool    // must stay passable: roads, hidden routes between towns, entrances, paths to objects
	keepDist    []int     // Chebyshev distance to the nearest keep tile at the start (roads and entrances)
	roads       []bool    // painted road tiles (nil without roads)
	paths       []bool    // dirt paths from buildings to the roads
	townDist    []float64 // distance to the nearest tile of a town's footprint
	towns       []Town
	dist        [][]int // walking distance from each town's entrance
	near        []int   // index of the nearest town by walking, -1 when unreached
	objects     []Object
	footOf      map[int]bool // tiles of the footprint being checked (scratch)
	seen        []bool       // tiles reached by putReachable (scratch)
	changes     int          // count of changes to the occupied tiles and the navigable seas
	reachBase   int          // tiles the towns reach, as of baseChanges
	baseChanges int
	depth       map[string][]int // per biome: Chebyshev distance of each tile to a tile of another biome
	land        []int            // land mass of each walkable tile (-1 elsewhere)
	sea         []bool           // tiles with four corners in water (water.go)
	rampTile    []bool           // walkable tiles with a ramp corner: kept free (heroes cross cliffs and board there)
	berth       []bool           // sea tiles beside a ramp tile: boats wait there
	coastDist   []int            // Chebyshev distance of each tile to a tile that is not sea
	seaOf       []int            // sea of each sea tile (-1 elsewhere)
	seaSize     map[int]int      // tiles of each sea
	seaWide     map[int]bool     // seas reaching seaWide tiles from the shore somewhere
	seaRiver    map[int]bool     // seas painted as rivers, at least in part: no boats of the Water setting
	navigable   map[int]bool     // seas with a shipyard
	landing     []bool           // ground tiles a hero reaches from a boat
	rnd         *rand.Rand
	warnings    []string
}

func (c *content) inside(x, y int) bool { return x >= 1 && y >= 1 && x < c.size-1 && y < c.size-1 }

// footprint returns the blocked and visit tiles of a catalog object at (x, y) turned by turns.
func footprint(o catalog.Object, x, y, turns int) (blocked, active [][2]int) {
	for _, b := range o.Blocked {
		d := turn(b, turns)
		blocked = append(blocked, [2]int{x + d[0], y + d[1]})
	}
	for _, a := range o.Active {
		d := turn(a, turns)
		active = append(active, [2]int{x + d[0], y + d[1]})
	}
	return
}

// building tells whether an object is a building: it stays away from the towns and from other buildings.
func building(key string) bool {
	for _, prefix := range []string{"mine_", "stat_", "building_", "dwelling_", "bank_", "shipyard"} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// pathed lists the buildings other than mines that get a dirt path to the roads (the user's choice).
var pathed = map[string]bool{
	"stat_war_academy": true, "stat_arena": true, "stat_school_of_magic": true, "stat_library": true,
	"building_trading_post": true, "building_tavern": true, "building_black_market": true, "shipyard": true,
}

// important tells whether a building gets a dirt path to the roads: mines, dwellings, banks and the pathed
// buildings.
func important(key string) bool {
	return strings.HasPrefix(key, "mine_") || strings.HasPrefix(key, "dwelling_") || strings.HasPrefix(key, "bank_") || pathed[key]
}

// fits tells whether a catalog object can stand at (x, y): every tile inside the map, walkable, free, off
// the kept tiles and at least margin from roads, buildings away from the towns, objects objectSpace apart,
// and a free approach tile beside each visit tile.
func (c *content) fits(key string, x, y, turns, margin int) bool {
	blocked, active := footprint(catalog.Get(key), x, y, turns)
	clear(c.footOf)
	isBuilding, water := building(key), isWater(key)
	clearance := buildingClear
	if key == "shipyard" {
		clearance = shipyardTown // it serves the town
	}
	for _, t := range append(blocked, active...) {
		if !c.inside(t[0], t[1]) {
			return false
		}
		i := t[1]*c.size + t[0]
		if water && !c.sea[i] || !water && !c.walk[i] || c.occ[i] != tFree || c.keep[i] || c.keepDist[i] < margin {
			return false
		}
		if isBuilding && c.townDist[i] <= clearance {
			return false
		}
		c.footOf[i] = true
	}
	for _, t := range append(blocked, active...) {
		for dy := -objectSpace; dy <= objectSpace; dy++ {
			for dx := -objectSpace; dx <= objectSpace; dx++ {
				x, y := t[0]+dx, t[1]+dy
				if x >= 0 && y >= 0 && x < c.size && y < c.size && !c.footOf[y*c.size+x] && c.occ[y*c.size+x] != tFree && c.occ[y*c.size+x] != tBlocked {
					return false // too close to another object's visit tile
				}
			}
		}
	}
	for _, a := range active {
		if c.approach(key, a, nil) < 0 {
			return false
		}
	}
	return true
}

// sides are the 4 orthogonal neighbours.
var sides = [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

// approach returns a free walkable tile beside a visit tile (orthogonal, so that a guard there stands in
// front of it; outside the footprint being checked), the one with the lowest score when score is given,
// or -1.
func (c *content) approach(key string, a [2]int, score func(i int) int) int {
	water := isWater(key) // visited from a boat
	if cornerEntry(key) {
		return c.approachFrom(a, score, corners[:], water) // walled in on its sides by its own model (Tomb of the Warrior)
	}
	return c.approachFrom(a, score, sides[:], water)
}

// cornerEntry tells whether an object's first visit tile has its 4 sides blocked by the object itself, so
// that it is entered from a corner.
func cornerEntry(key string) bool {
	o := catalog.Get(key)
	if len(o.Active) == 0 {
		return false
	}
	a := o.Active[0]
	for _, d := range sides {
		if !slices.Contains(o.Blocked, [2]int{a[0] + d[0], a[1] + d[1]}) {
			return false
		}
	}
	return true
}

// corners are the 4 diagonal neighbours.
var corners = [4][2]int{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}}

func (c *content) approachFrom(a [2]int, score func(i int) int, dirs [][2]int, water bool) int {
	best, bestScore := -1, unreached+1
	for _, d := range dirs {
		x, y := a[0]+d[0], a[1]+d[1]
		if !c.inside(x, y) {
			continue
		}
		i := y*c.size + x
		if water && !c.sea[i] || !water && !c.walk[i] || c.occ[i] != tFree || c.footOf[i] {
			continue
		}
		s := 0
		if score != nil {
			s = score(i)
		}
		if s < bestScore {
			best, bestScore = i, s
		}
	}
	return best
}

// put places a catalog object and returns its index.
func (c *content) put(key string, x, y, turns int) int {
	o := catalog.Get(key)
	blocked, active := footprint(o, x, y, turns)
	for _, t := range blocked {
		c.occ[t[1]*c.size+t[0]] = tBlocked
	}
	for _, t := range active {
		c.occ[t[1]*c.size+t[0]] = tVisit
	}
	c.objects = append(c.objects, Object{Key: key, X: float64(x), Y: float64(y), Turns: turns, Guards: -1})
	c.changes++
	return len(c.objects) - 1
}

// reach marks in seen the tiles the towns reach, and counts them. Heroes walk, and sail the seas that have a
// shipyard.
func (c *content) reach() int {
	n := c.size * c.size
	if len(c.seen) != n {
		c.seen = make([]bool, n)
	}
	clear(c.seen)
	var queue []int
	for _, t := range c.towns {
		e := t.Entrance()
		i := e[1]*c.size + e[0]
		c.seen[i] = true
		queue = append(queue, i)
	}
	for q := 0; q < len(queue); q++ {
		cx, cy := queue[q]%c.size, queue[q]/c.size
		for _, d := range neighbours {
			x, y := cx+d[0], cy+d[1]
			if x < 0 || y < 0 || x >= c.size || y >= c.size {
				continue
			}
			if i := y*c.size + x; !c.seen[i] && (c.walk[i] && c.occ[i] == tFree || c.boatable(i)) {
				c.seen[i] = true
				queue = append(queue, i)
			}
		}
	}
	return len(queue)
}

// putReachable places a building unless it cuts an object placed before, or more ground than a small
// pocket, off the towns (it then takes it back and returns false). The objects of the other kinds are small
// enough not to.
func (c *content) putReachable(key string, x, y, turns int) bool {
	if c.baseChanges != c.changes || c.reachBase == 0 {
		c.reachBase, c.baseChanges = c.reach(), c.changes
	}
	k := c.put(key, x, y, turns)
	blocked, active := footprint(catalog.Get(key), x, y, turns)
	count := c.reach()
	undo := func() bool {
		c.remove(k)
		c.baseChanges = c.changes // the tiles are as before
		return false
	}
	if c.reachBase-count > len(blocked)+len(active)+pocket {
		return undo()
	}
	for _, o := range c.objects {
		if o.Key == "" {
			continue
		}
		_, active := footprint(catalog.Get(o.Key), int(o.X), int(o.Y), o.Turns)
		for _, a := range active {
			ok := false
			for _, d := range append(sides[:], corners[:]...) { // heroes step diagonally too
				if x, y := a[0]+d[0], a[1]+d[1]; x >= 0 && y >= 0 && x < c.size && y < c.size && c.seen[y*c.size+x] {
					ok = true
				}
			}
			if !ok {
				return undo()
			}
		}
	}
	c.reachBase, c.baseChanges = count, c.changes
	return true
}

// remove frees the tiles of an object (it stays in the list, with an empty key, until compact).
func (c *content) remove(k int) {
	o := c.objects[k]
	blocked, active := footprint(catalog.Get(o.Key), int(o.X), int(o.Y), o.Turns)
	for _, t := range append(blocked, active...) {
		c.occ[t[1]*c.size+t[0]] = tFree
	}
	c.objects[k].Key = ""
	c.changes++
}

// bestTurns returns the rotation that fits with its visit tile closest to a town (or -1).
func (c *content) bestTurns(key string, x, y, margin int) int {
	o := catalog.Get(key)
	best, bestDist := -1, unreached+1
	for turns := 0; turns < 4; turns++ {
		if !c.fits(key, x, y, turns, margin) {
			continue
		}
		d := unreached
		if _, active := footprint(o, x, y, turns); len(active) > 0 {
			a := active[0]
			if k := c.near[a[1]*c.size+a[0]]; k >= 0 {
				d = c.dist[k][a[1]*c.size+a[0]]
			}
		}
		if d < bestDist {
			best, bestDist = turns, d
		}
	}
	return best
}

// walkDistances fills dist (from each town's entrance, over walkable unblocked tiles) and near.
func (c *content) walkDistances() {
	n := c.size * c.size
	c.dist = make([][]int, len(c.towns))
	c.near = make([]int, n)
	for k, t := range c.towns {
		d := make([]int, n)
		for i := range d {
			d[i] = unreached
		}
		e := t.Entrance()
		start := e[1]*c.size + e[0]
		d[start] = 0
		queue := []int{start}
		for q := 0; q < len(queue); q++ {
			cur := queue[q]
			cx, cy := cur%c.size, cur/c.size
			for _, s := range [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
				x, y := cx+s[0], cy+s[1]
				if x < 0 || y < 0 || x >= c.size || y >= c.size {
					continue
				}
				i := y*c.size + x
				if c.walk[i] && c.occ[i] == tFree && d[i] == unreached {
					d[i] = d[cur] + 1
					queue = append(queue, i)
				}
			}
		}
		c.dist[k] = d
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

// placeContent places the brush objects, the mines, the treasures and their guards, the paths from the
// buildings to the roads, then the decorations. roads are the painted road tiles (nil without roads), keep
// the tiles that must stay passable (roads and hidden routes between towns).
//
// lvl is the level of each vertex, for the sea. The towns get their ship tiles (Town.Ship) in place.
func placeContent(l *layout.Layout, size int, cellOf []int, towns []Town, walk, roads, keep []bool, lvl []int, ramp []bool, rnd *rand.Rand, field noise) (objects, decor []Object, paths []bool, warnings []string) {
	c := &content{l: l, size: size, cellOf: cellOf, walk: walk, occ: make([]byte, size*size), keep: append([]bool(nil), keep...),
		roads: roads, paths: make([]bool, size*size), towns: towns, footOf: map[int]bool{}, rnd: rnd}
	c.townDist = make([]float64, size*size)
	for i := range c.townDist {
		c.townDist[i] = math.Inf(1)
	}
	reach := int(buildingClear) + 1
	for _, t := range towns {
		for _, b := range t.Blocked() {
			for y := max(0, b[1]-reach); y < min(size, b[1]+reach+1); y++ {
				for x := max(0, b[0]-reach); x < min(size, b[0]+reach+1); x++ {
					c.townDist[y*size+x] = math.Min(c.townDist[y*size+x], math.Hypot(float64(x-b[0]), float64(y-b[1])))
				}
			}
		}
	}
	for _, t := range towns {
		for _, b := range t.Blocked() {
			if b[0] >= 0 && b[1] >= 0 && b[0] < size && b[1] < size {
				c.occ[b[1]*size+b[0]] = tBlocked
			}
		}
		e := t.Entrance()
		out := turn([2]int{0, -1}, t.Turns)
		for _, k := range [][2]int{e, {e[0] + out[0], e[1] + out[1]}} {
			c.keep[k[1]*size+k[0]] = true
		}
	}
	// ramps stay free: they are the only ways up the cliffs and onto the boats
	c.rampTile = make([]bool, size*size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			for _, v := range [4]int{y*(size+1) + x, y*(size+1) + x + 1, (y+1)*(size+1) + x, (y+1)*(size+1) + x + 1} {
				if ramp[v] && walk[y*size+x] {
					c.rampTile[y*size+x], c.keep[y*size+x] = true, true
				}
			}
		}
	}
	c.keepDist = chebyshevDistance(c.keep, size)
	c.walkDistances()
	c.land = landComponents(walk, size)
	c.waterTiles(lvl)

	guards := layout.Amount(l.Guards)
	c.userObjects()
	c.shipyards()
	c.townShips()
	c.seaDistances()
	c.mines()
	c.statBuildings()
	c.neutral([]buildingKind{buildingKinds["stat_library"]}, neutralCount(l.Stats))
	c.dwellings()
	c.neutral(neutralDwellings, neutralCount(l.Dwellings))
	c.neutral(vaults, neutralCount(l.Banks))
	c.buildings(banks, bankRate, l.Banks)
	c.buildings(visitBuildings, visitRate, l.Buildings)
	c.treasures()
	c.seaContent()
	c.connect()
	if guards > 0 {
		c.guard(guards)
	}
	c.buildingPaths()
	decor = append(c.decorate(field), c.seaDecor(field)...)

	// drop the removed objects; Guards indices follow
	index := make([]int, len(c.objects))
	for i, o := range c.objects {
		index[i] = -1
		if o.Key != "" {
			index[i] = len(objects)
			objects = append(objects, o)
		}
	}
	for i := range objects {
		if g := objects[i].Guards; g >= 0 {
			objects[i].Guards = index[g]
		}
	}
	return objects, decor, c.paths, c.warnings
}

// chebyshevDistance is the distance (8 neighbours) of every tile to the nearest marked tile.
func chebyshevDistance(marked []bool, size int) []int {
	d := make([]int, len(marked))
	var queue []int
	for i, m := range marked {
		if m {
			queue = append(queue, i)
		} else {
			d[i] = unreached
		}
	}
	for q := 0; q < len(queue); q++ {
		cur := queue[q]
		cx, cy := cur%size, cur/size
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				x, y := cx+dx, cy+dy
				if x >= 0 && y >= 0 && x < size && y < size && d[y*size+x] == unreached {
					d[y*size+x] = d[cur] + 1
					queue = append(queue, y*size+x)
				}
			}
		}
	}
	return d
}

// guardLevel is the monster level guarding an object, before the Guards setting (0: unguarded).
func (c *content) guardLevel(o Object) int {
	switch o.Key {
	case "mine_wood", "mine_ore":
		return 1
	case "mine_mercury", "mine_crystal", "mine_sulfur", "mine_gem":
		return 3
	case "mine_abandoned":
		return 4
	case "mine_gold":
		return 5
	case "artifact_minor":
		return 2
	case "artifact_major":
		return 4
	case "artifact_relic":
		return 6
	case "chest":
		return 2
	}
	return buildingKinds[o.Key].guard
}

// userObjects places the brush objects near where they were put.
func (c *content) userObjects() {
	cs := float64(c.size) / float64(c.l.Grid)
	for i, u := range c.l.Objects {
		tx, ty := int(u.X*cs), int(u.Y*cs)
		margin := guardMargin
		if c.guardLevel(Object{Key: u.Kind}) == 0 {
			margin = 1
		}
		placed := false
		for r := 0; r <= int(userSearch*cs) && !placed; r++ {
			for _, p := range ring(tx, ty, r) {
				if u.Kind == "water_whirlpool" && c.fits(u.Kind, p[0], p[1], 0, 1) { // the brush's whirlpools lead to one another
					c.put(u.Kind, p[0], p[1], 0)
					c.objects[len(c.objects)-1].Group = 1
					placed = true
					break
				}
				if u.Kind == "shipyard" {
					if turns := c.shipyardTurns(p[0], p[1], seaNeeded); turns >= 0 && c.putShipyard(p[0], p[1], turns, seaNeeded) {
						placed = true
						break
					}
					continue
				}
				if turns := c.bestTurns(u.Kind, p[0], p[1], margin); turns >= 0 {
					c.put(u.Kind, p[0], p[1], turns)
					if k := c.near[p[1]*c.size+p[0]]; k >= 0 && isRandomDwelling(u.Kind) {
						c.objects[len(c.objects)-1].Player = c.towns[k].Player // the faction of the nearest town
					}
					placed = true
					break
				}
			}
		}
		if !placed {
			c.warnings = append(c.warnings, fmt.Sprintf("object %d (%s) has no room where it was placed", i+1, u.Kind))
		}
	}
}

// ring lists the tiles at Chebyshev distance r from (x, y).
func ring(x, y, r int) [][2]int {
	if r == 0 {
		return [][2]int{{x, y}}
	}
	var out [][2]int
	for d := -r; d <= r; d++ {
		out = append(out, [2]int{x + d, y - r}, [2]int{x + d, y + r})
		if d > -r && d < r {
			out = append(out, [2]int{x - r, y + d}, [2]int{x + r, y + d})
		}
	}
	return out
}

// mines places each town's mines at walking distances from it, on tiles nearer to it than to other towns.
func (c *content) mines() {
	amount := layout.Amount(c.l.Mines)
	if amount == 0 {
		return
	}
	rare := []string{"mine_mercury", "mine_crystal", "mine_sulfur", "mine_gem"}
	for k := range c.towns {
		c.rnd.Shuffle(len(rare), func(i, j int) { rare[i], rare[j] = rare[j], rare[i] })
		type want struct{ key, band string }
		list := []want{{"mine_wood", "near"}, {"mine_ore", "near"}}
		switch amount {
		case 1:
			list = append(list, want{rare[0], "mid"})
		case 2:
			list = append(list, want{rare[0], "mid"}, want{rare[1], "mid"}, want{"mine_gold", "far"})
		case 3:
			list = append(list, want{"mine_wood", "near"}, want{"mine_ore", "near"})
			for _, r := range rare {
				list = append(list, want{r, "mid"})
			}
			list = append(list, want{"mine_gold", "far"}, want{"mine_abandoned", "near"})
		}
		for _, w := range list {
			if !c.placeNear(k, w.key, mineBands[w.band]) {
				c.warnings = append(c.warnings, fmt.Sprintf("player %d: no room for %s", c.towns[k].Player, w.key))
			}
		}
	}
}

// placeNear places an object on a tile of town k at a walking distance in band. When the band has no room
// it reaches farther first, and comes closer only as a last resort.
func (c *content) placeNear(k int, key string, band [2]int) bool {
	margin := guardMargin
	if c.guardLevel(Object{Key: key}) == 0 {
		margin = 1
	}
	for _, widen := range [][2]float64{{1, 1}, {1, 1.4}, {1, 2}, {0.7, 2}, {0.4, 3}} {
		lo, hi := int(float64(band[0])*widen[0]), int(float64(band[1])*widen[1])
		var tiles []int
		for i, d := range c.dist[k] {
			if d >= lo && d <= hi && c.near[i] == k && c.walk[i] {
				tiles = append(tiles, i)
			}
		}
		c.rnd.Shuffle(len(tiles), func(a, b int) { tiles[a], tiles[b] = tiles[b], tiles[a] })
		for _, i := range tiles {
			if turns := c.bestTurns(key, i%c.size, i/c.size, margin); turns >= 0 && c.putReachable(key, i%c.size, i/c.size, turns) {
				return true
			}
		}
	}
	return false
}

// treasures spreads resource piles, chests and artifacts: the farther from the towns, the richer.
func (c *content) treasures() {
	rate := []float64{0, 1.0 / 260, 1.0 / 140, 1.0 / 75}[layout.Amount(c.l.Treasures)]
	if rate == 0 {
		return
	}
	var tiles []int
	var far []int
	for i, k := range c.near {
		if k >= 0 && c.walk[i] && c.occ[i] == tFree {
			tiles = append(tiles, i)
			far = append(far, c.dist[k][i])
		}
	}
	if len(tiles) == 0 {
		return
	}
	sort.Ints(far)
	maxDist := float64(max(1, far[len(far)*95/100])) // 95th percentile of the distance to the nearest town
	want := int(float64(len(tiles)) * rate)
	c.rnd.Shuffle(len(tiles), func(a, b int) { tiles[a], tiles[b] = tiles[b], tiles[a] })
	var placed [][2]int
	for _, i := range tiles {
		if want == 0 {
			break
		}
		x, y := i%c.size, i/c.size
		if c.occ[i] != tFree {
			continue
		}
		close := false
		for _, p := range placed {
			close = close || (abs(p[0]-x) < treasureSpace && abs(p[1]-y) < treasureSpace)
		}
		if close {
			continue
		}
		f := float64(c.dist[c.near[i]][i]) / maxDist
		key := c.treasureKind(f)
		margin := 1
		if c.guardLevel(Object{Key: key}) > 0 {
			margin = guardMargin
		}
		if !c.fits(key, x, y, 0, margin) {
			continue
		}
		c.put(key, x, y, c.rnd.Intn(4))
		placed = append(placed, [2]int{x, y})
		want--
	}
}

// treasureKind picks a treasure for a tile at relative distance f from the towns (0 at a town, 1 far).
func (c *content) treasureKind(f float64) string {
	pick := func(weights map[string]int) string {
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
	switch {
	case f < 0.25:
		return pick(map[string]int{"res_wood": 25, "res_ore": 25, "res_mercury": 5, "res_crystal": 5, "res_sulfur": 5, "res_gem": 5, "res_gold": 15, "campfire": 15})
	case f < 0.6:
		return pick(map[string]int{"chest": 35, "res_mercury": 4, "res_crystal": 4, "res_sulfur": 4, "res_gem": 4, "res_gold": 15, "campfire": 10, "artifact_minor": 24})
	case f < 0.85:
		return pick(map[string]int{"chest": 35, "artifact_minor": 35, "artifact_major": 30})
	}
	return pick(map[string]int{"chest": 25, "artifact_minor": 25, "artifact_major": 35, "artifact_relic": 15})
}

// connect keeps a path from every object's visit tile to the kept tiles (roads, routes, entrances) or to a
// landing, and drops the objects that can't be reached. The objects of the sea are reached by boat.
func (c *content) connect() {
	n := c.size * c.size
	dist := make([]int, n)
	parent := make([]int, n)
	var queue []int
	for i := range dist {
		dist[i], parent[i] = unreached, -1
		if (c.keep[i] || c.landing[i]) && c.walk[i] && c.occ[i] == tFree {
			dist[i] = 0
			queue = append(queue, i)
		}
	}
	for q := 0; q < len(queue); q++ {
		cur := queue[q]
		cx, cy := cur%c.size, cur/c.size
		for _, s := range [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
			x, y := cx+s[0], cy+s[1]
			if x < 0 || y < 0 || x >= c.size || y >= c.size {
				continue
			}
			i := y*c.size + x
			if c.walk[i] && c.occ[i] == tFree && dist[i] == unreached {
				dist[i], parent[i] = dist[cur]+1, cur
				queue = append(queue, i)
			}
		}
	}
	clear(c.footOf)
	for k, o := range c.objects {
		if o.Key == "" || isWater(o.Key) {
			continue
		}
		_, active := footprint(catalog.Get(o.Key), int(o.X), int(o.Y), o.Turns)
		ok := true
		for _, a := range active {
			start := c.approach(o.Key, a, func(i int) int { return dist[i] })
			if start < 0 || dist[start] == unreached {
				ok = false
				break
			}
			for t := start; t >= 0; t = parent[t] {
				c.keep[t] = true
			}
		}
		if !ok {
			c.remove(k)
			c.warnings = append(c.warnings, fmt.Sprintf("%s at (%d, %d) can't be reached: left out", o.Key, int(o.X), int(o.Y)))
		}
	}
}

// guard puts a random monster next to the visit tile of each object worth guarding; amount is the Guards
// setting (1 low, 2 normal, 3 high).
func (c *content) guard(amount int) {
	clear(c.footOf)
	count := len(c.objects)
	for k := 0; k < count; k++ {
		o := c.objects[k]
		level := c.guardLevel(o)
		if o.Key == "" || level == 0 {
			continue
		}
		if o.Key == "chest" && c.rnd.Intn(2) == 0 {
			continue // half the chests lie in the open
		}
		level = max(1, min(7, level+amount-2))
		_, active := footprint(catalog.Get(o.Key), int(o.X), int(o.Y), o.Turns)
		if len(active) == 0 {
			continue
		}
		// on the kept path when possible, and with its zone off the roads and entrances
		g := c.approach(o.Key, active[0], func(i int) int {
			s := 0
			if !c.keep[i] {
				s += 10
			}
			if c.keepDist[i] < 2 {
				s += 100
			}
			return s
		})
		if g < 0 || c.keepDist[g] < 2 {
			continue
		}
		c.occ[g] = tVisit
		c.keep[g] = true
		c.changes++
		c.objects = append(c.objects, Object{Key: fmt.Sprintf("monster_%d", level), X: float64(g % c.size), Y: float64(g / c.size), Turns: c.rnd.Intn(4), Guards: -1})
		c.objects[k].Guards = len(c.objects) - 1
	}
}

// buildingPaths lays a dirt path from each building's visit tile, through its guard, to the nearest road
// or path already laid (4-connected, so that the painted tiles join).
func (c *content) buildingPaths() {
	if c.roads == nil {
		return
	}
	from := make([]int, c.size*c.size)
	for _, o := range c.objects {
		if o.Key == "" || !important(o.Key) {
			continue
		}
		_, active := footprint(catalog.Get(o.Key), int(o.X), int(o.Y), o.Turns)
		if len(active) == 0 {
			continue
		}
		a := active[0]
		start := -1
		if o.Guards >= 0 {
			g := c.objects[o.Guards]
			start = int(g.Y)*c.size + int(g.X)
		} else {
			clear(c.footOf)
			start = c.approach(o.Key, a, nil)
		}
		if start < 0 {
			continue
		}
		// breadth first from the start over free ground, until a road or a path; a diagonal step is filled
		// with a free side tile when there is one, so that the painted path stays joined
		for i := range from {
			from[i] = -2
		}
		from[start] = -1
		queue := []int{start}
		end := -1
		passable := func(i int) bool { return c.walk[i] && (c.occ[i] == tFree || c.roads[i]) }
		for q := 0; q < len(queue); q++ {
			cur := queue[q]
			if q > 0 && (c.roads[cur] || c.paths[cur]) {
				end = cur
				break
			}
			cx, cy := cur%c.size, cur/c.size
			for _, d := range [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
				x, y := cx+d[0], cy+d[1]
				if x < 0 || y < 0 || x >= c.size || y >= c.size {
					continue // the edge tiles are walkable (only objects keep off them)
				}
				if i := y*c.size + x; from[i] == -2 && passable(i) {
					from[i] = cur
					queue = append(queue, i)
				}
			}
		}
		if end < 0 {
			continue // rare: the object was joined to a hidden route only
		}
		mark := func(i int) { c.paths[i], c.keep[i] = true, true }
		c.paths[a[1]*c.size+a[0]] = true
		for prev, t := end, from[end]; t >= 0; prev, t = t, from[t] {
			mark(t)
			px, py, tx, ty := prev%c.size, prev/c.size, t%c.size, t/c.size
			if px != tx && py != ty {
				if i := ty*c.size + px; passable(i) && !c.roads[i] {
					mark(i)
				} else if i := py*c.size + tx; passable(i) && !c.roads[i] {
					mark(i)
				}
			}
		}
	}
}

// decorate fills the open ground with the decorations of each biome: clusters of large obstacles where a
// noise is high, small ones scattered around, ground cover that blocks nothing; never on kept tiles, near
// a visit tile or in a town's square.
func (c *content) decorate(field noise) []Object {
	amount := layout.Amount(c.l.Obstacles)
	if amount == 0 {
		return nil
	}
	threshold := []float64{1, 0.64, 0.58, 0.52}[amount]
	scatter := []float64{0, 0.015, 0.03, 0.05}[amount]
	n := c.size + 1
	type classes struct{ large, small, flat []catalog.Decor }
	byBiome := map[string]*classes{}
	for id, list := range catalog.All.Decor {
		cl := &classes{}
		for _, d := range list {
			switch nb := len(d.Blocked); {
			case nb == 0:
				cl.flat = append(cl.flat, d)
			case nb <= 4:
				cl.small = append(cl.small, d)
			default:
				cl.large = append(cl.large, d)
			}
		}
		byBiome[id] = cl
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
	nearTown := func(x, y int) bool {
		for _, t := range c.towns {
			if math.Hypot(float64(x-t.X), float64(y-t.Y)) < townClear {
				return true
			}
		}
		return false
	}
	// a ring of one tile around the buildings stays clear: their models are often wider than their footprint
	around := make([]bool, c.size*c.size)
	for _, o := range c.objects {
		if o.Key == "" || !building(o.Key) {
			continue
		}
		blocked, active := o.Tiles()
		for _, t := range append(blocked, active...) {
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if x, y := t[0]+dx, t[1]+dy; x >= 0 && y >= 0 && x < c.size && y < c.size {
						around[y*c.size+x] = true
					}
				}
			}
		}
	}
	free := func(x, y int) bool {
		if !c.inside(x, y) {
			return false
		}
		i := y*c.size + x
		if !c.walk[i] || c.occ[i] != tFree || c.keep[i] || around[i] || nearTown(x, y) {
			return false
		}
		for _, s := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} { // keep visit tiles reachable from every side
			if j := (y+s[1])*c.size + x + s[0]; c.occ[j] == tVisit {
				return false
			}
		}
		return true
	}

	var out []Object
	order := c.rnd.Perm(c.size * c.size)
	for _, i := range order {
		x, y := i%c.size, i/c.size
		if !free(x, y) {
			continue
		}
		cl := byBiome[c.l.Biomes[c.cellOf[y*n+x]]]
		if cl == nil {
			continue
		}
		v := field.fbm(float64(x)/decorScale, float64(y)/decorScale, 3)
		var d *catalog.Decor
		r := c.rnd.Float64()
		switch {
		case v > threshold && r < 0.35 && len(cl.large) > 0:
			d = pick(cl.large)
		case v > threshold && r < 0.75:
			d = pick(cl.small)
		case v > threshold-0.08 && r < 0.12:
			d = pick(cl.small)
		case r < scatter:
			d = pick(cl.small)
		case r < scatter+0.04:
			d = pick(cl.flat)
		}
		if d == nil {
			continue
		}
		if len(d.Blocked) == 0 {
			out = append(out, Object{Href: d.Href, X: float64(x) + c.rnd.Float64(), Y: float64(y) + c.rnd.Float64(), Rot: c.rnd.Float64() * 2 * math.Pi})
			continue
		}
		turns := c.rnd.Intn(4)
		ok := true
		var tiles [][2]int
		for _, b := range d.Blocked {
			t := turn(b, turns)
			tx, ty := x+t[0], y+t[1]
			if !free(tx, ty) {
				ok = false
				break
			}
			tiles = append(tiles, [2]int{tx, ty})
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

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Tiles returns the blocked and visit tiles of a placed object or decoration.
func (o Object) Tiles() (blocked, active [][2]int) {
	if o.Key != "" {
		return footprint(catalog.Get(o.Key), int(o.X), int(o.Y), o.Turns)
	}
	for _, b := range catalog.DecorByHref[o.Href].Blocked {
		t := turn(b, o.Turns)
		blocked = append(blocked, [2]int{int(o.X) + t[0], int(o.Y) + t[1]})
	}
	return blocked, nil
}
