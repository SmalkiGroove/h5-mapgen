// Package biome lists the ground types a layout cell can take, with the tile textures each one paints.
package biome

import "fmt"

// Biome is a ground type: a base tile, accent tiles scattered over it in patches, and a road tile.
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
}

// All biomes, in palette order. The first one is the default ground.
var All = []Biome{
	{ID: "grass", Name: "Grass", Color: "#5c8f3e", Base: "Grass/Grass",
		Accents: []string{"Grass/Dark_Grass", "Grass/Flowers", "Grass/Field"}, Road: "Grass/StoneRoad"},
	{ID: "dirt", Name: "Dirt", Color: "#8a6a45", Base: "Dirt/Ground",
		Accents: []string{"Dirt/DarkGround", "Dirt/Weed", "Dirt/Ground 2"}, Road: "Dirt/DirtRoad"},
	{ID: "sand", Name: "Sand", Color: "#d6be78", Base: "Sand/Dunes",
		Accents: []string{"Sand/Sand_Rock", "Sand/Sand_Cracked", "Sand/Sand_Dirt"}, Road: "Sand/SandRoad"},
	{ID: "snow", Name: "Snow", Color: "#e4ebf0", Base: "Snow/Snow",
		Accents: []string{"Snow/Snow_(2)", "Snow/Snow_(3)"}, Road: "Snow/SnowRoad2"},
	{ID: "steppe", Name: "Steppe", Color: "#7f9a4a", Base: "Orc_Terrain/Nidle_Grass",
		Accents: []string{"Orc_Terrain/Moss", "Orc_Terrain/Nidle_Dirt", "Orc_Terrain/Scatter_stone_Grass"}, Road: "Orc_Terrain/Nidle_Road"},
	{ID: "deadland", Name: "Dead land", Color: "#6e3a2c", Base: "Lava/Dead_Land",
		Accents: []string{"Lava/Dead_Land_2", "Lava/Dead_Land_3", "Lava/Dead_Land_4"}, Road: "Lava/LavaRoad"},
	{ID: "subterrain", Name: "Subterranean", Color: "#5f5068", Base: "SubTerrain/SubTerrain",
		Accents: []string{"SubTerrain/SubTerrain_(2)", "SubTerrain/SubTerrain_(3)"}, Road: "SubTerrain/SubTerrainRoad"},
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
