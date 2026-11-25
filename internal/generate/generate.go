// Package generate expands a layout into a terrain and the objects of a map.
package generate

import (
	"math"
	"math/rand"
	"slices"
	"sort"

	"h5mapgen/internal/biome"
	"h5mapgen/internal/layout"
	"h5mapgen/internal/terrain"
)

// Town is a player's starting town.
type Town struct {
	Player int `json:"player"` // 1..8
	X      int `json:"x"`      // tile
	Y      int `json:"y"`
	Turns  int `json:"turns"` // quarter turns counter-clockwise; the entrance faces south at 0, east at 1, north at 2, west at 3
}

// TownEntrance is the visit tile of RandomTown at 0 turns, relative to its position.
var TownEntrance = [2]int{1, -5}

// Result is a generated map: its ground terrain and objects.
type Result struct {
	Terrain *terrain.Terrain
	Tiles   []string // tile hrefs used by the terrain, for the tiles list of map.xdb
	Towns   []Town
	Roads   []bool     // road tiles, size x size, row = y (nil without roads)
	Links   [][2]int   // the roads built, as pairs of player numbers
	Rand    *rand.Rand // continues the layout seed, for ids
}

const (
	borderWarp  = 0.45 // how far cell borders wander, in cells
	borderBlur  = 2    // vertices of soft blending between biomes
	accentScale = 14.0 // size of accent patches, tiles
	accentCover = 0.6  // noise threshold of accents (higher = fewer)
	hillScale   = 12.0 // size of hills, tiles
	hillHeight  = 0.45 // max height change of hills (ground is 2.0; 8 Kingdoms has 1.6 to 2.2)
	townFlat    = 9.0  // tiles kept flat around a town
)

