package generate

import (
	"math"
	"math/rand"
	"slices"
	"strings"
	"testing"

	"h5mapgen/internal/biome"
	"h5mapgen/internal/catalog"
	"h5mapgen/internal/layout"
)

// checkContent checks the placed content of a generated map.
func checkContent(t *testing.T, name string, l *layout.Layout, res *Result) {
	t.Helper()
	size, n := l.Size, l.Size+1
	lvl, ramp := make([]int, n*n), make([]bool, n*n)
	for i, v := range res.Terrain.Grids[7].Data {
		lvl[i] = int(v)
		ramp[i] = res.Terrain.Grids[8].Data[i] != 0
	}
	walk := walkableTiles(size, lvl, ramp)
	sea := make([]bool, size*size) // four corners in water
	for i := range sea {
		x, y := i%size, i/size
		sea[i] = lvl[y*n+x] == 0 && lvl[y*n+x+1] == 0 && lvl[(y+1)*n+x] == 0 && lvl[(y+1)*n+x+1] == 0
	}
	onSea := false            // the object being claimed stands on the sea
	owner := map[int]string{} // tile -> what stands there
	visit := map[int]bool{}
	monster := map[int]bool{}
	claim := func(tile [2]int, what string) {
		i := tile[1]*size + tile[0]
		if tile[0] < 0 || tile[1] < 0 || tile[0] >= size || tile[1] >= size {
			t.Fatalf("%s: %s outside the map at %v", name, what, tile)
		}
		if onSea && !sea[i] {
			t.Fatalf("%s: %s off the sea at %v", name, what, tile)
		}
		if !onSea && !walk[i] {
			t.Fatalf("%s: %s on water or a cliff at %v", name, what, tile)
		}
		if res.Roads != nil && res.Roads[i] {
			t.Fatalf("%s: %s on a road at %v", name, what, tile)
		}
		if o, ok := owner[i]; ok {
			t.Fatalf("%s: %s and %s both on %v", name, what, o, tile)
		}
		owner[i] = what
	}
	for _, town := range res.Towns {
		for _, b := range town.Blocked() {
			owner[b[1]*size+b[0]] = "town"
		}
	}
	for _, o := range append(append([]Object(nil), res.Objects...), res.Decor...) {
		blocked, active := o.Tiles()
		what := o.Key
		if what == "" {
			what = o.Href[strings.LastIndex(o.Href, "/")+1:]
		}
		onSea = o.Key != "" && catalog.Get(o.Key).Water || o.Key == "" && strings.Contains(strings.ToLower(o.Href), "/water/")
		for _, b := range blocked {
			claim(b, what)
		}
		for _, a := range active {
			claim(a, what)
			visit[a[1]*size+a[0]] = true
			if strings.HasPrefix(o.Key, "monster_") {
				monster[a[1]*size+a[0]] = true
			}
		}
	}
	// a boat tile is beside a walkable ramp tile: heroes board there (the coast is a cliff elsewhere)
	berth := func(b int) bool {
		for _, d := range neighbours {
			x, y := b%size+d[0], b/size+d[1]
			if x < 0 || y < 0 || x >= size || y >= size || !walk[y*size+x] {
				continue
			}
			for _, v := range [4]int{y*n + x, y*n + x + 1, (y+1)*n + x, (y+1)*n + x + 1} {
				if ramp[v] {
					return true
				}
			}
		}
		return false
	}
	for _, town := range res.Towns {
		if town.Ship != nil {
			b := (town.Y+town.Ship[1])*size + town.X + town.Ship[0]
			if _, taken := owner[b]; taken || !sea[b] || !berth(b) {
				t.Fatalf("%s: town %d's ship tile is off the sea, taken or away from a ramp", name, town.Player)
			}
		}
	}
	// the sea objects: those of the coasts near the land, those of the open sea farther; no whirlpool unless
	// brushed
	notSea := make([]bool, size*size)
	for i, s := range sea {
		notSea[i] = !s
	}
	coast := chebyshevDistance(notSea, size)
	for _, o := range res.Objects {
		i := int(o.Y)*size + int(o.X)
		switch {
		case coastKinds[o.Key] > 0 && coast[i] >= openSea:
			t.Errorf("%s: %s %d tiles off the coast", name, o.Key, coast[i])
		case openKinds[o.Key] > 0 && coast[i] < openSea:
			t.Errorf("%s: %s %d tiles from the coast", name, o.Key, coast[i])
		}
		if o.Key == "water_whirlpool" && !slices.ContainsFunc(l.Objects, func(u layout.Object) bool { return u.Kind == o.Key }) {
			t.Errorf("%s: a whirlpool nobody placed", name)
		}
	}
	// nothing but the seabed textures under water
	for _, layer := range res.Terrain.Layers {
		if strings.Contains(layer.Href, biome.SeaBed+".") || strings.Contains(layer.Href, biome.DeepSeaBed+".") {
			continue
		}
		for i, w := range layer.Weights.Data {
			if w > 0 && lvl[i] == 0 {
				t.Fatalf("%s: %s on the water at vertex (%d, %d)", name, layer.Href, i%n, i/n)
			}
		}
	}
	// heroes walk (the ramps lead up and down the cliffs and onto the boats), and sail from the shipyards'
	// boat tiles over the sea
	sail := map[int]bool{}
	var boats []int
	for _, o := range res.Objects {
		if o.Key == "shipyard" {
			if o.Ship == nil {
				t.Fatalf("%s: a shipyard without a boat tile", name)
			}
			b := (int(o.Y)+o.Ship[1])*size + int(o.X) + o.Ship[0]
			if !sea[b] || !berth(b) {
				t.Fatalf("%s: a shipyard's boat tile off the sea or away from a ramp", name)
			}
			if _, taken := owner[b]; taken {
				t.Fatalf("%s: a shipyard's boat tile taken by %s", name, owner[b])
			}
			sail[b] = true
			boats = append(boats, b)
		}
	}
	for q := 0; q < len(boats); q++ {
		cx, cy := boats[q]%size, boats[q]/size
		for _, d := range neighbours {
			x, y := cx+d[0], cy+d[1]
			if i := y*size + x; x >= 0 && y >= 0 && x < size && y < size && sea[i] && !sail[i] {
				sail[i] = true
				boats = append(boats, i)
			}
		}
	}
	// walk from the first town: free tiles and monsters (fought) are crossed, visit tiles are reached
	e := res.Towns[0].Entrance()
	seen := map[int]bool{e[1]*size + e[0]: true}
	queue := []int{e[1]*size + e[0]}
	for q := 0; q < len(queue); q++ {
		cur := queue[q]
		if visit[cur] && !monster[cur] && q > 0 {
			continue // a visit tile ends the walk
		}
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				x, y := cur%size+dx, cur/size+dy
				i := y*size + x
				if x < 0 || y < 0 || x >= size || y >= size || seen[i] || !walk[i] && !sail[i] {
					continue
				}
				if _, taken := owner[i]; taken && !visit[i] {
					continue
				}
				seen[i] = true
				queue = append(queue, i)
			}
		}
	}
	for _, town := range res.Towns {
		if e := town.Entrance(); !seen[e[1]*size+e[0]] {
			t.Errorf("%s: town %d can't be reached from town %d", name, town.Player, res.Towns[0].Player)
		}
	}
	for _, o := range res.Objects {
		_, active := o.Tiles()
		for _, a := range active {
			if !seen[a[1]*size+a[0]] {
				t.Errorf("%s: %s at (%v, %v) can't be reached", name, o.Key, o.X, o.Y)
			}
		}
		if o.Guards >= 0 {
			g := res.Objects[o.Guards]
			dx, dy := abs(int(g.X)-active[0][0]), abs(int(g.Y)-active[0][1])
			if dx+dy != 1 && !(dx == 1 && dy == 1 && o.Key == "stat_tomb_of_the_warrior") { // the tomb is entered from its corners
				t.Errorf("%s: the guard of %s at (%v, %v) is not beside its visit tile", name, o.Key, o.X, o.Y)
			}
		}
		if building(o.Key) {
			blocked, _ := o.Tiles()
			for _, b := range append(blocked, active...) {
				for _, town := range res.Towns {
					for _, tb := range town.Blocked() {
						if d := math.Hypot(float64(b[0]-tb[0]), float64(b[1]-tb[1])); d <= buildingClear {
							t.Errorf("%s: %s at (%v, %v) is %.1f tiles from town %d", name, o.Key, o.X, o.Y, d, town.Player)
						}
					}
				}
			}
			// mines and pathed buildings have a path to a road (tiles meeting at a corner share a painted vertex)
			if res.Roads != nil && important(o.Key) {
				a := active[0][1]*size + active[0][0]
				seen := map[int]bool{a: true}
				queue, found := []int{a}, false
				for q := 0; q < len(queue) && !found; q++ {
					cx, cy := queue[q]%size, queue[q]/size
					for _, d := range [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
						x, y := cx+d[0], cy+d[1]
						i := y*size + x
						if x < 0 || y < 0 || x >= size || y >= size || seen[i] {
							continue
						}
						if res.Roads[i] {
							found = true
							break
						}
						if res.Paths[i] {
							seen[i] = true
							queue = append(queue, i)
						}
					}
				}
				if !found {
					t.Errorf("%s: %s at (%v, %v) has no path to a road", name, o.Key, o.X, o.Y)
				}
			}
		}
		if strings.HasPrefix(o.Key, "monster_") && res.Roads != nil {
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if x, y := int(o.X)+dx, int(o.Y)+dy; res.Roads[y*size+x] {
						t.Errorf("%s: %s at (%v, %v) guards a road tile", name, o.Key, o.X, o.Y)
					}
				}
			}
		}
	}
}

