package rule

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/mikluko/slopguard/internal/comment"
	"github.com/mikluko/slopguard/internal/lang"
	"github.com/mikluko/slopguard/internal/prose"
)

// declared reads a signature, and what counts as the signature differs by what
// is being declared. A struct's fields are its contract and belong in it; a
// function's body is not and does not.
func TestDeclared(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want []string
		deny []string
	}{
		{
			name: "function signature",
			src:  "package p\n\nfunc Verdict(command string) string { totalCount := 0; return \"\" }\n",
			want: []string{"verdict", "command", "string"},
			deny: []string{"total", "count"},
		},
		{
			name: "struct fields are the contract",
			src:  "package p\n\ntype Response struct {\n\tStatus string\n\tBody io.ReadCloser\n}\n",
			// Stemmed the same way content() stems, so "status" is "statu" on
			// both sides and the two still meet.
			want: []string{"response", "statu", "body"},
		},
		{
			name: "method receiver and results",
			src:  "package p\n\nfunc (t *Tree) Walk(fn Visitor) (int, error) { hidden := 1; return 0, nil }\n",
			want: []string{"tree", "walk", "visitor", "int", "error"},
			deny: []string{"hidden"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			node, src := declOf(t, c.src)
			got := declared(node, src)
			for _, word := range c.want {
				if !got[word] {
					t.Errorf("signature is missing %q; has %s", word, keysOf(got))
				}
			}
			for _, word := range c.deny {
				if got[word] {
					t.Errorf("signature wrongly carries %q from the body; has %s", word, keysOf(got))
				}
			}
		})
	}
}

// declOf returns the last top-level declaration of a Go source string. The tree
// is deliberately leaked for the length of the test: closing it invalidates
// every node taken out of it.
func declOf(t *testing.T, source string) (*tree_sitter.Node, []byte) {
	t.Helper()
	src := []byte(source)
	parser := tree_sitter.NewParser()
	if err := parser.SetLanguage(tree_sitter.NewLanguage(golang.Grammar())); err != nil {
		t.Fatal(err)
	}
	tree := parser.Parse(src, nil)
	if tree == nil {
		t.Fatal("source did not parse")
	}
	root := tree.RootNode()
	var last *tree_sitter.Node
	for i := uint(0); i < root.NamedChildCount(); i++ {
		if child := root.NamedChild(i); child.Kind() != "package_clause" {
			last = child
		}
	}
	if last == nil {
		t.Fatal("no declaration in source")
	}
	return last, src
}

// A finding here has to be fixable by editing one word in one list, which means
// it has to be attributable to one word. This prints, per sentence, which
// bucket every content word fell into, and it is how the lists in padding.go
// were tuned and how a report of a false positive gets diagnosed.
func TestAttribution(t *testing.T) {
	source := os.Getenv("SLOPGUARD_ATTRIBUTE")
	if source == "" {
		t.Skip("set SLOPGUARD_ATTRIBUTE to a file to attribute every doc comment in it")
	}
	src, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	language := lang.Lookup(source)
	if language == nil {
		t.Fatalf("no grammar reads %s", source)
	}

	only := os.Getenv("SLOPGUARD_ATTRIBUTE_LINE")
	comments, release := comment.Scan(src, language, []comment.Span{{Start: 0, End: uint(len(src))}})
	defer release()
	for _, c := range comments {
		declaration := documents(c, src)
		if declaration == nil || c.Trailing || c.Heads {
			continue
		}
		pieces := prose.Split(c.Text)
		if len(pieces) < 2 {
			continue
		}
		if only != "" && only != strconv.FormatUint(uint64(c.Line), 10) {
			continue
		}
		spelled := declared(declaration, src)
		inside := map[string]bool{}
		if body := declaration.ChildByFieldName("body"); body != nil {
			inside = identifiers(body, src)
		}
		t.Logf("line %d  %s", c.Line, firstLine(declaration.Utf8Text(src)))
		for i, sentence := range pieces {
			var novel, covered []string
			for _, word := range content(sentence) {
				switch {
				case evaluative[word]:
					covered = append(covered, word+"(rates)")
				case spelled[word]:
					covered = append(covered, word+"(sig)")
				case scaffold[word]:
					covered = append(covered, word+"(scaffold)")
				case inside[word]:
					covered = append(covered, word+"(body)")
				default:
					novel = append(novel, word)
				}
			}
			mark := "earns"
			switch {
			case len(content(sentence)) < 3:
				mark = "short"
			case structured(sentence):
				mark = "tagged"
			case len(novel) == 0 && eliminates(sentence, spelled):
				mark = "vetoed"
			case len(novel) == 0:
				mark = "HOLLOW"
			}
			t.Logf("  s%d %-7s novel[%s] covered[%s]", i+1, mark,
				strings.Join(novel, " "), strings.Join(covered, " "))
			t.Logf("       %s", sentence)
		}
	}
}

func keysOf(set map[string]bool) string {
	var out []string
	for word := range set {
		out = append(out, word)
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}
