package main

import (
	"errors"
	"sync"
)

// A class is something the semantic pass recognises a comment as, and the nudge
// that recognition earns. What decides membership is a direction fitted from
// labelled comments, in fit.go; the labelled comments themselves are in
// corpus.go and in the held-out table the fitting split comes from.
type class struct {
	name   string
	reason string
	// exemplars are this class's share of the fitting corpus.
	exemplars []string
}

// judge returns the nudge each comment earns, or the zero verdict for the ones
// it leaves alone. The model does the reading, alone: a phrase list cannot tell
// "the consumer no longer runs these durables", which is a contract, from "this
// no longer clamps", which is a change event, and on real code that costs more
// in false positives than it buys. Where ONNX Runtime is missing the phrases
// are all there is, and judging worse beats staying silent.
//
// Each comment arrives as its sentences, and bias moves its threshold by
// whatever [allowance] made of where it sits and how long it is. A caller
// passing a bare zero is judging a one-line comment above a declaration,
// whatever it actually handed over.
func judge(comments [][]string, bias []float64) []verdict {
	var flat []string
	var owner []int
	for i, sentences := range comments {
		for _, sentence := range sentences {
			flat = append(flat, sentence)
			owner = append(owner, i)
		}
	}
	found := make([]verdict, len(flat))
	vectors, err := embedAll(flat)
	for j, sentence := range flat {
		if err != nil {
			found[j] = verdict{literal(sentence), 0.5, "phrase"}
			continue
		}
		found[j] = nearest(vectors[j], bias[owner[j]])
	}
	out := make([]verdict, len(comments))
	for j, v := range found {
		if v.reason != "" && v.score > out[owner[j]].score {
			out[owner[j]] = v
		}
	}
	return out
}

// reasonFor returns the nudge a named class carries, so that one class speaks
// with one wording however it was recognised.
func reasonFor(name string) string {
	for _, c := range classes {
		if c.name == name {
			return c.reason
		}
	}
	return ""
}

// nearest names the class a comment reads as, or the zero verdict when no class
// clears its threshold. The score reported is how far past it the sentence went,
// which is what ranks one finding against another.
func nearest(vector []float32, bias float64) verdict {
	best, margin := -1, clear
	for i := range classes {
		if over := dot(vector, fitted.directions[i]) - (fitted.thresholds[i] - bias); over > margin {
			best, margin = i, over
		}
	}
	if best < 0 {
		return verdict{}
	}
	return verdict{classes[best].reason, margin, classes[best].name}
}

// clear is how far past its threshold a sentence has to land before the class
// is taken to have recognised anything: the width of the noise the thresholds
// are fitted through rather than a reading.
//
// TestClear prints what it costs at each threshold a comment can meet. At the
// shipped tilt no contract prose is nudged at any of these values, so what it
// buys is not visible in the labelled corpus and what it costs is: 0.005 keeps
// eleven catches of twenty-three where 0.02 keeps ten and 0.04 keeps five.
// It is set at the width of the noise the thresholds are fitted through, which
// is the smallest value that means anything at all.
const clear = 0.005

// perDraw is how much each additional sentence in a comment raises the bar for
// all of them, since the verdict is the best of them and a longer comment
// otherwise clears the same threshold for no better reason than length.
//
// This is insurance rather than a measured win, and TestPerDraw says so: at the
// tilt this ships with, contract rows read three to a comment survive 25 of 25
// with no correction at all, and every step costs a catch — eleven of
// twenty-three at zero, ten at 0.005, nine at 0.02. The case it guards cannot
// appear in that measurement, because every labelled row is a single sentence
// and the three-to-a-comment reading is built by grouping them.
//
// It stays because the inflation is real whatever the corpus can show: the
// verdict is the best of a comment's sentences, so a longer comment clears the
// same threshold more often for no better reason than length. It is set as
// small as means anything.
const perDraw = 0.005

// allowance is how far a comment's threshold moves before it is judged: down by
// what its position already argues against it, and back up by one step for each
// sentence past the first.
//
// Every sentence is another draw against the same threshold and the verdict is
// the best of them, so without the second term a long comment clears the bar for
// no better reason than length.
func allowance(bias float64, sentences int) float64 {
	return bias - float64(sentences-1)*perDraw
}

// fitted is the head this process decides with, and fittedErr is why there is
// none. The error is kept beside it rather than only returned from the first
// call: sync.Once runs its function once, so a caller that reported the failure
// and returned would leave every later caller reading a head that was never
// filled in.
var (
	fitted        head
	fittedErr     error
	catalogueOnce sync.Once
)

// embedAll loads the fitted head, once, and then embeds the texts.
//
// The head comes from the asset built beside the binary, and a binary whose
// asset does not match its corpus fails here rather than judging along a stale
// direction: fitting needs the labelled corpus, which lives in the tests.
// What [judge] does with that failure is [judge]'s business.
func embedAll(texts []string) ([][]float32, error) {
	e, err := model()
	if err != nil {
		return nil, err
	}
	catalogueOnce.Do(func() {
		var ok bool
		if fitted, ok = decodeHead(headBytes, fingerprint()); !ok {
			fittedErr = errors.New("assets/head.bin was fitted from a corpus this binary no longer carries: go test -update")
		}
	})
	if fittedErr != nil {
		return nil, fittedErr
	}
	return e.embed(texts)
}
