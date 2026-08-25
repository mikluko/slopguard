package main

import "testing"

var probes = []string{
	"Twice is kept for backwards compatibility",
	"kept for backwards compatibility",
	"this was bumped from 3 when the disruption budget changed",
	"the replica count was raised when the budget changed",
	"double no longer clamps v",
}

func TestScores(t *testing.T) {
	skipWithoutRuntime(t)
	texts := append(append([]string{}, probes...), func() []string {
		out := make([]string, len(readings))
		for i, r := range readings {
			out[i] = r.text
		}
		return out
	}()...)
	vectors, err := embedAll(texts)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("baselines: %.3f", catalogue.baseline)
	for i, v := range vectors {
		best, score := -1, -1.0
		for c := range classes {
			if s := excess(v, catalogue.classes[c], catalogue.baseline[c]); s > score {
				best, score = c, s
			}
		}
		contract := excess(v, catalogue.contract, catalogue.baseline[len(classes)])
		want := "probe"
		if i >= len(probes) {
			want = readings[i-len(probes)].class
		}
		t.Logf("want %-10s got %-10s %+.3f  contract %+.3f  margin %+.3f  %s",
			want, classes[best].name, score, contract, score-contract, texts[i])
	}
}
