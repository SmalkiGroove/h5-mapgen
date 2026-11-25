// Command terraingrid draws a terrain field of a map as text, one character per value (top row = highest y).
//
//	go run ./cmd/terraingrid MAP.h5m FIELD [x0 y0 x1 y1]
//
// FIELD is height (the [5] heights x 2, rounded), or the tag of a byte grid (7, 8, 10, 15). Values are
// printed in base 36; '.' is 0 for byte grids.
package main

import (
	"fmt"
	"math"
	"os"
	"strconv"

	"h5mapgen/internal/h5m"
	"h5mapgen/internal/terrain"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: terraingrid MAP.h5m height|TAG [x0 y0 x1 y1]")
		os.Exit(2)
	}
	archive, err := h5m.Open(os.Args[1])
	check(err)
	data, err := archive.Read("GroundTerrain.bin")
	check(err)
	t, err := terrain.Parse(data)
	check(err)

	var w, h int
	var at func(x, y int) int
	if os.Args[2] == "height" {
		w, h = t.Size+1, t.Size+1
		at = func(x, y int) int { return int(math.Round(float64(t.Height[y*w+x]) * 2)) }
	} else {
		tag, err := strconv.Atoi(os.Args[2])
		check(err)
		g, ok := t.Grids[byte(tag)]
		if !ok {
			check(fmt.Errorf("no grid %d", tag))
		}
		w, h = g.W, g.H
		at = func(x, y int) int { return int(g.Data[y*w+x]) }
	}
	x0, y0, x1, y1 := 0, 0, w-1, h-1
	if len(os.Args) >= 7 {
		x0, y0, x1, y1 = atoi(os.Args[3]), atoi(os.Args[4]), atoi(os.Args[5]), atoi(os.Args[6])
	}
	fmt.Printf("%d x %d, x %d..%d, y %d..%d\n", w, h, x0, x1, y0, y1)
	for y := y1; y >= y0; y-- {
		line := fmt.Sprintf("%4d ", y)
		for x := x0; x <= x1; x++ {
			v := at(x, y)
			switch {
			case v == 0 && os.Args[2] != "height":
				line += "."
			case v < 0:
				line += "-"
			case v < 36:
				line += strconv.FormatInt(int64(v), 36)
			default:
				line += "#"
			}
		}
		fmt.Println(line)
	}
}

func atoi(s string) int {
	v, err := strconv.Atoi(s)
	check(err)
	return v
}

func check(err error) {
	if err != nil {
		fmt.Println("ERROR", err)
		os.Exit(1)
	}
}
