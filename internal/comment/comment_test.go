package comment

import (
	"testing"

	"github.com/mikluko/slopguard/internal/lang"
)

// Every grammar spells a statement container differently, and all of them spell
// it in the name. The cases that matter are the ones a substring match gets
// wrong in either direction.
func TestContainer(t *testing.T) {
	for _, c := range []struct {
		kind string
		want bool
	}{
		{"block", true},
		{"statement_list", true},
		{"function_body", true},
		{"source_file", true},
		{"module", true},
		{"suite", true},
		// Go puts the left side of `total := 0` in one of these. Reading "list"
		// alone as a container stops the climb at the identifier, and the
		// statement is never seen.
		{"expression_list", false},
		{"identifier", false},
		{"binary_expression", false},
	} {
		if got := Container(c.kind); got != c.want {
			t.Errorf("Container(%q) = %v, want %v", c.kind, got, c.want)
		}
	}
}

// A comment is inside the write when any line of it is, and the ranges are
// half-open at both ends.
func TestWithin(t *testing.T) {
	src := []byte("package p\n\n// a comment\nfunc f() {}\n")
	comments, release := Scan(src, lang.Go, []Span{{Start: 0, End: uint(len(src))}})
	defer release()
	if len(comments) != 1 {
		t.Fatalf("want the one comment, got %d", len(comments))
	}
	c := comments[0]
	start, end := c.Nodes[0].StartByte(), c.Nodes[0].EndByte()
	for _, s := range []struct {
		name string
		span Span
		want bool
	}{
		{"covering it", Span{Start: 0, End: uint(len(src))}, true},
		{"one byte inside its start", Span{Start: start, End: start + 1}, true},
		{"one byte inside its end", Span{Start: end - 1, End: end}, true},
		{"ending where it starts", Span{Start: 0, End: start}, false},
		{"starting where it ends", Span{Start: end, End: uint(len(src))}, false},
	} {
		if got := c.Within([]Span{s.span}); got != s.want {
			t.Errorf("%s: Within = %v, want %v", s.name, got, s.want)
		}
	}
	if c.Within(nil) {
		t.Error("a comment was inside a write of nothing")
	}
}

// A run of single-line comments in one column is one piece of prose. A comment
// sharing a line with code never joins one: a constant table's column of notes
// is forty comments rather than one comment of forty sentences.
func TestGrouping(t *testing.T) {
	for _, c := range []struct {
		name  string
		src   string
		want  int
		texts []string
	}{
		{
			name: "a stacked run is one comment",
			src: `package p

// double returns v twice over.
// It panics when v overflows.
func double(v int) int { return v * 2 }
`,
			want:  1,
			texts: []string{"double returns v twice over. It panics when v overflows."},
		},
		{
			name: "a blank line between them ends the run",
			src: `package p

// one

// two
func f() {}
`,
			want: 2,
		},
		{
			name: "trailing notes stay separate",
			src: `package p

const (
	a = 1 // first
	b = 2 // second
)
`,
			want: 2,
		},
		{
			name: "a different column ends the run",
			src: `package p

func f() {
	// outer
		// indented further
	_ = 1
}
`,
			want: 2,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := []byte(c.src)
			comments, release := Scan(src, lang.Go, []Span{{Start: 0, End: uint(len(src))}})
			defer release()
			if len(comments) != c.want {
				var got []string
				for _, one := range comments {
					got = append(got, one.Text)
				}
				t.Fatalf("want %d comments, got %d: %q", c.want, len(comments), got)
			}
			for i, want := range c.texts {
				if comments[i].Text != want {
					t.Errorf("text %d = %q, want %q", i, comments[i].Text, want)
				}
			}
		})
	}
}

// What the walk down already knew: whether a comment sits inside a function
// body, what it stands above, and whether the file has said anything yet.
func TestPosition(t *testing.T) {
	src := []byte(`// Package p is a package.
package p

// double returns v twice over.
func double(v int) int {
	// close the connection
	return v * 2
}
`)
	comments, release := Scan(src, lang.Go, []Span{{Start: 0, End: uint(len(src))}})
	defer release()
	if len(comments) != 3 {
		t.Fatalf("want three comments, got %d", len(comments))
	}
	heading, doc, inside := comments[0], comments[1], comments[2]

	if !heading.Heads {
		t.Error("the package doc does not head the file")
	}
	if doc.Heads {
		t.Error("a comment below the package clause still heads the file")
	}
	if doc.Buried || !inside.Buried {
		t.Errorf("buried: doc=%v inside=%v, want false and true", doc.Buried, inside.Buried)
	}
	if doc.Annotates == nil || doc.Annotates.Kind() != "function_declaration" {
		t.Errorf("a doc comment should stand above its declaration, got %v", doc.Annotates)
	}
	if inside.Annotates == nil || inside.Annotates.Kind() != "return_statement" {
		t.Errorf("a note in a body should stand above its statement, got %v", inside.Annotates)
	}
	if heading.Line != 1 || doc.Line != 4 || inside.Line != 6 {
		t.Errorf("lines %d %d %d, want 1 4 6", heading.Line, doc.Line, inside.Line)
	}
}

// A licence header sits above a package doc rather than instead of it, so
// heading the file is not the same as being the first comment.
func TestHeadsIsNotJustTheFirst(t *testing.T) {
	src := []byte(`// Copyright 2009 The Go Authors.

// Package p is a package.
package p
`)
	comments, release := Scan(src, lang.Go, []Span{{Start: 0, End: uint(len(src))}})
	defer release()
	if len(comments) != 2 {
		t.Fatalf("want two comments, got %d", len(comments))
	}
	if !comments[0].Heads || !comments[1].Heads {
		t.Errorf("both head the file, got %v and %v", comments[0].Heads, comments[1].Heads)
	}
}

// A comment made entirely of directives never reaches a rule, so it never
// leaves the scanner.
func TestPragmasAreNotReturned(t *testing.T) {
	src := []byte(`package p

//go:generate stringer -type=Kind
//nolint:gocyclo
func f() {}
`)
	comments, release := Scan(src, lang.Go, []Span{{Start: 0, End: uint(len(src))}})
	defer release()
	if len(comments) != 0 {
		t.Errorf("want no comments, got %q", comments[0].Text)
	}
}

// A file that does not parse is not evidence of a comment.
func TestBrokenFileYieldsNothing(t *testing.T) {
	src := []byte("package p\n\nfunc f( {\n\t// a comment\n")
	comments, release := Scan(src, lang.Go, []Span{{Start: 0, End: uint(len(src))}})
	defer release()
	for _, c := range comments {
		if c.Annotates != nil && c.Annotates.HasError() {
			t.Errorf("read a comment out of a broken parse: %q", c.Text)
		}
	}
}

// Only Python's docstrings are read as documentation, and only where they open
// what they document.
func TestDocstring(t *testing.T) {
	src := []byte(`def double(v):
    """Double v."""
    x = "not a docstring"
    return v * 2
`)
	comments, release := Scan(src, lang.Python, []Span{{Start: 0, End: uint(len(src))}})
	defer release()
	if len(comments) != 1 {
		t.Fatalf("want the one docstring, got %d", len(comments))
	}
	if !comments[0].Doc {
		t.Error("the docstring is not marked as one")
	}
	if comments[0].Text != "Double v." {
		t.Errorf("text = %q", comments[0].Text)
	}
}
