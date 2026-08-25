package main

import "testing"

// probes are sentences under examination right now: what a test expects and
// the tool no longer says, or the other way round.
var probes = []string{
	"previously this pointed at the docker hub mirror",
	"Twice is kept for backwards compatibility",
	"kept for backwards compatibility",
	"we swapped the map for a slice here",
	"multiply it by two",
}

// TestScores prints where a sentence falls along each class direction, against
// the threshold fitted for that class. It asserts nothing: it is what a
// threshold, an exemplar or a test expectation is chosen from.
func TestScores(t *testing.T) {
	skipWithoutRuntime(t)
	texts := append([]string{}, probes...)
	for _, r := range readings {
		texts = append(texts, r.text)
	}
	vectors, err := embedAll(texts)
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range classes {
		t.Logf("%-10s threshold %+.3f", c.name, fitted.thresholds[i])
	}
	for i, v := range vectors {
		line := ""
		for c := range classes {
			line += " " + classes[c].name[:4] + fmtScore(dot(v, fitted.directions[c])-fitted.thresholds[c])
		}
		t.Logf("%s  %s", line, texts[i])
	}
}

func fmtScore(v float64) string {
	if v > 0 {
		return " FIRES"
	}
	return " ....."
}
