package generate

import (
	"fmt"
	"math"
	"sort"

	"h5mapgen/internal/layout"
)

// buildingKind is a stat giver or a visitable building the generator places.
type buildingKind struct {
	key    string
	guard  int     // guard level before the Guards setting, 0 unguarded
	zone   float64 // nearest relative distance from the towns (0 at a town, 1 among the farthest tiles)
	weight int     // how often the game's maps place it
	biome  string  // "" anywhere, else only this deep inside this biome (biomeDepth)
}

// biomeDepth is how far from another biome a building restricted to a biome stands, in tiles.
const biomeDepth = 8

// statGivers raise a hero's skills once or every week; the H5X texts give their effects. They are placed
// by statUnits, not drawn by weight.
var statGivers = []buildingKind{
	{"stat_temple", 3, 0.3, 0, ""},               // experience to all heroes, every week
	{"stat_war_academy", 5, 0.5, 0, ""},          // +1 to the lowest attribute, every week
	{"stat_witch_hut", 3, 0.3, 0, ""},            // a brew for mercury, every week
	{"stat_mercenary_camp", 2, 0.2, 0, ""},       // +1 attack
	{"stat_marletto_tower", 2, 0.2, 0, ""},       // +1 defence
	{"stat_star_axis", 2, 0.2, 0, ""},            // +1 spellpower
	{"stat_garden_of_revelation", 2, 0.2, 0, ""}, // +1 knowledge
	{"stat_arena", 4, 0.45, 0, ""},               // +2 attack or defence
	{"stat_school_of_magic", 4, 0.45, 0, ""},     // +2 spellpower or knowledge
	{"stat_learning_stone", 1, 0, 0, ""},         // +1000 experience
	{"stat_tree_of_knowledge", 4, 0.45, 0, ""},   // a level
	{"stat_tomb_of_the_warrior", 3, 0.35, 0, ""}, // +1 attack and defence and 3000 experience, outside parity
	{"stat_library", 5, 0.6, 0, ""},              // +2 to every attribute for experienced heroes: neutral areas only
}

// Stat givers come in units that a town gets whole, so that the attributes stay at par: the four +1
// buildings together, the Arena with the School of Magic. Weekly stat givers come first.
var (
	weeklyStats  = []string{"stat_temple", "stat_war_academy", "stat_witch_hut"}
	plusOneStats = []string{"stat_mercenary_camp", "stat_marletto_tower", "stat_star_axis", "stat_garden_of_revelation"}
	plusTwoStats = []string{"stat_arena", "stat_school_of_magic"}
	otherStats   = [][]string{{"stat_learning_stone"}, {"stat_tree_of_knowledge"}, {"stat_tomb_of_the_warrior"}}
)

// visitBuildings are the other buildings: weekly resources, mana, luck and morale, movement, spells,
// trade.
var visitBuildings = []buildingKind{
	{"building_windmill", 2, 0.2, 85, ""},
	{"building_water_wheel", 2, 0.2, 40, ""},
	{"building_mystical_garden", 1, 0.1, 51, ""},
	{"building_magic_well", 0, 0, 204, ""},
	{"building_magic_spring", 2, 0.4, 36, ""},
	{"building_rally_flag", 0, 0, 189, ""},
	{"building_idol_of_fortune", 0, 0, 122, ""},
	{"building_fountain_of_fortune", 0, 0, 115, ""},
	{"building_faerie_ring", 0, 0, 37, ""},
	{"building_fountain_of_youth", 0, 0.1, 79, ""},
	{"building_stables", 0, 0, 80, ""},
	{"building_redwood_observatory", 0, 0, 118, ""},
	{"building_shrine_1", 1, 0, 93, ""},
	{"building_shrine_2", 3, 0.3, 99, ""},
	{"building_shrine_3", 5, 0.6, 73, ""},
	{"building_trading_post", 0, 0, 54, ""},
	{"building_tavern", 0, 0.2, 20, ""},
	{"building_black_market", 3, 0.4, 70, ""},
	{"building_sanctuary", 0, 0.2, 42, ""},
}

