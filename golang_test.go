package main

import (
	"strings"
	"testing"
)

// The commented-out-code rule reads a comment as source, and a tree-sitter
// grammar accepts text the Go compiler does not. These are the two sides of
// that gap, taken from the Go standard library: notation the grammar parses
// cleanly and the compiler would refuse, and code somebody really did switch
// off.
var goLeftovers = []struct {
	name string
	body string
	want bool
}{
	{"bessel identity", "// j0(x) = 1/sqrt(pi) * (P(0,x)*cc - Q(0,x)*ss) / sqrt(x)", false},
	{"complex invariant", "// complex(e, f) = n/m", false},
	{"bare comparison", "// f == g", false},
	{"rewrite rule match", "// match: (Add16 (Const16 [c]) (Const16 [d]))", false},
	{"rewrite rule cond", "// cond: c+d == c+d", false},
	{"rewrite rule result", "// result: (MOVVconst [int64(val)])", false},
	{"constant provenance", "// (664-0.03306235651)*2**20", false},
	{"labelled note", "// Have: s += expr", false},
	{"table semantics", "// arg0 == arg1", false},

	{"disabled if", "// if initmap != nil {", true},
	{"disabled assignment", "// cr  = buildReg(\"CR\")", true},
	{"disabled call", "// fmt.Println(\"x\")", true},
	{"disabled multiline", "// if n.Op == lexical.Range {\n//\treturn false\n// }", true},
	{"disabled declaration", "// var count int", true},
	{"disabled loop", "// for i := range xs {\n//\tuse(i)\n// }", true},
}

func TestGoLeftover(t *testing.T) {
	for _, c := range goLeftovers {
		t.Run(c.name, func(t *testing.T) {
			found := only(t, inBody(c.body), golang)
			if got := found.class == "leftover"; got != c.want {
				t.Errorf("leftover = %v, want %v\n  %s\n  read as: %s %s",
					got, c.want, c.body, found.class, found.reason)
			}
		})
	}
}

// inBody puts a comment inside a function, which is where code gets commented
// out and where a comment above a declaration would instead be its doc.
func inBody(body string) string {
	var indented []string
	for _, line := range strings.Split(body, "\n") {
		indented = append(indented, "\t"+line)
	}
	return "package p\n\nfunc f() {\n" + strings.Join(indented, "\n") + "\n\tprintln(1)\n}\n"
}

// only scans a whole file and returns the first finding, or the zero finding
// where nothing fired.
func only(t *testing.T, src string, lang *language) finding {
	t.Helper()
	if found := scan([]byte(src), lang, []span{{start: 0, end: uint(len(src))}}); len(found) > 0 {
		return found[0]
	}
	return finding{}
}

// A licence notice is exempt from every rule: some of them ask in their own
// text to be preserved, and shortening one is not a style call.
func TestNoticeIsExempt(t *testing.T) {
	src := "// Copyright 2009 The Go Authors. All rights reserved.\n" +
		"// Use of this source code is governed by a BSD-style\n" +
		"// license that can be found in the LICENSE file.\n" +
		"// One. Two. Three. Four. Five. Six. Seven. Eight. Nine. Ten. Eleven.\n" +
		"// Twelve. Thirteen. Fourteen. Fifteen. Sixteen. Seventeen. Eighteen.\n" +
		"package p\n"
	if f := only(t, src, golang); f.reason != "" {
		t.Errorf("nudged a licence notice: %s %s", f.class, f.reason)
	}
}

// A comment sharing a line with code is a note about that code. The notation
// those notes use is the notation the commented-out-code rule reads as source.
func TestTrailingNote(t *testing.T) {
	src := "package p\n\nfunc f(x1 float64) float64 {\n" +
		"\tx2 := Sqrt(x1) // x2 = sqrt(1 - x*x)\n\treturn x2\n}\n"
	found := scan([]byte(src), golang, []span{{start: 0, end: uint(len(src))}})
	for _, f := range found {
		if f.class == "leftover" {
			t.Errorf("read a trailing note as commented-out code: line %d", f.line)
		}
	}
}
