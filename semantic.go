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
// Each comment arrives as its sentences, and bias lowers a comment's threshold
// by however much its position already argues against it.
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
		// Every sentence of a comment is another draw against the same
		// threshold, and the verdict is the best of them, so a long comment is
		// likelier to clear it for no better reason than length. Each extra
		// draw raises the bar the others have to clear.
		draws := float64(len(comments[owner[j]]) - 1)
		found[j] = nearest(vectors[j], bias[owner[j]]-draws*perDraw)
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
// The case that prompted it — a stock chart header clearing by 0.003 — is now
// answered structurally, by requiring a restatement to share a word with the
// line it restates. What is left is the noise floor, and the classes have very
// different bands: measured held out, 0.02 costs restatement two true positives
// of fifteen and buys one fewer false positive of seventy-five, while
// self-justification does not notice it until 0.04. It sits just above the
// evidence rather than four times it.
const clear = 0.005

// perDraw is how much each additional sentence in a comment raises the bar for
// all of them. Read one sentence at a time, the tool leaves 75 of 75 held-out
// contract comments alone; read three to a comment, as a real doc comment
// arrives, it left 23 of 25 without this and 24 of 25 with it — the same
// threshold met three times over. It costs nothing on the one-line comments
// most findings come from.
const perDraw = 0.02

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
