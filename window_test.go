package main

import (
	"fmt"
	"testing"
)

// TestWindow reports what each candidate margin window costs and buys on the
// comments the fit never saw. It asserts nothing: it is how the constant in
// fit.go was chosen, and 0.02 was chosen because the band from 0.01 to 0.02
// nudges no contract prose at all and still reaches the most true positives —
// wider throws recall away for nothing, and narrower nudges prose that was
// right.
func TestWindow(t *testing.T) {
	skipWithoutRuntime(t)
	e, err := model()
	if err != nil {
		t.Fatal(err)
	}
	corpus, labels := exemplars()
	corpusVectors, err := e.embed(corpus)
	if err != nil {
		t.Fatal(err)
	}
	texts, want := heldOut()
	vectors, err := e.embed(texts)
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []float64{0.0, 0.01, 0.02, 0.03, 0.05, 0.08} {
		h := fitWith(corpusVectors, labels, precision, width)
		line, wrong := "", 0
		for ci, c := range classes {
			fired, right := 0, 0
			for i, v := range vectors {
				if fires(v, h) != ci {
					continue
				}
				fired++
				if want[i] == c.name {
					right++
				}
			}
			wrong += fired - right
			line += fmt.Sprintf(" %s %d/%d", c.name[:4], right, fired)
		}
		t.Logf("window %.2f  thresholds %.3f %.3f %s  wrong %d",
			width, h.thresholds[0], h.thresholds[1], line, wrong)
	}
}

// fires returns the class a vector fires, or -1, under the rule the binary
// runs.
func fires(vector []float32, h head) int {
	best, over := -1, clear
	for k := range classes {
		if s := dot(vector, h.directions[k]) - h.thresholds[k]; s > over {
			best, over = k, s
		}
	}
	return best
}

// heldOut returns the labelled comments no part of the fit has seen.
func heldOut() (texts, classes []string) {
	inCorpus := map[string]bool{}
	for _, mined := range mined {
		for _, text := range mined {
			inCorpus[text] = true
		}
	}
	for _, r := range labelled {
		if !inCorpus[r.text] {
			texts = append(texts, r.text)
			classes = append(classes, r.class)
		}
	}
	return texts, classes
}
