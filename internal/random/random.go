// Package random makes layouts from a preset, a number of players and a seed, the same for every player:
// each cell is seen from the start of its player's sector, and the noise that shapes the terrain is read in
// those folded coordinates, so every sector gets the same terrain (exactly for 2 and 4 players, closely
// for the others on a square grid).
package random

import (
	"fmt"
	"math"
	"math/rand"
	"slices"

	"h5mapgen/internal/generate"
	"h5mapgen/internal/layout"
)

// Preset is a kind of random layout.
type Preset struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Presets lists the random layouts.
var Presets = []Preset{
	{"open", "Open land", "Mostly flat land with hills, a few small lakes and plateaus."},
	{"highlands", "Highlands", "Each player on a plateau, joined by ramps to lower land in the middle."},
	{"divided", "Divided", "Rivers split the players' areas, with a ford on each and the centre open; no boats."},
	{"islands", "Islands", "Each player on an island with a shipyard, smaller islands between them across the sea."},
}

// Options are the inputs of a random layout.
type Options struct {
	Preset  string   `json:"preset"`
	Size    int      `json:"size"`
	Grid    int      `json:"grid"`
	Players int      `json:"players"`
	Seed    int64    `json:"seed"`
	Biomes  []string `json:"biomes,omitempty"` // per player, "" or missing for a random one
}

// surface lists the biomes a player's area gets at random (the subterranean ground is left out, and so is
// the Inferno one, deadland, which only an Inferno player's area takes when asked in Options.Biomes).
var surface = []string{"grass", "dirt", "sand", "snow", "steppe"}

// inferno is the Inferno biome: it is never part of the mixed ground.
const inferno = "deadland"

// Grounds are the biomes a player's area may be given in Options.Biomes.
var Grounds = append(append([]string{}, surface...), inferno)

// Shape of the layouts, in units of half the map side (the centre is 0, the middle of an edge 1).
const (
	startRadius  = 0.68 // distance of the starts from the centre for 3 players (0.75 for the others)
	cornerRadius = 0.75
	neutralR     = 0.26 // the centre is neutral ground
	neutralBand  = 0.10 // so is a band along the borders between players
	tries        = 8    // geometries tried before giving up on a seed
	// Islands: the starts farther out with more than 4 players, and the land a town needs around it, in tiles
	islandsRadius = 0.8
	townLand      = 13.0
	// Islands, in tiles: the largest player's island (radius) and centre island, and how far beyond a
	// player's island the small islands reach. On small maps the islands fill the map; on large ones these
	// caps leave open sea between the archipelagos, 20 tiles and more from land (the user's rule).
	islandMax  = 30.0
	centreMax  = 18.0
	smallReach = 10.0
	smallGap   = 2.0 // cells of strait between a player's island and the small ones (cell borders wander up to 0.9 cell)
)

// Generate makes a random layout. It is checked by expanding its terrain (without content): a geometry
// that leaves a town in water or players cut apart (on Islands: two players on one island, or one
// without a shipyard) is replaced by the next one of the seed.
func Generate(o Options) (*layout.Layout, error) {
	preset := -1
	for i, p := range Presets {
		if p.ID == o.Preset {
			preset = i
		}
	}
	switch {
	case preset < 0:
		return nil, fmt.Errorf("unknown preset %q", o.Preset)
	case o.Players < 2 || o.Players > layout.MaxPlayers:
		return nil, fmt.Errorf("%d players is out of 2..%d", o.Players, layout.MaxPlayers)
	}
	for k, b := range o.Biomes {
		if b != "" && !slices.Contains(Grounds, b) {
			return nil, fmt.Errorf("player %d: %q is not a ground a player's area can get", k+1, b)
		}
	}
	var last error
	for try := 0; try < tries; try++ {
		l := build(o, Presets[preset], o.Seed*31+int64(try))
		if err := l.Check(); err != nil {
			return nil, err
		}
		test := *l
		test.Mines, test.Treasures, test.Stats, test.Buildings = layout.AmountNone, layout.AmountNone, layout.AmountNone, layout.AmountNone
		test.Dwellings, test.Banks, test.Guards, test.Obstacles = layout.AmountNone, layout.AmountNone, layout.AmountNone, layout.AmountNone
		test.Objects = nil
		res, err := generate.Generate(&test)
		if err == nil {
			err = playable(res, Presets[preset].ID == "islands")
		}
		if err != nil {
			last = err
			continue
		}
		return l, nil
	}
	return nil, fmt.Errorf("no playable layout from seed %d: %v", o.Seed, last)
}