// Random layouts with every relief get content that is reachable and never overlaps.
func TestContent(t *testing.T) {
	rnd := rand.New(rand.NewSource(21))
	for k := 0; k < 6; k++ {
		size := []int{96, 136, 176, 216}[rnd.Intn(4)]
		l := layout.New(size, 12, rnd.Int63n(1000))
		for i := range l.Relief {
			l.Biomes[i] = biome.All[rnd.Intn(len(biome.All))].ID
			if r := rnd.Intn(10); r == 0 {
				l.Relief[i] = layout.Water
			} else if r < 3 {
				l.Relief[i] = layout.Plateau
			} else if r < 5 {
				l.Relief[i] = layout.Hills
			}
		}
		l.Starts = []layout.Start{{X: 2.5, Y: 9.5}, {X: 9.5, Y: 2.5}}
		for _, s := range l.Starts {
			l.Relief[int(s.Y)*12+int(s.X)] = layout.Flat
		}
		l.Objects = []layout.Object{{Kind: "mine_gold", X: 6, Y: 6}, {Kind: "artifact_relic", X: 3, Y: 3}}
		res, err := Generate(l)
		if err != nil {
			if strings.Contains(err.Error(), "no road") {
				continue // water cut the towns apart
			}
			t.Fatal(err)
		}
		mines := 0
		for _, o := range res.Objects {
			if strings.HasPrefix(o.Key, "mine_") {
				mines++
			}
		}
		t.Logf("layout %d (size %d): %d objects (%d mines), %d decorations, %d warnings %v", k, size, len(res.Objects), mines, len(res.Decor), len(res.Warnings), res.Warnings)
		checkContent(t, "layout "+string(rune('0'+k)), l, res)
		checkStatParity(t, "layout "+string(rune('0'+k)), res)
	}
}

