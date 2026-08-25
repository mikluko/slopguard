package main

import (
	"flag"
	"math"
	"os"
	"testing"
)

var update = flag.Bool("update", false, "refit assets/head.bin from the corpus in this source")

// The head ships as an asset, so editing the corpus without refitting would
// leave the binary judging along the old directions. The asset carries a
// fingerprint of the labelled text it was fitted from, and this test is what
// holds the two together: run `go test -update` after editing corpus.go.
func TestHeadAsset(t *testing.T) {
	if *update {
		skipWithoutRuntime(t)
		e, err := model()
		if err != nil {
			t.Fatal(err)
		}
		texts, labels := exemplars()
		vectors, err := e.embed(texts)
		if err != nil {
			t.Fatal(err)
		}
		h := fit(vectors, labels)
		if err := os.WriteFile("assets/head.bin", encodeHead(h, fingerprint()), 0o644); err != nil {
			t.Fatal(err)
		}
		for i, c := range classes {
			t.Logf("%-10s threshold %.3f", c.name, h.thresholds[i])
		}
		return
	}
	h, ok := decodeHead(headBytes, fingerprint())
	if !ok {
		t.Fatal("assets/head.bin does not match the corpus in this source: run go test -update")
	}
	for i, c := range classes {
		if math.IsInf(h.thresholds[i], 1) {
			t.Errorf("%s reaches no threshold at the required precision, so it can never fire", c.name)
		}
	}
}
