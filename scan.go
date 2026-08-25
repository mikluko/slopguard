package main

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// buriedBias is how much less evidence a comment inside a function body needs
// before the semantic pass names it. Prose is harder to justify there, but a
// line pointing at a constraint enforced elsewhere still justifies itself, so
// this tilts the reading rather than deciding it.
const buriedBias = 0.06

// docSentences is how long documentation runs before its length alone is worth
// a word. One sentence is the rule and a second is earned; the floor sits well
// above both, because a doc carrying a real constraint often needs the room and
// a nudge on every third sentence would be noise.
const docSentences = 5

// span is a byte range of a file that the tool call just wrote.
type span struct{ start, end uint }

// finding is one comment slopguard objects to, at the line it starts on. The
// score ranks findings against each other, so that a nudge naming three carries
// the three the tool is surest of.
type finding struct {
	line   uint
	reason string
	score  float64
}

// examined bounds how many comments one write is judged on. A file rewritten
// whole can carry thousands, and the nudge names three: reading every one of
// them costs a forward pass per sentence and changes nothing.
const examined = 64

// comment is a run of comment nodes that reads as one piece of prose: a block
// comment, or the consecutive single-line comments above a declaration.
type comment struct {
	nodes []*tree_sitter.Node
	raw   []string
	// text is the prose on one line, for the rules that read it as a sentence.
	text string
	// body keeps the line breaks and the indentation, for the rule that reads
	// it as source.
	body string
	line uint
}

// scan parses src and reports the comments inside added that the
// explanation-placement rules turn away. A file that does not parse yields
// nothing: a broken tree is not evidence of a comment.
func scan(src []byte, lang *language, added []span) []finding {
	parser := tree_sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(tree_sitter.NewLanguage(lang.grammar())); err != nil {
		return nil
	}
	tree := parser.Parse(src, nil)
	if tree == nil {
		return nil
	}
	defer tree.Close()
	var candidates []comment
	for _, c := range group(collect(tree.RootNode(), lang, src), src) {
		if c.within(added) && !c.pragma() {
			candidates = append(candidates, c)
		}
		if len(candidates) == examined {
			break
		}
	}
	return weigh(candidates, lang)
}

// weigh runs the structural rules over the candidates and hands whatever they
// leave standing to the semantic pass, in one batch: the model is loaded once
// and only when there is something for it to read.
//
// Sitting inside a function body is a bias, not a verdict. A line that points
// outward, at a constraint enforced somewhere the reader cannot see, earns its
// place there; only what the semantic pass recognises is nudged.
func weigh(candidates []comment, lang *language) []finding {
	verdicts := make([]verdict, len(candidates))
	var pending []int
	for i, c := range candidates {
		if verdicts[i] = inspect(c, lang); verdicts[i].reason == "" {
			pending = append(pending, i)
		}
	}
	if len(pending) > 0 {
		texts := make([][]string, len(pending))
		bias := make([]float64, len(pending))
		for j, i := range pending {
			texts[j] = opening(split(candidates[i].text))
			if buried(candidates[i].nodes[0], lang) {
				bias[j] = buriedBias
			}
		}
		for j, v := range judge(texts, bias) {
			verdicts[pending[j]] = v
		}
	}
	var out []finding
	for i, v := range verdicts {
		if v.reason != "" {
			out = append(out, finding{line: candidates[i].line, reason: v.reason, score: v.score})
		}
	}
	slices.SortStableFunc(out, func(a, b finding) int {
		return cmp.Compare(b.score, a.score)
	})
	return out
}

// verdict is what one pass makes of a comment: the nudge, and how sure the pass
// is of it. The score orders the findings and nothing else.
type verdict struct {
	reason string
	score  float64
}

