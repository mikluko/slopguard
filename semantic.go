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
	best, margin := -1, 0.0
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

// fitted is the head this process decides with.
var fitted head

var catalogueOnce sync.Once

// embedAll loads the fitted head, once, and then embeds the texts.
//
// The head comes from the asset built beside the binary. There is no runtime
// fallback: fitting needs the labelled corpus, which lives in the tests, so a
// binary whose asset does not match its corpus judges nothing rather than
// judging against a stale direction.
func embedAll(texts []string) ([][]float32, error) {
	e, err := model()
	if err != nil {
		return nil, err
	}
	catalogueOnce.Do(func() {
		var ok bool
		if fitted, ok = decodeHead(headBytes, fingerprint()); !ok {
			err = errors.New("assets/head.bin was fitted from a corpus this binary no longer carries: go test -update")
		}
	})
	if err != nil {
		return nil, err
	}
	return e.embed(texts)
}
