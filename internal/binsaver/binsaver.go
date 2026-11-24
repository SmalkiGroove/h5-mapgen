// Package binsaver reads and writes the engine's binary saver encoding, used by the terrain files (and the
// save files).
//
// A field is a tag byte, a length, then the content. The length is a varint: one byte n<<1 when n < 128,
// else a little-endian u32 (n<<1)|1. A content is either raw bytes or a list of fields; the encoding
// doesn't say which, so the parser takes a schema: the set of tag paths whose content is a list of fields.
package binsaver

import (
	"encoding/binary"
	"fmt"
)

// Node is a field: a leaf (Data) or a container (Children).
type Node struct {
	Tag       byte
	Container bool
	Data      []byte
	Children  []Node
}

// Schema is the set of tag paths (from the root) whose content is parsed as fields.
type Schema map[string]bool

// NewSchema builds a schema from tag paths, e.g. NewSchema([]byte{1}, []byte{1, 1}).
func NewSchema(paths ...[]byte) Schema {
	s := Schema{}
	for _, p := range paths {
		s[string(p)] = true
	}
	return s
}

// ReadLength decodes a length at data[i:], returning it and the index after it.
func ReadLength(data []byte, i int) (int, int, error) {
	if i >= len(data) {
		return 0, i, fmt.Errorf("length at %#x past the end", i)
	}
	if data[i]&1 == 0 {
		return int(data[i] >> 1), i + 1, nil
	}
	if i+4 > len(data) {
		return 0, i, fmt.Errorf("long length at %#x past the end", i)
	}
	return int(binary.LittleEndian.Uint32(data[i:]) >> 1), i + 4, nil
}

// AppendLength encodes a length, in the short form whenever n < 128.
func AppendLength(out []byte, n int) []byte {
	if n < 128 {
		return append(out, byte(n<<1))
	}
	return binary.LittleEndian.AppendUint32(out, uint32(n<<1)|1)
}

// Parse returns the fields of data.
func Parse(data []byte, schema Schema) ([]Node, error) {
	return parse(data, schema, 0, len(data), nil)
}

func parse(data []byte, schema Schema, start, end int, path []byte) ([]Node, error) {
	var nodes []Node
	for i := start; i < end; {
		tag := data[i]
		n, j, err := ReadLength(data, i+1)
		if err != nil {
			return nil, err
		}
		child := append(append([]byte{}, path...), tag)
		if j+n > end {
			return nil, fmt.Errorf("field %v at %#x runs past its parent (%#x > %#x)", child, i, j+n, end)
		}
		node := Node{Tag: tag}
		if schema[string(child)] {
			node.Container = true
			if node.Children, err = parse(data, schema, j, j+n, child); err != nil {
				return nil, err
			}
		} else {
			node.Data = data[j : j+n]
		}
		nodes = append(nodes, node)
		i = j + n
	}
	return nodes, nil
}

// Serialize encodes fields.
func Serialize(nodes []Node) []byte {
	return appendNodes(nil, nodes)
}

func appendNodes(out []byte, nodes []Node) []byte {
	for _, node := range nodes {
		content := node.Data
		if node.Container {
			content = Serialize(node.Children)
		}
		out = append(out, node.Tag)
		out = AppendLength(out, len(content))
		out = append(out, content...)
	}
	return out
}

// Find returns the first child with this tag, or nil.
func Find(nodes []Node, tag byte) *Node {
	for i := range nodes {
		if nodes[i].Tag == tag {
			return &nodes[i]
		}
	}
	return nil
}

// FindAll returns every child with this tag.
func FindAll(nodes []Node, tag byte) []*Node {
	var out []*Node
	for i := range nodes {
		if nodes[i].Tag == tag {
			out = append(out, &nodes[i])
		}
	}
	return out
}

// U32 decodes a leaf holding a little-endian u32.
func (n *Node) U32() uint32 {
	return binary.LittleEndian.Uint32(n.Data)
}

// U32Bytes encodes a little-endian u32.
func U32Bytes(v uint32) []byte {
	return binary.LittleEndian.AppendUint32(nil, v)
}

// Leaf and Container build nodes.
func Leaf(tag byte, data []byte) Node         { return Node{Tag: tag, Data: data} }
func Container(tag byte, children ...Node) Node { return Node{Tag: tag, Container: true, Children: children} }
