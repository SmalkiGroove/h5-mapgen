// Command genmap generates a map from a layout file, or from a demo layout when none is given.
//
//	go run ./cmd/genmap [-layout FILE.json] OUT.h5m
//
// The demo layout is a 136 map in four biome quarters, a band of hills across the middle, and two players.
package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"h5mapgen/internal/generate"
	"h5mapgen/internal/h5m"
	"h5mapgen/internal/layout"
	"h5mapgen/internal/mapfile"
	"h5mapgen/internal/terrain"
)

func demo() *layout.Layout {
	l := layout.New(136, 16, 42)
	l.Name = "Mapgen demo"
	quarters := [2][2]string{{"grass", "dirt"}, {"snow", "steppe"}}
	for y := 0; y < l.Grid; y++ {
		for x := 0; x < l.Grid; x++ {
			l.Biomes[y*l.Grid+x] = quarters[y*2/l.Grid][x*2/l.Grid]
			if y >= 6 && y < 10 {
				l.Relief[y*l.Grid+x] = layout.Hills
			}
		}
	}
	return l
}

func run() error {
	layoutPath := flag.String("layout", "", "layout JSON file (default: demo layout)")
	flag.Parse()
	if flag.NArg() != 1 {
		return fmt.Errorf("usage: genmap [-layout FILE.json] OUT.h5m")
	}
	l := demo()
	if *layoutPath != "" {
		data, err := os.ReadFile(*layoutPath)
		if err != nil {
			return err
		}
		if l, err = layout.Parse(data); err != nil {
			return err
		}
	}
	res, err := generate.Generate(l)
	if err != nil {
		return err
	}
	data, err := mapfile.Build(l, res)
	if err != nil {
		return err
	}
	out := flag.Arg(0)
	if err := os.WriteFile(out, data, 0o644); err != nil {
		return err
	}

	// Check: the archive reads back, its XML files are well formed, the terrain parses
	archive, err := h5m.Open(out)
	if err != nil {
		return err
	}
	for _, name := range archive.Names() {
		if strings.HasSuffix(name, ".xdb") {
			content, _ := archive.Read(name)
			dec := xml.NewDecoder(strings.NewReader(string(content)))
			for {
				if _, err := dec.Token(); err == io.EOF {
					break
				} else if err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
			}
		}
	}
	bin, _ := archive.Read("GroundTerrain.bin")
	ter, err := terrain.Parse(bin)
	if err != nil {
		return err
	}
	fmt.Printf("%s: %s, %d x %d, %d layers, %d towns, %d bytes\n", out, archive.Folder, ter.Size, ter.Size, len(ter.Layers), len(res.Towns), len(data))
	for _, t := range res.Towns {
		fmt.Printf("  player %d town at (%d, %d), %d quarter turns\n", t.Player, t.X, t.Y, t.Turns)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Println("ERROR", err)
		os.Exit(1)
	}
}
