package h5m

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"hash/crc32"
	"time"
)

// File is a file to put in an archive.
type File struct {
	Name string
	Data []byte
}

// Zip writes files as a zip the game can read: each local header holds the CRC and sizes. The game's
// reader takes them from there: an archive written by zip.Writer.Create (sizes in a data descriptor after
// the data, flag bit 3, zeros in the local header) is "corrupted" for the map editor and missing from the
// game's map list.
func Zip(files []File) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	now := time.Now()
	for _, f := range files {
		var packed bytes.Buffer
		fw, err := flate.NewWriter(&packed, flate.BestCompression)
		if err != nil {
			return nil, err
		}
		if _, err := fw.Write(f.Data); err != nil {
			return nil, err
		}
		if err := fw.Close(); err != nil {
			return nil, err
		}
		header := &zip.FileHeader{
			Name:               f.Name,
			Method:             zip.Deflate,
			Flags:              0x2, // deflate "maximum compression", as in the editor's archives
			CRC32:              crc32.ChecksumIEEE(f.Data),
			CompressedSize64:   uint64(packed.Len()),
			UncompressedSize64: uint64(len(f.Data)),
			Modified:           now,
		}
		out, err := w.CreateRaw(header)
		if err != nil {
			return nil, err
		}
		if _, err := out.Write(packed.Bytes()); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