// dwellingKinds are the random dwellings of a player's area, by level, and the military post: linked to the
// player, they take the faction of its town (the military post is the Haven one, turned like the random
// dwellings). Random dwellings of levels 4 to 7 exist too, but what they turn into is not known.
var dwellingKinds = []buildingKind{
	{"dwelling_1", 2, 0.15, 0, ""},
	{"dwelling_2", 3, 0.3, 0, ""},
	{"dwelling_3", 4, 0.45, 0, ""},
	{"dwelling_military_post", 4, 0.5, 0, ""},
}

// isRandomDwelling tells whether a key is a random dwelling of dwellingKinds.
func isRandomDwelling(key string) bool {
	for _, d := range dwellingKinds {
		if d.key == key {
			return true
		}
	}
	return false
}

// neutralDwellings recruit creatures of every faction: neutral areas only.
var neutralDwellings = []buildingKind{
	{"dwelling_refugee_camp", 5, 0.5, 0, ""},      // high level creatures, a new kind every week
	{"dwelling_elemental_conflux", 4, 0.5, 0, ""}, // elementals
}

// banks fight with their own creatures (no guard), the stronger the farther from the towns.
var banks = []buildingKind{
	{"bank_crypt", 0, 0.3, 56, ""},
	{"bank_treant_thicket", 0, 0.35, 20, ""},
	{"bank_gargoyle_stonevault", 0, 0.4, 21, ""},
	{"bank_dwarven_treasury", 0, 0.45, 15, ""},
	{"bank_naga_bank", 0, 0.5, 36, ""},
	{"bank_elemental_stockpile", 0, 0.5, 38, ""},
	{"bank_cyclops_stockpile", 0, 0.55, 20, ""},
	{"bank_blood_temple", 0, 0.55, 12, ""},
}

// vaults are the strongest banks: neutral areas only.
var vaults = []buildingKind{
	{"bank_pyramid", 0, 0.6, 6, "sand"},   // a spell; sand only
	{"bank_dragon_utopia", 0, 0.6, 9, ""}, // dragons
}

var buildingKinds = func() map[string]buildingKind {
	m := map[string]buildingKind{}
	for _, set := range [][]buildingKind{statGivers, visitBuildings, dwellingKinds, neutralDwellings, banks, vaults} {
		for _, b := range set {
			m[b.key] = b
		}
	}
	return m
}()

// Building placement
const (
	statRate    = 1.0 / 450  // stat givers per reachable tile at Normal (9 per town on a 136 map with 3 players)
	visitRate   = 1.0 / 650  // other buildings per reachable tile at Normal
	bankRate    = 1.0 / 1300 // banks per reachable tile at Normal (3 per town on a 136 map with 3 players)
	buildingGap = 8          // tiles between two buildings
	sameKindGap = 24         // tiles between two buildings of the same kind
	zoneWidth   = 0.45       // a building goes in [zone, zone + zoneWidth] of relative distance when it can
	neutralBand = 0.15       // a neutral tile is at most this much (relative) nearer to its town than to another
)

// buildingCount is the number of buildings of a set per town for an amount setting (Low 0.6 times Normal,
// High 1.6 times).
func buildingCount(reachable, towns int, rate float64, amount int) int {
	if amount == 0 || towns == 0 {
		return 0
	}
	scale := []float64{0, 0.6, 1, 1.6}[amount]
	return max(1, int(math.Round(float64(reachable)*rate*scale/float64(towns))))
}

// farDistance is the relative distance scale of the map: the 95th percentile of the walking distance to
// the nearest town.
func (c *content) farDistance() float64 {
	var far []int
	for i, k := range c.near {
		if k >= 0 {
			far = append(far, c.dist[k][i])
		}
	}
	if len(far) == 0 {
		return 1
	}
	sort.Ints(far)
	return float64(max(1, far[len(far)*95/100]))
}

func (c *content) reachable() int {
	n := 0
	for _, k := range c.near {
		if k >= 0 {
			n++
		}
	}
	return n
}

