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
//
// An equation with a bare identifier on the left is on neither side of it: the
// compiler accepts it, so legality decides nothing and the namespace decides
// instead. Those rows carry `spells`, since what the file names is the whole
// question.
var goLeftovers = []struct {
	name string
	body string
	// spells is what the file declares besides the comment, for a row whose
	// verdict turns on whether the comment's names are the code's own.
	spells string
	want   bool
	// gap names what has to change before the row passes, for a case the tool
	// does not answer today. The table keeps it anyway: deleting it would
	// remove the only record that the behaviour was ever wanted.
	gap string
}{
	{name: "bessel identity", body: "// j0(x) = 1/sqrt(pi) * (P(0,x)*cc - Q(0,x)*ss) / sqrt(x)"},
	{name: "complex invariant", body: "// complex(e, f) = n/m"},
	{name: "bare comparison", body: "// f == g"},
	{name: "rewrite rule match", body: "// match: (Add16 (Const16 [c]) (Const16 [d]))"},
	{name: "rewrite rule cond", body: "// cond: c+d == c+d"},
	{name: "rewrite rule result", body: "// result: (MOVVconst [int64(val)])"},
	{name: "constant provenance", body: "// (664-0.03306235651)*2**20"},
	{name: "labelled note", body: "// Have: s += expr"},
	{name: "table semantics", body: "// arg0 == arg1"},

	// The namespace rows. A name the file never spells is the cited
	// reference's and not the code's, and one of them is enough: `password` is
	// the pbkdf2 file's own, `PRF` and `U_n` are RFC 8018's.
	{
		name: "rsa encoding block",
		body: "// EM = 0x00 || 0x02 || PS || 0x00 || M",
	},
	{
		name:   "pbkdf2 recurrence",
		body:   "// U_n = PRF(password, U_(n-1))",
		spells: "var password []byte",
	},
	{
		name:   "pseudocode from a paper",
		body:   "// s = nlz(v); v <<= s",
		spells: "var s, v uint",
	},
	// The other direction, and the reason the veto is not the whole rule: an
	// assignment naming what the file names is code somebody switched off.
	{
		name:   "delay mask",
		body:   "//DELAY = LOAD|BRANCH|FCMP",
		spells: "const (\n\tDELAY = 1\n\tLOAD  = 2\n\tBRANCH = 4\n\tFCMP  = 8\n)",
		want:   true,
	},
	// A selector already names something the file has, so the veto does not
	// reach it whatever the right side resolves to.
	{
		name: "field assignment",
		body: "// m.directory = newDir",
		want: true,
	},

	// Notation whose names are all the file's own. The namespace says nothing
	// about these, because the paper and the code chose the same letters.
	{
		name:   "field arithmetic",
		body:   "// r = x^(2^127-1) * x",
		spells: "func h(r, x *int) { _, _ = r, x }",
		gap:    "r and x are the file's own parameters, so resolution succeeds and the veto does not fire",
	},
	{
		name:   "rotation",
		body:   "// y0 = x0*kcos + x1*ksin\n// y1 = -x0*ksin + x1*kcos",
		spells: "func h(x0, x1, kcos, ksin int) (y0, y1 int) { return 0, 0 }",
		gap:    "every name is declared by the signature the comment sits under, so resolution succeeds",
	},

	// An equation broken across lines leaves a paren open, so [balance] closes
	// it and the closer lands on the statement's own end byte. Descending into
	// a node that fills the fragment exactly hands back its parts, which match
	// no rule and pass by default.
	{name: "unbalanced identity", body: "// j0(x) = 1/sqrt(pi) * (P(0,x)*cos(X) - Q(0,x)*sin(X)"},
	{name: "unbalanced invariant", body: "// complex(e, f) = n/(m"},
	// A bare label reaches goLegal only if the statement_list holding the run
	// is unwrapped first.
	{name: "bare label", body: "// Cases:"},
	{name: "output label", body: "// Output:"},
	// Illegal one level down is illegal.
	{name: "nested comparison", body: "// if x {\n//\ty == z\n// }"},
	// A conversion is not a call, and a built-in that exists for its value
	// cannot stand alone.
	{name: "conversion", body: "// int64(x)"},
	{name: "builtin length", body: "// len(b)"},
	{name: "builtin new", body: "// new(T)"},
	{name: "conversion to any", body: "// any(x)"},
	{name: "unsafe size", body: "// unsafe.Sizeof(x)"},

	{name: "disabled if", body: "// if initmap != nil {", want: true},
	{
		name:   "disabled assignment",
		body:   "// cr  = buildReg(\"CR\")",
		spells: "var cr int\n\nfunc buildReg(name string) int { return len(name) }",
		want:   true,
	},
	{name: "disabled call", body: "// fmt.Println(\"x\")", want: true},
	{name: "disabled multiline", body: "// if n.Op == lexical.Range {\n//\treturn false\n// }", want: true},
	{name: "disabled declaration", body: "// var count int", want: true},
	{name: "disabled loop", body: "// for i := range xs {\n//\tuse(i)\n// }", want: true},
	// Go puts a label on any statement, so the veto is narrowed to the one a
	// note takes: a label on a loop is running code.
	{name: "labelled loop", body: "// Loop:\n//\tfor i := range xs {\n//\t\tuse(i)\n//\t}", want: true},
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
			found := only(t, inBody(c.body, c.spells), golang)
			got := found.Class == "leftover"
			if c.gap != "" {
				if got == c.want {
					t.Fatalf("this case is marked as a gap and now passes: drop the mark. %s", c.gap)
				}
				t.Skip(c.gap)
			}
			if got != c.want {
				t.Errorf("leftover = %v, want %v\n  %s\n  read as: %s %s",
					got, c.want, c.body, found.Class, found.Reason)
			}
		})
	}
}

// inBody puts a comment inside a function, which is where code gets commented
// out and where a comment above a declaration would instead be its doc. spells
// is declared at file scope, for a row whose verdict turns on what names the
// file has of its own.
func inBody(body, spells string) string {
	var indented []string
	for _, line := range strings.Split(body, "\n") {
		indented = append(indented, "\t"+line)
	}
	return "package p\n\n" + spells + "\n\nfunc f() {\n" +
		strings.Join(indented, "\n") + "\n\tprintln(1)\n}\n"
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
// and a comment is bounded by nothing. A run of them held the hook for 23
// seconds before the rule stopped reading past a bound.
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
