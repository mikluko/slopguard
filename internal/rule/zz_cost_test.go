package rule

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mikluko/slopguard/internal/lang"
)

// Wall-clock rather than behaviour, which is why this sits apart from the tests
// that state a contract: it is the only one here that a loaded machine can
// fail, and it stands in for a shape that once held the hook for minutes.
// The scanner's own bounds are measured in that package.

// A comment with no sentence punctuation in it reaches the tokenizer as one
// unit, and the tokenizer is quadratic in its input: 900 KB of base64 in a
// comment ran for nine minutes before it was killed. Generated text is exactly
// the text that is both long and unpunctuated, so the two conditions arrive
// together.
func TestLongCommentIsBounded(t *testing.T) {
	skipWithoutRuntime(t)
	var b strings.Builder
	b.WriteString("package p\n\n// ")
	for range 40000 {
		b.WriteString("aGVsbG8gd29ybGQgdGhpcyBpcyBiYXNlNjQ=")
	}
	b.WriteString("\nfunc f() {}\n")
	src := []byte(b.String())

	start := time.Now()
	scan(src, lang.Go, whole(src))
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("%d bytes of one comment took %s: the tokenizer is seeing all of it", len(src), took)
	} else {
		t.Logf("%d bytes of one comment in %s", len(src), took)
	}
}

// The identifier set the namespace veto reads is a walk over the whole file,
// and a file is read for as many comments as the scanner examines. Walking per
// comment makes the cost the product of the two, which is what this bounds: a
// large file, and every comment in it shaped like the assignment the veto
// gates.
func TestNamespaceIsWalkedPerFile(t *testing.T) {
	var b strings.Builder
	b.WriteString("package p\n\n")
	for i := range 20000 {
		fmt.Fprintf(&b, "var declared%d int\n", i)
	}
	b.WriteString("\nfunc f() {\n")
	for i := range 200 {
		fmt.Fprintf(&b, "\t// cited%d = elsewhere%d + 1\n\tprintln(%d)\n", i, i, i)
	}
	b.WriteString("}\n")
	src := []byte(b.String())

	start := time.Now()
	scan(src, lang.Go, whole(src))
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("%d bytes with a comment on every other line took %s: the file is being walked per comment", len(src), took)
	} else {
		t.Logf("%d bytes with a comment on every other line in %s", len(src), took)
	}
}
