// Package comment turns a source file into the comments it holds, each one
// carrying what the walk down to it learned: where it sits, what it sits above,
// and whether it is prose at all.
//
// It judges nothing. Which comments a tool objects to, and why, is the rule
// layer's business; this package's answer is the same whatever rules read it.
package comment

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/mikluko/slopguard/internal/prose"
)

// A Span is a byte range of a file that the tool call just wrote.
type Span struct{ Start, End uint }

// A Comment is a run of comment nodes that reads as one piece of prose: a block
// comment, or the consecutive single-line comments above a declaration.
type Comment struct {
	Nodes []*tree_sitter.Node
	// Text is the prose on one line, for the rules that read it as a sentence.
	Text string
	// Body keeps the line breaks and the indentation, for the rule that reads
	// it as source.
	Body string
	Line uint
	// Annotates is the code this comment sits above, where it sits above any:
	// what a comment restates, if it restates anything.
	Annotates *tree_sitter.Node
	// Doc marks a docstring, which documents the symbol it opens rather than
	// sitting inside it: the rules for prose in a body do not reach it.
	Doc bool
	// Buried marks a comment inside a function body, where prose is harder to
	// justify and the thresholds are lower.
	Buried bool
	// Trailing marks a comment with code before it on its own line. Nobody
	// disables a statement by appending it to a live one, so the
	// commented-out-code rule does not reach these.
	Trailing bool
	// Heads marks a comment with nothing but whitespace and other comments
	// before it in the file, which is where a file documents itself. A licence
	// header sits above a package doc rather than instead of it, so this is not
	// the same as being the first comment.
	Heads bool
	// Root is the tree the comment came from, for the rules that need to look
	// at what surrounds it.
	Root *tree_sitter.Node

	// raw is each node's text as it was written, markers and all. The rules
	// read Text or Body; this is what those two are rendered from.
	raw []string
}

// Within reports whether any line of the comment falls in the text just written.
func (c Comment) Within(added []Span) bool {
	for _, node := range c.Nodes {
		for _, s := range added {
			if node.StartByte() < s.End && s.Start < node.EndByte() {
				return true
			}
		}
	}
	return false
}

// pragma reports whether every line of the comment is machine-readable.
func (c Comment) pragma() bool {
	for _, line := range c.raw {
		for _, one := range strings.Split(line, "\n") {
			if strings.TrimSpace(prose.Strip(one)) != "" && !prose.Directive(one) {
				return false
			}
		}
	}
	return true
}

// annotated returns the code a comment sits above, found by position rather
// than by sibling: a comment inside a Go block is a sibling of the whole
// statement list, not of the statement under it, and every grammar arranges
// this differently.
func annotated(root, node *tree_sitter.Node, src []byte) *tree_sitter.Node {
	at := int(node.EndByte())
	for at < len(src) && (src[at] == ' ' || src[at] == '\t' || src[at] == '\n' || src[at] == '\r') {
		at++
	}
	if at >= len(src) {
		return nil
	}
	found := root.NamedDescendantForByteRange(uint(at), uint(at))
	if found == nil || strings.Contains(found.Kind(), "comment") {
		return nil
	}
	// The smallest node at that byte may be a bare identifier; the statement is
	// what encloses it while still beginning there. Climbing stops at a
	// container, which holds the statements rather than being one.
	for {
		parent := found.Parent()
		if parent == nil || parent.StartByte() != found.StartByte() || Container(parent.Kind()) {
			return found
		}
		found = parent
	}
}

// Container reports whether a node kind holds a run of statements rather than
// being one. Every grammar spells it differently and all of them spell it in
// the name.
//
// "list" alone is too broad: Go puts the left side of `total := 0` in an
// expression_list, so climbing would stop at the identifier and the statement
// would never be seen.
func Container(kind string) bool {
	for _, mark := range []string{"statement_list", "statements", "block", "body", "suite", "source_file", "program", "module", "document"} {
		if strings.Contains(kind, mark) {
			return true
		}
	}
	return false
}

// docstring reports whether a node is a documentation string: a bare string in
// statement position, opening a module, a class or a function body.
//
// It is where essentially all of Python's standing documentation lives, so a
// tool that reads only `#` comments is blind to the place its own rules are
// most often broken — a fourteen-line Args/Returns/Raises block passes while
// the same prose in Go is nudged.
func docstring(node *tree_sitter.Node) bool {
	if node.Kind() != "expression_statement" || node.NamedChildCount() != 1 {
		return false
	}
	if node.NamedChild(0).Kind() != "string" {
		return false
	}
	parent := node.Parent()
	if parent == nil {
		return false
	}
	switch parent.Kind() {
	case "module", "block":
	default:
		return false
	}
	return parent.NamedChild(0) != nil && parent.NamedChild(0).Equals(*node)
}

// after reports whether a node has code before it on its own line, which makes
// a comment a note about that code rather than a line in its own right.
func after(node *tree_sitter.Node, src []byte) bool {
	for at := int(node.StartByte()) - 1; at >= 0 && src[at] != '\n'; at-- {
		if src[at] != ' ' && src[at] != '\t' && src[at] != '\r' {
			return true
		}
	}
	return false
}

// adjacent reports whether two single-line comments are stacked in one column
// on consecutive lines.
func adjacent(prev, next *tree_sitter.Node) bool {
	if prev.StartPosition().Row != prev.EndPosition().Row {
		return false
	}
	if next.StartPosition().Row != next.EndPosition().Row {
		return false
	}
	return prev.EndPosition().Row+1 == next.StartPosition().Row &&
		prev.StartPosition().Column == next.StartPosition().Column
}

// clean reports whether the bytes between two offsets are all whitespace.
func clean(src []byte, from, to uint) bool {
	if to > uint(len(src)) {
		return false
	}
	for _, b := range src[from:to] {
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			return false
		}
	}
	return true
}

// joined returns a run's prose on one line and its body with the line breaks
// kept, from the raw text of each of its lines.
func joined(raw []string) (string, string) {
	var prosed, bodied strings.Builder
	for i, line := range raw {
		if piece := plain(line); piece != "" {
			if prosed.Len() > 0 {
				prosed.WriteByte(' ')
			}
			prosed.WriteString(piece)
		}
		if i > 0 {
			bodied.WriteByte('\n')
		}
		// body, not indented: one entry of a run is a whole block comment where
		// the language has them, and its own line breaks have to survive.
		bodied.WriteString(body(line))
	}
	return prosed.String(), bodied.String()
}

// plain renders a raw comment as the text it reads as, markers removed.
func plain(raw string) string {
	var out string
	for _, line := range strings.Split(raw, "\n") {
		out = join(out, prose.Strip(line))
	}
	return out
}

// body renders a raw comment with its markers removed and its line breaks and
// indentation kept.
func body(raw string) string {
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		lines[i] = prose.Indented(line)
	}
	return strings.Join(lines, "\n")
}

func join(left, right string) string {
	switch {
	case left == "":
		return right
	case right == "":
		return left
	}
	return left + " " + right
}
