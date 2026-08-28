package rule

import (
	"strings"
	"testing"
	"time"

	"github.com/mikluko/slopguard/internal/comment"
	"github.com/mikluko/slopguard/internal/lang"
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

	// An equation broken across lines leaves a paren open, so [balance] closes
	// it and the closer lands on the statement's own end byte. Descending into
	// a node that fills the fragment exactly hands back its parts, which match
	// no rule and pass by default.
	{"unbalanced identity", "// j0(x) = 1/sqrt(pi) * (P(0,x)*cos(X) - Q(0,x)*sin(X)", false},
	{"unbalanced invariant", "// complex(e, f) = n/(m", false},
	// A bare label reaches goLegal only if the statement_list holding the run
	// is unwrapped first.
	{"bare label", "// Cases:", false},
	{"output label", "// Output:", false},
	// Illegal one level down is illegal.
	{"nested comparison", "// if x {\n//\ty == z\n// }", false},
	// A conversion is not a call, and a built-in that exists for its value
	// cannot stand alone.
	{"conversion", "// int64(x)", false},
	{"builtin length", "// len(b)", false},
	{"builtin new", "// new(T)", false},
	{"conversion to any", "// any(x)", false},
	{"unsafe size", "// unsafe.Sizeof(x)", false},

	{"disabled if", "// if initmap != nil {", true},
	{"disabled assignment", "// cr  = buildReg(\"CR\")", true},
	{"disabled call", "// fmt.Println(\"x\")", true},
	{"disabled multiline", "// if n.Op == lexical.Range {\n//\treturn false\n// }", true},
	{"disabled declaration", "// var count int", true},
	{"disabled loop", "// for i := range xs {\n//\tuse(i)\n// }", true},
	// Go puts a label on any statement, so the veto is narrowed to the one a
	// note takes: a label on a loop is running code.
	{"labelled loop", "// Loop:\n//\tfor i := range xs {\n//\t\tuse(i)\n//\t}", true},
}

// A whole function, a method or an import is commented out at file scope, and
// is an error inside a function body. Trying the one position made the rule
// silent on the canonical shape of dead Go.
func TestGoLeftoverAtFileScope(t *testing.T) {
	for _, c := range []struct {
		name string
		body string
	}{
		{"function", "// func helper() int {\n//\treturn 1\n// }"},
		{"import", "// import \"fmt\""},
		{"method", "// func (t *T) M() {\n//\tt.x = 1\n// }"},
	} {
		c := c
		t.Run(c.name, func(t *testing.T) {
			src := "package p\n\n" + c.body + "\n\nfunc g() {}\n"
			found := scan([]byte(src), golang, []comment.Span{{Start: 0, End: uint(len(src))}})
			for _, f := range found {
				if f.Class == "leftover" {
					return
				}
			}
			t.Errorf("commented-out declaration not reported: %v", found)
		})
	}
}

