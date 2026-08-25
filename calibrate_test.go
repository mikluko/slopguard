package main

import (
	"fmt"
	"testing"
)

// TestCalibrate fits the head at a range of required precisions and reports
// what each costs on the comments the fit never saw. It asserts nothing: it is
// the procedure a threshold rule is chosen by, and the numbers it prints are
// what the choice in fit.go has to be defensible against.
func TestCalibrate(t *testing.T) {
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

	inCorpus := map[string]bool{}
	for _, texts := range mined {
		for _, text := range texts {
			inCorpus[text] = true
		}
	}
	var texts, want []string
	for _, r := range labelled {
		if !inCorpus[r.text] {
			texts = append(texts, r.text)
			want = append(want, r.class)
		}
	}
	vectors, err := e.embed(texts)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("fitting corpus %d comments, held out %d", len(corpus), len(texts))
	for _, p := range []float64{0.70, 0.75, 0.80, 0.85, 0.90, 0.95} {
		h := fitAt(corpusVectors, labels, p)
		line := fmt.Sprintf("precision %.2f  ", p)
		wrong, missed := 0, 0
		for ci, c := range classes {
			fired, right, labelledAs := 0, 0, 0
			for i, v := range vectors {
				if want[i] == c.name {
					labelledAs++
				}
				best, over := -1, 0.0
				for k := range classes {
					if s := dot(v, h.directions[k]) - h.thresholds[k]; s > over {
						best, over = k, s
					}
				}
				if best != ci {
					continue
				}
				fired++
				if want[i] == c.name {
					right++
				}
			}
			wrong += fired - right
			missed += labelledAs - right
			line += fmt.Sprintf("%s %d/%d  ", c.name[:4], right, fired)
		}
		t.Logf("%s wrong %d  missed %d", line, wrong, missed)
	}
}
