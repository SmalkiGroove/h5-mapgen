# h5-mapgen

Map generator for **Heroes of Might and Magic 5: Tribes of the East** (H5X mod), in the spirit of the Age of Empires IV Terrain Layout tool: paint a coarse layout grid, expand it into terrain and objects, write a playable `.h5m`. It writes the map files directly; it doesn't use the game's random map generator.

Related projects: the mod `G:\__H5__\heroes-expansion` and the DLL `G:\__H5__\H5X_DLL`. **The map format is documented in `G:\__H5__\H5X_DLL\.cursor\rules\h5x-map-format.mdc`** (archive, `map.xdb` objects and players, terrain `.bin`, footprints, coordinates); update it there when you learn something about the format. Game folder: `G:\__H5__\H5Xdev` (maps in `Maps\`, paks in `data\`).

## Plan

1. **Terrain round trip** (done): read and write the terrain `.bin` byte for byte, then a painted map checked in game. ← in-game check pending
2. **Painter and terrain:** Wails app with the layout grid painter; expansion into textures, heights, water, roads, player towns and heroes; a playable empty map.
3. **Content:** mines, buildings, treasures, guards and decorations, placed with their footprints, with a walkability check.
4. **Random layouts:** presets and a seed, like AoE4's Generate button.

## Code

Go 1.26+ (installed: 1.27.4 in `D:\Tools\Golang`, not on PATH; Go 1.19 kept in `D:\Tools\Golang-1.19`). The app is a [Wails](https://wails.io) v2.17 program: Go core, HTML/canvas UI in WebView2, one `.exe` (about 12 MB) shipped with the mod. The Wails CLI is in `%USERPROFILE%\go\bin`. The core uses only the standard library.

| Path | Content |
|---|---|
| `main.go`, `app.go` | Wails entry point; `App` holds the methods the UI calls (`window.go.main.App.*`): `Game`, `LoadMap`, `OpenMap` |
| `frontend/dist/` | The UI, plain HTML/CSS/JS with no build step (no npm); embedded in the exe. Wails injects its runtime itself. |
| `wails.json`, `build/` | Wails config; `build/appicon.png` and `build/windows/` are the icon and manifest (default ones for now) |
| `internal/binsaver` | The engine's binary saver encoding (tag, varint length, content), schema-driven parse and serialize |
| `internal/terrain` | `Terrain`: texture layers, height and the other vertex grids; unknown fields are kept and written back |
| `internal/h5m` | `Archive`: read / write a `.h5m` zip, rename its map folder |
| `cmd/roundtrip` | Rebuilds every terrain file of the game (maps, paks, last random map) and compares the bytes |
| `cmd/painttest` | Writes a copy of a map with painted texture layers (step 1 in-game check) |

```bash
export PATH="/d/Tools/Golang/bin:$HOME/go/bin:$PATH"
wails build                                  # build/bin/h5-mapgen.exe
wails dev -browser=false                     # live app; the UI with working Go calls is also at http://localhost:34115
go run ./cmd/roundtrip                       # 407/407 on 2026-11-24
go run ./cmd/painttest G:/__H5__/H5Xdev/Maps/00-test.h5m out/mapgen-test.h5m
```

The app finds the game folder (one with `Maps\` and `data\`) from its own location upward, then from `$H5_Folder`. For now it lists the game maps and shows the ground terrain of the one picked (dominant texture per vertex, shaded by height).

The first version was a Python prototype; the Go port gives byte-identical output (round trip and paint test). Ignored: `out/`, `build/bin/`, `frontend/wailsjs/` (bindings Wails regenerates on each build; the UI calls `window.go` directly).

## Format facts learned here

- Lengths use the short varint form whenever n < 128; with that rule every terrain file is rebuilt byte for byte.
- Grid `[15]` can be 0 × 0 with no data field (two vanilla arenas, `Maps/SmallSpecialArenas/A2C3/M3S1`).
- A new texture layer needs its tile href in the `tiles` list of `map.xdb` as well.
- Two maps must never share the folder `Maps/<Kind>/<Name>/` inside their archive: the game merges every map into one file system.
