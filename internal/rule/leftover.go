package rule

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/mikluko/slopguard/internal/comment"
	"github.com/mikluko/slopguard/internal/lang"
	"github.com/mikluko/slopguard/internal/prose"
)

// parsedBytes bounds what the commented-out-code rule will hand to a parser.
// The cost is linear in the bytes and the constant is the grammar's error
// recovery over prose, which is about seven microseconds a byte, so the bound
// is what keeps a pathological comment from holding the hook: 16 KB is 0.1
// seconds where 1.75 MB was 14.7.
const parsedBytes = 16 << 10

// leftover reports whether a comment is commented-out code: text that parses
// cleanly as source and is shaped like source. Languages where any word
// sequence parses, shell and Ruby among them, are exempt. A language whose
// prose parses as a plain value, YAML, needs a structure to appear and needs
// more than one line, because `# note: read this` is a mapping too.
//
// A comment sharing its line with code is exempt whatever it parses as. Code is
// disabled by commenting the line it is on, which leaves the comment alone on
// that line; a comment after a live statement is a note about it, and the
// notation those notes use is the notation this rule reads as source —
// `x2 := Sqrt(x1) // x2 = sqrt(1 - x*x)` says what the variable now holds.
//
// A file that registers its settings by commenting them is exempt outright: see
// [lang.Language.Registers].
func leftover(c comment.Comment, language *lang.Language, src []byte) bool {
	if c.Doc || c.Trailing || language.Registers || arm(c) {
		return false
	}
	// The lexical prefilter is what a language has instead of a legality check:
	// a bare parse succeeds on too much prose to be evidence by itself, so text
	// that does not even look like source is turned away before parsing. A
	// language that can say what the compiler would have refused does not need
	// it, and it costs real findings — `fmt.Println("x")` is a call with an
	// argument, which no list of leading keywords matches.
	evident, decides := evidence[language.Name]
	compiles, checked := legal[language.Name]
	if !decides && !checked && (!language.Strict || !prose.Code(c.Text)) {
		return false
	}
	parser := tree_sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(tree_sitter.NewLanguage(language.Grammar())); err != nil {
		return false
	}
	// The newline is not cosmetic: some grammars, Dockerfile's among them,
	// require a line ending and report a MISSING node without one, which reads
	// as a broken parse and stops this rule before it starts.
	// Parsing prose as source runs the grammar's error recovery over every byte
	// of it, at roughly seven microseconds each, and a comment is not bounded
	// by anything the way a file is. A 1.75 MB run of them held the hook for 23
	// seconds. Nobody comments out this much in one run and then wants three
	// lines of nudge back, so past the bound the rule declines.
	if len(c.Body) > parsedBytes {
		return false
	}
	body := prose.Dedent(c.Body) + "\n"
	if language.Wrapper != nil {
		// A run of commented-out lines is usually cut out of a larger block, so
		// it opens braces it never closes. Closing them is what makes the rest
		// of the fragment readable at all, and needing to is itself evidence:
		// prose does not leave a brace open.
		body += balance(body)
	}
	// Code is commented out of a function body far more often than out of file
	// scope, and the two parse by different rules, so the body is tried first.
	// But a whole function, a method or an import is commented out at file
	// scope and is an error inside a body — that is the canonical shape of dead
	// Go, and trying only the one scope makes the rule silent on it.
	for _, wrap := range scopes(language) {
		prefix, suffix := wrap()
		tree := parser.Parse([]byte(prefix+body+suffix), nil)
		if tree == nil {
			continue
		}
		defer tree.Close()
		root := tree.RootNode()
		if root.HasError() {
			continue
		}
		if decides {
			return evident(c, root, []byte(body), src)
		}
		inside := fragment(root, uint(len(prefix)), uint(len(prefix)+len(body)))
		if len(inside) == 0 {
			continue
		}
		// The wrapped text, not the fragment: every offset the nodes carry is
		// into what was parsed, so reading a node's text out of the fragment
		// alone returns whatever sits len(prefix) bytes further along.
		return !checked || compiles(inside, []byte(prefix+body+suffix))
	}
	return false
}

// arm reports whether a comment sits on the boundary of a switch arm, where it
// names the construct that arm handles rather than disabling anything:
//
//	// no: let NODE = init;
//	// yes: let id = NODE;
//	case 'VariableDeclarator':
//
//	case _Clear:
//		// clear(m)
//
// Both forms occur, above the label and as the arm's first line, and both write
// the construct in the host language because that is what makes them legible.
// Hand-judged, this one position was 21 of 189 findings across 24 repositories
// and 12 of 142 on the Go standard library, and all but a handful were the
// naming shape rather than residue: a case arm is where a reader most needs an
// example of what is being matched.
//
// **A comment at the tail of an arm is exempt too, and should not be.** Where a
// grammar files a trailing comment beside the arms rather than inside one — Go
// and TypeScript both do — it is the same tree shape as a label comment: a case
// on either side of it. Two repairs were built and measured and both reverted.
// Testing the previous sibling recovers two of the true positives below and
// brings twenty Vue false positives with it; testing the parent moves nothing.
//
// So this silences a `//dump(...)` in `staticinit`, a commented-out
// `case goimporterMagic:` arm, a disabled `if` block in `reflectlite`, and jq's
// `/*create_pt_key();*/`. Four of the thirty-three findings it removes are
// residue, which makes it about 88% right, and the alternative measured worse.
func arm(c comment.Comment) bool {
	if len(c.Nodes) == 0 {
		return false
	}
	if c.Annotates != nil && labelled(c.Annotates.Kind()) {
		return true
	}
	parent := c.Nodes[0].Parent()
	if parent == nil || !labelled(parent.Kind()) {
		return false
	}
	return opens(c, parent)
}

