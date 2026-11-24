// Package terrain reads and writes the terrain files of adventure maps (GroundTerrain.bin /
// UndergroundTerrain.bin).
//
// Layout (format notes: H5X_DLL/.cursor/rules/h5x-map-format.mdc), N = map size in tiles, vertex grids are
// (N + 1) x (N + 1), row = y (bottom to top), column = x:
//
//	[4] u32 4                          version
//	[1] { [1] {                        body
//	    [2] u32 N   [3] u32 N
//	    [4] { [2] u32 count            texture layers, in paint order
//	          [1] { [2] grid u8 weights   [3] { [3] tile href } } x count }
//	    [5] grid f32 height            2.0 ground, 0 lowered (water, lava, rim), 4.0 raised
//	    [7] grid u8                    height x 8
//	    [8] grid u8                    unknown (0/1 on a few vertices)
//	    [10] grid u8 (2N+1)^2          half-tile mask of lowered areas and their shores
//	    [13] { [1] u32 [2] u32 }  [14] u8  [15] grid u8  [16] { [1] u32 [2] u32 [3] {[2] u8 [3] u8} x M^2 }
//	} }
//	[0] [2] [5] empty
//
// A grid is { [1] u32 width, [2] u32 height, [3] raw bytes }. The model keeps the parsed tree, so fields it
// doesn't interpret are written back unchanged.
package terrain

import (
	"encoding/binary"
	"fmt"
	"math"

	"h5mapgen/internal/binsaver"
)

// Tags of the body fields.
const (
	TagLayers   = 4
	TagHeight   = 5
	TagHeight8  = 7
	TagGrid8    = 8
	TagHalfMask = 10
	TagGrid15   = 15
)

// GridTags are the body grids kept in Terrain.Grids (raw bytes; the height is decoded in Terrain.Height).
var GridTags = []byte{TagHeight, TagHeight8, TagGrid8, TagHalfMask, TagGrid15}

var schema = func() binsaver.Schema {
	paths := [][]byte{{1}, {1, 1}, {1, 1, 4}, {1, 1, 4, 1}, {1, 1, 4, 1, 2}, {1, 1, 4, 1, 3},
		{1, 1, 13}, {1, 1, 16}, {1, 1, 16, 3}}
	for _, t := range GridTags {
		paths = append(paths, []byte{1, 1, t})
	}
	return binsaver.NewSchema(paths...)
}()

// Grid is a W x H grid of fixed-size cells, stored row by row (row = y).
type Grid struct {
	W, H int
	Data []byte // nil in an empty grid without a data field (0 x 0, seen on a few vanilla arenas)
}

func readGrid(nodes []binsaver.Node) Grid {
	g := Grid{W: int(binsaver.Find(nodes, 1).U32()), H: int(binsaver.Find(nodes, 2).U32())}
	if data := binsaver.Find(nodes, 3); data != nil {
		g.Data = append([]byte{}, data.Data...)
	}
	return g
}

// writeGrid writes g into a grid node list, keeping its field order (new fields when nodes is empty).
func writeGrid(nodes []binsaver.Node, g Grid) []binsaver.Node {
	values := map[byte][]byte{1: binsaver.U32Bytes(uint32(g.W)), 2: binsaver.U32Bytes(uint32(g.H)), 3: g.Data}
	if len(nodes) == 0 {
		return []binsaver.Node{binsaver.Leaf(1, values[1]), binsaver.Leaf(2, values[2]), binsaver.Leaf(3, values[3])}
	}
	out := make([]binsaver.Node, len(nodes))
	for i, n := range nodes {
		if v, ok := values[n.Tag]; ok && !n.Container {
			n.Data = v
		}
		out[i] = n
	}
	return out
}

// Layer is a texture layer: a tile href (/MapObjects/_(AdvMapTile)/...) and a weight 0..255 per vertex.
type Layer struct {
	Href    string
	Weights Grid
	nodes   []binsaver.Node
}

// NewLayer makes a layer with zero weights over a (size + 1) x (size + 1) vertex grid.
func NewLayer(href string, size int) *Layer {
	n := size + 1
	return &Layer{Href: href, Weights: Grid{W: n, H: n, Data: make([]byte, n*n)},
		nodes: []binsaver.Node{binsaver.Container(2), binsaver.Container(3, binsaver.Leaf(3, nil))}}
}

