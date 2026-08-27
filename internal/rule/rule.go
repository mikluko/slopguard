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
	"os"
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
	// Source is what the comment says with its markers removed, line breaks
	// kept. It is here so a caller holding the text this write replaced can ask
	// whether these lines were live code a moment ago, which is the difference
	// between a comment that reads like source and one that just stopped being
	// source. Nothing in this package reads it.
	Source string
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
	return WeighAt(candidates, language, src, 0)
}

// WeighAt is [Weigh] with every semantic threshold moved by offset, which is
// what a sweep varies to trace what the rules would catch and what they would
// nudge at each tilt. A positive offset lowers the thresholds, so the classes
// fire on more; the structural rules carry no threshold and do not move.
//
// It exists so that a calibration measures the pipeline rather than the model
// underneath it. Sweeping [model.Judge] directly skips the structural gates
// this function applies to a reading, and a curve traced that way describes
// something the tool never does.
func WeighAt(candidates []comment.Comment, language *lang.Language, src []byte, offset float64) []Finding {
	return WeighOnly(candidates, language, src, offset, "")
}

// WeighOnly is [WeighAt] with every class but one switched off, which is what
// measuring a class costs rather than what it adds on top of the ones before it.
//
// The rules run in a fixed precedence and a comment gets one verdict, so a table
// built from a whole run reports each class's share of a partition: `tautology`
// only ever sees what `leftover` and `echo` declined. Read as though it were the
// class on its own, that understates every rule below the first and makes the
// order look like a ranking. An empty name runs all of them.
func WeighOnly(candidates []comment.Comment, language *lang.Language, src []byte, offset float64, only string) []Finding {
	return weigh(candidates, language, src, offset, only, Wider())
}

// weigh is [WeighOnly] with the wider set decided by the caller rather than by
// the environment. The environment is read once, at the exported edge, so that
// the judgment itself takes no process-global state: a test naming what it wants
// does not have to set a variable every other test in the package then sees.
func weigh(candidates []comment.Comment, language *lang.Language, src []byte, offset float64, only string, wider bool) []Finding {
	verdicts := make([]verdict, len(candidates))
	var pending []int
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
		if verdicts[i] = inspectOnly(c, language, src, only, wider); verdicts[i].reason == "" {
			pending = append(pending, i)
		}
	}
	if only != "" && only != "tautology" && only != "compat" {
		// Every structural class was asked for by name and answered above; the
		// semantic pass would only add what this call is meant to exclude.
		pending = nil
	}
	// The model is loaded lazily, so skipping the pass skips the 86 MB and the
	// second it costs. `only` naming a semantic class overrides the default,
	// which is what lets the scorer measure them without the hook running them.
	if !wider && only != "tautology" && only != "compat" {
		pending = nil
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
			bias[j] = model.Allowance(position, len(texts[j])) + offset
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
			verdicts[pending[j]] = keeping(verdict{read.Reason, read.Score, read.Class}, only)
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
				Source: candidates[i].Body,
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

// widerEnv turns the classes measured at no recall back on, all of them: the two
// structural ones this package gates, and the semantic pass the rule layer gates
// beside them.
const widerEnv = "SLOPGUARD_WIDER"

// Wider reports whether the classes that catch nothing measurable are wanted.
//
// Off by default, and the default is what the numbers say. On the mined corpus
// the whole pipeline catches 21 comments people deleted and nudges 25 they kept;
// `leftover` alone catches 20 and nudges 5. So the other four classes buy one
// catch for twenty false positives, and lift goes from 7.6 to about 13 when they
// go. On the Go standard library they are three quarters of everything the tool
// says. And the semantic half of them costs a second per invocation and 86 MB of
// embedded model: measured on one real file, 1.207s against 0.026s.
//
// This is a default rather than a deletion because the corpus provably cannot
// see the defect those classes target, so the case against them is a cost
// argument and not a verdict on whether they are right. Somebody who wants the
// wider reading sets one variable and has it back.
//
// The value is parsed as a boolean, so `SLOPGUARD_WIDER=0` is off. Read as mere
// presence, the one documented way to turn this on read its own negation as a
// yes, and took the second of model load with it.
func Wider() bool {
	on, err := strconv.ParseBool(os.Getenv(widerEnv))
	return err == nil && on
}

// keeping returns the verdict where it is the class asked for, and the zero
// verdict otherwise. An empty name keeps everything.
func keeping(v verdict, only string) verdict {
	if only == "" || v.class == only {
		return v
	}
	return verdict{}
}

// inspectOnly runs the structural rules with every one but the named class
// switched off.
//
// Filtering [inspect]'s answer instead does not do it, and the difference is the
// whole point of the table this feeds. `inspect` returns at its first firing
// rule, so a comment `leftover` claimed never reaches `echoes`, and discarding
// that verdict afterwards reports `echo` as silent where it would have fired.
// That is the partition the isolated table exists to replace.
func inspectOnly(c comment.Comment, language *lang.Language, src []byte, only string, wider bool) verdict {
	if only == "" {
		return inspect(c, language, src, wider)
	}
	switch only {
	case "leftover":
		if leftover(c, language, src) {
			return verdict{"commented-out code: delete it, or make it real", 1, "leftover"}
		}
	case "echo":
		if echoes(c, src) {
			return verdict{model.ReasonFor("tautology"), 0.95, "echo"}
		}
	case "hollow":
		if empty := hollows(c, src); len(empty) > 0 && wordy(c, language) {
			return thin(c, empty)
		}
	}
	return verdict{}
}

// inspect runs the structural rules in precedence order.
//
// `echo` and `hollow` are behind [Wider], and default to off. Neither has ever
// caught a comment anybody deleted, across nine versions of the mined corpus and
// four definitions of its negative class, while `echo` supplies 172 findings on
// the Go standard library and `hollow` four across 15,372 files of which all
// four were read and all four were wrong.
//
// The corpus cannot see what `echo` targets, and that is honestly argued: a
// trivial comment bothers nobody, so nobody deletes it, and fifteen of the
// twenty comments the two classes fire on there are trivial by Steidl's
// criterion. What is measurable is the other side. On real code `echo` is right
// about two thirds of the time and its false positives are section headings; on
// a faithful port to the population Steidl actually validated, documentation
// against the signature, the rule scores below chance.
//
// So the value is unmeasurable and the volume is not. They stay in the tree,
// tested, one environment variable away.
func inspect(c comment.Comment, language *lang.Language, src []byte, wider bool) verdict {
	if leftover(c, language, src) {
		return verdict{"commented-out code: delete it, or make it real", 1, "leftover"}
	}
	if !wider {
		return verdict{}
	}
	if echoes(c, src) {
		return verdict{model.ReasonFor("tautology"), 0.95, "echo"}
	}
	if empty := hollows(c, src); len(empty) > 0 && wordy(c, language) {
		return thin(c, empty)
	}
	return verdict{}
}

// thin is the hollow rule's verdict, named so that the isolated pass and the
// whole pipeline cannot word it differently.
func thin(c comment.Comment, empty []padded) verdict {
	return verdict{
		"padded documentation: cut " + strconv.Quote(empty[0].text) + ", which " +
			hollowReasons[empty[0].why] +
			". A sentence past the first is earned by a precondition, an invariant the caller must hold, a failure mode, or a cost the signature cannot show",
		0.4 + 0.3*float64(len(empty))/float64(prose.Sentences(c.Text)),
		"hollow",
	}
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
