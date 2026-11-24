// Command terraininfo prints what a terrain file holds, field by field, with value histograms of the grids.
//
//	go run ./cmd/terraininfo MAP.h5m [Ground|Underground]
package main

import (
	"fmt"
	"os"
	"sort"

	"h5mapgen/internal/binsaver"
	"h5mapgen/internal/h5m"
	"h5mapgen/internal/terrain"
)

func histogram[T comparable](values []T, limit int) string {
	counts := map[T]int{}
	var order []T
	for _, v := range values {
		if counts[v] == 0 {
			order = append(order, v)
		}
		counts[v]++
	}
	sort.Slice(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })
	out := ""
	for i, v := range order {
		if i == limit {
			out += fmt.Sprintf(" … (%d values)", len(order))
			break
		}
		out += fmt.Sprintf(" %v×%d", v, counts[v])
	}
	return out
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: terraininfo MAP.h5m [Ground|Underground]")
		os.Exit(2)
	}
	level := "Ground"
	if len(os.Args) > 2 {
		level = os.Args[2]
	}
	archive, err := h5m.Open(os.Args[1])
	if err != nil {
		fmt.Println("ERROR", err)
		os.Exit(1)
	}
	data, err := archive.Read(level + "Terrain.bin")
	if err != nil {
		fmt.Println("ERROR", err)
		os.Exit(1)
	}
	t, err := terrain.Parse(data)
	if err != nil {
		fmt.Println("ERROR", err)
		os.Exit(1)
	}
	fmt.Printf("size %d, %d layers\n", t.Size, len(t.Layers))
	for i, l := range t.Layers {
		fmt.Printf("  layer %d %s:%s\n", i, l.Href, histogram(l.Weights.Data, 4))
	}
	fmt.Printf("height %d values:%s\n", len(t.Height), histogram(t.Height, 12))
	for _, n := range t.Body() {
		switch {
		case n.Tag == terrain.TagLayers || n.Tag == terrain.TagHeight:
		case !n.Container:
			fmt.Printf("[%d] leaf % x\n", n.Tag, n.Data)
		case n.Tag == 16:
			m := binsaver.Find(n.Children, 1).U32()
			var pairs []string
			for _, c := range binsaver.FindAll(n.Children, 3) {
				pair := ""
				for _, f := range c.Children {
					pair += fmt.Sprintf("%d:% x ", f.Tag, f.Data)
				}
				pairs = append(pairs, pair)
			}
			fmt.Printf("[16] M %d x %d, %d cells:%s\n", m, binsaver.Find(n.Children, 2).U32(), len(pairs), histogram(pairs, 8))
		default:
			if g, ok := t.Grids[n.Tag]; ok {
				fmt.Printf("[%d] grid %d x %d:%s\n", n.Tag, g.W, g.H, histogram(g.Data, 10))
			} else {
				fmt.Printf("[%d] container %d fields\n", n.Tag, len(n.Children))
				for _, c := range n.Children {
					fmt.Printf("    [%d] % x\n", c.Tag, c.Data)
				}
			}
		}
	}
}
