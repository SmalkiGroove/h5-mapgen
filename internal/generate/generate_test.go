package generate

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	"h5mapgen/internal/biome"
	"h5mapgen/internal/layout"
	"h5mapgen/internal/terrain"
)

// On random layouts, the roads join every town entrance (4-connected road tiles) and never cross a town.
func TestRoadsJoinTowns(t *testing.T) {
	rnd := rand.New(rand.NewSource(7))
	for k := 0; k < 12; k++ {
		size := layout.Sizes[rnd.Intn(len(layout.Sizes))]
		l := layout.New(size, 16, rnd.Int63n(1000))
		for i := range l.Biomes {
			l.Biomes[i] = biome.All[rnd.Intn(len(biome.All))].ID
			if rnd.Intn(2) == 0 {
				l.Relief[i] = layout.Hills
			}
		}
		l.Starts = nil
		players := 2 + rnd.Intn(layout.MaxPlayers-1)
		for tries := 0; len(l.Starts) < players && tries < 1000; tries++ {
			s := layout.Start{X: 1 + rnd.Float64()*14, Y: 1 + rnd.Float64()*14}
			apart := true
			for _, o := range l.Starts { // towns 15+ tiles apart (footprint about 11 x 11)
				apart = apart && math.Hypot(s.X-o.X, s.Y-o.Y)*float64(size)/16 >= 15
			}
			if apart {
				l.Starts = append(l.Starts, s)
			}
		}
		start := time.Now()
		res, err := Generate(l)
		if err != nil {
			t.Fatal(err)
		}
		took := time.Since(start)
		s := l.Size
		road := res.Roads
		for _, town := range res.Towns {
			for _, b := range town.Blocked() {
				if b[0] >= 0 && b[1] >= 0 && b[0] < s && b[1] < s && road[b[1]*s+b[0]] {
					t.Fatalf("layout %d: road on town %d at %v", k, town.Player, b)
				}
			}
		}
		// flood fill the road tiles from the first entrance
		first := res.Towns[0].Entrance()
		seen := map[int]bool{first[1]*s + first[0]: true}
		todo := []int{first[1]*s + first[0]}
		for len(todo) > 0 {
			c := todo[len(todo)-1]
			todo = todo[:len(todo)-1]
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				x, y := c%s+d[0], c/s+d[1]
				if x >= 0 && y >= 0 && x < s && y < s && road[y*s+x] && !seen[y*s+x] {
					seen[y*s+x] = true
					todo = append(todo, y*s+x)
				}
			}
		}
		for _, town := range res.Towns {
			e := town.Entrance()
			if !seen[e[1]*s+e[0]] {
				t.Errorf("layout %d (size %d, %d players): town %d entrance %v not reached", k, s, len(res.Towns), town.Player, e)
			}
		}
		t.Logf("layout %d: size %d, %d towns, generated in %v", k, s, len(res.Towns), took.Round(time.Millisecond))
	}
}

// Towns too close to each other are refused.
func TestTownsTooClose(t *testing.T) {
	l := layout.New(136, 16, 1)
	l.Starts = []layout.Start{{X: 8, Y: 8}, {X: 8.5, Y: 8.5}}
	if _, err := Generate(l); err == nil {
		t.Fatal("overlapping towns were accepted")
	}
}

// reachable lists the road tiles 4-connected to a town entrance.
func reachable(res *Result, size int, from Town) map[int]bool {
	e := from.Entrance()
	seen := map[int]bool{e[1]*size + e[0]: true}
	todo := []int{e[1]*size + e[0]}
	for len(todo) > 0 {
		c := todo[len(todo)-1]
		todo = todo[:len(todo)-1]
		for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			x, y := c%size+d[0], c/size+d[1]
			if x >= 0 && y >= 0 && x < size && y < size && res.Roads[y*size+x] && !seen[y*size+x] {
				seen[y*size+x] = true
				todo = append(todo, y*size+x)
			}
		}
	}
	return seen
}

