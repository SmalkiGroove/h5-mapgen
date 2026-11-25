// Package layout is the coarse map plan the user paints: a square grid of cells, each with a biome and a
// relief, and the player starts.
package layout

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"h5mapgen/internal/biome"
	"h5mapgen/internal/catalog"
)

// Placeable tells whether the brush can place a catalog object (mines, treasures, artifacts, monsters).
func Placeable(kind string) bool {
	for _, prefix := range []string{"mine_", "res_", "artifact_", "monster_"} {
		if strings.HasPrefix(kind, prefix) {
			return true
		}
	}
	return kind == "chest" || kind == "campfire"
}

// Map sizes of the game (side in tiles), smallest to largest.
var Sizes = []int{72, 96, 136, 176, 216, 256, 320}

// Reliefs a cell can take.
const (
	Water   = "water"
	Flat    = "flat"
	Hills   = "hills"
	Plateau = "plateau" // one cliff above the ground
	High    = "high"    // two cliffs above the ground
)

// Reliefs lists the reliefs, lowest first.
var Reliefs = []string{Water, Flat, Hills, Plateau, High}

// MaxPlayers is the number of player slots of a map.
const MaxPlayers = 8

// Start is a player start, in cell units (0..Grid, y up; a cell centre is at +0.5).
type Start struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Layout is the plan of a map. Cells are stored row by row, row 0 at the bottom (map y up).
type Layout struct {
	Version int      `json:"version"`
	Name    string   `json:"name"`
	Size    int      `json:"size"` // map side in tiles
	Grid    int      `json:"grid"` // cells per side
	Seed    int64    `json:"seed"`
	Biomes  []string `json:"biomes"`
	Relief  []string `json:"relief"`
	Starts  []Start  `json:"starts"`            // player i+1 starts at Starts[i]
	Roads   string   `json:"roads,omitempty"`   // RoadsAuto (also ""), RoadsCustom or RoadsNone
	Links   [][2]int `json:"links,omitempty"`   // custom roads: pairs of player numbers (1..8)
	NoRoads bool     `json:"noRoads,omitempty"` // old form of Roads = RoadsNone, read only

	// Content: amounts of each kind ("" = Normal), and the objects placed with the brush
	Mines     string   `json:"mines,omitempty"`
	Treasures string   `json:"treasures,omitempty"`
	Guards    string   `json:"guards,omitempty"`
	Obstacles string   `json:"obstacles,omitempty"`
	Objects   []Object `json:"objects,omitempty"`
}

// Object is an object placed with the brush: a catalog key (mine_gold, chest, artifact_major, monster_5, ...)
// at a position in cell units, like the starts. The generator moves it to the nearest tile where it fits.
type Object struct {
	Kind string  `json:"kind"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

// Content amounts.
const (
	AmountNone   = "none"
	AmountLow    = "low"
	AmountNormal = "normal"
	AmountHigh   = "high"
)

// Amounts lists the content amounts, least first.
var Amounts = []string{AmountNone, AmountLow, AmountNormal, AmountHigh}

// Amount returns the index of an amount in Amounts (Normal when unset).
func Amount(a string) int {
	if a == "" {
		return 2
	}
	return slices.Index(Amounts, a)
}

// Road modes.
const (
	RoadsAuto   = "auto"   // the shortest set of roads joining every town (minimum spanning tree)
	RoadsCustom = "custom" // the roads of Links
	RoadsNone   = "none"
)

// RoadMode returns the road mode, RoadsAuto when unset.
func (l *Layout) RoadMode() string {
	switch {
	case l.NoRoads && l.Roads == "":
		return RoadsNone
	case l.Roads == "":
		return RoadsAuto
	}
	return l.Roads
}

// New makes a layout filled with the default biome, flat, two players in opposite corners.
func New(size, grid int, seed int64) *Layout {
	l := &Layout{Version: 1, Name: "Generated map", Size: size, Grid: grid, Seed: seed,
		Biomes: make([]string, grid*grid), Relief: make([]string, grid*grid)}
	for i := range l.Biomes {
		l.Biomes[i] = biome.All[0].ID
		l.Relief[i] = Flat
	}
	g := float64(grid)
	l.Starts = []Start{{X: g * 0.2, Y: g * 0.8}, {X: g * 0.8, Y: g * 0.2}}
	return l
}

var namePattern = regexp.MustCompile(`^[A-Za-z0-9 _\-]{1,40}$`)

// Check returns the first problem of a layout, or nil.
func (l *Layout) Check() error {
	sizeOK := false
	for _, s := range Sizes {
		sizeOK = sizeOK || s == l.Size
	}
	switch {
	case !sizeOK:
		return fmt.Errorf("map size %d is not one of %v", l.Size, Sizes)
	case l.Grid < 4 || l.Grid > 64:
		return fmt.Errorf("grid %d is out of 4..64", l.Grid)
	case len(l.Biomes) != l.Grid*l.Grid || len(l.Relief) != l.Grid*l.Grid:
		return fmt.Errorf("the layout has %d biome and %d relief cells, expected %d", len(l.Biomes), len(l.Relief), l.Grid*l.Grid)
	case !namePattern.MatchString(l.Name):
		return fmt.Errorf("map name %q: use 1 to 40 letters, digits, spaces, _ or -", l.Name)
	case len(l.Starts) < 1 || len(l.Starts) > MaxPlayers:
		return fmt.Errorf("%d player starts, expected 1 to %d", len(l.Starts), MaxPlayers)
	}
	for _, id := range l.Biomes {
		if _, err := biome.ByID(id); err != nil {
			return err
		}
	}
	for _, r := range l.Relief {
		if !slices.Contains(Reliefs, r) {
			return fmt.Errorf("unknown relief %q", r)
		}
	}
	for i, s := range l.Starts {
		if s.X < 0 || s.Y < 0 || s.X > float64(l.Grid) || s.Y > float64(l.Grid) {
			return fmt.Errorf("player %d starts outside the grid", i+1)
		}
	}
	for _, a := range []struct{ name, value string }{{"mines", l.Mines}, {"treasures", l.Treasures}, {"guards", l.Guards}, {"obstacles", l.Obstacles}} {
		if Amount(a.value) < 0 {
			return fmt.Errorf("%s: unknown amount %q", a.name, a.value)
		}
	}
	for i, o := range l.Objects {
		if _, ok := catalog.All.Objects[o.Kind]; !ok || !Placeable(o.Kind) {
			return fmt.Errorf("object %d: unknown kind %q", i+1, o.Kind)
		}
		if o.X < 0 || o.Y < 0 || o.X > float64(l.Grid) || o.Y > float64(l.Grid) {
			return fmt.Errorf("object %d (%s) is outside the grid", i+1, o.Kind)
		}
	}
	switch l.RoadMode() {
	case RoadsAuto, RoadsNone:
	case RoadsCustom:
		for _, k := range l.Links {
			if k[0] < 1 || k[1] < 1 || k[0] > len(l.Starts) || k[1] > len(l.Starts) || k[0] == k[1] {
				return fmt.Errorf("road %d-%d: no such pair of players", k[0], k[1])
			}
		}
	default:
		return fmt.Errorf("unknown road mode %q", l.Roads)
	}
	return nil
}

// Parse reads a layout saved as JSON and checks it.
func Parse(data []byte) (*Layout, error) {
	var l Layout
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, err
	}
	return &l, l.Check()
}

// JSON encodes a layout, indented.
func (l *Layout) JSON() []byte {
	data, _ := json.MarshalIndent(l, "", "  ")
	return data
}