// opens reports whether nothing but a case label precedes a comment in its arm.
//
// Most grammars here spell the matched expression as the arm's `value`, so a
// comment whose only earlier sibling is that value opens the arm. Three do not —
// Go's `type_case` uses `type` and its `communication_case` uses
// `communication`, Python's `case_clause` uses `alternative` — so in those a
// comment on the arm's first line is not exempt. That is a gap rather than a
// design: it errs toward reporting, which is the safe direction here.
func opens(c comment.Comment, parent *tree_sitter.Node) bool {
	previous := c.Nodes[0].PrevNamedSibling()
	if previous == nil {
		return true
	}
	value := parent.ChildByFieldName("value")
	return value != nil && previous.StartByte() == value.StartByte()
}

// labelled reports whether a node kind is a switch arm.
//
// Named exhaustively rather than matched as a substring. `strings.Contains` on
// "case" or "default" both over- and under-matches: it misses Rust's `match_arm`
// and Java's `switch_label`, which spell neither, and it catches Python's
// `default_parameter`, PHP's `enum_case` and C++'s `lambda_default_capture`,
// which are not arms at all — a commented-out line above `timeout=5` was exempt
// while the same line above `timeout` fired.
var labels = set(
	// Go
	"expression_case", "default_case", "type_case", "communication_case",
	// JavaScript, TypeScript, TSX
	"switch_case", "switch_default",
	// C, C++, Java
	"case_statement", "switch_label",
	// PHP, which spells the two halves separately
	"default_statement",
	// Rust
	"match_arm",
	// Python
	"case_clause",
	// Ruby
	"when",
)

func labelled(kind string) bool { return labels[kind] }

// scopes returns the positions a fragment is tried in, in the order to try
// them. A language with no wrapper is parsed as a file and nothing else, which
// is what every language did before any of them was measured.
func scopes(language *lang.Language) []func() (string, string) {
	bare := func() (string, string) { return "", "" }
	if language.Wrapper == nil {
		return []func() (string, string){bare}
	}
	return []func() (string, string){language.Wrapper, bare}
}

// balance returns the closing delimiters a fragment leaves open, innermost
// first. Text inside quotes is skipped, since a brace in a string closes
// nothing.
func balance(body string) string {
	var open []byte
	var quote byte
	for i := 0; i < len(body); i++ {
		ch := body[i]
		switch {
		case quote != 0:
			switch ch {
			case '\\':
				i++
			case quote:
				quote = 0
			}
		// Not the apostrophe: a rune literal and the possessive are the same
		// byte, and this reads text that may be either. Treating "don't" as an
		// opening quote hides every delimiter after it, so the fragment stays
		// unbalanced and a real finding is lost.
		case ch == '"' || ch == '`':
			quote = ch
		case ch == '{' || ch == '(' || ch == '[':
			open = append(open, ch)
		case ch == '}' || ch == ')' || ch == ']':
			if n := len(open); n > 0 && closes(open[n-1]) == ch {
				open = open[:n-1]
			}
		}
	}
	var out []byte
	for i := len(open) - 1; i >= 0; i-- {
		out = append(out, closes(open[i]))
	}
	return string(out)
}

// closes returns the delimiter that closes an opening one.
func closes(open byte) byte {
	switch open {
	case '{':
		return '}'
	case '(':
		return ')'
	}
	return ']'
}

// fragment returns the statements of the wrapped text that came from the
// comment, which are the children of whatever node the wrapper put them in.
// Without a wrapper that node is the root and this is every top-level node.
func fragment(root *tree_sitter.Node, start, end uint) []*tree_sitter.Node {
	// Descend to the deepest node still holding the whole fragment: that is the
	// block the wrapper opened, since no statement of the fragment spans all of
	// it. Without a wrapper the root holds it and the descent does not move.
	holder := root
	for {
		inner := (*tree_sitter.Node)(nil)
		for i := uint(0); i < holder.ChildCount(); i++ {
			child := holder.Child(i)
			// A child filling the fragment exactly is the fragment, not
			// something holding it. Descending into it hands back its parts,
			// which match no rule and pass by default — and that is the common
			// case, since a closer [balance] appended lands on the statement's
			// own end byte.
			if child.StartByte() == start && child.EndByte() == end {
				continue
			}
			if child.StartByte() <= start && child.EndByte() >= end {
				inner = child
				break
			}
		}
		if inner == nil {
			break
		}
		holder = inner
	}
	// A grammar may put the run in a node of its own rather than directly under
	// the block: tree-sitter-go ends a statement_list at the last terminated
	// statement, so it falls short of the fragment and the descent stops above
	// it. Unwrapped, every statement of the run reads as one node of a kind no
	// rule has a case for, and the whole legality check passes by default.
	for {
		out := within(holder, start, end)
		if len(out) != 1 || !comment.Container(out[0].Kind()) {
			return out
		}
		holder = out[0]
	}
}

// within returns the named children of a node that lie inside a byte range.
func within(holder *tree_sitter.Node, start, end uint) []*tree_sitter.Node {
	var out []*tree_sitter.Node
	for i := uint(0); i < holder.NamedChildCount(); i++ {
		child := holder.NamedChild(i)
		if child.StartByte() >= start && child.EndByte() <= end {
			out = append(out, child)
		}
	}
	return out
}

// holds reports whether the tree under node contains one of the kinds.
func holds(node *tree_sitter.Node, kinds map[string]bool) bool {
	if kinds[node.Kind()] {
		return true
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		if holds(node.Child(i), kinds) {
			return true
		}
	}
	return false
}
