package main

import (
	"bytes"
	"cmp"
	"hash/fnv"
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
	// class names the rule that fired, for the log and for nothing else.
	class string
	// key identifies this comment at this path, so that a comment already
	// named once is not named again when the agent's own edit re-enters.
	key uint64
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
	// annotates is the code this comment sits above, where it sits above any:
	// what a comment restates, if it restates anything.
	annotates *tree_sitter.Node
	// doc marks a docstring, which documents the symbol it opens rather than
	// sitting inside it: the rules for prose in a body do not reach it.
	doc bool
	// buried marks a comment inside a function body, where prose is harder to
	// justify and the thresholds are lower.
	buried bool
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
	tree := parser.Parse(blank(src, lang), nil)
	if tree == nil {
		return nil
	}
	defer tree.Close()
	var candidates []comment
	root := tree.RootNode()
	for _, c := range group(root, collect(root, lang, false), src) {
		if c.within(added) && !c.pragma() {
			candidates = append(candidates, c)
		}
		if len(candidates) == examined {
			break
		}
	}
	return weigh(candidates, lang, src)
}

// weigh runs the structural rules over the candidates and hands whatever they
// leave standing to the semantic pass, in one batch: the model is loaded once
// and only when there is something for it to read.
//
// Sitting inside a function body is a bias, not a verdict. A line that points
// outward, at a constraint enforced somewhere the reader cannot see, earns its
// place there; only what the semantic pass recognises is nudged.
func weigh(candidates []comment, lang *language, src []byte) []finding {
	verdicts := make([]verdict, len(candidates))
	var pending []int
	for i, c := range candidates {
		if verdicts[i] = inspect(c, lang, src); verdicts[i].reason == "" {
			pending = append(pending, i)
		}
	}
	if len(pending) > 0 {
		texts := make([][]string, len(pending))
		bias := make([]float64, len(pending))
		for j, i := range pending {
			texts[j] = opening(split(candidates[i].text))
			if !candidates[i].doc && candidates[i].buried {
				bias[j] = buriedBias
			}
		}
		for j, v := range judge(texts, bias) {
			// Restatement is a relation, so the model's reading of one is
			// taken only where the code supports it: a single line below, at
			// least two content words, and at least one of them already
			// spelled by that line. A section banner — "User data" over
			// `ami_type` — shares nothing with what it heads and restates
			// nothing, whatever it reads like on its own.
			if v.class == "tautology" && !restates(candidates[pending[j]], src) {
				continue
			}
			verdicts[pending[j]] = v
		}
	}
	var out []finding
	for i, v := range verdicts {
		if v.reason != "" {
			out = append(out, finding{
				line:   candidates[i].line,
				reason: v.reason,
				score:  v.score,
				class:  v.class,
				key:    site(candidates[i].text),
			})
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
	class  string
}

// inspect returns why the shape of a comment rules it out, or the zero verdict
// to leave that judgment to the semantic pass. The first rule that fires wins:
// one line of nudge per comment.
func inspect(c comment, lang *language, src []byte) verdict {
	if leftover(c, lang, src) {
		return verdict{"commented-out code: delete it, or make it real", 1, "leftover"}
	}
	if echoes(c, src) {
		return verdict{reasonFor("tautology"), 0.95, "echo"}
	}
	if n := sentences(c.text); n > docSentences {
		return verdict{
			strconv.Itoa(n) + " sentences of documentation: one is the default, a second is earned by a precondition, an invariant, a failure mode, or a cost",
			0.5 + float64(n-docSentences)/100,
			"length",
		}
	}
	return verdict{}
}

// site identifies a comment by what it says rather than by where it sits, so
// that the agent's own corrective edit — which moves every line below it — is
// still recognised as the same comment.
func site(text string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(normalize(text)))
	return h.Sum64()
}

// leftover reports whether a comment is commented-out code: text that parses
// cleanly as source and is shaped like source. Languages where any word
// sequence parses, shell and Ruby among them, are exempt. A language whose
// prose parses as a plain value, YAML, needs a structure to appear and needs
// more than one line, because `# note: read this` is a mapping too.
func leftover(c comment, lang *language, src []byte) bool {
	if c.doc {
		return false
	}
	if lang.evidence == nil && (!lang.strict || !code(c.text)) {
		return false
	}
	parser := tree_sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(tree_sitter.NewLanguage(lang.grammar())); err != nil {
		return false
	}
	// The newline is not cosmetic: some grammars, Dockerfile's among them,
	// require a line ending and report a MISSING node without one, which reads
	// as a broken parse and stops this rule before it starts.
	body := dedent(c.body) + "\n"
	tree := parser.Parse([]byte(body), nil)
	if tree == nil {
		return false
	}
	defer tree.Close()
	root := tree.RootNode()
	if root.HasError() {
		return false
	}
	if lang.evidence != nil {
		return lang.evidence(root, []byte(body), len(c.nodes))
	}
	return root.NamedChildCount() > 0
}

// yamlConfig separates configuration somebody commented out from a sentence
// that happens to hold a colon. Both parse as a mapping, so the tell is
// elsewhere: configuration nests, or repeats, or carries a value with no prose
// in it, while `# Note: this cluster is shared` carries a sentence and a
// capital.
func yamlConfig(root *tree_sitter.Node, src []byte, lines int) bool {
	if !holds(root, structures) {
		return false
	}
	var pairs []*tree_sitter.Node
	var walk func(*tree_sitter.Node)
	walk = func(node *tree_sitter.Node) {
		if node.Kind() == "block_mapping_pair" || node.Kind() == "flow_pair" {
			pairs = append(pairs, node)
		}
		for i := uint(0); i < node.ChildCount(); i++ {
			walk(node.Child(i))
		}
	}
	walk(root)
	if len(pairs) == 0 {
		return true
	}
	titled := 0
	for _, pair := range pairs {
		if key := pair.ChildByFieldName("key"); key != nil {
			if text := key.Utf8Text(src); text != "" && text[0] >= 'A' && text[0] <= 'Z' {
				titled++
			}
		}
	}
	if titled == len(pairs) {
		return false
	}
	// Nesting is configuration on its own: a sentence does not indent under a
	// key. Without it, every value has to read as a setting rather than as
	// prose, which is what tells `# replicas: 3` from a heading with a colon.
	if nested(root) {
		return true
	}
	for _, pair := range pairs {
		value := pair.ChildByFieldName("value")
		if value == nil || strings.Contains(strings.TrimSpace(value.Utf8Text(src)), " ") {
			return false
		}
	}
	return true
}

// structures are the YAML shapes configuration takes. Prose parses as a plain
// scalar and reaches none of them.
var structures = set("block_mapping", "block_sequence", "flow_mapping", "flow_sequence")

// nested reports whether a mapping holds another mapping or a sequence.
func nested(node *tree_sitter.Node) bool {
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "block_mapping_pair" || child.Kind() == "flow_pair" {
			if value := child.ChildByFieldName("value"); value != nil {
				if holds(value, structures) {
					return true
				}
			}
		}
		if nested(child) {
			return true
		}
	}
	return false
}

