package main

import (
	"flag"
	"os"
	"testing"
)

var update = flag.Bool("update", false, "recompute assets/exemplars.bin from the exemplars in this source")

// The exemplar vectors ship as an asset, so editing an exemplar without
// regenerating it would leave the binary scoring against the old wording. The
// asset carries a fingerprint of the text it came from, and this test is what
// holds the two together: run `go test -update` after editing any exemplar.
func TestExemplarAsset(t *testing.T) {
	if *update {
		skipWithoutRuntime(t)
		e, err := model()
		if err != nil {
			t.Fatal(err)
		}
		vectors, err := e.embed(exemplars())
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile("assets/exemplars.bin", encode(vectors), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %d exemplar vectors", len(vectors))
		return
	}
	if _, ok := decode(exemplarBytes); !ok {
		t.Fatal("assets/exemplars.bin does not match the exemplars in this source: run go test -update")
	}
}