// Custom roads join only the chosen towns; "none" builds no road.
func TestRoadModes(t *testing.T) {
	l := layout.New(176, 16, 3)
	l.Starts = []layout.Start{{X: 2, Y: 14}, {X: 14, Y: 14}, {X: 8, Y: 2}}
	l.Roads, l.Links = layout.RoadsCustom, [][2]int{{2, 1}, {1, 2}} // one road, given twice
	res, err := Generate(l)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) != 1 || res.Links[0] != [2]int{1, 2} {
		t.Errorf("links %v, expected [[1 2]]", res.Links)
	}
	seen := reachable(res, l.Size, res.Towns[0])
	e2, e3 := res.Towns[1].Entrance(), res.Towns[2].Entrance()
	if !seen[e2[1]*l.Size+e2[0]] {
		t.Error("players 1 and 2 are not joined")
	}
	if seen[e3[1]*l.Size+e3[0]] || res.Roads[e3[1]*l.Size+e3[0]] {
		t.Error("player 3 has a road")
	}

	l.Roads = layout.RoadsNone
	if res, err = Generate(l); err != nil || res.Roads != nil {
		t.Errorf("roads with mode none (err %v)", err)
	}
	l.Roads, l.Links = layout.RoadsCustom, [][2]int{{1, 4}}
	if _, err = Generate(l); err == nil {
		t.Error("a road to a missing player was accepted")
	}
}

// checkLevels checks the game's rules on a generated terrain: neighbouring vertices at most one step apart,
// water at height 0, the height of flat vertices from their level, and a ramp on every plateau of some size.
func checkLevels(t *testing.T, name string, res *Result) {
	t.Helper()
	ter := res.Terrain
	n := ter.Size + 1
	lvl, ramp := ter.Grids[terrain.TagLevel].Data, ter.Grids[terrain.TagRamp].Data
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			i := y*n + x
			for _, d := range [4][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}} {
				xx, yy := x+d[0], y+d[1]
				if xx >= n || yy < 0 || yy >= n {
					continue
				}
				if diff := int(lvl[i]) - int(lvl[yy*n+xx]); diff > terrain.LevelStep || diff < -terrain.LevelStep {
					t.Fatalf("%s: levels %d and %d side by side at (%d,%d)", name, lvl[i], lvl[yy*n+xx], x, y)
				}
			}
			if ramp[i] != 0 && lvl[i]%terrain.LevelStep != 0 { // middle row: the cliff beside it is below
				for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
					if j := (y+d[1])*n + x + d[0]; ramp[j] == 0 && lvl[j] > lvl[i] {
						t.Fatalf("%s: level %d beside the middle of a ramp at (%d,%d)", name, lvl[j], x, y)
					}
				}
			}
			if lvl[i] == terrain.LevelWater && ter.Height[i] != 0 {
				t.Fatalf("%s: water at height %v", name, ter.Height[i])
			}
			if lvl[i] != terrain.LevelGround && ter.Height[i] != float32(lvl[i])/8 {
				t.Fatalf("%s: level %d at height %v", name, lvl[i], ter.Height[i])
			}
		}
	}
	levels := make([]int, len(lvl))
	for i, v := range lvl {
		levels[i] = int(v)
	}
	for h := terrain.LevelGround + terrain.LevelStep; h <= 255; h += terrain.LevelStep {
		for _, comp := range plateaus(levels, n, h) {
			ramps := 0
			for _, i := range comp {
				if ramp[i] != 0 {
					ramps++
				}
			}
			if ramps == 0 && len(comp) >= 300 {
				t.Errorf("%s: a plateau of level %d (%d vertices, at %d,%d) has no ramp", name, h, len(comp), comp[0]%n, comp[0]/n)
			}
		}
	}
}