func TestGoLeftover(t *testing.T) {
	for _, c := range goLeftovers {
		t.Run(c.name, func(t *testing.T) {
			found := only(t, inBody(c.body), golang)
			if got := found.Class == "leftover"; got != c.want {
				t.Errorf("leftover = %v, want %v\n  %s\n  read as: %s %s",
					got, c.want, c.body, found.Class, found.Reason)
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
func only(t *testing.T, src string, language *lang.Language) Finding {
	t.Helper()
	if found := scan([]byte(src), language, []comment.Span{{Start: 0, End: uint(len(src))}}); len(found) > 0 {
		return found[0]
	}
	return Finding{}
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
	if f := only(t, src, golang); f.Reason != "" {
		t.Errorf("nudged a licence notice: %s %s", f.Class, f.Reason)
	}
}

// The marker has to open a line. A comment that merely mentions one is prose,
// and where a run of lines reads as one comment, matching anywhere in it would
// exempt everything stacked under a header.
func TestNoticeIsAnchored(t *testing.T) {
	// Two sentences that only respell the signature, which is a finding unless
	// something exempts the comment.
	padded := "// This function takes a value and returns a value.\n" +
		"// The implementation is simple and easy to read.\n"
	for _, c := range []struct {
		name string
		src  string
		want bool
	}{
		{"mentioned mid-sentence", "// We keep the copyright year here.\n" + padded, true},
		{"opens the line", "// Copyright 2009 The Go Authors.\n" + padded, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Below the package clause, not above it: a comment heading the
			// file is exempt whatever it says, and this asks what the notice
			// marker does.
			src := "package p\n\n" + c.src + "func double(value int) int { return value * 2 }\n"
			found := scan([]byte(src), golang, []comment.Span{{Start: 0, End: uint(len(src))}})
			if got := len(found) > 0; got != c.want {
				t.Errorf("nudged = %v, want %v (%v)", got, c.want, found)
			}
		})
	}
}

// A licence notice is out of both passes, not just the structural one. The zero
// verdict from inspect is what hands a comment to the model, so exempting it
// there would have exempted nothing.
func TestNoticeSkipsTheModel(t *testing.T) {
	skipWithoutRuntime(t)
	src := "// Copyright 2009 The Go Authors.\n" +
		"// This file is kept for backwards compatibility.\n" +
		"package p\n"
	for _, f := range scan([]byte(src), golang, []comment.Span{{Start: 0, End: uint(len(src))}}) {
		t.Errorf("nudged inside a licence notice: line %d %s %s", f.Line, f.Class, f.Reason)
	}
}

// A licence header inside a function body is a commented-out block that happens
// to open with one. A run reads as one comment, so a marker on any of its lines
// would otherwise pardon every line stacked under it.
//
// The pardon this closes is the one over prose. A commented-out block opening
// with a marker is pardoned by something else — the marker line does not parse
// as source, so the run does not either, and the rule declines before reaching
// any of this.
// The case is written in Python because it is only observable there. A licence
// line does not parse as Go, so a run opening with one is already turned away
// at the parse and the exemption never decides anything; `# copyright = 2024`
// is a clean Python assignment, so the run reaches the rule and the exemption
// is what would have spared it.
func TestNoticeIsNotABodyPardon(t *testing.T) {
	src := "def f():\n" +
		"    # copyright = 2024\n" +
		"    # DEBUG = True\n" +
		"    # SECRET = \"hunter2\"\n" +
		"    return 1\n"
	found := scan([]byte(src), python, []comment.Span{{Start: 0, End: uint(len(src))}})
	for _, f := range found {
		if f.Class == "leftover" {
			return
		}
	}
	t.Errorf("a licence line inside a body pardoned the commented-out code under it: %v", found)
}

// The nudge names three homes for a claim that will not fit: package
// documentation, symbol documentation, a test. It fires only where one of them
// exists, and file documentation is already the first of them.
func TestPaddingSpares(t *testing.T) {
	// Two sentences that only respell the declaration, which is what makes each
	// of these a finding unless its position exempts it.
	padded := "This function takes a value and returns a value. The implementation " +
		"is simple and easy to read."

	t.Run("file documentation", func(t *testing.T) {
		src := "// Package p does a thing. " + padded + "\npackage p\n"
		if f := only(t, src, golang); f.Class == "hollow" {
			t.Errorf("nudged the package's own documentation: %s", f.Reason)
		}
	})
	t.Run("under a licence header", func(t *testing.T) {
		src := "// Copyright 2009 The Go Authors.\n\n// Package p does a thing. " + padded + "\npackage p\n"
		for _, f := range scan([]byte(src), golang, []comment.Span{{Start: 0, End: uint(len(src))}}) {
			if f.Class == "hollow" {
				t.Errorf("a licence header above it made the package doc reachable: %s", f.Reason)
			}
		}
	})
	t.Run("a symbol still earns it", func(t *testing.T) {
		src := "package p\n\n// Double returns the value twice over. " + padded +
			"\nfunc Double(value int) int { return value * 2 }\n"
		found := scan([]byte(src), golang, []comment.Span{{Start: 0, End: uint(len(src))}})
		if len(found) == 0 || found[0].Class != "hollow" {
			t.Errorf("a symbol's documentation has somewhere to go and should still be nudged: %v", found)
		}
	})
	t.Run("configuration has nowhere to move it", func(t *testing.T) {
		src := "# " + padded + "\nreplicas: 3\n"
		for _, f := range scan([]byte(src), yaml, []comment.Span{{Start: 0, End: uint(len(src))}}) {
			if f.Class == "hollow" {
				t.Errorf("YAML has no symbol doc and no test to move a claim into: %s", f.Reason)
			}
		}
	})
}

// Parsing prose as source runs the grammar's error recovery over every byte,
// and a comment is bounded by nothing. This is the only claim about that cost
// the repository makes, because it is the only one that has held: five figures
// for how long an unbounded run takes have been written down and four refuted,
// and [rule.parsedBytes] records why none is quoted any more. What has to be
// true is that this finishes.
func TestHugeCommentRunIsBounded(t *testing.T) {
	var b strings.Builder
	b.WriteString("package p\n\nfunc f() {\n")
	for i := 0; i < 40000; i++ {
		b.WriteString("\t// the quick brown fox jumps over the lazy dog and keeps going\n")
	}
	b.WriteString("\tprintln(1)\n}\n")
	src := []byte(b.String())

	done := make(chan int, 1)
	go func() {
		done <- len(scan(src, golang, []comment.Span{{Start: 0, End: uint(len(src))}}))
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("scanning %d bytes of comment took longer than 5s", len(src))
	}
}

// A comment sharing a line with code is a note about that code. The notation
// those notes use is the notation the commented-out-code rule reads as source.
func TestTrailingNote(t *testing.T) {
	src := "package p\n\nfunc f(x1 float64) float64 {\n" +
		"\tx2 := Sqrt(x1) // x2 = sqrt(1 - x*x)\n\treturn x2\n}\n"
	found := scan([]byte(src), golang, []comment.Span{{Start: 0, End: uint(len(src))}})
	for _, f := range found {
		if f.Class == "leftover" {
			t.Errorf("read a trailing note as commented-out code: line %d", f.Line)
		}
	}
}
