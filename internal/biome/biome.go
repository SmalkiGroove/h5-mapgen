// Package biome lists the ground types a layout cell can take, with the tile textures each one paints.
package biome

import "fmt"

// Biome is a ground type: a base tile, accent tiles scattered over it in patches, a road tile, and the bed
// tile under water and on the shores (the vanilla maps mostly have Sand/River-bed under their water).
//
// Road tiles must have a road Type in their AdvMapTile (TT_COBBLESTONE_ROAD, TT_DIRT_ROAD, TT_GRAVEL_ROAD) to
// give the movement bonus: Grass/Road looks like a road but is TT_GRASS, Snow/SnowRoad is TT_SNOW (and hidden
// in the editor).
type Biome struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Color   string   `json:"color"` // painter colour
	Base    string   `json:"base"`  // tile names relative to /MapObjects/_(AdvMapTile)/
	Accents []string `json:"accents"`
	Road    string   `json:"road"`
	Bed     string   `json:"bed"`
}

// PathTile is the dirt path from the buildings to the roads, on every biome. It is TT_GRASS (no movement
// bonus), named Road in the editor's grass tiles.
const PathTile = "Grass/Road"

// SeaBed is the ground under the water and on the beaches, DeepSeaBed the darker one away from the coasts.
// They are the textures the game's maps use under water (with Grass/River-bed_grass): no ground texture
// may show there.
const (
	SeaBed     = "Sand/River-bed"
	DeepSeaBed = "Water/Water"
)

// All biomes, in palette order. The first one is the default ground.
var All = []Biome{
	{ID: "grass", Name: "Grass", Color: "#5c8f3e", Base: "Grass/Grass",
		Accents: []string{"Grass/Dark_Grass", "Grass/Flowers", "Grass/Field"}, Road: "Grass/StoneRoad", Bed: "Sand/River-bed"},
	{ID: "dirt", Name: "Dirt", Color: "#8a6a45", Base: "Dirt/Ground",
		Accents: []string{"Dirt/DarkGround", "Dirt/Weed", "Dirt/Ground 2"}, Road: "Dirt/DirtRoad", Bed: "Sand/River-bed"},
	{ID: "sand", Name: "Sand", Color: "#d6be78", Base: "Sand/Dunes",
		Accents: []string{"Sand/Sand_Rock", "Sand/Sand_Cracked", "Sand/Sand_Dirt"}, Road: "Sand/SandRoad", Bed: "Sand/River-bed"},
	{ID: "snow", Name: "Snow", Color: "#e4ebf0", Base: "Snow/Snow",
		Accents: []string{"Snow/Snow_(2)", "Snow/Snow_(3)"}, Road: "Snow/SnowRoad2", Bed: "Sand/River-bed"},
	{ID: "steppe", Name: "Steppe", Color: "#7f9a4a", Base: "Orc_Terrain/Nidle_Grass",
		Accents: []string{"Orc_Terrain/Moss", "Orc_Terrain/Nidle_Dirt", "Orc_Terrain/Scatter_stone_Grass"}, Road: "Orc_Terrain/Nidle_Road", Bed: "Sand/River-bed"},
	// the Inferno ground (its id is kept for saved layouts): only for an Inferno player's area, never mixed
	// into other ground by the random layouts
	{ID: "deadland", Name: "Inferno", Color: "#6e3a2c", Base: "Lava/Dead_Land",
		Accents: []string{"Lava/Dead_Land_2", "Lava/Dead_Land_3", "Lava/Dead_Land_4"}, Road: "Lava/LavaRoad", Bed: "Sand/River-bed"},
	{ID: "subterrain", Name: "Subterranean", Color: "#5f5068", Base: "SubTerrain/SubTerrain",
		Accents: []string{"SubTerrain/SubTerrain_(2)", "SubTerrain/SubTerrain_(3)"}, Road: "SubTerrain/SubTerrainRoad", Bed: "Sand/River-bed"},
}

// ByID returns the biome with this id.
func ByID(id string) (*Biome, error) {
	for i := range All {
		if All[i].ID == id {
			return &All[i], nil
		}
	}
	return nil, fmt.Errorf("unknown biome %q", id)
}

// Href is the reference of a tile texture, as written in the terrain file and in map.xdb.
func Href(tile string) string {
	return "/MapObjects/_(AdvMapTile)/" + tile + ".xdb#xpointer(/AdvMapTile)"
}
