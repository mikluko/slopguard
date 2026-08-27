package rule

import (
	"testing"

	"github.com/mikluko/slopguard/internal/comment"
)

// Whether a comment restates the code is a fact about the pair, so the table
// carries both halves.
//
// Most cases here are answered without the model: the words are already in the
// line below, or they are not. A case marked `reads` is one only the semantic
// pass answers, and it skips where the model is absent — this header claimed no
// such case existed while one sat at the top of the table, and it is why the
// repository's own "test without the model" job was failing.
var echoCases = []struct {
	name string
	lang *language
	src  string
	want bool
	// reads marks a case the structural rule declines by design, leaving the
	// model as the only thing that can answer it.
	reads bool
}{
	{
		// A comment heading a run of statements, which [echoes] declines
		// deliberately: it can only see the first statement, so scoring the
		// comment against it is a claim about a span the rule never read. The
		// model has no such restriction and catches this one.
		name: "the words are the identifiers",
		lang: golang,
		src: `package p

func f(items []int) {
	// increment the counter
	counter++
	_ = items
}
`,
		want:  true,
		reads: true,
	},
	{
		name: "a comment pointing outward",
		lang: golang,
		src: `package p

func f(v int) int {
	// the caller has already bounded this, so it cannot overflow
	return v * 2
}
`,
	},
	{
		name: "a summary over a block is not a restatement",
		lang: golang,
		src: `package p

func f(items []int) int {
	// sum the items
	total := 0
	for _, item := range items {
		total += item
	}
	return total
}
`,
	},
	{
		name: "a doc comment naming its own symbol",
		lang: golang,
		src: `package p

// Counter returns the counter.
func Counter() int { return counter }
`,
	},
	{
		name: "a heading is not a restatement of what it heads",
		lang: hcl,
		src: `variable "node" {
  # User data
  type = string
}
`,
	},
	{
		name: "python, snake case split",
		lang: python,
		src: `def f(user):
    # save the user profile
    save_user_profile(user)
`,
		want: true,
	},
}

func TestEchoes(t *testing.T) {
	for _, c := range echoCases {
		t.Run(c.name, func(t *testing.T) {
			if !c.want || c.reads {
				// A case that must stay silent has to hold against the model
				// as well, since that is the other way this class fires. A case
				// the structural rule declines by design needs it outright.
				skipWithoutRuntime(t)
			}
			found := false
			for _, f := range scan([]byte(c.src), c.lang, []comment.Span{{Start: 0, End: uint(len(c.src))}}) {
				if f.Class == "echo" || f.Class == "tautology" {
					found = true
				}
			}
			if found != c.want {
				t.Fatalf("read as restatement = %v, want %v", found, c.want)
			}
		})
	}
}
