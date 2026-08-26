package model

import (
	"fmt"
	"testing"
)

// Nothing here asserts anything. Each of these is the procedure a constant in
// this package was chosen by — the required precision and the margin window in
// fit.go, and clear, perDraw and BuriedBias in semantic.go — and what it prints
// is what those choices have to stay defensible against. A run that prints
// numbers nobody likes is a reason to move a constant, not a failure.
//
// They are gathered here, and prefixed, because a file that can only log is a
// different kind of thing from a file that states a contract. The contracts are
// in fit_test.go and semantic_test.go, and they fail.

// TestCalibrate fits the head at a range of required precisions and reports
// what each costs on the comments the fit never saw.
func TestCalibrate(t *testing.T) {
	skipWithoutRuntime(t)
	e, err := model()
	if err != nil {
		t.Fatal(err)
	}

	corpus, labels := Exemplars()
	corpusVectors, err := e.embed(corpus)
	if err != nil {
		t.Fatal(err)
	}
	texts, want := heldOut()
	vectors, err := e.embed(texts)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("fitting corpus %d comments, held out %d", len(corpus), len(texts))
	for _, p := range []float64{0.70, 0.75, 0.80, 0.85, 0.90, 0.95} {
		h := FitAt(corpusVectors, labels, p)
		line := fmt.Sprintf("precision %.2f  ", p)
		wrong, missed := 0, 0
		for ci, c := range classes {
			fired, right, labelledAs := 0, 0, 0
			for i, v := range vectors {
				if want[i] == c.name {
					labelledAs++
				}
				// The same rule the binary runs, margin included: a procedure
				// that justifies a threshold by measuring a different rule
				// justifies nothing.
				if fires(v, h) != ci {
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

// TestWindow reports what each candidate margin window costs and buys on the
// comments the fit never saw. 0.02 was chosen because the band from 0.01 to
// 0.02 nudges no contract prose at all and still reaches the most true
// positives — wider throws recall away for nothing, and narrower nudges prose
// that was right.
func TestWindow(t *testing.T) {
	skipWithoutRuntime(t)
	e, err := model()
	if err != nil {
		t.Fatal(err)
	}
	corpus, labels := Exemplars()
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
		h := FitWith(corpusVectors, labels, precision, width)
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

// TestPerDraw reports what each correction for a comment's length costs and
// buys, on the same held-out rows read one sentence at a time and three to a
// comment. It is how perDraw was chosen.
func TestPerDraw(t *testing.T) {
	skipWithoutRuntime(t)
	texts, want := heldOut()

	// A row labelled for a class this build does not carry is catchable by
	// nothing, so counting it among the positives would credit a correction
	// with rows no setting could ever reach.
	live := map[string]bool{}
	for _, c := range classes {
		live[c.name] = true
	}
	var contract []string
	var positives [][]string
	for i, text := range texts {
		switch {
		case want[i] == "":
			contract = append(contract, text)
		case live[want[i]]:
			positives = append(positives, []string{text})
		}
	}
	// A positive padded with contract prose is the case the correction has to
	// survive: the comment is still one that should be nudged, and it now
	// arrives with two innocent neighbours.
	padded := make([][]string, len(positives))
	for i := range positives {
		padded[i] = []string{positives[i][0], contract[i%len(contract)], contract[(i+7)%len(contract)]}
	}
	var docs [][]string
	for at := 0; at+3 <= len(contract); at += 3 {
		docs = append(docs, contract[at:at+3])
	}

	for _, step := range []float64{0.0, 0.005, 0.01, 0.02, 0.04} {
		left := 0
		for _, v := range Judge(docs, biasFor(docs, step)) {
			if v.Reason == "" {
				left++
			}
		}
		caught := 0
		for _, v := range Judge(padded, biasFor(padded, step)) {
			if v.Reason != "" {
				caught++
			}
		}
		t.Logf("perDraw %.3f  contract three-to-a-comment %d/%d  positives padded to three %d/%d",
			step, left, len(docs), caught, len(padded))
	}
}

// biasFor is [Allowance] with the step under test, for a set of comments read
// inside a function body.
func biasFor(comments [][]string, step float64) []float64 {
	out := make([]float64, len(comments))
	for i, sentences := range comments {
		out[i] = BuriedBias - float64(len(sentences)-1)*step
	}
	return out
}

// TestClear reports what each noise floor costs and buys, across the range of
// thresholds a comment can meet: above a declaration at bias 0, and inside a
// function body where the bar is lower. It is how clear and BuriedBias were
// both chosen, and the rows behave differently enough that a number quoted from
// one of them says little about another.
func TestClear(t *testing.T) {
	skipWithoutRuntime(t)
	texts, want := heldOut()
	// embedAll rather than the embedder directly: it is what loads the head
	// this reads, and running alone there is nothing else to have loaded it.
	vectors, err := embedAll(texts)
	if err != nil {
		t.Fatal(err)
	}
	live := map[string]bool{}
	for _, c := range classes {
		live[c.name] = true
	}
	// Literals, not the constant this fits: a grid written in terms of
	// BuriedBias prints its row twice and drops whichever alternative the
	// constant used to hold, which is the row a reader came for.
	for _, bias := range []float64{0, 0.03, 0.06, 0.09} {
		for _, floor := range []float64{0.0, 0.005, 0.01, 0.02, 0.04} {
			caught, nudged, catchable := 0, 0, 0
			for i, v := range vectors {
				if live[want[i]] {
					catchable++
				}
				best, over := -1, floor
				for k := range classes {
					if s := Dot(v, fitted.directions[k]) - (fitted.thresholds[k] - bias); s > over {
						best, over = k, s
					}
				}
				switch {
				case best < 0:
				case want[i] == "":
					nudged++
				case classes[best].name == want[i]:
					caught++
				}
			}
			t.Logf("bias %.2f  clear %.3f  caught %d/%d  contract nudged %d/75",
				bias, floor, caught, catchable, nudged)
		}
	}
}

// fires returns the class a vector fires, or -1, under the rule the binary
// runs.
func fires(vector []float32, h head) int {
	best, over := -1, clear
	for k := range classes {
		if s := Dot(vector, h.directions[k]) - h.thresholds[k]; s > over {
			best, over = k, s
		}
	}
	return best
}

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
// the threshold fitted for that class. It is what a threshold, an exemplar or a
// test expectation is chosen from.
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
			line += " " + classes[c].name[:4] + fmtScore(Dot(v, fitted.directions[c])-fitted.thresholds[c])
		}
		t.Logf("%s  %s", line, texts[i])
	}
}

// fmtScore reports what the binary would do with a score, margin included: a
// diagnostic that models a rule the tool does not run explains nothing.
func fmtScore(v float64) string {
	if v > clear {
		return " FIRES"
	}
	return " ....."
}
