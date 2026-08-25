package main

import (
	_ "embed"
	"hash/fnv"
)

// The fitted head travels in the binary beside the model: four directions and
// four thresholds, a few kilobytes, computed once when the corpus changes.

//go:embed assets/head.bin
var headBytes []byte

// exemplars returns every labelled comment in corpus order: each class in turn,
// then the contract set. The labels follow in the same order.
func exemplars() ([]string, []string) {
	var texts, labels []string
	add := func(label string, from []string) {
		for _, text := range from {
			texts = append(texts, text)
			labels = append(labels, label)
		}
	}
	for _, c := range classes {
		add(c.name, c.exemplars)
		add(c.name, mined[c.name])
	}
	add("", contract)
	add("", mined[""])
	return texts, labels
}

// fingerprint identifies the labelled text this binary carries, so that an
// edited exemplar is noticed rather than silently scored against a direction
// fitted from the old wording.
func fingerprint() uint64 {
	h := fnv.New64a()
	texts, labels := exemplars()
	for i, text := range texts {
		h.Write([]byte(labels[i]))
		h.Write([]byte{'='})
		h.Write([]byte(text))
		h.Write([]byte{0})
	}
	return h.Sum64()
}