// playable checks how the players reach one another: on foot, or on Islands each from its own island by
// its shipyard.
func playable(res *generate.Result, islands bool) error {
	if !islands {
		if res.Apart {
			return fmt.Errorf("water cuts the players apart")
		}
		return nil
	}
	if len(res.Links) > 0 {
		return fmt.Errorf("players %d and %d share an island", res.Links[0][0], res.Links[0][1])
	}
	yards := 0
	for _, o := range res.Objects {
		if o.Key == "shipyard" {
			yards++
		}
	}
	if yards < len(res.Towns) {
		return fmt.Errorf("%d shipyards for %d players: %v", yards, len(res.Towns), res.Warnings)
	}
	return nil
}

// geometry is where a cell stands from its player's start.
type geometry struct {
	sector   int     // index of the player whose sector holds the cell
	r        float64 // distance from the centre
	u, v     float64 // folded coordinates: the same for the matching cell of every sector
	start    float64 // distance from the sector's start
	boundary float64 // distance from the nearest border between two sectors
}

func build(o Options, p Preset, geoSeed int64) *layout.Layout {
	rnd := rand.New(rand.NewSource(geoSeed))
	g, n := o.Grid, o.Players
	l := layout.New(o.Size, o.Grid, o.Seed)
	l.Name = p.Name

	sector := 2 * math.Pi / float64(n)
	a0, rs := math.Pi/2, startRadius // the first start at the top
	switch {
	case n == 2 || n == 4:
		a0, rs = 3*math.Pi/4, cornerRadius // in the corners, the first at the top left
	case n > 4:
		rs = cornerRadius // more room between neighbours
		if p.ID == "islands" {
			rs = islandsRadius
		}
	}
	if p.ID == "islands" {
		// on large maps the archipelagos keep their size in tiles: the starts move out as far as an
		// archipelago still fits inside the map, which frees the middle for open sea
		arch := islandMax/(float64(o.Size)/2) + smallGap*(2/float64(g)) + smallReach/(float64(o.Size)/2)
		out := 1 - 0.85*arch // a start this far from the centre has its archipelago against an edge
		if n == 2 || n == 4 {
			out *= math.Sqrt2 // in a corner
		}
		rs = math.Max(rs, out)
	}
	l.Starts = nil
	for k := 0; k < n; k++ {
		a := a0 + float64(k)*sector
		l.Starts = append(l.Starts, layout.Start{X: float64(g) / 2 * (1 + rs*math.Cos(a)), Y: float64(g) / 2 * (1 + rs*math.Sin(a))})
	}

	// one biome per player: the one asked, else a random one, different from the others while there are enough
	biomes := make([]string, n)
	pool := rnd.Perm(len(surface))
	used := map[string]int{}
	for k := range biomes {
		if k < len(o.Biomes) && o.Biomes[k] != "" {
			biomes[k] = o.Biomes[k]
			used[biomes[k]]++
		}
	}
	for k := range biomes {
		if biomes[k] != "" {
			continue
		}
		best := surface[pool[0]] // the least used ground, in the order of the pool
		for _, p := range pool {
			if used[surface[p]] < used[best] {
				best = surface[p]
			}
		}
		biomes[k] = best
		used[best]++
	}
	// the mixed ground of the centre and the borders: the players' biomes but the Inferno one
	var mixed []string
	for _, b := range biomes {
		if b != inferno {
			mixed = append(mixed, b)
		}
	}
	if len(mixed) == 0 {
		mixed = []string{surface[pool[0]]}
	}

	hills, lakes, plateaus, mix, wobble := newField(rnd), newField(rnd), newField(rnd), newField(rnd), newField(rnd)
	cell := 2 / float64(g)                // a cell, in the same units
	startClear := math.Max(0.2, 1.6*cell) // flat ground around a start
	// a player's plateau (Highlands) reaches about 40 % of the way to the next start
	spacing := 2 * rs * math.Sin(sector/2)
	plateauR := math.Max(startClear+cell/2, math.Min(0.38, 0.4*spacing))
	// a player's island (Islands) leaves a strait of 1.5 cells or more to the next one, and is at least what
	// a town needs; it is all the flat ground kept around the start
	half := float64(o.Size) / 2 // tiles per unit
	islandR := math.Max(townLand/half+cell/2, math.Min(math.Min(0.36, islandMax/half), spacing/2-1.5*cell))
	centreR := math.Min(0.18, centreMax/half)
	if p.ID == "islands" {
		startClear = math.Min(startClear, islandR-cell/2)
	}
	for cy := 0; cy < g; cy++ {
		for cx := 0; cx < g; cx++ {
			px, py := (float64(cx)+0.5)*cell-1, (float64(cy)+0.5)*cell-1
			geo := locate(px, py, a0, sector, n, rs)
			i := cy*g + cx

			// biome: the player's, mixed ground in the centre and along the borders
			l.Biomes[i] = biomes[geo.sector]
			if geo.r < neutralR || geo.boundary < neutralBand {
				m := len(mixed)
				k := int(mix.fbm(geo.u*2.5, geo.v*2.5) * float64(m) * 1.6)
				l.Biomes[i] = mixed[((k%m)+m)%m]
			}

			// relief
			relief := layout.Flat
			h, w, pl := hills.fbm(geo.u*2.6, geo.v*2.6), lakes.fbm(geo.u*3, geo.v*3), plateaus.fbm(geo.u*2.8, geo.v*2.8)
			if h > 0.55 {
				relief = layout.Hills
			}
			switch p.ID {
			case "open":
				if pl > 0.66 && geo.start > 0.4 {
					relief = layout.Plateau
				}
				if w > 0.74 && geo.start > 0.38 && geo.r > neutralR*0.6 {
					relief = layout.Water
				}
			case "highlands":
				radius := plateauR * (1 + 0.3*(wobble.fbm(geo.u*3, geo.v*3)-0.5))
				if geo.start < radius {
					relief = layout.Plateau
				} else if w > 0.7 && geo.r < 0.45 {
					relief = layout.Water
				}
			case "islands":
				relief = layout.Water
				wob := 1 + 0.3*(wobble.fbm(geo.u*3, geo.v*3)-0.5)
				switch {
				case geo.start < islandR*wob: // the player's island
					relief = layout.Flat
				case geo.r < centreR*wob && rs-centreR > islandR+1.5*cell: // one in the centre
					relief = layout.Flat
				case pl > 0.64 && geo.start > islandR+smallGap*cell && geo.start < islandR+smallGap*cell+smallReach/half && geo.r > centreR+2*cell:
					relief = layout.Flat // small ones around the player's island
				}
				if relief == layout.Flat && h > 0.55 {
					relief = layout.Hills
				}
			case "divided": // rivers and ponds: no boats, the fords are the way across (the user's choice)
				ford := math.Abs(geo.r-0.52) < 1.6*cell/2 // one ford on each river
				if geo.boundary < 0.75*cell && geo.r > neutralR*0.6 && !ford {
					relief = layout.River
				} else if w > 0.72 && geo.start > 0.4 && geo.boundary > 2*cell {
					relief = layout.River
				}
			}
			if geo.start < startClear {
				relief = layout.Flat
				if p.ID == "highlands" {
					relief = layout.Plateau
				}
			}
			l.Relief[i] = relief
		}
	}
	return l
}

