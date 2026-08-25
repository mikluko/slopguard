package main

import (
	"strings"
	"unicode"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// Whether a comment restates the code is a fact about the pair, not about the
// comment. "increment the counter" is a restatement above `n++` and a fair
// summary above a forty-line loop, so the text alone cannot tell them apart —
// which is why the class fired on correct comments before it was given the code
// to compare against.
//
// Two things come of that. A comment whose words are already in the identifiers
// below it is a restatement on the evidence, with no model involved. And the
// model's own reading of a comment as restatement is only accepted where the
// code below is a single line, because a comment over a block is summarising it
// rather than repeating it.

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
		if parent == nil || parent.StartByte() != found.StartByte() || container(parent.Kind()) {
			return found
		}
		found = parent
	}
}

// container reports whether a node kind holds a run of statements rather than
// being one. Every grammar spells it differently and all of them spell it in
// the name.
func container(kind string) bool {
	for _, mark := range []string{"list", "block", "body", "suite", "statements", "source_file", "program", "module", "document"} {
		if strings.Contains(kind, mark) {
			return true
		}
	}
	return false
}

// echoes reports whether a comment's own words are already spelled by the
// identifiers of the code it sits above.
//
// The comparison is over content words: what remains of the comment once the
// words every sentence carries are dropped, against the identifiers below split
// on camel case and underscores. Half of a comment's content words appearing in
// one line of code is a comment that reads the line back.
// A doc comment is exempt whatever it repeats: naming the symbol it documents
// is what a doc does, and a one-line function makes its whole body available to
// be echoed. Only prose inside a body is read this way.
func echoes(c comment, lang *language, src []byte) bool {
	if c.annotates == nil || !oneLine(c.annotates) || !buried(c.nodes[0], lang) {
		return false
	}
	words := content(c.text)
	if len(words) < 2 {
		return false
	}
	spelled := identifiers(c.annotates, src)
	hits := 0
	for _, word := range words {
		if spelled[word] {
			hits++
		}
	}
	return hits*2 >= len(words)
}

// oneLine reports whether a node begins and ends on the same line.
func oneLine(node *tree_sitter.Node) bool {
	return node.StartPosition().Row == node.EndPosition().Row
}

// content returns a comment's words with the ones every sentence carries
// dropped, singularised crudely so that "items" matches "item".
func content(text string) []string {
	var out []string
	for _, word := range strings.Fields(normalize(text)) {
		if empty[word] || len(word) < 3 {
			continue
		}
		out = append(out, strings.TrimSuffix(word, "s"))
	}
	return out
}

// empty holds the words that carry no subject: they appear in every comment and
// in most identifiers, so counting them would make everything an echo.
var empty = map[string]bool{
	"the": true, "a": true, "an": true, "of": true, "to": true, "in": true,
	"on": true, "for": true, "and": true, "or": true, "is": true, "are": true,
	"be": true, "it": true, "its": true, "this": true, "that": true, "with": true,
	"from": true, "by": true, "as": true, "at": true, "we": true, "our": true,
	"if": true, "then": true, "else": true, "not": true, "no": true, "any": true,
	"all": true, "each": true, "every": true, "into": true, "out": true,
	"here": true, "there": true, "when": true, "where": true, "which": true,
	"what": true, "will": true, "can": true, "may": true, "must": true,
	"do": true, "does": true, "so": true, "up": true, "down": true, "new": true,
}

// identifiers returns every word spelled by the identifiers under a node,
// split on camel case and underscores.
func identifiers(node *tree_sitter.Node, src []byte) map[string]bool {
	out := map[string]bool{}
	var walk func(*tree_sitter.Node)
	walk = func(n *tree_sitter.Node) {
		if n.ChildCount() == 0 {
			for _, word := range pieces(n.Utf8Text(src)) {
				out[strings.TrimSuffix(word, "s")] = true
			}
			return
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(node)
	return out
}

// pieces cuts an identifier into the words it was assembled from.
func pieces(text string) []string {
	var out []string
	var word strings.Builder
	flush := func() {
		if word.Len() > 0 {
			out = append(out, strings.ToLower(word.String()))
			word.Reset()
		}
	}
	for i, r := range text {
		switch {
		case unicode.IsUpper(r):
			if i > 0 {
				flush()
			}
			word.WriteRune(r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			word.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return out
}
