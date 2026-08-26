package main

import (
	"strings"
	"testing"
	"time"
)

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
	scan(src, golang, []span{{start: 0, end: uint(len(src))}})
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("%d bytes of one comment took %s: the tokenizer is seeing all of it", len(src), took)
	} else {
		t.Logf("%d bytes of one comment in %s", len(src), took)
	}
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
	scan(src, golang, []span{{start: 0, end: uint(len(src))}})
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("%d bytes at depth %d took %s: the parent climb is back", len(src), depth, took)
	} else {
		t.Logf("%d bytes at depth %d in %s", len(src), depth, took)
	}
}
