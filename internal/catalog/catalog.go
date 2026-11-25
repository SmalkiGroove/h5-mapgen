// Package catalog holds the objects the generator can place: their shared definitions and footprints, read
// from the game paks by cmd/catalog into catalog.json (embedded, so the app never reads the paks).
package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// Object is an object placed by name (mines, treasures, random artifacts and monsters).
type Object struct {
	Href    string   `json:"href"`    // the Shared href, with its xpointer
	Type    string   `json:"type"`    // inline type in map.xdb (AdvMapMine, AdvMapTreasure, ...)
	Blocked [][2]int `json:"blocked"` // tiles relative to the position, at rotation 0
	Active  [][2]int `json:"active"`  // visit tiles
}

// Decor is a decoration (AdvMapStatic) of a biome.
type Decor struct {
	Href    string   `json:"href"`
	Group   string   `json:"group"` // subfolder: trees, rocks, bushes, ...
	Uses    int      `json:"uses"`  // times the game's maps place it
	Blocked [][2]int `json:"blocked"`
}

// Catalog is the content of catalog.json.
type Catalog struct {
	Objects map[string]Object  `json:"objects"`
	Decor   map[string][]Decor `json:"decor"` // by biome id
}

//go:embed catalog.json
var data []byte

// All is the embedded catalog.
var All = func() Catalog {
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		panic(fmt.Sprintf("catalog.json: %v", err))
	}
	return c
}()

// Get returns an object by key; it panics on an unknown key (a programming error).
func Get(key string) Object {
	o, ok := All.Objects[key]
	if !ok {
		panic("catalog: no object " + key)
	}
	return o
}

// DecorByHref finds a decoration of any biome by its href.
var DecorByHref = func() map[string]Decor {
	m := map[string]Decor{}
	for _, list := range All.Decor {
		for _, d := range list {
			m[d.Href] = d
		}
	}
	return m
}()
