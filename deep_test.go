package main

import (
	"strings"
	"testing"
	"time"
)

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
