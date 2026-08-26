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
//
// The sweep in window_test.go prices the tilt: held out, 0.03 catches one more
// comment than no tilt at all and still nudges nothing, while 0.06 catches
// three more and nudges one piece of contract prose, and 0.09 catches four and
// nudges two. This is the last setting at which every contract reading is
// perfect.
const buriedBias = 0.03

// docSentences is how long documentation runs before its length alone is worth
// a word.
//
// It is set against what documentation actually does rather than against what a
// style guide says it should. Across 90,000 comments in four repositories
// written on purpose (TestLengthDistribution measures it): half are one
// sentence, 81% are two or fewer, 95% four or fewer, and 99% eight or fewer.
// A threshold of five flags one comment in thirty-five, which on a densely
// documented repository is 386 findings and drowns everything else the tool
// has to say; at eight it flags the last percent, which is documentation that
// has genuinely run away.
const docSentences = 8

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

// parsedBytes bounds what the commented-out-code rule will hand to a parser.
// The cost is linear in the bytes and the constant is the grammar's error
// recovery over prose, which is about seven microseconds a byte, so the bound
// is what keeps a pathological comment from holding the hook: 16 KB is 0.1
// seconds where 1.75 MB was 14.7.
const parsedBytes = 16 << 10

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
	// trailing marks a comment with code before it on its own line. Nobody
	// disables a statement by appending it to a live one, so the
	// commented-out-code rule does not reach these.
	trailing bool
	// heads marks a comment with nothing but whitespace and other comments
	// before it in the file, which is where a file documents itself. A licence
	// header sits above a package doc rather than instead of it, so this is not
	// the same as being the first comment.
	heads bool
	// root is the tree the comment came from, for the rules that need to look
	// at what surrounds it.
	root *tree_sitter.Node
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
			// Deferred to here rather than done in [group]: locating the code
			// under a comment walks the tree from the root, and a file of
			// nothing but comments pays that for every one of them before
			// [examined] gets to cut. The write being judged is a few lines, so
			// almost none of them reach this.
			c.annotates = annotated(root, c.nodes[len(c.nodes)-1], src)
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
		// A licence notice is exempt from every rule, which means leaving it out
		// of both passes. Returning the zero verdict from [inspect] would not do
		// it: that is the value meaning "the shape rules nothing out", and it is
		// what puts a comment in front of the model.
		// Not inside a function body: a run reads as one comment, so a marker
		// opening any line of it pardons every line stacked under it, and a
		// licence header buried in a body is a commented-out block that happens
		// to start with one. Nothing legitimate puts a notice there.
		if notice(c.body) && !c.buried {
			continue
		}
		if verdicts[i] = inspect(c, lang, src); verdicts[i].reason == "" {
			pending = append(pending, i)
		}
	}
	if len(pending) > 0 {
		texts := make([][]string, len(pending))
		bias := make([]float64, len(pending))
		for j, i := range pending {
			texts[j] = opening(split(candidates[i].text))
			position := 0.0
			if !candidates[i].doc && candidates[i].buried {
				position = buriedBias
			}
			bias[j] = allowance(position, len(texts[j]))
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
	if n := sentences(c.text); n > docSentences && wordy(c, lang) {
		return verdict{
			strconv.Itoa(n) + " sentences of documentation: one is the default, a second is earned by a precondition, an invariant, a failure mode, or a cost",
			0.5 + float64(n-docSentences)/100,
			"length",
		}
	}
	return verdict{}
}

// wordy reports whether a long comment has anywhere else to go, which is what
// the length rule is asking. Its nudge names three homes — package
// documentation, symbol documentation, a test — and it fires only where at
// least one of them exists.
//
// Two places have none. File documentation is already the first of those homes,
// so running long there is the correct form and not a finding: the `testing`
// package's own doc is eighty sentences and every one of them earns its place.
// And a language with no function has no symbol to document and no test to move
// a claim into: in YAML and HCL the nudge resolves to "delete it", which on the
// only record of a constraint is worse than saying nothing.
func wordy(c comment, lang *language) bool {
	return !c.heads && len(lang.functions) > 0
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
//
// A comment sharing its line with code is exempt whatever it parses as. Code is
// disabled by commenting the line it is on, which leaves the comment alone on
// that line; a comment after a live statement is a note about it, and the
// notation those notes use is the notation this rule reads as source —
// `x2 := Sqrt(x1) // x2 = sqrt(1 - x*x)` says what the variable now holds.
func leftover(c comment, lang *language, src []byte) bool {
	if c.doc || c.trailing {
		return false
	}
	// The lexical prefilter is what a language has instead of a legality check:
	// a bare parse succeeds on too much prose to be evidence by itself, so text
	// that does not even look like source is turned away before parsing. A
	// language that can say what the compiler would have refused does not need
	// it, and it costs real findings — `fmt.Println("x")` is a call with an
	// argument, which no list of leading keywords matches.
	if lang.evidence == nil && lang.legal == nil && (!lang.strict || !code(c.text)) {
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
	// Parsing prose as source runs the grammar's error recovery over every byte
	// of it, at roughly seven microseconds each, and a comment is not bounded
	// by anything the way a file is. A 1.75 MB run of them held the hook for 23
	// seconds. Nobody comments out this much in one run and then wants three
	// lines of nudge back, so past the bound the rule declines.
	if len(c.body) > parsedBytes {
		return false
	}
	body := dedent(c.body) + "\n"
	if lang.wrapper != nil {
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
	for _, wrap := range scopes(lang) {
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
		if lang.evidence != nil {
			return lang.evidence(c, root, []byte(body), src)
		}
		inside := fragment(root, uint(len(prefix)), uint(len(prefix)+len(body)))
		if len(inside) == 0 {
			continue
		}
		// The wrapped text, not the fragment: every offset the nodes carry is
		// into what was parsed, so reading a node's text out of the fragment
		// alone returns whatever sits len(prefix) bytes further along.
		return lang.legal == nil || lang.legal(inside, []byte(prefix+body+suffix))
	}
	return false
}

// scopes returns the positions a fragment is tried in, in the order to try
// them. A language with no wrapper is parsed as a file and nothing else, which
// is what every language did before any of them was measured.
func scopes(lang *language) []func() (string, string) {
	bare := func() (string, string) { return "", "" }
	if lang.wrapper == nil {
		return []func() (string, string){bare}
	}
	return []func() (string, string){lang.wrapper, bare}
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
		if len(out) != 1 || !container(out[0].Kind()) {
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

// documented reports whether a commented-out block is showing what could go in
// a key rather than leaving behind what used to.
//
// The tell is the key above it. A chart documents an optional setting by giving
// it an empty collection and commenting the shape underneath:
//
//	podSecurityContext: {}
//	  # fsGroup: 2000
//
// which is what `helm create` scaffolds, and where both of this rule's
// instructions are wrong: deleting the block deletes the documentation, and
// making it real changes what the chart deploys. A block with no such key above
// it introduces settings that appear nowhere else in the file, and that is a
// leftover wherever the file happens to be named.
func documented(c comment, src []byte) bool {
	above := opened(c.root, c.nodes[0], src)
	if above == nil {
		return false
	}
	value := above.ChildByFieldName("value")
	if value == nil {
		return false
	}
	// The grammar wraps a flow collection in a flow_node, so the shape being
	// looked for is one level down from the value.
	if value.Kind() == "flow_node" && value.NamedChildCount() == 1 {
		value = value.NamedChild(0)
	}
	if !hollow[value.Kind()] || value.NamedChildCount() > 0 {
		return false
	}
	return c.nodes[0].StartPosition().Column > above.StartPosition().Column
}

// hollow are the collections a key is given when it is documented but unset.
var hollow = map[string]bool{"flow_mapping": true, "flow_sequence": true}

// opened returns the mapping pair a comment sits under, found by walking back
// over whitespace the way [annotated] walks forward.
func opened(root, node *tree_sitter.Node, src []byte) *tree_sitter.Node {
	at := int(node.StartByte()) - 1
	for at >= 0 && (src[at] == ' ' || src[at] == '\t' || src[at] == '\n' || src[at] == '\r') {
		at--
	}
	if at < 0 || root == nil {
		return nil
	}
	found := root.NamedDescendantForByteRange(uint(at), uint(at))
	for found != nil && found.Kind() != "block_mapping_pair" {
		found = found.Parent()
	}
	return found
}

// yamlConfig separates configuration somebody commented out from a sentence
// that happens to hold a colon. Both parse as a mapping, so the tell is
// elsewhere: configuration nests, or repeats, or carries a value with no prose
// in it, while `# Note: this cluster is shared` carries a sentence and a
// capital.
func yamlConfig(c comment, root *tree_sitter.Node, body, src []byte) bool {
	if !holds(root, structures) || documented(c, src) {
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
			if text := key.Utf8Text(body); text != "" && text[0] >= 'A' && text[0] <= 'Z' {
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
		if value == nil || strings.Contains(strings.TrimSpace(value.Utf8Text(body)), " ") {
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
		// A `{{` somebody wrote in prose has no closer of its own, so the next
		// one anywhere in the file gets paired with it and every comment
		// between them is erased. An action does not span a blank line, so one
		// inside the pair means this `{{` was text.
		if at := bytes.Index(out[i:i+end], []byte("\n\n")); at >= 0 {
			// Past the blank line, not one byte on: no `{{` before it can pair
			// with that `}}` either, and advancing singly re-scans to the end
			// of the file for every one of them.
			i += at + 2
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
func collect(root *tree_sitter.Node, lang *language, buried bool) []found {
	cursor := root.Walk()
	defer cursor.Close()
	var out []found
	// depths[i] is whether the node at depth i+1 was inside a function body,
	// carried down so that the answer is never asked for on the way back up.
	depths := []bool{buried}
	for {
		node := cursor.Node()
		inside := depths[len(depths)-1]
		if lang.comments[node.Kind()] || (lang.docstrings && docstring(node)) {
			out = append(out, found{node: node, buried: inside})
		} else if cursor.GotoFirstChild() {
			depths = append(depths, inside || lang.functions[node.Kind()])
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
// that a doc comment written as four `//` lines is judged as one piece of prose.
//
// A comment sharing a line with code is never merged. Each one is a note about
// its own line, and a constant table's column of them is not one comment: forty
// trailing notes in the same column read as a single comment of forty
// sentences, which the length rule then has something to say about.
func group(root *tree_sitter.Node, nodes []found, src []byte) []comment {
	var out []comment
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
		if n := len(out); n > 0 && !trailing && !out[n-1].trailing &&
			adjacent(out[n-1].nodes[len(out[n-1].nodes)-1], f.node) {
			out[n-1].nodes = append(out[n-1].nodes, f.node)
			out[n-1].raw = append(out[n-1].raw, raw)
			continue
		}
		out = append(out, comment{
			nodes:    []*tree_sitter.Node{f.node},
			raw:      []string{raw},
			line:     f.node.StartPosition().Row + 1,
			doc:      docstring(f.node),
			buried:   f.buried,
			trailing: trailing,
			heads:    opens,
			root:     root,
		})
	}
	// The prose is built once a run is closed rather than extended line by
	// line: appending to a string per line copies the run so far each time,
	// which is quadratic in a run's length and is what a file of forty thousand
	// stacked comment lines costs.
	for i := range out {
		out[i].text, out[i].body = joined(out[i].raw)
	}
	return out
}

// joined returns a run's prose on one line and its body with the line breaks
// kept, from the raw text of each of its lines.
func joined(raw []string) (string, string) {
	var prosed, bodied strings.Builder
	for i, line := range raw {
		if piece := prose(line); piece != "" {
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