// checkStatParity checks that the stat givers keep the attributes at par: as many of each +1 building, as
// many Arenas as Schools of Magic, at most one Library per neutral area at Normal.
func checkStatParity(t *testing.T, name string, res *Result) {
	t.Helper()
	for _, w := range res.Warnings {
		if strings.HasPrefix(w, "stat_") {
			return // a stat giver was left out after placing: parity isn't promised
		}
	}
	count := map[string]int{}
	for _, o := range res.Objects {
		count[o.Key]++
	}
	for _, key := range plusOneStats[1:] {
		if count[key] != count[plusOneStats[0]] {
			t.Errorf("%s: %d %s but %d %s", name, count[key], key, count[plusOneStats[0]], plusOneStats[0])
		}
	}
	if count["stat_arena"] != count["stat_school_of_magic"] {
		t.Errorf("%s: %d arenas but %d schools of magic", name, count["stat_arena"], count["stat_school_of_magic"])
	}
	if count["stat_library"] > 1 {
		t.Errorf("%s: %d libraries at Normal", name, count["stat_library"])
	}
	weekly := 0
	for _, key := range weeklyStats {
		weekly += count[key]
	}
	if weekly == 0 {
		t.Errorf("%s: no weekly stat giver", name)
	}
	for _, o := range res.Objects {
		if isRandomDwelling(o.Key) && o.Player == 0 {
			t.Errorf("%s: %s at (%v, %v) is linked to no player", name, o.Key, o.X, o.Y)
		}
	}
	t.Logf("%s: stat givers %v", name, count)
}

// Islands: each town on its own island gets a shipyard, the sea gets its objects, and the content of the
// island in the middle is reached by boat.
func TestIslands(t *testing.T) {
	l := layout.New(136, 16, 11)
	for i := range l.Relief {
		l.Relief[i] = layout.Water
	}
	island := func(x0, y0, x1, y1 int) {
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				l.Relief[y*16+x] = layout.Flat
			}
		}
	}
	island(0, 0, 5, 5)
	island(10, 10, 15, 15)
	island(6, 6, 9, 9)
	l.Starts = []layout.Start{{X: 2.5, Y: 2.5}, {X: 13.5, Y: 13.5}}
	res, err := Generate(l)
	if err != nil {
		t.Fatal(err)
	}
	checkContent(t, "islands", l, res)
	count := map[string]int{}
	middle := 0
	cs := 136.0 / 16
	for _, o := range res.Objects {
		count[o.Key]++
		if catalog.Get(o.Key).Water {
			count["(sea)"]++
		}
		if o.X > 6*cs+2 && o.X < 10*cs-2 && o.Y > 6*cs+2 && o.Y < 10*cs-2 {
			middle++
		}
	}
	t.Logf("islands: %v, %d objects in the middle, warnings %v", count, middle, res.Warnings)
	if !res.Apart || len(res.Links) != 0 {
		t.Errorf("apart %v, roads %v", res.Apart, res.Links)
	}
	if count["shipyard"] != 2 {
		t.Errorf("%d shipyards", count["shipyard"])
	}
	if count["(sea)"] == 0 {
		t.Error("nothing on the sea")
	}
	if middle == 0 {
		t.Error("nothing on the island in the middle")
	}
}
