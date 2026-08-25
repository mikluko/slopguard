package main

import (
	_ "embed"
	"encoding/binary"
	"hash/fnv"
	"math"
)

// The exemplar vectors are computed once, when the asset is regenerated, and
// travel in the binary beside the model. Embedding 100-odd exemplars to judge
// four sentences was most of what a hook invocation cost.

//go:embed assets/exemplars.bin
var exemplarBytes []byte

// exemplars returns every exemplar in catalogue order: each class in turn, then
// the contract set.
func exemplars() []string {
	var out []string
	for _, c := range classes {
		out = append(out, c.exemplars...)
	}
	return append(out, contract...)
}

// fingerprint identifies the exemplar text this binary carries, so that an
// edited exemplar is noticed rather than silently scored against a stale
// vector.
func fingerprint() uint64 {
	h := fnv.New64a()
	for _, text := range exemplars() {
		h.Write([]byte(text))
		h.Write([]byte{0})
	}
	return h.Sum64()
}

// encode renders exemplar vectors as the asset: the fingerprint of the text
// they came from, the shape, and the values.
func encode(vectors [][]float32) []byte {
	out := make([]byte, 0, 16+len(vectors)*dimensions*4)
	out = binary.LittleEndian.AppendUint64(out, fingerprint())
	out = binary.LittleEndian.AppendUint32(out, uint32(len(vectors)))
	out = binary.LittleEndian.AppendUint32(out, dimensions)
	for _, vector := range vectors {
		for _, v := range vector {
			out = binary.LittleEndian.AppendUint32(out, math.Float32bits(v))
		}
	}
	return out
}

// decode reads the asset back, and reports false when it does not describe the
// exemplars this binary was built with.
func decode(blob []byte) ([][]float32, bool) {
	if len(blob) < 16 || binary.LittleEndian.Uint64(blob) != fingerprint() {
		return nil, false
	}
	count := int(binary.LittleEndian.Uint32(blob[8:]))
	width := int(binary.LittleEndian.Uint32(blob[12:]))
	if width != dimensions || count != len(exemplars()) || len(blob) != 16+count*width*4 {
		return nil, false
	}
	values := blob[16:]
	out := make([][]float32, count)
	for i := range out {
		vector := make([]float32, width)
		for k := range vector {
			vector[k] = math.Float32frombits(binary.LittleEndian.Uint32(values[(i*width+k)*4:]))
		}
		out[i] = vector
	}
	return out, true
}
