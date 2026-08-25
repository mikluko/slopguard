package main

import (
	"errors"
	"sync"
)

// A class is something the semantic pass recognises a comment as, and the nudge
// that recognition earns. Membership is decided against contract documentation
// rather than by a threshold alone: a comment has to read more like this class
// than like a statement of what a symbol promises before it is named. The
// exemplars themselves are in corpus.go.
type class struct {
	name      string
	reason    string
	floor     float64
	exemplars []string
}

// margin is how far a class has to beat contract documentation before slopguard
// says anything. Comments live close together in this space; without it, every
// doc that mentions a change at all would be nudged.
const margin = 0.05

// judge returns the nudge each comment earns, or the zero verdict for the ones
// it leaves alone. The model does the reading, alone: a phrase list cannot tell
// "the consumer no longer runs these durables", which is a contract, from "this
// no longer clamps", which is a change event, and on real code that costs more
// in false positives than it buys. Where ONNX Runtime is missing the phrases
// are all there is, and judging worse beats staying silent.
//
// Each comment arrives as its sentences, and bias lowers a comment's floor by
// however much its position already argues against it.
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
			found[j] = verdict{literal(sentence), 0.5}
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

// nearest names the class a comment reads as, or the zero verdict when it reads
// as contract documentation.
//
// The bias moves the floor and nothing else. Contract documentation is the
// negative class, and a comment that reads more like one than like its nearest
// class is left alone wherever it sits: letting position erode that comparison
// would turn the gate off exactly where the tool objects most.
func nearest(vector []float32, bias float64) verdict {
	best, score := -1, 0.0
	for i := range classes {
		if s := excess(vector, catalogue.classes[i], catalogue.baseline[i]); s > score {
			best, score = i, s
		}
	}
	if best < 0 || score < classes[best].floor-bias {
		return verdict{}
	}
	if score-excess(vector, catalogue.contract, catalogue.baseline[len(classes)]) < margin {
		return verdict{}
	}
	return verdict{classes[best].reason, score}
}

// excess is a set's affinity above what an unrelated sentence scores against
// that same set. Affinity grows with the size of the set it is measured
// against, so the raw numbers of a twelve-exemplar class and a ninety-exemplar
// contract set are not comparable; the baseline is what makes them so.
func excess(vector []float32, against [][]float32, baseline float64) float64 {
	return affinity(vector, against) - baseline
}

// affinity scores a vector against a set of exemplars as the mean of its two
// closest matches. One exemplar is a coincidence away from carrying a whole
// class; a set this small cannot afford a centroid either, which would let the
// spread of the wordings wash the score out.
func affinity(vector []float32, against [][]float32) float64 {
	first, second := 0.0, 0.0
	for _, other := range against {
		switch s := cosine(vector, other); {
		case s > first:
			first, second = s, first
		case s > second:
			second = s
		}
	}
	return (first + second) / 2
}

// baselines returns, for every set, what a sentence belonging to some other set
// scores against it. That is the level a set reaches on prose it has nothing to
// do with, and it rises with the size of the set.
func baselines(sets [][][]float32) []float64 {
	out := make([]float64, len(sets))
	for i, set := range sets {
		sum, count := 0.0, 0
		for j, other := range sets {
			if i == j {
				continue
			}
			for _, vector := range other {
				sum += affinity(vector, set)
				count++
			}
		}
		if count > 0 {
			out[i] = sum / float64(count)
		}
	}
	return out
}

// catalogue holds the exemplar vectors, read once per process, and the level
// each set reaches on prose that is not its own.
var catalogue struct {
	classes  [][][]float32
	contract [][]float32
	baseline []float64
	ready    bool
}

var catalogueOnce sync.Once

// embedAll fills the catalogue, once, and then embeds the texts.
//
// The catalogue comes from the asset built beside the binary. Falling back to
// the model is for a working copy whose exemplars have been edited since: it
// costs a forward pass over every exemplar, which is most of what an invocation
// would then take.
func embedAll(texts []string) ([][]float32, error) {
	e, err := model()
	if err != nil {
		return nil, err
	}
	catalogueOnce.Do(func() {
		vectors, ok := decode(exemplarBytes)
		if !ok {
			if vectors, err = e.embed(exemplars()); err != nil {
				return
			}
		}
		at := 0
		for _, c := range classes {
			catalogue.classes = append(catalogue.classes, vectors[at:at+len(c.exemplars)])
			at += len(c.exemplars)
		}
		catalogue.contract = vectors[at:]
		catalogue.baseline = baselines(append(append([][][]float32{}, catalogue.classes...), catalogue.contract))
		catalogue.ready = true
	})
	if err != nil {
		return nil, err
	}
	if !catalogue.ready {
		return nil, errors.New("the exemplars are not embedded")
	}
	return e.embed(texts)
}
