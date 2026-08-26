// Package model is the semantic half of the judgment: an embedder, a head of
// fitted directions, and the labelled corpus both were built from.
//
// What it hands back is what it recognised a sentence as. What that recognition
// earns is the rule layer's business, and keeping the two apart is what lets
// this package own its calibration — the corpus, the thresholds, and the tests
// that price them — without knowing what a finding is.
package model

import (
	"errors"
	"strings"
	"sync"

	"github.com/mikluko/slopguard/internal/prose"
)

// A Class is something the semantic pass recognises a comment as, and the nudge
// that recognition earns. What decides membership is a direction fitted from
// labelled comments, in fit.go; the labelled comments themselves are in
// corpus.go and in the held-out table the fitting split comes from.
type Class struct {
	name   string
	reason string
	// exemplars are this class's share of the fitting corpus.
	exemplars []string
}

// A Reading is what this package made of one comment: the class it read the
// comment as, the wording that class speaks with, and how far past its threshold
// the comment landed. The zero Reading is "nothing recognised".
//
// It stops here. What a recognition earns — whether it becomes a finding, and
// what else has to hold before it does — is the rule layer's judgment, and this
// package is better for not knowing.
type Reading struct {
	Reason string
	Score  float64
	Class  string
}

// Judge returns what each comment was read as, or the zero Reading for the ones
// nothing recognised. The model does the reading, alone: a phrase list cannot
// tell "the consumer no longer runs these durables", which is a contract, from
// "this no longer clamps", which is a change event, and on real code that costs
// more in false positives than it buys. Where ONNX Runtime is missing the
// phrases are all there is, and judging worse beats staying silent.
//
// Each comment arrives as its sentences, and bias moves its threshold by
// whatever [Allowance] made of where it sits and how long it is. A caller
// passing a bare zero is judging a one-line comment above a declaration,
// whatever it actually handed over.
func Judge(comments [][]string, bias []float64) []Reading {
	var flat []string
	var owner []int
	for i, sentences := range comments {
		for _, sentence := range sentences {
			flat = append(flat, sentence)
			owner = append(owner, i)
		}
	}
	found := make([]Reading, len(flat))
	vectors, err := embedAll(flat)
	for j, sentence := range flat {
		if err != nil {
			found[j] = Reading{literal(sentence), 0.5, "phrase"}
			continue
		}
		found[j] = nearest(vectors[j], bias[owner[j]])
	}
	out := make([]Reading, len(comments))
	for j, v := range found {
		if v.Reason != "" && v.Score > out[owner[j]].Score {
			out[owner[j]] = v
		}
	}
	return out
}

// literal is the degraded reading of a comment, used where ONNX Runtime is
// missing and the semantic pass cannot run. It matches phrases rather than
// meaning, so it catches the common wording of a self-justification and nothing
// beyond it.
//
// It sits beside the pass it stands in for rather than with the text
// primitives. There it named the wording each class speaks with, and the class
// list is here, so the two files each reached into the other.
func literal(text string) string {
	if compat(text) {
		return ReasonFor("compat")
	}
	return ""
}

// compat reports whether a comment justifies a symbol by its own history rather
// than by its contract.
func compat(text string) bool {
	haystack := prose.Normalize(text)
	for _, phrase := range []string{
		"backwards compatibility", "backward compatibility", "for compatibility",
		"kept for", "left for", "legacy callers", "old callers",
	} {
		if strings.Contains(haystack, " "+phrase+" ") {
			return true
		}
	}
	return false
}

// Speaks reports whether any class's wording carries the given text. It is how
// a caller checks that a nudge it is asserting on is one this package could
// actually produce, rather than a string that has since been reworded.
func Speaks(text string) bool {
	for _, c := range classes {
		if text != "" && strings.Contains(c.reason, text) {
			return true
		}
	}
	return false
}

// ReasonFor returns the nudge a named class carries, so that one class speaks
// with one wording however it was recognised — by the model here, or by a
// structural rule that reached the same conclusion without one.
func ReasonFor(name string) string {
	for _, c := range classes {
		if c.name == name {
			return c.reason
		}
	}
	return ""
}

// nearest names the class a comment reads as, or the zero Reading when no class
// clears its threshold. The score reported is how far past it the sentence went,
// which is what ranks one finding against another.
func nearest(vector []float32, bias float64) Reading {
	best, margin := -1, clear
	for i := range classes {
		if over := Dot(vector, fitted.directions[i]) - (fitted.thresholds[i] - bias); over > margin {
			best, margin = i, over
		}
	}
	if best < 0 {
		return Reading{}
	}
	return Reading{classes[best].reason, margin, classes[best].name}
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

// BuriedBias is how much less evidence a comment inside a function body needs
// before this pass names it. Prose is harder to justify there, but a line
// pointing at a constraint enforced elsewhere still justifies itself, so this
// tilts the reading rather than deciding it.
//
// Where a comment sits is the scanner's fact; what that fact is worth is this
// package's, which is why the number lives here. It was measured against this
// corpus by these tests and means nothing apart from the thresholds it moves.
//
// The sweep in window_test.go prices the tilt: held out, 0.03 catches one more
// comment than no tilt at all and still nudges nothing, while 0.06 catches three
// more and nudges one piece of contract prose, and 0.09 catches four and nudges
// two. This is the last setting at which every contract reading is perfect.
const BuriedBias = 0.03

// Allowance is how far a comment's threshold moves before it is judged: down by
// what its position already argues against it, and back up by one step for each
// sentence past the first.
//
// Every sentence is another draw against the same threshold and the reading is
// the best of them, so without the second term a long comment clears the bar for
// no better reason than length.
func Allowance(bias float64, sentences int) float64 {
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
		if fitted, ok = DecodeHead(HeadBytes, Fingerprint()); !ok {
			fittedErr = errors.New("assets/head.bin was fitted from a corpus this binary no longer carries: go test -update")
		}
	})
	if fittedErr != nil {
		return nil, fittedErr
	}
	return e.embed(texts)
}
