package rule

import (
	"strings"
	"testing"

	"github.com/mikluko/slopguard/internal/comment"
	"github.com/mikluko/slopguard/internal/lang"
)

// A run of commented-out lines is usually cut out of a larger block, so it
// opens delimiters it never closes. Closing them is what makes the fragment
// parse at all, and needing to is itself evidence: prose does not leave a brace
// open.
func TestBalance(t *testing.T) {
	for _, c := range []struct{ body, want string }{
		{"if ok {\n\tf()\n", "}"},
		{"f(a, b\n", ")"},
		{"if ok {\n\tg(x\n", ")}"},
		{"if ok {\n\tf()\n}\n", ""},
		{"a sentence with no delimiters\n", ""},
		// A brace inside a string closes nothing and opens nothing.
		{"f(\"{\")\n", ""},
		{"s := `}}`\n", ""},
		// Not the apostrophe: a rune literal and the possessive are the same
		// byte. Reading "don't" as an opening quote would hide every delimiter
		// after it, leaving the fragment unbalanced and the finding lost.
		{"if ok { // don't\n", "}"},
		{"", ""},
	} {
		if got := balance(c.body); got != c.want {
			t.Errorf("balance(%q) = %q, want %q", c.body, got, c.want)
		}
	}
}

func TestCloses(t *testing.T) {
	for open, want := range map[byte]byte{'{': '}', '(': ')', '[': ']'} {
		if got := closes(open); got != want {
			t.Errorf("closes(%q) = %q, want %q", open, got, want)
		}
	}
}

// A language with no wrapper is parsed as a file and nothing else. One with a
// wrapper is tried in a body first, because code is commented out of a function
// far more often than out of file scope — but file scope has to be tried too,
// since a whole function or an import is an error inside a body and that is the
// canonical shape of dead Go.
func TestScopes(t *testing.T) {
	if got := len(scopes(lang.YAML)); got != 1 {
		t.Errorf("a language with no wrapper has %d scopes, want 1", got)
	}
	tried := scopes(lang.Go)
	if len(tried) != 2 {
		t.Fatalf("a wrapped language has %d scopes, want 2", len(tried))
	}
	if prefix, _ := tried[0](); prefix == "" {
		t.Error("the body scope is not tried first")
	}
	if prefix, suffix := tried[1](); prefix != "" || suffix != "" {
		t.Errorf("the second scope wraps in %q and %q, want file scope", prefix, suffix)
	}
}

// The bound is what keeps a pathological comment from holding the hook: parsing
// prose as source runs the grammar's error recovery over every byte, and a
// comment is not bounded by anything the way a file is.
func TestOversizedCommentDeclines(t *testing.T) {
	var b strings.Builder
	b.WriteString("package p\n\n")
	for b.Len() < parsedBytes*2 {
		b.WriteString("// timeout = 5 * time.Second\n")
	}
	b.WriteString("func f() {}\n")
	src := []byte(b.String())

	for _, f := range scan(src, golang, []comment.Span{{Start: 0, End: uint(len(src))}}) {
		if f.Class == "leftover" {
			t.Fatalf("parsed a comment of %d bytes, over the %d bound", len(src), parsedBytes)
		}
	}
}