// locate places a point (centre 0, edges ±1) in the sectors of n players whose first start is at angle a0
// and distance rs from the centre.
func locate(px, py, a0, sector float64, n int, rs float64) geometry {
	r := math.Hypot(px, py)
	d := math.Mod(math.Atan2(py, px)-a0+4*math.Pi, 2*math.Pi)
	k := int(math.Round(d / sector))
	delta := d - float64(k)*sector // angle from the start's direction, -sector/2..sector/2
	fold := math.Abs(delta)
	return geometry{
		sector:   k % n,
		r:        r,
		u:        r * math.Cos(fold),
		v:        r * math.Sin(fold),
		start:    math.Sqrt(math.Max(0, r*r+rs*rs-2*r*rs*math.Cos(delta))),
		boundary: r * math.Sin(math.Max(0, sector/2-fold)),
	}
}

// field is a value noise in 0..1.
type field struct{ seed uint64 }

func newField(rnd *rand.Rand) field { return field{rnd.Uint64()} }

func (f field) hash(x, y int) float64 {
	h := f.seed ^ uint64(int64(x))*0x9E3779B97F4A7C15 ^ uint64(int64(y))*0xC2B2AE3D27D4EB4F
	h ^= h >> 31
	h *= 0xBF58476D1CE4E5B9
	h ^= h >> 29
	return float64(h>>11) / float64(1<<53)
}

func (f field) at(x, y float64) float64 {
	x0, y0 := math.Floor(x), math.Floor(y)
	tx, ty := x-x0, y-y0
	tx, ty = tx*tx*(3-2*tx), ty*ty*(3-2*ty)
	ix, iy := int(x0), int(y0)
	a := f.hash(ix, iy)*(1-tx) + f.hash(ix+1, iy)*tx
	b := f.hash(ix, iy+1)*(1-tx) + f.hash(ix+1, iy+1)*tx
	return a*(1-ty) + b*ty
}

// fbm sums three octaves, spread back to about 0..1.
func (f field) fbm(x, y float64) float64 {
	v := f.at(x, y)*0.57 + f.at(x*2.03+17, y*2.03+5)*0.29 + f.at(x*4.01+3, y*4.01+29)*0.14
	return math.Max(0, math.Min(1, (v-0.5)*1.8+0.5))
}
