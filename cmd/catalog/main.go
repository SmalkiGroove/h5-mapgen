// Command catalog reads the game paks and writes internal/catalog/catalog.json: the shared definition and
// footprint of every object the generator places, and the decorations of each biome with how often the
// game's maps use them.
//
//	go run ./cmd/catalog [GAME_FOLDER]     (default G:/__H5__/H5Xdev)
package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"h5mapgen/internal/catalog"
)

// fixed lists the objects placed by name: catalog key, shared file, inline type.
var fixed = []struct{ key, file, typ string }{
	{"mine_wood", "MapObjects/Sawmill.(AdvMapMineShared).xdb", "AdvMapMine"},
	{"mine_ore", "MapObjects/Ore_Pit.(AdvMapMineShared).xdb", "AdvMapMine"},
	{"mine_mercury", "MapObjects/Alchemist_Lab.(AdvMapMineShared).xdb", "AdvMapMine"},
	{"mine_crystal", "MapObjects/Crystal_Cavern.(AdvMapMineShared).xdb", "AdvMapMine"},
	{"mine_sulfur", "MapObjects/Sulfur_Dune.(AdvMapMineShared).xdb", "AdvMapMine"},
	{"mine_gem", "MapObjects/Gem_Pond.(AdvMapMineShared).xdb", "AdvMapMine"},
	{"mine_gold", "MapObjects/Gold_Mine.(AdvMapMineShared).xdb", "AdvMapMine"},
	{"mine_abandoned", "MapObjects/Abandoned_Mine.(AdvMapAbanMineShared).xdb", "AdvMapAbanMine"},
	{"res_wood", "MapObjects/Wood.(AdvMapTreasureShared).xdb", "AdvMapTreasure"},
	{"res_ore", "MapObjects/Ore.(AdvMapTreasureShared).xdb", "AdvMapTreasure"},
	{"res_mercury", "MapObjects/Mercury.(AdvMapTreasureShared).xdb", "AdvMapTreasure"},
	{"res_crystal", "MapObjects/Crystal.(AdvMapTreasureShared).xdb", "AdvMapTreasure"},
	{"res_sulfur", "MapObjects/Sulfur.(AdvMapTreasureShared).xdb", "AdvMapTreasure"},
	{"res_gem", "MapObjects/Gems.(AdvMapTreasureShared).xdb", "AdvMapTreasure"},
	{"res_gold", "MapObjects/Gold.(AdvMapTreasureShared).xdb", "AdvMapTreasure"},
	{"chest", "MapObjects/Chest.(AdvMapTreasureShared).xdb", "AdvMapTreasure"},
	{"campfire", "MapObjects/Campfire.(AdvMapTreasureShared).xdb", "AdvMapTreasure"},
	{"artifact_minor", "MapObjects/Random/Random-Minor.(AdvMapArtifactShared).xdb", "AdvMapArtifact"},
	{"artifact_major", "MapObjects/Random/Random-Major.(AdvMapArtifactShared).xdb", "AdvMapArtifact"},
	{"artifact_relic", "MapObjects/Random/Random-Relic.(AdvMapArtifactShared).xdb", "AdvMapArtifact"},
	{"monster_1", "MapObjects/Random/Random-Monster-L1.(AdvMapMonsterShared).xdb", "AdvMapMonster"},
	{"monster_2", "MapObjects/Random/Random-Monster-L2.(AdvMapMonsterShared).xdb", "AdvMapMonster"},
	{"monster_3", "MapObjects/Random/Random-Monster-L3.(AdvMapMonsterShared).xdb", "AdvMapMonster"},
	{"monster_4", "MapObjects/Random/Random-Monster-L4.(AdvMapMonsterShared).xdb", "AdvMapMonster"},
	{"monster_5", "MapObjects/Random/Random-Monster-L5.(AdvMapMonsterShared).xdb", "AdvMapMonster"},
	{"monster_6", "MapObjects/Random/Random-Monster-L6.(AdvMapMonsterShared).xdb", "AdvMapMonster"},
	{"monster_7", "MapObjects/Random/Random-Monster-L7.(AdvMapMonsterShared).xdb", "AdvMapMonster"},
}

// decorFolders maps a biome to the decoration folder of /MapObjects/; skipped lists subfolders that are not
// scenery (arena props, bridges, lake meshes, light rays).
var decorFolders = map[string]string{
	"grass": "grass", "dirt": "dirt", "sand": "sand", "snow": "snow", "steppe": "taiga", "deadland": "lava",
	"subterrain": "subterra",
}
var skipped = map[string]bool{"combat_arena": true, "bridges": true, "lakes": true, "rays": true, "ice": true}

const minUses = 8 // decorations used fewer times on the game's maps are left out

type pakFile struct {
	zip  *zip.ReadCloser
	file *zip.File
}

var (
	reSection = regexp.MustCompile(`(?s)<(blockedTiles|activeTiles)>(.*?)</(?:blockedTiles|activeTiles)>`)
	rePoint   = regexp.MustCompile(`<x>(-?\d+)</x>\s*<y>(-?\d+)</y>`)
	reStatic  = regexp.MustCompile(`(?s)inline\(AdvMapStatic\).*?<Shared href="([^"#]*)`)
	rePair    = regexp.MustCompile(`\[\s+(-?\d+),\s+(-?\d+)\s+\]`)
	rePairs   = regexp.MustCompile(`\[(\s*\[-?\d+,-?\d+\],?)+\s*\]`) // a list of points, on one line
	reSpace   = regexp.MustCompile(`\s+`)
)

