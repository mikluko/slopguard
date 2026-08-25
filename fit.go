package main

import (
	"encoding/binary"
	"math"
	"slices"
)

// A head is what the semantic pass actually decides with: one direction per
// class, and the score along it above which that class fires.
//
// A direction is fitted rather than listed. Nearest-exemplar scoring matched
// wordings — it recognised the sentences the corpus already spelled and missed
// the rest — because a set of exemplars is a set of points, and what separates
// "this used to allocate on every call" from "the caller must hold the lock" is
// a direction through the space rather than proximity to any one sentence.
type head struct {
	directions [][]float32
	thresholds []float64
}

// precision is the share of a class's firings on the fitting corpus that have
// to be right before its threshold is accepted. The tool's failure mode is
// prose nudged wrongly, so recall is what gives way. The value is measured
// rather than chosen: calibrate_test.go fits at a range of them and reports
// what each costs on comments the fit never saw.
const precision = 0.85

// fit computes a direction and a threshold per class from labelled vectors.
//
// The direction is the difference between what the class occupies and what
// contract prose occupies: the axis along which the two separate, which a
// centroid alone cannot give. The threshold is the lowest score at which the
// class still fires at the required precision, on this same labelled corpus.
func fit(vectors [][]float32, labels []string) head {
	return fitAt(vectors, labels, precision)
}

func fitAt(vectors [][]float32, labels []string, precision float64) head {
	out := head{
		directions: make([][]float32, len(classes)),
		thresholds: make([]float64, len(classes)),
	}
	negative := centroid(vectors, labels, "")
	for i, c := range classes {
		positive := centroid(vectors, labels, c.name)
		if positive == nil || negative == nil {
			out.directions[i] = make([]float32, dimensions)
			out.thresholds[i] = math.Inf(1)
			continue
		}
		out.directions[i] = unit(difference(positive, negative))
		out.thresholds[i] = cut(vectors, labels, c.name, out.directions[i], precision)
	}
	return out
}

// centroid averages the vectors carrying one label, or returns nil when none do.
func centroid(vectors [][]float32, labels []string, label string) []float32 {
	sum := make([]float64, dimensions)
	count := 0
	for i, v := range vectors {
		if labels[i] != label {
			continue
		}
		for k, x := range v {
			sum[k] += float64(x)
		}
		count++
	}
	if count == 0 {
		return nil
	}
	out := make([]float32, dimensions)
	for k := range out {
		out[k] = float32(sum[k] / float64(count))
	}
	return out
}

func difference(a, b []float32) []float32 {
	out := make([]float32, len(a))
	for k := range a {
		out[k] = a[k] - b[k]
	}
	return out
}

func unit(v []float32) []float32 {
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	if norm = math.Sqrt(norm); norm == 0 {
		return v
	}
	for k := range v {
		v[k] = float32(float64(v[k]) / norm)
	}
	return v
}

// cut returns the lowest threshold at which this class fires with the required
// precision, or +Inf when no threshold reaches it and the class is better off
// silent.
func cut(vectors [][]float32, labels []string, name string, direction []float32, precision float64) float64 {
	type scored struct {
		score float64
		right bool
	}
	var scores []scored
	for i, v := range vectors {
		scores = append(scores, scored{dot(v, direction), labels[i] == name})
	}
	slices.SortFunc(scores, func(a, b scored) int {
		switch {
		case a.score > b.score:
			return -1
		case a.score < b.score:
			return 1
		}
		return 0
	})
	best, right, wrong := math.Inf(1), 0, 0
	for _, s := range scores {
		if s.right {
			right++
		} else {
			wrong++
		}
		if right > 0 && float64(right)/float64(right+wrong) >= precision {
			best = s.score
		}
	}
	return best
}

func dot(a, b []float32) float64 {
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

// encodeHead renders a fitted head as the asset, behind the fingerprint of the
// labelled text it was fitted from.
func encodeHead(h head, mark uint64) []byte {
	out := make([]byte, 0, 16+len(h.directions)*(dimensions*4+8))
	out = binary.LittleEndian.AppendUint64(out, mark)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(h.directions)))
	out = binary.LittleEndian.AppendUint32(out, dimensions)
	for i, direction := range h.directions {
		out = binary.LittleEndian.AppendUint64(out, math.Float64bits(h.thresholds[i]))
		for _, v := range direction {
			out = binary.LittleEndian.AppendUint32(out, math.Float32bits(v))
		}
	}
	return out
}

// decodeHead reads the asset back, and reports false when it was fitted from
// text this binary no longer carries.
func decodeHead(blob []byte, mark uint64) (head, bool) {
	if len(blob) < 16 || binary.LittleEndian.Uint64(blob) != mark {
		return head{}, false
	}
	count := int(binary.LittleEndian.Uint32(blob[8:]))
	width := int(binary.LittleEndian.Uint32(blob[12:]))
	if width != dimensions || count != len(classes) || len(blob) != 16+count*(width*4+8) {
		return head{}, false
	}
	out := head{
		directions: make([][]float32, count),
		thresholds: make([]float64, count),
	}
	at := 16
	for i := range out.directions {
		out.thresholds[i] = math.Float64frombits(binary.LittleEndian.Uint64(blob[at:]))
		at += 8
		direction := make([]float32, width)
		for k := range direction {
			direction[k] = math.Float32frombits(binary.LittleEndian.Uint32(blob[at:]))
			at += 4
		}
		out.directions[i] = direction
	}
	return out, true
}
