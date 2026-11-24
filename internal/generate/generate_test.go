package generate

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"h5mapgen/internal/biome"
	"h5mapgen/internal/layout"
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