// inspect returns why the shape of a comment rules it out, or the zero verdict
// to leave that judgment to the semantic pass. The first rule that fires wins:
// one line of nudge per comment.
func inspect(c comment, lang *language) verdict {
	if leftover(c, lang) {
		return verdict{"commented-out code: delete it, the previous version is in git", 1}
	}
	if n := sentences(c.text); n > docSentences {
		return verdict{
			strconv.Itoa(n) + " sentences of documentation: one is the default, a second is earned by a precondition, an invariant, a failure mode, or a cost",
			0.5 + float64(n-docSentences)/100,
		}
	}
	return verdict{}
}

// leftover reports whether a comment is commented-out code: text that parses
// cleanly as source and is shaped like source. Languages where any word
// sequence parses, shell and Ruby among them, are exempt. A language whose
// prose parses as a plain value, YAML, needs a structure to appear and needs
// more than one line, because `# note: read this` is a mapping too.
func leftover(c comment, lang *language) bool {
	switch {
	case lang.structure != nil:
		if len(c.nodes) < 2 {
			return false
		}
	case !lang.strict || !code(c.text):
		return false
	}
	parser := tree_sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(tree_sitter.NewLanguage(lang.grammar())); err != nil {
		return false
	}
	tree := parser.Parse([]byte(dedent(c.body)), nil)
	if tree == nil {
		return false
	}
	defer tree.Close()
	root := tree.RootNode()
	if root.HasError() {
		return false
	}
	if lang.structure != nil {
		return holds(root, lang.structure)
	}
	return root.NamedChildCount() > 0
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

// buried reports whether a node sits inside a function body, where the rules
// allow no prose at all.
func buried(node *tree_sitter.Node, lang *language) bool {
	for parent := node.Parent(); parent != nil; parent = parent.Parent() {
		if lang.functions[parent.Kind()] {
			return true
		}
	}
	return false
}

// collect returns the comment nodes of a tree in document order.
func collect(node *tree_sitter.Node, lang *language, src []byte) []*tree_sitter.Node {
	var out []*tree_sitter.Node
	if lang.comments[node.Kind()] {
		return []*tree_sitter.Node{node}
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		out = append(out, collect(node.Child(i), lang, src)...)
	}
	return out
}

// group merges the single-line comments of one run into a single comment, so
// that a doc comment written as four `//` lines is judged as one piece of prose.
func group(nodes []*tree_sitter.Node, src []byte) []comment {
	var out []comment
	for _, node := range nodes {
		raw := node.Utf8Text(src)
		if n := len(out); n > 0 && adjacent(out[n-1].nodes[len(out[n-1].nodes)-1], node) {
			out[n-1].nodes = append(out[n-1].nodes, node)
			out[n-1].raw = append(out[n-1].raw, raw)
			out[n-1].text = join(out[n-1].text, prose(raw))
			out[n-1].body += "\n" + body(raw)
			continue
		}
		out = append(out, comment{
			nodes: []*tree_sitter.Node{node},
			raw:   []string{raw},
			text:  prose(raw),
			body:  body(raw),
			line:  node.StartPosition().Row + 1,
		})
	}
	return out
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

// within reports whether any line of the comment falls in the text just written.
func (c comment) within(added []span) bool {
	for _, node := range c.nodes {
		for _, s := range added {
			if node.StartByte() < s.end && s.start < node.EndByte() {
				return true
			}
		}
	}
	return false
}

// pragma reports whether every line of the comment is machine-readable.
func (c comment) pragma() bool {
	for _, line := range c.raw {
		for _, one := range strings.Split(line, "\n") {
			if strings.TrimSpace(strip(one)) != "" && !directive(one) {
				return false
			}
		}
	}
	return true
}

// prose renders a raw comment as the text it reads as, markers removed.
func prose(raw string) string {
	var out string
	for _, line := range strings.Split(raw, "\n") {
		out = join(out, strip(line))
	}
	return out
}

// body renders a raw comment with its markers removed and its line breaks and
// indentation kept.
func body(raw string) string {
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		lines[i] = indented(line)
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
