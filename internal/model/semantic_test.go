package model

import (
	"testing"

	"github.com/mikluko/slopguard/internal/prose"
)

// The semantic table is the specification for what the model is asked to
// recognise: the comment as it would be written, and the class it belongs to,
// or "" for prose that states a contract and must be left alone.
var readings = []struct {
	text  string
	class string
}{
	{"previously this pointed at the docker hub mirror", "history"},
	{"we swapped the map for a slice here", "history"},
	{"this used to allocate on every call", "history"},
	{"dropped the retry loop, the client handles it", "history"},
	{"added to fix the flaky test on CI", "history"},
	{"bumped the deadline because CI is slower than a laptop", "history"},
	{"multiply it by two", "tautology"},
	{"close the connection", "tautology"},
	{"kept for backwards compatibility", "compat"},
	{"left here so that v1 clients keep working", "compat"},
	{"legacy field, new code should use spec instead", "compat"},
	{"we walk the tree, collect the comments, and then score each one", "narrative"},
	{"this function first validates the input and then writes it to disk", "narrative"},

	{"verdict returns the reason to deny the command, or an empty string to allow it", ""},
	{"Replicas is the number of pods to run", ""},
	{"the caller must hold the lock", ""},
	{"requests are per replica", ""},
	{"ingress is off by default", ""},
	{"Timeout bounds how long a request may take", ""},
	{"default values for the api chart", ""},
	{"image pull secret for the private registry", ""},
	{"double returns v twice over", ""},
	{"order is significant: callers index by position", ""},
	{"Put zeroes the buffer, so it is safe to reuse", ""},
	{"a command that does not parse is allowed", ""},
	{"raising this above four needs a node pool change", ""},
	{"the zero value is ready to use", ""},
	{"cpu limits are per container, not per pod", ""},
	{"returns every durable the observer no longer runs", ""},
	{"these fields are enforced by the operator and stay unset here", ""},
	{"the caller has already bounded v, so this cannot overflow", ""},
}

func TestJudge(t *testing.T) {
	skipWithoutRuntime(t)
	texts := make([]string, len(readings))
	for i, r := range readings {
		texts[i] = r.text
	}
	comments := make([][]string, len(texts))
	for i, text := range texts {
		comments[i] = prose.Split(text)
	}
	reasons := Judge(comments, make([]float64, len(texts)))

	// Two directions, judged differently. Nudging prose that states a contract
	// is the failure this tool cannot afford, so it fails the test. Missing a
	// comment that should have been nudged is logged: which classes reach which
	// wordings is what the held-out set measures, and a recall this table could
	// enforce would only be the recall on the sentences it already contains.
	missed := 0
	for i, r := range readings {
		fired := reasons[i].Reason != ""
		switch {
		case r.class == "" && fired:
			t.Errorf("nudged contract prose: %q\n  %s", r.text, reasons[i].Reason)
		case r.class != "" && !fired:
			missed++
			t.Logf("missed %-10s %s", r.class, r.text)
		case r.class != "" && reasons[i].Reason != ReasonFor(r.class):
			t.Logf("read %-10s as another class: %s", r.class, r.text)
		}
	}
	t.Logf("%d of %d labelled comments went unnudged", missed, len(readings))
}

// The phrase lists carry the common wording when the model is unavailable.
func TestLiteralFallback(t *testing.T) {
	for _, c := range []struct {
		text string
		want bool
	}{
		{"kept for backwards compatibility", true},
		{"Replicas is the number of pods to run", false},
	} {
		if got := literal(c.text) != ""; got != c.want {
			t.Errorf("literal(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

// Go requires a declaration comment to end in a period and PEP 257 requires it
// of a docstring, so the two spellings below are one comment as far as any
// reader is concerned and the class has to score them alike.
func TestTerminatorDoesNotMoveTheScore(t *testing.T) {
	skipWithoutRuntime(t)
	for _, claim := range []string{
		"kept for backwards compatibility with older callers",
		"legacy field, do not use it in new code",
		"increment the counter",
		"the returned slice is valid only until the next call on this reader",
	} {
		readings := Judge([][]string{{claim}, {claim + "."}}, []float64{0, 0})
		plain, stopped := readings[0], readings[1]
		if plain.Class != stopped.Class {
			t.Errorf("%q read as %q and %q with a period", claim, plain.Class, stopped.Class)
			continue
		}
		if delta := plain.Score - stopped.Score; delta > 0.0005 || delta < -0.0005 {
			t.Errorf("%q scored %.4f and %.4f with a period, a gap of %.4f",
				claim, plain.Score, stopped.Score, delta)
		}
	}
}

// bare has to leave the words alone. Trimming into the sentence would change
// what is embedded rather than how it is spelled.
func TestBareKeepsTheWords(t *testing.T) {
	for _, pair := range [][2]string{
		{"kept for backwards compatibility.", "kept for backwards compatibility"},
		{"is it ready?", "is it ready"},
		{"  spaced out  ", "spaced out"},
		{"no terminator here", "no terminator here"},
		{"ends in an ellipsis...", "ends in an ellipsis"},
		{"a version like 1.2.3 stays", "a version like 1.2.3 stays"},
	} {
		if got := bare(pair[0]); got != pair[1] {
			t.Errorf("bare(%q) = %q, want %q", pair[0], got, pair[1])
		}
	}
}

// skipWithoutRuntime skips a test that needs the model, for either reason it
// can be absent: no library to load, or the semantic pass switched off.
func skipWithoutRuntime(t *testing.T) {
	t.Helper()
	if why := Absent(); why != "" {
		t.Skip(why)
	}
}
