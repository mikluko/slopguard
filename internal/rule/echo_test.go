package rule

import (
	"testing"

	"github.com/mikluko/slopguard/internal/comment"
)

// Whether a comment restates the code is a fact about the pair, so the table
// carries both halves. No case here reaches the model: the words are already
// in the line below, or they are not.
var echoCases = []struct {
	name string
	lang *language
	src  string
	want bool
}{
	{
		name: "the words are the identifiers",
		lang: golang,
		src: `package p

func f(items []int) {
	// increment the counter
	counter++
	_ = items
}
`,
		want: true,
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
			if !c.want {
				// A case that must stay silent has to hold against the model
				// as well, since that is the other way this class fires.
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