// statBuildings gives every town the same stat givers, in units (see statUnits) up to a number per town:
// a weekly one, the four +1, a weekly one, the Arena and the School of Magic, a weekly one, the Learning
// Stone, the Tree of Knowledge, and again. A unit that doesn't fit in what is left is skipped.
func (c *content) statBuildings() {
	budget := buildingCount(c.reachable(), len(c.towns), statRate, layout.Amount(c.l.Stats))
	if budget == 0 {
		return
	}
	weekly := append([]string(nil), weeklyStats...)
	c.rnd.Shuffle(len(weekly), func(i, j int) { weekly[i], weekly[j] = weekly[j], weekly[i] })
	units := [][]string{{weekly[0]}, plusOneStats, {weekly[1]}, plusTwoStats, {weekly[2]}}
	units = append(units, otherStats...)
	far := c.farDistance()
	for pass := 0; pass < 3 && budget > 0; pass++ {
		for _, unit := range units {
			if len(unit) > budget {
				continue
			}
			budget -= len(unit)
			for k := range c.towns {
				c.placeUnit(k, unit, far)
			}
		}
	}
}

// placeUnit places a unit of stat givers in town k's part of the map, all of them or none.
func (c *content) placeUnit(k int, unit []string, far float64) {
	var placed []int
	for _, key := range unit {
		if !c.placeBuilding(k, buildingKinds[key], far) {
			for _, i := range placed {
				c.remove(i)
			}
			c.warnings = append(c.warnings, fmt.Sprintf("player %d: no room for %s, left out with its set %v", c.towns[k].Player, key, unit))
			return
		}
		placed = append(placed, len(c.objects)-1)
	}
}

// dwellings gives every town dwellings of its faction: level 1 at Low, levels 1 to 3 and a military post at
// Normal, two of levels 1 and 2, one of level 3 and a military post at High.
func (c *content) dwellings() {
	keys := [][]string{nil, {"dwelling_1"}, {"dwelling_1", "dwelling_2", "dwelling_3", "dwelling_military_post"},
		{"dwelling_1", "dwelling_1", "dwelling_2", "dwelling_2", "dwelling_3", "dwelling_military_post"}}[layout.Amount(c.l.Dwellings)]
	far := c.farDistance()
	for _, key := range keys {
		kind := buildingKinds[key]
		for k := range c.towns {
			if c.placeBuilding(k, kind, far) {
				c.objects[len(c.objects)-1].Player = c.towns[k].Player
			} else {
				c.warnings = append(c.warnings, fmt.Sprintf("player %d: no room for %s", c.towns[k].Player, kind.key))
			}
		}
	}
}

// neutralCount is the number of neutral buildings of a setting: none at Low, 1 at Normal, 2 at High.
func neutralCount(setting string) int { return []int{0, 0, 1, 2}[layout.Amount(setting)] }

// neutral places count buildings in neutral areas: tiles about as far from two towns, away from all of
// them (none on a map of one town). The kinds take turns, in a random order.
func (c *content) neutral(set []buildingKind, count int) {
	if count == 0 || len(c.towns) < 2 {
		return
	}
	order := c.rnd.Perm(len(set))
	for n := 0; n < count; n++ {
		placed := false
		for k := 0; k < len(set) && !placed; k++ { // another kind when this one has no room
			placed = c.neutralOne(set[order[(n+k)%len(set)]])
		}
		if !placed {
			c.warnings = append(c.warnings, "no neutral area for "+set[order[n%len(set)]].key)
		}
	}
}

// neutralOne places a building on a neutral tile.
func (c *content) neutralOne(kind buildingKind) bool {
	margin := guardMargin
	if kind.guard == 0 {
		margin = 1
	}
	far := c.farDistance()
	var tiles []int
	for i, k := range c.near {
		if k < 0 || c.occ[i] != tFree {
			continue
		}
		d1, d2 := c.dist[k][i], unreached
		for j := range c.towns {
			if j != k {
				d2 = min(d2, c.dist[j][i])
			}
		}
		if d2 < unreached && float64(d2-d1) <= neutralBand*far && float64(d1) >= kind.zone*far && c.inBiome(kind, i) {
			tiles = append(tiles, i)
		}
	}
	c.rnd.Shuffle(len(tiles), func(a, b int) { tiles[a], tiles[b] = tiles[b], tiles[a] })
	for _, i := range tiles {
		x, y := i%c.size, i/c.size
		if !c.buildingRoom(kind.key, x, y) {
			continue
		}
		if turns := c.bestTurns(kind.key, x, y, margin); turns >= 0 && c.putReachable(kind.key, x, y, turns) {
			return true
		}
	}
	return false
}

