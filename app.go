package main

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"h5mapgen/internal/h5m"
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

// GameInfo is what the UI shows at start.
type GameInfo struct {
	Folder string   `json:"folder"`
	Maps   []string `json:"maps"` // .h5m paths in <game>\Maps
}

func (a *App) Game() GameInfo {
	info := GameInfo{Folder: a.game, Maps: []string{}}
	if a.game != "" {
		maps, _ := filepath.Glob(filepath.Join(a.game, "Maps", "*.h5m"))
		sort.Strings(maps)
		info.Maps = append(info.Maps, maps...)
	}
	return info
}

// MapView is a map's ground terrain, for display.
type MapView struct {
	Path     string   `json:"path"`
	Folder   string   `json:"folder"` // Maps/<Kind>/<Name>/ in the archive
	Size     int      `json:"size"`   // tiles; vertex grids are size + 1 wide
	Layers   []string `json:"layers"` // tile hrefs, in paint order
	Dominant []byte   `json:"dominant"`
	Height   []byte   `json:"height"`
}

// LoadMap reads a map's ground terrain. Per vertex (row = y, bottom to top): Dominant is the index of the
// last layer weighing at least half (255 when none), Height the height x 50 clamped to 0..255.
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
	n := t.Size + 1
	view := &MapView{Path: path, Folder: archive.Folder, Size: t.Size, Layers: []string{},
		Dominant: make([]byte, n*n), Height: make([]byte, n*n)}
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
			view.Height[i] = byte(min(max(h*50, 0), 255))
		}
	}
	return view, nil
}

// OpenMap asks for a .h5m file and loads it (nil when cancelled).
func (a *App) OpenMap() (*MapView, error) {
	dir := ""
	if a.game != "" {
		dir = filepath.Join(a.game, "Maps")
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Open a map",
		DefaultDirectory: dir,
		Filters:          []runtime.FileFilter{{DisplayName: "Heroes 5 maps (*.h5m)", Pattern: "*.h5m"}},
	})
	if err != nil || path == "" {
		return nil, err
	}
	return a.LoadMap(path)
}
