package rule

import (
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
