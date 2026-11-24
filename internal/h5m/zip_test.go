package h5m

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"io"
	"testing"
)

// Every local header holds the CRC and sizes (no data descriptor), and the archive reads back.
func TestZipLocalHeaders(t *testing.T) {
	files := []File{{"Maps/Multiplayer/T/map.xdb", bytes.Repeat([]byte("<a>b</a>\r\n"), 500)}, {"Maps/Multiplayer/T/empty.txt", nil}}
	data, err := Zip(files)
	if err != nil {
		t.Fatal(err)
	}
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range r.File {
		off, err := f.DataOffset()
		if err != nil {
			t.Fatal(err)
		}
		local := data[off-30-int64(len(f.Name)) : off]
		flags := binary.LittleEndian.Uint16(local[6:])
		crc := binary.LittleEndian.Uint32(local[14:])
		packed := binary.LittleEndian.Uint32(local[18:])
		size := binary.LittleEndian.Uint32(local[22:])
		if flags&0x8 != 0 || crc != crc32.ChecksumIEEE(files[i].Data) || size != uint32(len(files[i].Data)) || uint64(packed) != f.CompressedSize64 {
			t.Errorf("%s: local header flags %#x crc %#x sizes %d / %d", f.Name, flags, crc, packed, size)
		}
		rc, _ := f.Open()
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil || !bytes.Equal(content, files[i].Data) {
			t.Errorf("%s doesn't read back: %v", f.Name, err)
		}
	}
}
