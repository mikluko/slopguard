package model

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
		texts, labels := Exemplars()
		vectors, err := e.embed(texts)
		if err != nil {
			t.Fatal(err)
		}
		h := fit(vectors, labels)
		if err := os.WriteFile("assets/head.bin", EncodeHead(h, Fingerprint()), 0o644); err != nil {
			t.Fatal(err)
		}
		for i, c := range classes {
			t.Logf("%-10s threshold %.3f", c.name, h.thresholds[i])
		}
		return
	}
	h, ok := DecodeHead(HeadBytes, Fingerprint())
	if !ok {
		t.Fatal("assets/head.bin does not match the corpus in this source: run go test -update")
	}
	for i, c := range classes {
		if math.IsInf(h.thresholds[i], 1) {
			t.Errorf("%s reaches no threshold at the required precision, so it can never fire", c.name)
		}
	}
}

// A head that does not describe this binary's corpus is refused rather than
// read: every rejection is checked before any length is trusted for indexing.
func TestHeadAssetRejects(t *testing.T) {
	mark := Fingerprint()
	whole := head{
		directions: make([][]float32, len(classes)),
		thresholds: make([]float64, len(classes)),
	}
	for i := range whole.directions {
		whole.directions[i] = make([]float32, dimensions)
	}
	good := EncodeHead(whole, mark)
	for _, c := range []struct {
		name string
		blob []byte
	}{
		{"empty", nil},
		{"a header and nothing else", good[:16]},
		{"truncated body", good[:len(good)-4]},
		{"another corpus", EncodeHead(whole, mark+1)},
		{"trailing bytes", append(append([]byte{}, good...), 0)},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, ok := DecodeHead(c.blob, mark); ok {
				t.Fatal("accepted a head it should have refused")
			}
		})
	}
}
