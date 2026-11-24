package terrain

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"h5mapgen/internal/binsaver"
)

// readGame reads the ground terrain of a map of the game folder ($H5_Folder), or skips the test.
func readGame(t *testing.T, mapName string) []byte {
	game := os.Getenv("H5_Folder")
	if game == "" {
		t.Skip("H5_Folder not set")
	}
	r, err := zip.OpenReader(game + "/Maps/" + mapName)
	if err != nil {
		t.Skip(err)
	}
	defer r.Close()
	for _, f := range r.File {
		if strings.HasSuffix(f.Name, "/GroundTerrain.bin") {
			rc, _ := f.Open()
			defer rc.Close()
			data, _ := io.ReadAll(rc)
			return data
		}
	}
	t.Fatalf("%s: no GroundTerrain.bin", mapName)
	return nil
}

// A terrain built from scratch with the layers, heights and [16] bytes of a real flat map is that map.
func TestNewRebuildsFlatMap(t *testing.T) {
	data := readGame(t, "00-test.h5m")
	original, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var detail []byte
	for _, cell := range binsaver.FindAll(binsaver.Find(original.Body(), 16).Children, 3) {
		detail = append(detail, binsaver.Find(cell.Children, 3).Data[0])
	}
	built := newTerrain(original.Size, detail)
	for _, l := range original.Layers {
		layer := NewLayer(l.Href, original.Size)
		copy(layer.Weights.Data, l.Weights.Data)
		built.Layers = append(built.Layers, layer)
	}
	copy(built.Height, original.Height)
	out := built.Bytes()
	if !bytes.Equal(out, data) {
		first := 0
		for first < len(out) && first < len(data) && out[first] == data[first] {
			first++
		}
		t.Fatalf("%d -> %d bytes, first difference at %#x", len(data), len(out), first)
	}
}
