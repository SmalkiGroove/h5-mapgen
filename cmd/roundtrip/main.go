// Command roundtrip reads every terrain file it finds and checks that writing it back gives the same bytes.
//
//	go run ./cmd/roundtrip [GAME_FOLDER]     (default G:/__H5__/H5Xdev)
//
// Sources: the maps of <game>/Maps/*.h5m, the maps inside the game paks (data/*.pak), and the last random
// map (data/RMGTemp/CurrentMap).
package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"h5mapgen/internal/terrain"
)

type source struct {
	name string
	data []byte
}

func terrainFiles(game string) ([]source, error) {
	var out []source
	h5m, _ := filepath.Glob(filepath.Join(game, "Maps", "*.h5m"))
	paks, _ := filepath.Glob(filepath.Join(game, "data", "*.pak"))
	sort.Strings(h5m)
	sort.Strings(paks)
	for _, path := range append(h5m, paks...) {
		r, err := zip.OpenReader(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		for _, f := range r.File {
			if !strings.HasSuffix(strings.ToLower(f.Name), "terrain.bin") {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, err
			}
			out = append(out, source{filepath.Base(path) + ":" + f.Name, data})
		}
		r.Close()
	}
	rmg, _ := filepath.Glob(filepath.Join(game, "data", "RMGTemp", "CurrentMap", "*Terrain.bin"))
	for _, path := range rmg {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, source{path, data})
	}
	return out, nil
}

func main() {
	game := "G:/__H5__/H5Xdev"
	if len(os.Args) > 1 {
		game = os.Args[1]
	}
	files, err := terrainFiles(game)
	if err != nil {
		fmt.Println("ERROR", err)
		os.Exit(1)
	}
	failed := 0
	for _, f := range files {
		t, err := terrain.Parse(f.data)
		if err != nil {
			failed++
			fmt.Printf("ERROR %s: %v\n", f.name, err)
			continue
		}
		out := t.Bytes()
		if !bytes.Equal(out, f.data) {
			failed++
			first := 0
			for first < len(out) && first < len(f.data) && out[first] == f.data[first] {
				first++
			}
			fmt.Printf("DIFF  %s: %d -> %d bytes, first difference at %#x\n", f.name, len(f.data), len(out), first)
		}
	}
	fmt.Printf("%d/%d terrain files rebuilt byte for byte\n", len(files)-failed, len(files))
	if failed > 0 {
		os.Exit(1)
	}
}