func main() {
	game := "G:/__H5__/H5Xdev"
	if len(os.Args) > 1 {
		game = os.Args[1]
	}
	// Later paks override earlier ones: vanilla data first, then the mod
	paks, _ := filepath.Glob(filepath.Join(game, "data", "*.pak"))
	sort.SliceStable(paks, func(i, j int) bool { return rank(paks[i]) < rank(paks[j]) })
	files := map[string]pakFile{} // lower-case path -> file
	var maps []*zip.File
	for _, p := range paks {
		z, err := zip.OpenReader(p)
		check(err)
		for _, f := range z.File {
			name := strings.ToLower(strings.ReplaceAll(f.Name, "\\", "/"))
			files[name] = pakFile{z, f}
			if strings.HasPrefix(name, "maps/") && strings.HasSuffix(name, "/map.xdb") {
				maps = append(maps, f)
			}
		}
	}
	h5ms, _ := filepath.Glob(filepath.Join(game, "Maps", "*.h5m"))
	for _, p := range h5ms {
		z, err := zip.OpenReader(p)
		check(err)
		for _, f := range z.File {
			if strings.HasSuffix(strings.ToLower(f.Name), "/map.xdb") {
				maps = append(maps, f)
			}
		}
	}

	read := func(path string) (string, error) {
		pf, ok := files[strings.ToLower(strings.TrimPrefix(path, "/"))]
		if !ok {
			return "", fmt.Errorf("%s not found in the paks", path)
		}
		rc, err := pf.file.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		return strings.ReplaceAll(string(data), "\r", ""), err
	}
	footprint := func(path string) (blocked, active [][2]int, root string, err error) {
		s, err := read(path)
		if err != nil {
			return nil, nil, "", err
		}
		for _, m := range reSection.FindAllStringSubmatch(s, -1) {
			for _, p := range rePoint.FindAllStringSubmatch(m[2], -1) {
				x, _ := strconv.Atoi(p[1])
				y, _ := strconv.Atoi(p[2])
				if m[1] == "blockedTiles" {
					blocked = append(blocked, [2]int{x, y})
				} else {
					active = append(active, [2]int{x, y})
				}
			}
		}
		if i := strings.Index(s, "?>"); i >= 0 {
			s = s[i+2:]
		}
		if m := regexp.MustCompile(`<(\w+)`).FindStringSubmatch(s); m != nil {
			root = m[1]
		}
		return blocked, active, root, nil
	}

	cat := catalog.Catalog{Objects: map[string]catalog.Object{}, Decor: map[string][]catalog.Decor{}}
	for _, o := range fixed {
		blocked, active, root, err := footprint(o.file)
		check(err)
		cat.Objects[o.key] = catalog.Object{Href: "/" + o.file + "#xpointer(/" + root + ")", Type: o.typ, Blocked: blocked, Active: active}
	}

	uses := map[string]int{}
	for _, f := range maps {
		rc, err := f.Open()
		check(err)
		data, err := io.ReadAll(rc)
		rc.Close()
		check(err)
		for _, m := range reStatic.FindAllStringSubmatch(string(data), -1) {
			uses[strings.ToLower(strings.TrimPrefix(m[1], "/"))]++
		}
	}
	for biome, folder := range decorFolders {
		var list []catalog.Decor
		for path, n := range uses {
			parts := strings.Split(path, "/")
			if n < minUses || len(parts) < 3 || parts[0] != "mapobjects" || parts[1] != folder {
				continue
			}
			if len(parts) > 3 && skipped[parts[2]] {
				continue
			}
			pf, ok := files[path]
			if !ok {
				continue
			}
			blocked, active, root, err := footprint(path)
			if err != nil || len(active) > 0 || root == "" {
				continue // decorations have no visit tile
			}
			list = append(list, catalog.Decor{
				Href:    "/" + pf.file.Name + "#xpointer(/" + root + ")",
				Group:   group(parts),
				Uses:    n,
				Blocked: blocked,
			})
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Uses > list[j].Uses || (list[i].Uses == list[j].Uses && list[i].Href < list[j].Href) })
		cat.Decor[biome] = list
	}

	data, err := json.MarshalIndent(cat, "", " ")
	check(err)
	data = rePair.ReplaceAll(data, []byte("[$1,$2]")) // one point per line
	data = rePairs.ReplaceAllFunc(data, func(m []byte) []byte { return reSpace.ReplaceAll(m, nil) })
	out := filepath.Join("internal", "catalog", "catalog.json")
	check(os.WriteFile(out, append(data, '\n'), 0o644))
	fmt.Printf("%s: %d objects, %d maps read\n", out, len(cat.Objects), len(maps))
	for _, b := range []string{"grass", "dirt", "sand", "snow", "steppe", "deadland", "subterrain"} {
		big, small, flat := 0, 0, 0
		for _, d := range cat.Decor[b] {
			switch n := len(d.Blocked); {
			case n == 0:
				flat++
			case n <= 4:
				small++
			default:
				big++
			}
		}
		fmt.Printf("  %-10s %3d decorations: %d large, %d small, %d not blocking\n", b, len(cat.Decor[b]), big, small, flat)
	}
}

// group is the subfolder of a decoration (trees, rocks, …), or its folder when it sits at the top.
func group(parts []string) string {
	if len(parts) > 3 {
		return parts[2]
	}
	return parts[1]
}

// rank orders the paks as the game overrides them: vanilla first, then the mod.
func rank(path string) int {
	switch strings.ToLower(filepath.Base(path)) {
	case "data.pak":
		return 0
	case "a2p1-data.pak":
		return 1
	}
	if strings.HasPrefix(strings.ToLower(filepath.Base(path)), "h5x-") {
		return 3
	}
	return 2
}

func check(err error) {
	if err != nil {
		fmt.Println("ERROR", err)
		os.Exit(1)
	}
}