// Generate expands a checked layout.
func Generate(l *layout.Layout) (*Result, error) {
	if err := l.Check(); err != nil {
		return nil, err
	}
	rnd := rand.New(rand.NewSource(l.Seed))
	size, grid := l.Size, l.Grid
	n := size + 1
	ter := terrain.New(size, rnd)
	cs := float64(size) / float64(grid) // tiles per cell
	seed := noise(rnd.Uint64())

	// Cell of each vertex, through a noise warp so that borders are not straight
	warpX, warpY := seed+1, seed+2
	cellOf := make([]int, n*n)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			fx, fy := float64(x)/(cs*1.5), float64(y)/(cs*1.5)
			wx := float64(x) + cs*borderWarp*(2*warpX.fbm(fx, fy, 3)-1)*2
			wy := float64(y) + cs*borderWarp*(2*warpY.fbm(fx, fy, 3)-1)*2
			cx := max(0, min(grid-1, int(wx/cs)))
			cy := max(0, min(grid-1, int(wy/cs)))
			cellOf[y*n+x] = cy*grid + cx
		}
	}

	// Biome masks, most used first: the first one is the base layer
	masks := map[string][]float64{}
	var order []string
	for i, c := range cellOf {
		id := l.Biomes[c]
		if masks[id] == nil {
			masks[id] = make([]float64, n*n)
			order = append(order, id)
		}
		masks[id][i] = 1
	}
	coverage := func(id string) (s float64) {
		for _, v := range masks[id] {
			s += v
		}
		return
	}
	sort.SliceStable(order, func(i, j int) bool { return coverage(order[i]) > coverage(order[j]) })
	for _, id := range order {
		masks[id] = blur(masks[id], n, borderBlur, 2)
	}

	used := map[string]bool{}
	var tiles []string
	addLayer := func(tile string, weight func(i int) float64) {
		layer := terrain.NewLayer(biome.Href(tile), size)
		any := false
		for i := range layer.Weights.Data {
			w := byte(math.Round(255 * math.Max(0, math.Min(1, weight(i)))))
			layer.Weights.Data[i] = w
			any = any || w > 0
		}
		if !any {
			return
		}
		ter.Layers = append(ter.Layers, layer)
		if href := biome.Href(tile); !used[href] {
			used[href] = true
			tiles = append(tiles, href)
		}
	}
	for k, id := range order {
		b, _ := biome.ByID(id)
		mask := masks[id]
		if k == 0 {
			addLayer(b.Base, func(int) float64 { return 1 })
		} else {
			addLayer(b.Base, func(i int) float64 { return mask[i] })
		}
	}
	for _, id := range order {
		b, _ := biome.ByID(id)
		mask := masks[id]
		for a, tile := range b.Accents {
			field := seed + noise(100+a)
			addLayer(tile, func(i int) float64 {
				x, y := float64(i%n), float64(i/n)
				v := field.fbm(x/accentScale, y/accentScale, 3)
				return mask[i] * smoothstep(accentCover, accentCover+0.08, v)
			})
		}
	}

	// Towns on the starts, facing the map centre
	var towns []Town
	for p, s := range l.Starts {
		tx := max(7, min(size-8, int(math.Round(s.X*cs))))
		ty := max(7, min(size-8, int(math.Round(s.Y*cs))))
		dx, dy := float64(size)/2-float64(tx), float64(size)/2-float64(ty)
		turns := 0 // south
		switch {
		case math.Abs(dx) > math.Abs(dy) && dx > 0:
			turns = 1
		case math.Abs(dx) > math.Abs(dy):
			turns = 3
		case dy > 0:
			turns = 2
		}
		towns = append(towns, Town{Player: p + 1, X: tx, Y: ty, Turns: turns})
	}
	if err := checkTowns(size, towns); err != nil {
		return nil, err
	}

	// Plateau levels, water and ramps; the base height of a vertex is its level / 8
	lvl, ramp, err := levels(l, size, cellOf, towns, rnd)
	if err != nil {
		return nil, err
	}
	walk := walkableTiles(size, lvl, ramp)
	if err := checkTownGround(size, towns, walk, lvl); err != nil {
		return nil, err
	}
	levelData, rampData := ter.Grids[terrain.TagLevel].Data, ter.Grids[terrain.TagRamp].Data
	for i, v := range lvl {
		levelData[i] = byte(v)
		if ramp[i] {
			rampData[i] = 1
		}
		ter.Height[i] = float32(v) / 8
	}

	// Lakebeds: the bed tile of the biome under water, fading out over about two tiles of shore. Biomes that
	// share a bed tile share its layer.
	water := make([]float64, n*n)
	anyWater := false
	for i, v := range lvl {
		if v == terrain.LevelWater {
			water[i], anyWater = 1, true
		}
	}
	if anyWater {
		shore := blur(append([]float64(nil), water...), n, 2, 1)
		var beds []string
		for _, id := range order {
			b, _ := biome.ByID(id)
			if !slices.Contains(beds, b.Bed) {
				beds = append(beds, b.Bed)
			}
		}
		for _, bed := range beds {
			addLayer(bed, func(i int) float64 {
				if b, _ := biome.ByID(l.Biomes[cellOf[i]]); b.Bed != bed {
					return 0
				}
				return math.Max(water[i], math.Min(1, shore[i]*2))
			})
		}
	}

	// Hills: noise heights where the relief is hills (on the ground level, off ramps), flat around the towns
	hills := make([]float64, n*n)
	for i, c := range cellOf {
		if l.Relief[c] == layout.Hills {
			hills[i] = 1
		}
	}
	hills = blur(hills, n, max(1, int(cs/3)), 3)
	hillField := seed + 3
	for i := range ter.Height {
		if hills[i] == 0 || lvl[i] != terrain.LevelGround || ramp[i] {
			continue
		}
		x, y := float64(i%n), float64(i/n)
		flat := 1.0
		for _, t := range towns {
			flat = math.Min(flat, smoothstep(townFlat, townFlat+5, math.Hypot(x-float64(t.X), y-float64(t.Y))))
		}
		v := 2*hillField.fbm(x/hillScale, y/hillScale, 3) - 1 // -1..1
		ter.Height[i] = float32(2 + hills[i]*flat*hillHeight*v*1.6)
	}

	// Roads between the towns, in the road tile of the biome under each vertex. Every corner of a road tile
	// is fully painted, so the tile counts as road whichever corner the game samples; a light fringe
	// softens the edges.
	var roads []bool
	var links [][2]int
	switch l.RoadMode() {
	case layout.RoadsAuto:
		links = AutoLinks(towns)
	case layout.RoadsCustom:
		links = cleanLinks(l.Links)
	}
	if len(links) > 0 {
		if roads, err = routeRoads(size, towns, links, ter.Height, walk, seed+4); err != nil {
			return nil, err
		}
		full := make([]float64, n*n)
		for t, on := range roads {
			if on {
				x, y := t%size, t/size
				full[y*n+x], full[y*n+x+1], full[(y+1)*n+x], full[(y+1)*n+x+1] = 1, 1, 1, 1
			}
		}
		fringe := blur(append([]float64(nil), full...), n, 1, 1)
		for _, id := range order {
			b, _ := biome.ByID(id)
			id := id
			addLayer(b.Road, func(i int) float64 {
				if l.Biomes[cellOf[i]] != id {
					return 0
				}
				return math.Max(full[i], fringe[i]*0.5)
			})
		}
	}

	return &Result{Terrain: ter, Tiles: tiles, Towns: towns, Roads: roads, Links: links, Rand: rnd}, nil
}
