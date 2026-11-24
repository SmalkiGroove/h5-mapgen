package terrain

import (
	"math/rand"

	"h5mapgen/internal/binsaver"
)

// Plateau levels of grid [7] (height x 8): lowered (water, lava, rim), ground, raised.
const (
	LevelLowered = 0
	LevelGround  = 16
	LevelRaised  = 32
)

// DetailSize is the side of the [16] grid for a map of size tiles: floor((size + 1) / 3) + 1.
func DetailSize(size int) int { return (size+1)/3 + 1 }

// New makes a flat terrain of size x size tiles with no texture layer: height 2.0, level ground, no
// lowered area. Grid [16] (unknown, one byte per cell that looks uniformly random in every map) is filled
// from rnd.
func New(size int, rnd *rand.Rand) *Terrain {
	m := DetailSize(size)
	detail := make([]byte, m*m)
	rnd.Read(detail)
	return newTerrain(size, detail)
}

func newTerrain(size int, detail []byte) *Terrain {
	n := size + 1
	u32 := func(tag byte, v int) binsaver.Node { return binsaver.Leaf(tag, binsaver.U32Bytes(uint32(v))) }
	grid := func(tag byte, w, h int) binsaver.Node {
		return binsaver.Container(tag, u32(1, w), u32(2, h), binsaver.Leaf(3, nil))
	}
	m := DetailSize(size)
	cells := make([]binsaver.Node, 0, 2+m*m)
	cells = append(cells, u32(1, m), u32(2, m))
	for _, d := range detail {
		cells = append(cells, binsaver.Container(3, binsaver.Leaf(2, []byte{0}), binsaver.Leaf(3, []byte{d})))
	}
	body := []binsaver.Node{
		u32(2, size), u32(3, size),
		binsaver.Container(TagLayers, u32(2, 0)),
		grid(TagHeight, n, n), grid(TagHeight8, n, n), grid(TagGrid8, n, n), grid(TagHalfMask, 2*size+1, 2*size+1),
		binsaver.Container(13, u32(1, 0), u32(2, 0)),
		binsaver.Leaf(14, []byte{1}),
		grid(TagGrid15, n, n),
		binsaver.Container(16, cells...),
	}
	empty := func(tag byte) binsaver.Node { return binsaver.Leaf(tag, nil) }
	t := &Terrain{
		tree: []binsaver.Node{u32(4, 4), binsaver.Container(1, binsaver.Container(1, body...)), empty(0), empty(2), empty(5)},
		Size: size,
		Grids: map[byte]Grid{
			TagHeight:   {W: n, H: n, Data: []byte{}},
			TagHeight8:  {W: n, H: n, Data: filled(n*n, LevelGround)},
			TagGrid8:    {W: n, H: n, Data: make([]byte, n*n)},
			TagHalfMask: {W: 2*size + 1, H: 2*size + 1, Data: make([]byte, (2*size+1)*(2*size+1))},
			TagGrid15:   {W: n, H: n, Data: filled(n*n, 1)},
		},
		Height: make([]float32, n*n),
	}
	for i := range t.Height {
		t.Height[i] = 2
	}
	return t
}

func filled(n int, v byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = v
	}
	return out
}
