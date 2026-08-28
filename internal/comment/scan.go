package comment

import (
	"bytes"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/mikluko/slopguard/internal/lang"
)

// examined bounds how many comments one write is read as. A file rewritten
// whole can carry thousands, and the nudge names three: reading every one of
// them costs a forward pass per sentence and changes nothing.
const examined = 64

// Scan parses src and returns the comments inside added, at most [examined] of
// them, together with the function that releases them. A file that does not
// parse is still read, for the reason given where the gate would go.
//
// A Comment is a handful of pointers into the parse tree, so the tree has to
// outlive every rule that reads one. Releasing is the caller's because only the
// caller knows when it is done reading; calling it and then reading a Comment
// is a use-after-free rather than an empty answer.
func Scan(src []byte, language *lang.Language, added []Span) (comments []Comment, release func()) {
	return scan(src, language, added, examined)
}

// ScanAll parses src and returns every comment in it, unbounded, together with
// the function that releases them. It reads a file as corpus where [Scan] reads
// one write, and it goes through the same reading: a corpus counted by a
// different notion of what a comment is would describe a different object than
// the rules judge.
func ScanAll(src []byte, language *lang.Language) (comments []Comment, release func()) {
	return scan(src, language, []Span{{Start: 0, End: uint(len(src))}}, 0)
}

// Inert returns the lines of src that hold no code, trimmed, which is every line
// covered end to end by comments and string literals.
//
// The question it answers is whether a line was code before a write touched it,
// and the hook's caller answers that about text it reconstructs rather than a
// file on disk. Reading it off the parse is the only way that holds across
// sixteen grammars: four rounds of deciding it from delimiters — a marker at
// the start of a line, a `/*` opening a block, a backtick after an `=` — each
// closed one shape and reopened another, because a delimiter's meaning is a
// question about the grammar and not about the characters.
//
// A text that does not parse yields every line, which is the opposite of what
// [Scan] does with one and is said again at the branch.
func Inert(src []byte, language *lang.Language) map[string]bool {
	parser := tree_sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(tree_sitter.NewLanguage(language.Grammar())); err != nil {
		return nil
	}
	// Through [blank], because [scan] parses through it and a text one of them
	// can read and the other cannot is a text where the two disagree about the
	// same file. Reading raw here cost the tier every templated YAML in the
	// corpus — 835 files that the finding path parses and this one did not,
	// and a file this cannot parse loses the tier outright. `blank` preserves
	// every byte's width, so the spans it yields still index the original.
	tree := parser.Parse(blank(src, language), nil)
	if tree == nil {
		return nil
	}
	defer tree.Close()

	// A text that does not parse cannot be read for what held code, and the
	// caller's question is whether it may say something without hedging. Every
	// line is returned as holding none, which makes it say nothing: a write
	// whose file did not parse before it loses the certain tier rather than
	// earning it on a guess. [scan] takes the opposite view for findings, where
	// the cost of a gate is silence about real defects rather than a false
	// claim about one.
	if tree.RootNode().HasError() {
		return still(src, nil)
	}

	// Bytes covered by a comment or a string, marked so that a line can be asked
	// whether anything of it was left over.
	quiet := make([]bool, len(src))
	cursor := tree.RootNode().Walk()
	defer cursor.Close()
	mark := func(from, to uint) {
		for i := from; i < to && i < uint(len(src)); i++ {
			quiet[i] = true
		}
	}
	for {
		node := cursor.Node()
		if language.Comments[node.Kind()] || opaque(node) {
			mark(node.StartByte(), node.EndByte())
		} else {
			// What a node spans past its last child, when that reaches another
			// line, is text the grammar took in and never parsed. A YAML block
			// scalar is exactly this: `block_scalar` has one child, the `|`, and
			// its whole body is the tail. Ordinary code has no such tail — what
			// follows a construct's last child is punctuation on that same line.
			if n := node.ChildCount(); n > 0 {
				if last := node.Child(n - 1); node.EndPosition().Row > last.EndPosition().Row {
					mark(last.EndByte(), node.EndByte())
				}
			}
			if cursor.GotoFirstChild() {
				continue
			}
		}
		for !cursor.GotoNextSibling() {
			if !cursor.GotoParent() {
				return still(src, quiet)
			}
		}
	}
}

// opaque reports whether a node is content the grammar did not read as code.
//
// Two tests, and the second is the one that generalises. A kind naming a string
// covers most of it, but only most: no grammar agrees on the spelling, and
// tree-sitter-yaml calls a `|` block a `block_scalar`, which names neither a
// string nor a heredoc. Reading only the name left every YAML block scalar as
// live code — 630 payloads over 45 ordinary files in the mined corpus, workflow
// `run: |` blocks and Helm values among them — which is the same defect the
// four rounds of delimiter rules were about, in the mechanism that replaced them.
//
// So a leaf spanning more than one line counts too, whatever it is called. Code
// has structure: a construct the grammar parsed into a single token across
// several lines is content it declined to read, which is what a raw string, a
// heredoc body, a block scalar and Ruby's `__END__` section all are. A named
// kind matching neither is not covered, and the caller loses nothing by it:
// a line holding any code at all is code either way.
func opaque(node *tree_sitter.Node) bool {
	kind := node.Kind()
	if strings.Contains(kind, "string") || strings.Contains(kind, "heredoc") {
		return true
	}
	return node.ChildCount() == 0 && node.EndPosition().Row > node.StartPosition().Row
}

// still returns the trimmed lines of src whose every non-blank byte is quiet.
// A nil quiet answers that no line held code, which is what a text nobody could
// parse is worth.
func still(src []byte, quiet []bool) map[string]bool {
	out := map[string]bool{}
	at := 0
	for _, line := range bytes.Split(src, []byte("\n")) {
		text := bytes.TrimSpace(line)
		if len(text) == 0 {
			at += len(line) + 1
			continue
		}
		code := false
		for i := at; i < at+len(line); i++ {
			if i < len(quiet) && !quiet[i] && !isSpace(src[i]) {
				code = true
				break
			}
		}
		if !code {
			out[string(text)] = true
		}
		at += len(line) + 1
	}
	return out
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r'
}

// scan returns the comments of src that fall inside added, stopping after most
// of them, which is unbounded when most is zero.
func scan(src []byte, language *lang.Language, added []Span, most int) (comments []Comment, release func()) {
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
	// Deliberately not gated on root.HasError(). The contract claimed for a long
	// time that a file which does not parse yields silence, and implementing it
	// costs real findings rather than noise: an error node means tree-sitter
	// could not parse the file, not that the file is broken. Measured, it drops
	// 16 of 168 findings on the mined clones, every one of them C — jq's own
	// sources and the decNumber it vendors, where the grammar loses its footing
	// in the preprocessor and, for the vendored files, on a translation unit
	// that is `#include`d rather than compiled alone. Several were hand-judged
	// as residue. The standard library does not move, which is what makes the
	// cost invisible to the invariant that would otherwise catch it.
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
		if most > 0 && len(out) == most {
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