// Random reliefs of every kind give terrains the game accepts.
func TestLevelRules(t *testing.T) {
	rnd := rand.New(rand.NewSource(11))
	for k := 0; k < 8; k++ {
		size := layout.Sizes[rnd.Intn(len(layout.Sizes))]
		grid := 8 + rnd.Intn(20)
		l := layout.New(size, grid, rnd.Int63n(1000))
		l.Roads = layout.RoadsNone
		for i := range l.Relief {
			l.Relief[i] = layout.Reliefs[rnd.Intn(len(layout.Reliefs))]
		}
		var land []string
		for _, r := range layout.Reliefs {
			if reliefLevel(r) > terrain.LevelWater {
				land = append(land, r)
			}
		}
		for _, s := range l.Starts { // towns on land
			l.Relief[int(s.Y)*grid+int(s.X)] = land[rnd.Intn(len(land))]
		}
		res, err := Generate(l)
		if err != nil {
			t.Fatal(err)
		}
		checkLevels(t, fmt.Sprintf("layout %d (size %d, grid %d)", k, size, grid), res)
	}
}

// A town on a high plateau is joined to towns below by roads over ramps; a town in water is refused; water
// across the whole map cuts the roads.
func TestPlateauRoads(t *testing.T) {
	l := layout.New(136, 16, 5)
	set := func(x0, y0, x1, y1 int, relief string) {
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				l.Relief[y*16+x] = relief
			}
		}
	}
	set(4, 4, 11, 11, layout.Plateau)
	set(6, 6, 9, 9, layout.High)
	set(12, 0, 15, 3, layout.Water)
	l.Starts = []layout.Start{{X: 8, Y: 8}, {X: 2, Y: 14}, {X: 14, Y: 5.5}}
	res, err := Generate(l)
	if err != nil {
		t.Fatal(err)
	}
	checkLevels(t, "plateau", res)
	n := l.Size + 1
	lvl := res.Terrain.Grids[terrain.TagLevel].Data
	if town := res.Towns[0]; lvl[town.Y*n+town.X] != 48 {
		t.Errorf("town 1 is at level %d, expected 48", lvl[town.Y*n+town.X])
	}
	if town := res.Towns[2]; lvl[town.Y*n+town.X] != terrain.LevelGround {
		t.Errorf("town 3 is at level %d, expected the ground", lvl[town.Y*n+town.X])
	}
	var bed []byte // the lakebed covers all the water
	for _, layer := range res.Terrain.Layers {
		if layer.Href == biome.Href("Sand/River-bed") {
			bed = layer.Weights.Data
		}
	}
	for i, v := range lvl {
		if v == terrain.LevelWater && (bed == nil || bed[i] != 255) {
			t.Fatalf("water at (%d,%d) without its bed", i%n, i/n)
		}
	}
	seen := reachable(res, l.Size, res.Towns[0])
	for _, town := range res.Towns[1:] {
		if e := town.Entrance(); !seen[e[1]*l.Size+e[0]] {
			t.Errorf("town %d is not joined to the town on the plateau", town.Player)
		}
	}

	l.Starts[2] = layout.Start{X: 14, Y: 2}
	if _, err := Generate(l); err == nil || !strings.Contains(err.Error(), "in water") {
		t.Errorf("a town in water: err %v", err)
	}

	set(0, 7, 15, 8, layout.Water) // a sea across the map: no road over it, a shipyard on each side
	l.Starts = []layout.Start{{X: 3, Y: 3}, {X: 13, Y: 13}}
	res, err = Generate(l)
	if err != nil {
		t.Fatalf("towns across the sea: %v", err)
	}
	if !res.Apart || len(res.Links) != 0 {
		t.Errorf("towns across the sea: apart %v, roads %v", res.Apart, res.Links)
	}
	yards := 0
	for _, o := range res.Objects {
		if o.Key == "shipyard" {
			yards++
		}
	}
	if yards != 2 {
		t.Errorf("towns across the sea: %d shipyards, warnings %v", yards, res.Warnings)
	}
}
