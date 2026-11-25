// Command terrainsurvey prints, for every terrain of the game, how the plateau levels ([7]), ramps ([8]),
// field [10] and heights are used, to check assumptions about the format across all maps, and which
// textures lie under water and on the shores.
//
//	go run ./cmd/terrainsurvey [GAME_FOLDER]     (default G:/__H5__/H5Xdev)
package main

import (
	"archive/zip"
	"fmt"
	"io"
	"math"
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

func terrainFiles(game string) []source {
	var out []source
	h5m, _ := filepath.Glob(filepath.Join(game, "Maps", "*.h5m"))
	paks, _ := filepath.Glob(filepath.Join(game, "data", "*.pak"))
	sort.Strings(h5m)
	sort.Strings(paks)
	for _, path := range append(h5m, paks...) {
		r, err := zip.OpenReader(path)
		if err != nil {
			continue
		}
		for _, f := range r.File {
			if !strings.HasSuffix(strings.ToLower(f.Name), "terrain.bin") {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				continue
			}
			data, _ := io.ReadAll(rc)
			rc.Close()
			out = append(out, source{filepath.Base(path) + ":" + f.Name, data})
		}
		r.Close()
	}
	return out
}

func main() {
	game := "G:/__H5__/H5Xdev"
	if len(os.Args) > 1 {
		game = os.Args[1]
	}
	levels := map[int]int{}
	steps := map[int]int{} // level difference between 4-neighbours outside ramps
	rampLevels := map[int]int{}
	underWater, shore := map[string]int{}, map[string]int{} // top texture (last layer weighing >= 128)
	var waterHeight, groundDev [2]float64 = [2]float64{math.Inf(1), math.Inf(-1)}, [2]float64{math.Inf(1), math.Inf(-1)}
	for _, f := range terrainFiles(game) {
		t, err := terrain.Parse(f.data)
		if err != nil {
			continue
		}
		g7, g8, g10 := t.Grids[7], t.Grids[8], t.Grids[10]
		n := t.Size + 1
		if len(g7.Data) != n*n || len(t.Height) != n*n {
			continue
		}
		top := func(i int) string {
			name := "-"
			for _, l := range t.Layers {
				if len(l.Weights.Data) == n*n && l.Weights.Data[i] >= 128 {
					name = strings.TrimSuffix(strings.TrimPrefix(l.Href, "/MapObjects/_(AdvMapTile)/"), ".xdb#xpointer(/AdvMapTile)")
				}
			}
			return name
		}
		for i, v := range g7.Data {
			x, y := i%n, i/n
			if v == 0 {
				underWater[top(i)]++
				continue
			}
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				if xx, yy := x+d[0], y+d[1]; xx >= 0 && yy >= 0 && xx < n && yy < n && g7.Data[yy*n+xx] == 0 {
					shore[top(i)]++
					break
				}
			}
		}
		nonMul, ramps, mask := 0, 0, 0
		local := map[int]int{}
		for i, v := range g7.Data {
			levels[int(v)]++
			local[int(v)]++
			if v%8 != 0 {
				nonMul++
			}
			h := float64(t.Height[i])
			if v == 0 {
				waterHeight[0], waterHeight[1] = math.Min(waterHeight[0], h), math.Max(waterHeight[1], h)
			}
			if v == 16 {
				groundDev[0], groundDev[1] = math.Min(groundDev[0], h-2), math.Max(groundDev[1], h-2)
			}
			ramp := len(g8.Data) == n*n && g8.Data[i] != 0
			if ramp {
				ramps++
				rampLevels[int(v)]++
			}
			x, y := i%n, i/n
			if !ramp && x+1 < n && (len(g8.Data) != n*n || g8.Data[i+1] == 0) {
				steps[abs(int(v)-int(g7.Data[i+1]))]++
			}
			if !ramp && y+1 < n && (len(g8.Data) != n*n || g8.Data[i+n] == 0) {
				steps[abs(int(v)-int(g7.Data[i+n]))]++
			}
		}
		for _, v := range g10.Data {
			if v != 0 {
				mask++
			}
		}
		var keys []int
		for k := range local {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		if len(keys) > 1 || mask > 0 || nonMul > 0 {
			fmt.Printf("%-70s size %3d levels %v ramps %d [10]!=0 %d off-grid %d\n", f.name, t.Size, keys, ramps, mask, nonMul)
		}
	}
	fmt.Println("levels:", levels)
	fmt.Println("steps between neighbours (outside ramps):", steps)
	fmt.Println("levels on ramps:", rampLevels)
	fmt.Println("top texture under water:", ranked(underWater, 15))
	fmt.Println("top texture on shores:", ranked(shore, 15))
	fmt.Printf("height where level 0: %.2f..%.2f; height - 2 where level 16: %.2f..%.2f\n", waterHeight[0], waterHeight[1], groundDev[0], groundDev[1])
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func ranked(counts map[string]int, limit int) string {
	var keys []string
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return counts[keys[i]] > counts[keys[j]] })
	out := ""
	for i, k := range keys {
		if i == limit {
			break
		}
		out += fmt.Sprintf("\n    %-40s %d", k, counts[k])
	}
	return out
}
