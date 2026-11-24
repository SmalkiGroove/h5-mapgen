// Package h5m reads and writes adventure map archives (.h5m): a zip with every file in Maps/<Kind>/<Name>/.
package h5m

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

// Archive is a map archive held in memory, its files kept in archive order.
type Archive struct {
	Folder string // Maps/<Kind>/<Name>/
	names  []string
	files  map[string][]byte
}

// Open reads a map archive.
func Open(path string) (*Archive, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	a := &Archive{files: map[string][]byte{}}
	var maps []string
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %s: %w", path, f.Name, err)
		}
		a.names = append(a.names, f.Name)
		a.files[f.Name] = content
		if strings.HasSuffix(strings.ToLower(f.Name), "/map.xdb") {
			maps = append(maps, f.Name)
		}
	}
	if len(maps) != 1 {
		return nil, fmt.Errorf("%s: expected one map.xdb, found %d", path, len(maps))
	}
	a.Folder = maps[0][:len(maps[0])-len("map.xdb")]
	return a, nil
}

// Read returns a file of the map folder (name relative to it).
func (a *Archive) Read(name string) ([]byte, error) {
	data, ok := a.files[a.Folder+name]
	if !ok {
		return nil, fmt.Errorf("%s%s: not in the archive", a.Folder, name)
	}
	return data, nil
}

// Write adds or replaces a file of the map folder.
func (a *Archive) Write(name string, data []byte) {
	full := a.Folder + name
	if _, ok := a.files[full]; !ok {
		a.names = append(a.names, full)
	}
	a.files[full] = data
}

// Names lists the files of the map folder, relative to it.
func (a *Archive) Names() []string {
	var out []string
	for _, n := range a.names {
		if strings.HasPrefix(n, a.Folder) {
			out = append(out, n[len(a.Folder):])
		}
	}
	return out
}

// Save writes the archive; folder, when not empty, renames Maps/.../<Name>/ (two maps must never share one).
func (a *Archive) Save(path, folder string) error {
	if folder == "" {
		folder = a.Folder
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range a.names {
		out := name
		if strings.HasPrefix(name, a.Folder) {
			out = folder + name[len(a.Folder):]
		}
		f, err := w.Create(out)
		if err != nil {
			return err
		}
		if _, err := f.Write(a.files[name]); err != nil {
			return err
		}
	}
	if err := w.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
