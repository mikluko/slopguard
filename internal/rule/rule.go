// Package rule decides what to object to. It reads the comments a file holds
// and returns the ones whose claim belongs somewhere else: in a commit message,
// in a symbol's doc, in a test, or nowhere.
//
// The rules divide into two kinds and the division is the reason this package
// runs them in one order. A structural rule answers from the shape of the
// comment and the code under it, on any machine, for nothing. The semantic pass
// answers from what a sentence means, costs a forward pass, and needs a model
// that may not be there. What the model recognises is [model]'s to say; what a
// recognition earns is decided here.
package rule

import (
	"cmp"
	"hash/fnv"
	"slices"
	"strconv"

	"github.com/mikluko/slopguard/internal/comment"
	"github.com/mikluko/slopguard/internal/lang"
	"github.com/mikluko/slopguard/internal/model"
	"github.com/mikluko/slopguard/internal/prose"
)

// set builds a lookup from a list of names. Every package here keeps its own:
// it is six lines, and importing another package for it would couple them over
// nothing.
func set(names ...string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		out[name] = true
	}
	return out
}

// A Finding is one comment to object to, at the line it starts on. The score
// ranks findings against each other, so that a nudge naming three carries the
// three the rules are surest of.
type Finding struct {
	Line   uint
	Reason string
	Score  float64
	// Class names the rule that fired, for the log and for nothing else.
	Class string
	// Key identifies this comment by what it says, so that a comment already
	// named once is not named again when the agent's own edit re-enters.
	Key uint64
}

// Judge parses src and returns what the rules object to in the text just
// written, worst first.
func Judge(src []byte, language *lang.Language, added []comment.Span) []Finding {
	candidates, release := comment.Scan(src, language, added)
	defer release()
	return Weigh(candidates, language, src)
}

// Weigh runs the structural rules over the candidates and hands whatever they
// leave standing to the semantic pass, in one batch: the model is loaded once
// and only when there is something for it to read.
//
// Sitting inside a function body is a bias, not a verdict. A line that points
// outward, at a constraint enforced somewhere the reader cannot see, earns its
// place there; only what the semantic pass recognises is nudged.
func Weigh(candidates []comment.Comment, language *lang.Language, src []byte) []Finding {
	verdicts := make([]verdict, len(candidates))
	var pending []int
	// One namespace for the file, not one per comment: it is a walk over the
	// whole tree, and every comment in the file would get the same answer.
	spelled := &namespace{src: src}
	if len(candidates) > 0 {
		spelled.root = candidates[0].Root
	}
	for i, c := range candidates {
		// A licence notice is exempt from every rule, which means leaving it out
		// of both passes. Returning the zero verdict from [inspect] would not do
		// it: that is the value meaning "the shape rules nothing out", and it is
		// what puts a comment in front of the model.
		// Not inside a function body: a run reads as one comment, so a marker
		// opening any line of it pardons every line stacked under it, and a
		// licence header buried in a body is a commented-out block that happens
		// to start with one. Nothing legitimate puts a notice there.
		if prose.Notice(c.Body) && !c.Buried {
			continue
		}
		if verdicts[i] = inspect(c, language, src, spelled); verdicts[i].reason == "" {
			pending = append(pending, i)
		}
	}
	if len(pending) > 0 {
		texts := make([][]string, len(pending))
		bias := make([]float64, len(pending))
		for j, i := range pending {
			texts[j] = prose.Opening(prose.Split(candidates[i].Text))
			position := 0.0
			if !candidates[i].Doc && candidates[i].Buried {
				position = model.BuriedBias
			}
			bias[j] = model.Allowance(position, len(texts[j]))
		}
		// What the model hands back is what it recognised. Turning that into a
		// verdict is this layer's job, and the next few lines are why the two
		// are separate: a reading alone is not a finding.
		for j, read := range model.Judge(texts, bias) {
			// Restatement is a relation, so the model's reading of one is
			// taken only where the code supports it: a single line below, at
			// least two content words, and at least one of them already
			// spelled by that line. A section banner — "User data" over
			// `ami_type` — shares nothing with what it heads and restates
			// nothing, whatever it reads like on its own.
			if read.Class == "tautology" && !restates(candidates[pending[j]], src) {
				continue
			}
			verdicts[pending[j]] = verdict{read.Reason, read.Score, read.Class}
		}
	}
	var out []Finding
	for i, v := range verdicts {
		if v.reason != "" {
			out = append(out, Finding{
				Line:   candidates[i].Line,
				Reason: v.reason,
				Score:  v.score,
				Class:  v.class,
				Key:    site(candidates[i].Text),
			})
		}
	}
	slices.SortStableFunc(out, func(a, b Finding) int {
		return cmp.Compare(b.Score, a.Score)
	})
	return out
}

// verdict is what one pass makes of a comment: the nudge, and how sure the pass
// is of it. The score orders the findings and nothing else.
type verdict struct {
	reason string
	score  float64
	class  string
}

// inspect returns why the shape of a comment rules it out, or the zero verdict
// to leave that judgment to the semantic pass. The first rule that fires wins:
// one line of nudge per comment.
func inspect(c comment.Comment, language *lang.Language, src []byte, spelled *namespace) verdict {
	if leftover(c, language, src, spelled) {
		return verdict{"commented-out code: delete it, or make it real", 1, "leftover"}
	}
	if echoes(c, src) {
		return verdict{model.ReasonFor("tautology"), 0.95, "echo"}
	}
	if empty := hollows(c, src); len(empty) > 0 && wordy(c, language) {
		return verdict{
			"padded documentation: cut " + strconv.Quote(empty[0].text) + ", which " +
				hollowReasons[empty[0].why] +
				". A sentence past the first is earned by a precondition, an invariant the caller must hold, a failure mode, or a cost the signature cannot show",
			0.4 + 0.3*float64(len(empty))/float64(prose.Sentences(c.Text)),
			"hollow",
		}
	}
	return verdict{}
}

// wordy reports whether a comment has anywhere else to put what will not fit,
// which is what the nudge is asking. It names three homes — package
// documentation, symbol documentation, a test — and fires only where at least
// one of them exists.
//
// Two places have none. File documentation is already the first of those homes,
// so running long there is the correct form and not a finding: the `testing`
// package's own doc is eighty sentences and every one of them earns its place.
// And a language with no function has no symbol to document and no test to move
// a claim into: in YAML and HCL the nudge resolves to "delete it", which on the
// only record of a constraint is worse than saying nothing.
func wordy(c comment.Comment, language *lang.Language) bool {
	return !c.Heads && len(language.Functions) > 0
}

// site identifies a comment by what it says rather than by where it sits, so
// that the agent's own corrective edit — which moves every line below it — is
// still recognised as the same comment.
func site(text string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(prose.Normalize(text)))
	return h.Sum64()
}
