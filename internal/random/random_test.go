package random

import (
	"fmt"
	"testing"

	"h5mapgen/internal/layout"
)

// Every preset makes playable layouts for 2 to 8 players; with 2 and 4 players the relief is exactly the
// same around every start.
func TestPresets(t *testing.T) {
	for _, p := range Presets {
		for _, players := range []int{2, 3, 4, 6, 8} {
			for seed := int64(1); seed <= 3; seed++ {
				size, grid := 136, 16
				if players > 4 {
					size, grid = 216, 24
				}
				name := fmt.Sprintf("%s, %d players, seed %d", p.ID, players, seed)
				l, err := Generate(Options{Preset: p.ID, Size: size, Grid: grid, Players: players, Seed: seed})
				if err != nil {
					t.Errorf("%s: %v", name, err)
					continue
				}
				if len(l.Starts) != players {
					t.Errorf("%s: %d starts", name, len(l.Starts))
				}
				count := map[string]int{}
				for _, r := range l.Relief {
					count[r]++
				}
				if seed == 1 {
					t.Logf("%s: %v", name, count)
				}
				g := l.Grid
				for cy := 0; cy < g; cy++ {
					for cx := 0; cx < g; cx++ {
						var x, y int
						switch players {
						case 2:
							x, y = g-1-cx, g-1-cy // half a turn
						case 4:
							x, y = g-1-cy, cx // a quarter turn
						default:
							continue
						}
						if a, b := l.Relief[cy*g+cx], l.Relief[y*g+x]; a != b {
							t.Errorf("%s: cell (%d, %d) is %s but its turned cell (%d, %d) is %s", name, cx, cy, a, x, y, b)
						}
					}
				}
			}
		}
	}
}

func TestOptions(t *testing.T) {
	if _, err := Generate(Options{Preset: "nope", Size: 136, Grid: 16, Players: 2}); err == nil {
		t.Error("an unknown preset is accepted")
	}
	if _, err := Generate(Options{Preset: "open", Size: 136, Grid: 16, Players: 9}); err == nil {
		t.Error("9 players are accepted")
	}
	l, err := Generate(Options{Preset: "open", Size: 136, Grid: 16, Players: 2, Seed: 5, Biomes: []string{"snow", "lava"}})
	if err == nil {
		t.Error("an unknown biome is accepted")
	}
	l, err = Generate(Options{Preset: "open", Size: 136, Grid: 16, Players: 2, Seed: 5, Biomes: []string{"snow"}})
	if err != nil {
		t.Fatal(err)
	}
	g := l.Grid
	at := func(s layout.Start) string { return l.Biomes[int(s.Y)*g+int(s.X)] }
	if at(l.Starts[0]) != "snow" {
		t.Errorf("player 1 starts on %s, not the snow asked", at(l.Starts[0]))
	}
	if at(l.Starts[1]) == "snow" {
		t.Error("player 2 got the snow asked for player 1 at random")
	}
	if _, err := Generate(Options{Preset: "open", Size: 136, Grid: 16, Players: 2, Biomes: []string{"", "subterrain"}}); err == nil {
		t.Error("the subterranean ground is accepted for a player's area")
	}
}

// The Inferno ground is never picked at random, and when a player asks for it, it stays in that player's
// area (out of the mixed centre and borders).
func TestInferno(t *testing.T) {
	for seed := int64(1); seed <= 10; seed++ {
		l, err := Generate(Options{Preset: "divided", Size: 216, Grid: 24, Players: 6, Seed: seed})
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range l.Biomes {
			if b == inferno {
				t.Fatalf("seed %d: Inferno ground picked at random", seed)
			}
		}
	}
	l, err := Generate(Options{Preset: "open", Size: 136, Grid: 16, Players: 4, Seed: 3, Biomes: []string{inferno}})
	if err != nil {
		t.Fatal(err)
	}
	g, s := l.Grid, l.Starts[0]
	if l.Biomes[int(s.Y)*g+int(s.X)] != inferno {
		t.Error("player 1 doesn't start on the Inferno ground asked")
	}
	side := func(c int, start float64) bool { return (c < g/2) == (start < float64(g)/2) }
	for i, b := range l.Biomes {
		cx, cy := i%g, i/g
		if b == inferno && !(side(cx, s.X) && side(cy, s.Y)) { // player 1 holds the quarter of its start
			t.Errorf("Inferno ground at (%d, %d), out of player 1's area", cx, cy)
		}
	}
}
