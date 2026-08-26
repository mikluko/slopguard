package comment

import (
	"strings"
	"testing"
	"time"

	"github.com/mikluko/slopguard/internal/lang"
)

// Wall-clock rather than behaviour, which is why these sit apart from the tests
// that state a contract: they are the only ones here that a loaded machine can
// fail, and each of them stands in for a shape that once held the hook for
// minutes.

// A `{{` in prose has no closer, so the search for one has to be bounded or it
// runs to the end of the file — once per paragraph, which is quadratic. The
// shape that costs the most is one unpaired opener per paragraph, since every
// one of them starts a fresh scan.
func TestBlankIsLinear(t *testing.T) {
	build := func(paragraphs int) []byte {
		var b strings.Builder
		for i := 0; i < paragraphs; i++ {
			b.WriteString("# the {{ here opens nothing and never closes\n")
			b.WriteString("key: value\n\n")
		}
		return []byte(b.String())
	}
	small, large := build(2000), build(8000)

	elapsed := func(src []byte) time.Duration {
		start := time.Now()
		blank(src, lang.YAML)
		return time.Since(start)
	}
	// Warm the allocator so the first call does not pay for both.
	elapsed(small)

	quick, slow := elapsed(small), elapsed(large)
	// Four times the input. Linear is about 4, the quadratic this replaced was
	// about 16, and the bound is loose enough that a shared machine does not
	// fail it.
	if slow > 8*quick+2*time.Millisecond {
		t.Errorf("4x the input cost %v against %v, which is not linear", slow, quick)
	}
	t.Logf("%d paragraphs %v, %d paragraphs %v", 2000, quick, 8000, slow)
}

// Deeply nested code used to cost twenty seconds for a 64 KB file, because
// asking a comment whether it sat inside a function climbed the tree with
// Parent(), which tree-sitter answers by re-descending from the root. The walk
// down carries the answer now, and this is what would notice if it stopped.
func TestDeepNestingIsLinear(t *testing.T) {
	const depth = 16000
	var b strings.Builder
	b.WriteString("package p\n\nfunc f() {\n")
	for range depth {
		b.WriteString("\tif true {\n")
	}
	b.WriteString("\t// close the connection\n")
	for range depth {
		b.WriteString("\t}\n")
	}
	b.WriteString("}\n")
	src := []byte(b.String())

	start := time.Now()
	_, release := Scan(src, lang.Go, []Span{{Start: 0, End: uint(len(src))}})
	release()
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("%d bytes at depth %d took %s: the parent climb is back", len(src), depth, took)
	} else {
		t.Logf("%d bytes at depth %d in %s", len(src), depth, took)
	}
}
