package mapfile

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"h5mapgen/internal/generate"
	"h5mapgen/internal/layout"
)

// A generated map has well-formed XML files with CRLF line endings only, and one town per player.
func TestBuild(t *testing.T) {
	l := layout.New(72, 8, 1)
	l.Name = "Test map"
	res, err := generate.Generate(l)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Build(l, res)
	if err != nil {
		t.Fatal(err)
	}
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.File {
		if !strings.HasPrefix(f.Name, Folder(l)) {
			t.Errorf("%s is outside the map folder", f.Name)
		}
		if !strings.HasSuffix(f.Name, ".xdb") {
			continue
		}
		rc, _ := f.Open()
		content, _ := io.ReadAll(rc)
		rc.Close()
		text := string(content)
		if strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\n") {
			t.Errorf("%s has LF-only line endings", f.Name)
		}
		dec := xml.NewDecoder(strings.NewReader(text))
		for {
			if _, err := dec.Token(); err == io.EOF {
				break
			} else if err != nil {
				t.Fatalf("%s: %v", f.Name, err)
			}
		}
		if strings.HasSuffix(f.Name, "/map.xdb") {
			if n := strings.Count(text, "<AdvMapTown>"); n != len(l.Starts) {
				t.Errorf("%d towns, expected %d", n, len(l.Starts))
			}
			if n := strings.Count(text, "<ActivePlayer>true</ActivePlayer>"); n != len(l.Starts) {
				t.Errorf("%d active players, expected %d", n, len(l.Starts))
			}
		}
	}
}