func (l *Layer) node() binsaver.Node {
	out := make([]binsaver.Node, len(l.nodes))
	for i, n := range l.nodes {
		switch n.Tag {
		case 2:
			n.Children = writeGrid(n.Children, l.Weights)
		case 3:
			children := make([]binsaver.Node, len(n.Children))
			for j, c := range n.Children {
				if c.Tag == 3 {
					c.Data = []byte(l.Href)
				}
				children[j] = c
			}
			n.Children = children
		}
		out[i] = n
	}
	return binsaver.Container(1, out...)
}

// Terrain is a parsed terrain file.
type Terrain struct {
	tree   []binsaver.Node
	Size   int
	Layers []*Layer
	Grids  map[byte]Grid // raw grids by tag; the TagHeight data is written from Height
	Height []float32     // (Size + 1)^2 vertex heights, row = y
}

// Parse reads a terrain file.
func Parse(data []byte) (*Terrain, error) {
	tree, err := binsaver.Parse(data, schema)
	if err != nil {
		return nil, err
	}
	outer := binsaver.Find(tree, 1)
	if outer == nil || binsaver.Find(outer.Children, 1) == nil {
		return nil, fmt.Errorf("no terrain body")
	}
	body := binsaver.Find(outer.Children, 1).Children
	t := &Terrain{tree: tree, Size: int(binsaver.Find(body, 2).U32()), Grids: map[byte]Grid{}}
	if layers := binsaver.Find(body, TagLayers); layers != nil {
		for _, n := range binsaver.FindAll(layers.Children, 1) {
			href := binsaver.Find(binsaver.Find(n.Children, 3).Children, 3).Data
			weights := readGrid(binsaver.Find(n.Children, 2).Children)
			t.Layers = append(t.Layers, &Layer{Href: string(href), Weights: weights, nodes: n.Children})
		}
	}
	for _, tag := range GridTags {
		if n := binsaver.Find(body, tag); n != nil {
			t.Grids[tag] = readGrid(n.Children)
		}
	}
	if g, ok := t.Grids[TagHeight]; ok {
		t.Height = make([]float32, len(g.Data)/4)
		for i := range t.Height {
			t.Height[i] = math.Float32frombits(binary.LittleEndian.Uint32(g.Data[4*i:]))
		}
	}
	return t, nil
}

// Bytes writes the terrain file.
func (t *Terrain) Bytes() []byte {
	if g, ok := t.Grids[TagHeight]; ok && g.Data != nil {
		g.Data = make([]byte, 0, 4*len(t.Height))
		for _, h := range t.Height {
			g.Data = binary.LittleEndian.AppendUint32(g.Data, math.Float32bits(h))
		}
		t.Grids[TagHeight] = g
	}
	outer := binsaver.Find(t.tree, 1)
	oldBody := binsaver.Find(outer.Children, 1).Children
	body := make([]binsaver.Node, 0, len(oldBody))
	for _, n := range oldBody {
		if n.Tag == TagLayers && n.Container {
			var children []binsaver.Node
			for _, c := range n.Children {
				if c.Tag == 2 && !c.Container {
					c.Data = binsaver.U32Bytes(uint32(len(t.Layers)))
				}
				if c.Tag != 1 {
					children = append(children, c)
				}
			}
			for _, l := range t.Layers {
				children = append(children, l.node())
			}
			n.Children = children
		} else if g, ok := t.Grids[n.Tag]; ok && n.Container {
			n.Children = writeGrid(n.Children, g)
		}
		body = append(body, n)
	}
	tree := make([]binsaver.Node, len(t.tree))
	for i, n := range t.tree {
		if n.Tag == 1 && n.Container {
			children := make([]binsaver.Node, len(n.Children))
			for j, c := range n.Children {
				if c.Tag == 1 && c.Container {
					c.Children = body
				}
				children[j] = c
			}
			n.Children = children
		}
		tree[i] = n
	}
	return binsaver.Serialize(tree)
}
