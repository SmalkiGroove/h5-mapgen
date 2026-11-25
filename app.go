package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"h5mapgen/internal/biome"
	"h5mapgen/internal/generate"
	"h5mapgen/internal/h5m"
	"h5mapgen/internal/layout"
	"h5mapgen/internal/mapfile"
	"h5mapgen/internal/terrain"
)

// App holds the methods the UI calls (bound by Wails: window.go.main.App.*).
type App struct {
	ctx  context.Context
	game string // game folder (with Maps\ and data\), "" when not found
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.game = findGame()
}

// findGame looks for the game folder: the exe's folder and its parents (the app ships with the mod), then
// $H5_Folder.
func findGame() string {
	isGame := func(dir string) bool {
		for _, sub := range []string{"Maps", "data"} {
			if info, err := os.Stat(filepath.Join(dir, sub)); err != nil || !info.IsDir() {
				return false
			}
		}
		return true
	}
	if exe, err := os.Executable(); err == nil {
		for dir := filepath.Dir(exe); ; dir = filepath.Dir(dir) {
			if isGame(dir) {
				return dir
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	if dir := os.Getenv("H5_Folder"); dir != "" && isGame(dir) {
		return dir
	}
	return ""
}

// Setup is what the UI needs at start.
type Setup struct {
	Folder     string        `json:"folder"`
	Maps       []string      `json:"maps"` // .h5m paths in <game>\Maps
	Biomes     []biome.Biome `json:"biomes"`
	Sizes      []int         `json:"sizes"`
	MaxPlayers int           `json:"maxPlayers"`
}

func (a *App) Setup() Setup {
	info := Setup{Folder: a.game, Maps: []string{}, Biomes: biome.All, Sizes: layout.Sizes, MaxPlayers: layout.MaxPlayers}
	if a.game != "" {
		maps, _ := filepath.Glob(filepath.Join(a.game, "Maps", "*.h5m"))
		sort.Strings(maps)
		info.Maps = append(info.Maps, maps...)
	}
	return info
}

// NewLayout makes a default layout with a random seed.
func (a *App) NewLayout(size, grid int) *layout.Layout {
	return layout.New(size, grid, rand.New(rand.NewSource(time.Now().UnixNano())).Int63n(1_000_000))
}

// MapView is a map's ground terrain, for display.
type MapView struct {
	Path     string            `json:"path"`   // "" for a preview
	Folder   string            `json:"folder"` // Maps/<Kind>/<Name>/ in the archive
	Size     int               `json:"size"`   // tiles; vertex grids are size + 1 wide
	Layers   []string          `json:"layers"` // tile hrefs, in paint order
	Dominant []byte            `json:"dominant"`
	Height   []byte            `json:"height"` // height x 25, clamped to 0..255
	Levels   []byte            `json:"levels"` // plateau level ([7]: 0 water, 16 ground, +16 per cliff)
	Ramps    []byte            `json:"ramps"`  // 1 on ramps ([8])
	Towns    []generate.Town   `json:"towns"`
	Links    [][2]int          `json:"links"`    // roads of a preview, as pairs of player numbers
	Objects  []generate.Object `json:"objects"`  // mines, treasures, artifacts, monsters of a preview
	Decor    []byte            `json:"decor"`    // size x size: 1 tile blocked by a decoration, 2 ground cover
	Warnings []string          `json:"warnings"` // content that could not be placed
}

// viewOf summarizes a terrain per vertex (row = y, bottom to top): Dominant is the index of the last layer
// weighing at least half (255 when none), Height the height x 25 clamped to 0..255, Levels and Ramps the
// plateau grids.
func viewOf(t *terrain.Terrain) *MapView {
	n := t.Size + 1
	view := &MapView{Size: t.Size, Layers: []string{}, Dominant: make([]byte, n*n), Height: make([]byte, n*n),
		Levels: make([]byte, n*n), Ramps: make([]byte, n*n), Towns: []generate.Town{}, Links: [][2]int{},
		Objects: []generate.Object{}, Warnings: []string{}}
	if g := t.Grids[terrain.TagLevel]; len(g.Data) == n*n {
		copy(view.Levels, g.Data)
	}
	if g := t.Grids[terrain.TagRamp]; len(g.Data) == n*n {
		copy(view.Ramps, g.Data)
	}
	for i := range view.Dominant {
		view.Dominant[i] = 255
	}
	for li, l := range t.Layers {
		view.Layers = append(view.Layers, l.Href)
		if len(l.Weights.Data) != n*n {
			continue
		}
		for i, w := range l.Weights.Data {
			if w >= 128 {
				view.Dominant[i] = byte(li)
			}
		}
	}
	for i, h := range t.Height {
		if i < len(view.Height) {
			view.Height[i] = byte(min(max(h*25, 0), 255))
		}
	}
	return view
}

// LoadMap reads a map's ground terrain.
func (a *App) LoadMap(path string) (*MapView, error) {
	archive, err := h5m.Open(path)
	if err != nil {
		return nil, err
	}
	data, err := archive.Read("GroundTerrain.bin")
	if err != nil {
		return nil, err
	}
	t, err := terrain.Parse(data)
	if err != nil {
		return nil, err
	}
	view := viewOf(t)
	view.Path, view.Folder = path, archive.Folder
	return view, nil
}

// Preview generates a layout without writing it.
func (a *App) Preview(l *layout.Layout) (*MapView, error) {
	res, err := generate.Generate(l)
	if err != nil {
		return nil, err
	}
	view := viewOf(res.Terrain)
	view.Folder = mapfile.Folder(l)
	view.Towns = res.Towns
	if res.Links != nil {
		view.Links = res.Links
	}
	view.Objects, view.Warnings = res.Objects, res.Warnings
	view.Decor = make([]byte, l.Size*l.Size)
	for _, d := range res.Decor {
		blocked, _ := d.Tiles()
		for _, t := range blocked {
			view.Decor[t[1]*l.Size+t[0]] = 1
		}
		if x, y := int(d.X), int(d.Y); len(blocked) == 0 && x >= 0 && y >= 0 && x < l.Size && y < l.Size && view.Decor[y*l.Size+x] == 0 {
			view.Decor[y*l.Size+x] = 2
		}
	}
	return view, nil
}

// mapFolderOf returns the map folder of a .h5m and whether it holds a h5-mapgen layout.
func mapFolderOf(path string) (folder string, generated bool, err error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return "", false, err
	}
	defer r.Close()
	for _, f := range r.File {
		lower := strings.ToLower(f.Name)
		if strings.HasSuffix(lower, "/map.xdb") {
			folder = f.Name[:len(f.Name)-len("map.xdb")]
		}
		if strings.HasSuffix(f.Name, "/"+mapfile.LayoutFile) {
			generated = true
		}
	}
	return folder, generated, nil
}

// GenerateMap writes the map of a layout to <game>\Maps\<name>.h5m and returns its path. It only replaces
// a map generated by this app, and refuses a map folder name another map already uses.
func (a *App) GenerateMap(l *layout.Layout) (string, error) {
	if a.game == "" {
		return "", fmt.Errorf("game folder not found: put the app in the game folder or set H5_Folder")
	}
	if err := l.Check(); err != nil {
		return "", err
	}
	path := filepath.Join(a.game, "Maps", strings.ReplaceAll(l.Name, " ", "_")+".h5m")
	folder := mapfile.Folder(l)
	others, _ := filepath.Glob(filepath.Join(a.game, "Maps", "*.h5m"))
	for _, other := range others {
		f, generated, err := mapFolderOf(other)
		if err != nil {
			continue
		}
		if strings.EqualFold(other, path) {
			if !generated {
				return "", fmt.Errorf("%s exists and was not made by the map generator: choose another name", filepath.Base(path))
			}
		} else if strings.EqualFold(f, folder) {
			return "", fmt.Errorf("%s already uses the map folder %s: choose another name", filepath.Base(other), folder)
		}
	}
	res, err := generate.Generate(l)
	if err != nil {
		return "", err
	}
	data, err := mapfile.Build(l, res)
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o644)
}

var layoutFilters = []runtime.FileFilter{
	{DisplayName: "Layouts and generated maps (*.json, *.h5m)", Pattern: "*.json;*.h5m"},
}

// SaveLayout asks where to save a layout ("" when cancelled).
func (a *App) SaveLayout(l *layout.Layout) (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title: "Save the layout", DefaultFilename: strings.ReplaceAll(l.Name, " ", "_") + ".json",
		Filters: []runtime.FileFilter{{DisplayName: "Layouts (*.json)", Pattern: "*.json"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	return path, os.WriteFile(path, l.JSON(), 0o644)
}

// ReadLayout reads a layout from a .json file or from a map generated by this app.
func (a *App) ReadLayout(path string) (*layout.Layout, error) {
	if !strings.EqualFold(filepath.Ext(path), ".h5m") {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return layout.Parse(data)
	}
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	for _, f := range r.File {
		if strings.HasSuffix(f.Name, "/"+mapfile.LayoutFile) {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, err
			}
			return layout.Parse(data)
		}
	}
	return nil, fmt.Errorf("%s was not made by the map generator: it has no layout", filepath.Base(path))
}

// OpenLayout asks for a layout or generated map and reads its layout (nil when cancelled).
func (a *App) OpenLayout() (*layout.Layout, error) {
	dir := ""
	if a.game != "" {
		dir = filepath.Join(a.game, "Maps")
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Open a layout", DefaultDirectory: dir, Filters: layoutFilters,
	})
	if err != nil || path == "" {
		return nil, err
	}
	return a.ReadLayout(path)
}
