// Command painttest writes a copy of a map with new texture layers painted, to open in the game and the
// editor (step 1 check).
//
//	go run ./cmd/painttest SOURCE.h5m OUT.h5m
//
// Paints, over the source terrain: a snow band along the left edge, a dead land disc in the middle and dirt
// stripes along the bottom, with soft 3-vertex borders. Only textures change (heights and masks are kept).
// The map folder inside the archive becomes Maps/Multiplayer/MapgenTest/ and its name "Mapgen test", so it
// doesn't clash with the source map.
package main

import (
	"fmt"
	"math"
	"os"
	"strings"
	"unicode/utf16"

	"h5mapgen/internal/h5m"
	"h5mapgen/internal/terrain"
)

const (
	tileHref = "/MapObjects/_(AdvMapTile)/%s.xdb#xpointer(/AdvMapTile)"
	fade     = 3.0 // vertices over which a painted shape fades out
)

// weight is 255 inside a shape, fading to 0 over fade vertices; d > 0 inside, < 0 outside.
func weight(d float32) byte {
	w := 128 + d/fade*127
	if w < 0 {
		return 0
	}
	if w > 255 {
		return 255
	}
	return byte(w)
}

// utf16File encodes a game text: UTF-16 LE with a byte order mark.
func utf16File(s string) []byte {
	out := []byte{0xFF, 0xFE}
	for _, c := range utf16.Encode([]rune(s)) {
		out = append(out, byte(c), byte(c>>8))
	}
	return out
}

func run(source, out string) error {
	archive, err := h5m.Open(source)
	if err != nil {
		return err
	}
	data, err := archive.Read("GroundTerrain.bin")
	if err != nil {
		return err
	}
	ter, err := terrain.Parse(data)
	if err != nil {
		return err
	}
	n := ter.Size + 1
	nf := float32(n)
	shapes := []struct {
		tile     string
		distance func(x, y float32) float32
	}{
		{"Snow/Snow_(2)", func(x, y float32) float32 { return float32(float64(n)*0.2) - x }}, // left band
		{"Lava/Dead_Land", func(x, y float32) float32 { // disc in the middle
			return float32(float64(n)*0.15) - float32(math.Hypot(float64(x-nf/2), float64(y-nf/2)))
		}},
		{"Dirt/Ground", func(x, y float32) float32 { // bottom stripes
			stripe := 4 - float32(math.Abs(float64(float32(math.Mod(float64(x), 24))-12)))
			return float32(math.Min(float64(float32(float64(n)*0.15)-y), float64(stripe)))
		}},
	}
	xdbBytes, err := archive.Read("map.xdb")
	if err != nil {
		return err
	}
	xdb := string(xdbBytes)
	for _, s := range shapes {
		href := fmt.Sprintf(tileHref, s.tile)
		layer := terrain.NewLayer(href, ter.Size)
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				layer.Weights.Data[y*n+x] = weight(s.distance(float32(x), float32(y)))
			}
		}
		ter.Layers = append(ter.Layers, layer)
		if !strings.Contains(xdb, href) {
			xdb = strings.Replace(xdb, "</tiles>", "\t<Item href=\""+href+"\"/>\r\n\t</tiles>", 1)
		}
	}
	archive.Write("GroundTerrain.bin", ter.Bytes())
	archive.Write("map.xdb", []byte(xdb))
	archive.Write("name.txt", utf16File("Mapgen test"))

	// Check: the written terrain reads back with the new layers
	data, _ = archive.Read("GroundTerrain.bin")
	check, err := terrain.Parse(data)
	if err != nil {
		return fmt.Errorf("written terrain doesn't read back: %w", err)
	}
	if err := archive.Save(out, "Maps/Multiplayer/MapgenTest/"); err != nil {
		return err
	}
	tiles := strings.Count(strings.SplitN(strings.SplitN(xdb, "<tiles>", 2)[1], "</tiles>", 2)[0], "<Item href=")
	fmt.Printf("%s: %d layers, %d tiles in map.xdb\n", out, len(check.Layers), tiles)
	return nil
}

func main() {
	if len(os.Args) != 3 {
		fmt.Println("usage: painttest SOURCE.h5m OUT.h5m")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Println("ERROR", err)
		os.Exit(1)
	}
}
