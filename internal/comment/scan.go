package comment

import (
	"bytes"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/mikluko/slopguard/internal/lang"
)

// examined bounds how many comments one write is read as. A file rewritten
// whole can carry thousands, and the nudge names three: reading every one of
// them costs a forward pass per sentence and changes nothing.
const examined = 64

// Scan parses src and returns the comments inside added, at most [examined] of
// them, together with the function that releases them. A file that does not
// parse yields nothing: a broken tree is not evidence of a comment.
//
// A Comment is a handful of pointers into the parse tree, so the tree has to
// outlive every rule that reads one. Releasing is the caller's because only the
// caller knows when it is done reading; calling it and then reading a Comment
// is a use-after-free rather than an empty answer.
func Scan(src []byte, language *lang.Language, added []Span) (comments []Comment, release func()) {
	parser := tree_sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(tree_sitter.NewLanguage(language.Grammar())); err != nil {
		return nil, func() {}
	}
	tree := parser.Parse(blank(src, language), nil)
	if tree == nil {
		return nil, func() {}
	}
	var out []Comment
	root := tree.RootNode()
	for _, c := range group(root, collect(root, language, false), src) {
		if c.Within(added) && !c.pragma() {
			// Deferred to here rather than done in [group]: locating the code
			// under a comment walks the tree from the root, and a file of
			// nothing but comments pays that for every one of them before
			// [examined] gets to cut. The write being judged is a few lines, so
			// almost none of them reach this.
			c.Annotates = annotated(root, c.Nodes[len(c.Nodes)-1], src)
			out = append(out, c)
		}
		if len(out) == examined {
			break
		}
	}
	return out, tree.Close
}

// found is a comment node together with what the walk down to it already knew:
// whether anything above it was a function.
//
// The answer travels down rather than being asked for on the way back up,
// because tree-sitter answers Parent() by re-descending from the root, which
// makes a climb quadratic in the depth of the file.
type found struct {
	node   *tree_sitter.Node
	buried bool
}

// collect returns the comment nodes of a tree in document order, each carrying
// whether it sits inside a function body.
//
// The walk is a cursor rather than an index. tree-sitter answers Child(i) by
// counting from the first child, so a loop over the children of one node is
// quadratic in how many it has, and a file whose top level is forty thousand
// comment lines is exactly that shape.
func collect(root *tree_sitter.Node, language *lang.Language, buried bool) []found {
	cursor := root.Walk()
	defer cursor.Close()
	var out []found
	// depths[i] is whether the node at depth i+1 was inside a function body,
	// carried down so that the answer is never asked for on the way back up.
	depths := []bool{buried}
	for {
		node := cursor.Node()
		inside := depths[len(depths)-1]
		if language.Comments[node.Kind()] || (language.Docstrings && docstring(node)) {
			out = append(out, found{node: node, buried: inside})
		} else if cursor.GotoFirstChild() {
			depths = append(depths, inside || language.Functions[node.Kind()])
			continue
		}
		for !cursor.GotoNextSibling() {
			if !cursor.GotoParent() {
				return out
			}
			depths = depths[:len(depths)-1]
		}
	}
}

// group merges the single-line comments of one run into a single comment, so
// that a doc comment written as four `//` lines is read as one piece of prose.
//
// A comment sharing a line with code is never merged. Each one is a note about
// its own line, and a constant table's column of them is not one comment: forty
// trailing notes in the same column read as a single comment of forty
// sentences, which a rule about length then has something to say about.
func group(root *tree_sitter.Node, nodes []found, src []byte) []Comment {
	var out []Comment
	// Everything before this byte is whitespace and comments, so a comment
	// starting at it is still part of the file's own heading. It stops
	// advancing at the first line of code, and every comment after that one
	// documents something rather than the file.
	heading := uint(0)
	for _, f := range nodes {
		raw := f.node.Utf8Text(src)
		opens := clean(src, heading, f.node.StartByte())
		if opens {
			heading = f.node.EndByte()
		}
		trailing := after(f.node, src)
		if n := len(out); n > 0 && !trailing && !out[n-1].Trailing &&
			adjacent(out[n-1].Nodes[len(out[n-1].Nodes)-1], f.node) {
			out[n-1].Nodes = append(out[n-1].Nodes, f.node)
			out[n-1].raw = append(out[n-1].raw, raw)
			continue
		}
		out = append(out, Comment{
			Nodes:    []*tree_sitter.Node{f.node},
			raw:      []string{raw},
			Line:     f.node.StartPosition().Row + 1,
			Doc:      docstring(f.node),
			Buried:   f.buried,
			Trailing: trailing,
			Heads:    opens,
			Root:     root,
		})
	}
	// The prose is built once a run is closed rather than extended line by
	// line: appending to a string per line copies the run so far each time,
	// which is quadratic in a run's length and is what a file of forty thousand
	// stacked comment lines costs.
	for i := range out {
		out[i].Text, out[i].Body = joined(out[i].raw)
	}
	return out
}

// blank replaces template actions with spaces of the same width, so that a
// Helm chart's manifests parse as the YAML they become.
//
// A `{{- if }}` at column zero otherwise makes the grammar drop every comment
// in the file, which is worse than no coverage: the same comment is read in one
// manifest and invisible in the next. Widths are preserved so that every byte
// offset the parse reports still addresses the file the agent wrote, and the
// text handed back is the file's own.
//
// A `{{` somebody wrote in prose has no closer of its own, so pairing it with
// the next one anywhere in the file erases every comment between them. An
// action does not span a blank line, which bounds the search for the closer to
// the paragraph the opener is in. Bounding it is also what keeps this linear:
// an unpaired `{{` otherwise scans to the end of the file, and there can be one
// of those per paragraph.
func blank(src []byte, language *lang.Language) []byte {
	if !language.Templated || !bytes.Contains(src, []byte("{{")) {
		return src
	}
	out := append([]byte(nil), src...)
	// The end of the paragraph holding the last opener looked at. It only ever
	// moves forward, so the scans that find it cover the file once between them.
	para := 0
	for i := 0; i+1 < len(out); {
		if out[i] != '{' || out[i+1] != '{' {
			i++
			continue
		}
		if para <= i {
			para = len(out)
			if at := bytes.Index(out[i:], []byte("\n\n")); at >= 0 {
				para = i + at
			}
		}
		end := bytes.Index(out[i:para], []byte("}}"))
		if end < 0 {
			// Nothing in this paragraph closes it, so it was text. Neither will
			// anything else before the blank line, so the whole paragraph goes.
			i = para + 2
			continue
		}
		for k := i; k < i+end+2; k++ {
			if out[k] != '\n' {
				out[k] = ' '
			}
		}
		i += end + 2
	}
	return out
}
