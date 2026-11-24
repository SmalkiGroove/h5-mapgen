# h5-mapgen

Map generator for **Heroes of Might and Magic 5: Tribes of the East** (H5X mod), in the spirit of the Age of Empires IV Terrain Layout tool: paint a coarse layout grid, expand it into terrain and objects, write a playable `.h5m`. It writes the map files directly; it doesn't use the game's random map generator.

Related projects: the mod `G:\__H5__\heroes-expansion` and the DLL `G:\__H5__\H5X_DLL`. **The map format is documented in `G:\__H5__\H5X_DLL\.cursor\rules\h5x-map-format.mdc`** (archive, `map.xdb` objects and players, terrain `.bin`, footprints, coordinates); update it there when you learn something about the format. Game folder: `G:\__H5__\H5Xdev` (maps in `Maps\`, paks in `data\`).

## Plan

1. **Terrain round trip** (done): read and write the terrain `.bin` byte for byte, then a painted map checked in game. ← in-game check pending
2. **Painter and terrain:** Wails app with the layout grid painter; expansion into textures, heights, water, roads, player towns and heroes; a playable empty map. ← in progress: painter, biomes, hills and player towns done (first map `Mapgen_first.h5m`, not yet checked in the editor or game); water, cliffs and roads to do
3. **Content:** mines, buildings, treasures, guards and decorations, placed with their footprints, with a walkability check.
4. **Random layouts:** presets and a seed, like AoE4's Generate button.

## Code

Go 1.26+ (installed: 1.27.4 in `D:\Tools\Golang`, not on PATH; Go 1.19 kept in `D:\Tools\Golang-1.19`). The app is a [Wails](https://wails.io) v2.17 program: Go core, HTML/canvas UI in WebView2, one `.exe` (about 12 MB) shipped with the mod. The Wails CLI is in `%USERPROFILE%\go\bin`. The core uses only the standard library.

| Path | Content |
|---|---|
| `main.go`, `app.go` | Wails entry point; `App` holds the methods the UI calls (`window.go.main.App.*`): `Setup`, `NewLayout`, `Preview`, `GenerateMap`, `SaveLayout` / `OpenLayout` / `ReadLayout`, `LoadMap` |
| `frontend/dist/` | The UI, plain HTML/CSS/JS with no build step (no npm); embedded in the exe. Wails injects its runtime itself. Layout painter (ground, relief and player brushes, undo), live preview, game map viewer; the last layout is kept in `localStorage` |
| `wails.json`, `build/` | Wails config; `build/appicon.png` and `build/windows/` are the icon and manifest (default ones for now) |
| `internal/layout` | `Layout`: name, map size, grid, seed, biome and relief per cell (row 0 at the bottom), player starts in cell units; JSON |
| `internal/biome` | Ground types: base tile, accent tiles, road tile, painter colour |
| `internal/generate` | Layout → terrain and towns: noise-warped cell borders, blurred biome masks as layers (most used biome = base layer), accent patches, noise hills, towns facing the map centre |
| `internal/mapfile` | Writes the `.h5m` from the templates of `base/` (see below) and the layout (`layout.h5mapgen.json` in the map folder, so the app can reopen a generated map) |
| `internal/binsaver` | The engine's binary saver encoding (tag, varint length, content), schema-driven parse and serialize |
| `internal/terrain` | `Terrain`: texture layers, height and the other vertex grids; unknown fields are kept and written back. `New` builds a flat terrain from scratch |
| `internal/h5m` | `Archive`: read / write a `.h5m` zip, rename its map folder |
| `cmd/genmap` | Generates a map from a layout JSON (or a demo layout) and checks the result |
| `cmd/terraininfo` | Prints each terrain field with value histograms |
| `cmd/roundtrip` | Rebuilds every terrain file of the game (maps, paks, last random map) and compares the bytes |
| `cmd/painttest` | Writes a copy of a map with painted texture layers (step 1 in-game check) |

**Base templates** (`internal/mapfile/base/`, CRLF, `{{NAME}}` placeholders): `map.xdb` and `map-tag.xdb` come from the H5X map 00-test (keeps the secondary objective `H5X` that `advmap-manager.lua` hooks, and the empty `H5X_MapScript.xdb`); `player.xml` is its player item; `town.xml` is a player `RandomTown` of the vanilla map A1M3 with no `Specialization` (47 vanilla multiplayer towns are set up this way: the faction is the one picked in the lobby) and its text refs emptied. Edit them with care: `sed -i` turned one to LF once; `go test ./internal/mapfile` checks the line endings.

`GenerateMap` writes `<game>\Maps\<Name>.h5m`. It only replaces a map that holds `layout.h5mapgen.json`, and refuses a map folder name another map uses.

```bash
export PATH="/d/Tools/Golang/bin:$HOME/go/bin:$PATH"
wails build                                  # build/bin/h5-mapgen.exe
wails dev -browser=false                     # live app; the UI with working Go calls is also at http://localhost:34115
go test ./...                                # with H5_Folder set: terrain.New rebuilds 00-test byte for byte
go run ./cmd/genmap out/demo.h5m             # demo layout, or -layout FILE.json
go run ./cmd/roundtrip                       # 407/407 on 2026-11-24
go run ./cmd/painttest G:/__H5__/H5Xdev/Maps/00-test.h5m out/mapgen-test.h5m
```

The app finds the game folder (one with `Maps\` and `data\`) from its own location upward, then from `$H5_Folder`. In dev mode the first page load in a browser can miss the Go bindings (websocket not up yet): reload. The browser pane at its natural size is about 766 px wide; the UI stacks the two views under 1100 px.

The first version was a Python prototype; the Go port gives byte-identical output (round trip and paint test). Ignored: `out/`, `build/bin/`, `frontend/wailsjs/` (bindings Wails regenerates on each build; the UI calls `window.go` directly).

## Format facts learned here

- Lengths use the short varint form whenever n < 128; with that rule every terrain file is rebuilt byte for byte.
- Grid `[15]` can be 0 × 0 with no data field (two vanilla arenas, `Maps/SmallSpecialArenas/A2C3/M3S1`).
- A new texture layer needs its tile href in the `tiles` list of `map.xdb` as well.
- A flat terrain built from scratch (`terrain.New`) with the layers, heights and `[16]` bytes of 00-test is that file byte for byte: `[13]` = 0/0, `[14]` = 1, `[15]` all 1, `[8]` all 0, `[10]` all 0 without water, `[16]` = M × M cells `{[2] 0, [3] byte}` with M = ⌊(N+1)/3⌋ + 1 and bytes that look uniformly random (filled from the seed).
- `[7]` is the plateau level (16 = ground), not the height: 8 Kingdoms has hills from 1.6 to 2.2 in `[5]` with `[7]` = 16 everywhere. Rolling hills only change `[5]`.
- `RandomTown` footprint at rotation 0: about x −4…6, y −5…5, entrance (visit tile) at (1, −5), so it faces south; each quarter turn is counter-clockwise.
- Two maps must never share the folder `Maps/<Kind>/<Name>/` inside their archive: the game merges every map into one file system.