// buildings places a set of buildings, the same kinds for every town, in each town's part of the map: a
// round picks a kind by weight, then puts one in every town's part at its relative distance.
func (c *content) buildings(set []buildingKind, rate float64, setting string) {
	rounds := buildingCount(c.reachable(), len(c.towns), rate, layout.Amount(setting))
	if rounds == 0 {
		return
	}
	far := c.farDistance()
	var left []buildingKind // kinds not drawn yet: a kind comes back only once every kind was drawn
	for r := 0; r < rounds; r++ {
		if len(left) == 0 {
			left = append(left, set...)
		}
		total := 0
		for _, b := range left {
			total += b.weight
		}
		pick, at := c.rnd.Intn(total), 0
		for i, b := range left {
			if pick -= b.weight; pick < 0 {
				at = i
				break
			}
		}
		kind := left[at]
		left = append(left[:at], left[at+1:]...)
		for k := range c.towns {
			if !c.placeBuilding(k, kind, far) {
				c.warnings = append(c.warnings, fmt.Sprintf("player %d: no room for %s", c.towns[k].Player, kind.key))
			}
		}
	}
}

// placeBuilding puts a building in town k's part of the map, in its distance zone when it can, then
// anywhere in that part.
func (c *content) placeBuilding(k int, kind buildingKind, far float64) bool {
	margin := guardMargin
	if kind.guard == 0 {
		margin = 1
	}
	var tiles []int
	for i, near := range c.near {
		if near == k && c.occ[i] == tFree {
			tiles = append(tiles, i)
		}
	}
	c.rnd.Shuffle(len(tiles), func(a, b int) { tiles[a], tiles[b] = tiles[b], tiles[a] })
	for _, zone := range [][2]float64{{kind.zone, kind.zone + zoneWidth}, {kind.zone * 0.6, 2}, {0, 2}} {
		for _, i := range tiles {
			f := float64(c.dist[k][i]) / far
			x, y := i%c.size, i/c.size
			if f < zone[0] || f > zone[1] || !c.buildingRoom(kind.key, x, y) {
				continue
			}
			if turns := c.bestTurns(kind.key, x, y, margin); turns >= 0 && c.putReachable(kind.key, x, y, turns) {
				return true
			}
		}
	}
	return false
}

// buildingRoom tells whether a building at (x, y) is far enough from the other buildings and from those of
// its kind.
func (c *content) buildingRoom(key string, x, y int) bool {
	for _, o := range c.objects {
		if o.Key == "" || !building(o.Key) {
			continue
		}
		d := max(abs(int(o.X)-x), abs(int(o.Y)-y))
		if d < buildingGap || (o.Key == key && d < sameKindGap) {
			return false
		}
	}
	return true
}

// inBiome tells whether a building restricted to a biome may stand on tile i: biomeDepth tiles inside it.
func (c *content) inBiome(kind buildingKind, i int) bool {
	if kind.biome == "" {
		return true
	}
	if c.depth == nil {
		c.depth = map[string][]int{}
	}
	d, ok := c.depth[kind.biome]
	if !ok {
		n := c.size + 1
		other := make([]bool, c.size*c.size)
		for t := range other {
			x, y := t%c.size, t/c.size
			for _, v := range [4]int{y*n + x, y*n + x + 1, (y+1)*n + x, (y+1)*n + x + 1} {
				other[t] = other[t] || c.l.Biomes[c.cellOf[v]] != kind.biome
			}
		}
		d = chebyshevDistance(other, c.size)
		c.depth[kind.biome] = d
	}
	return d[i] >= biomeDepth
}