// blank replaces template actions with spaces of the same width, so that a
// Helm chart's manifests parse as the YAML they become.
//
// A `{{- if }}` at column zero otherwise makes the grammar drop every comment
// in the file, which is worse than no coverage: the same comment is read in one
// manifest and invisible in the next. Widths are preserved so that every byte
// offset the parse reports still addresses the file the agent wrote, and the
// text handed back is the file's own.
func blank(src []byte, lang *language) []byte {
	if !lang.templated || !bytes.Contains(src, []byte("{{")) {
		return src
	}
	out := append([]byte(nil), src...)
	for i := 0; i+1 < len(out); {
		if out[i] != '{' || out[i+1] != '{' {
			i++
			continue
		}
		end := bytes.Index(out[i:], []byte("}}"))
		if end < 0 {
			break
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
func collect(node *tree_sitter.Node, lang *language, buried bool) []found {
	if lang.comments[node.Kind()] || (lang.docstrings && docstring(node)) {
		return []found{{node: node, buried: buried}}
	}
	buried = buried || lang.functions[node.Kind()]
	var out []found
	for i := uint(0); i < node.ChildCount(); i++ {
		out = append(out, collect(node.Child(i), lang, buried)...)
	}
	return out
}

// group merges the single-line comments of one run into a single comment, so
// that a doc comment written as four `//` lines is judged as one piece of prose.
func group(root *tree_sitter.Node, nodes []found, src []byte) []comment {
	var out []comment
	for _, f := range nodes {
		raw := f.node.Utf8Text(src)
		if n := len(out); n > 0 && adjacent(out[n-1].nodes[len(out[n-1].nodes)-1], f.node) {
			out[n-1].nodes = append(out[n-1].nodes, f.node)
			out[n-1].raw = append(out[n-1].raw, raw)
			out[n-1].text = join(out[n-1].text, prose(raw))
			out[n-1].body += "\n" + body(raw)
			out[n-1].annotates = annotated(root, f.node, src)
			continue
		}
		out = append(out, comment{
			nodes:     []*tree_sitter.Node{f.node},
			raw:       []string{raw},
			text:      prose(raw),
			body:      body(raw),
			line:      f.node.StartPosition().Row + 1,
			annotates: annotated(root, f.node, src),
			doc:       docstring(f.node),
			buried:    f.buried,
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
