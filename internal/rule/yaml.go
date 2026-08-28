package rule

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/mikluko/slopguard/internal/comment"
)

// yamlConfig separates configuration somebody commented out from a sentence
// that happens to hold a colon. Both parse as a mapping, so the tell is
// elsewhere: configuration nests, or repeats, or carries a value with no prose
// in it, while `# Note: this cluster is shared` carries a sentence and a
// capital.
func yamlConfig(c comment.Comment, root *tree_sitter.Node, body, src []byte) bool {
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

// documented reports whether a commented-out block is showing what could go in
// a key rather than leaving behind what used to.
//
// The tell is the key it opens under. A chart declares an optional setting by
// leaving it unset and commenting the shape underneath, indented under the key
// as `helm create` scaffolds it or flush left as an Ansible role's defaults
// spell it:
//
//	podSecurityContext: {}
//	  # fsGroup: 2000
//
//	redis_disabled_commands: []
//	# - FLUSHDB
//
// Both of this rule's instructions are wrong on that block: deleting it deletes
// the documentation, and making it real changes what deploys. So the carve-out
// is held to the shape the idiom has — the comment opens on the line under the
// key and does not dedent past it — because a block that is merely somewhere
// below an unset key introduces settings that appear nowhere else in the file,
// and that is a leftover wherever the file happens to be named.
func documented(c comment.Comment, src []byte) bool {
	above := opened(c.Root, c.Nodes[0], src)
	if above == nil || !unset(above, src) {
		return false
	}
	at := c.Nodes[0].StartPosition()
	return at.Row == above.EndPosition().Row+1 && at.Column >= above.StartPosition().Column
}

// unset reports whether a mapping pair names a setting without giving it one: a
// key with no value, an empty collection, or an empty string are the four ways
// a chart or a role writes that.
func unset(pair *tree_sitter.Node, src []byte) bool {
	value := pair.ChildByFieldName("value")
	if value == nil {
		return true
	}
	// The grammar wraps a flow collection and a quoted scalar alike in a
	// flow_node, so the shape being looked for is one level down from the value.
	if value.Kind() == "flow_node" && value.NamedChildCount() == 1 {
		value = value.NamedChild(0)
	}
	if hollow[value.Kind()] {
		return value.NamedChildCount() == 0
	}
	return quoted[value.Kind()] && strings.Trim(value.Utf8Text(src), `"'`) == ""
}

// hollow are the collections a key is given when it is documented but unset.
var hollow = map[string]bool{"flow_mapping": true, "flow_sequence": true}

// quoted are the scalars whose emptiness is written rather than implied.
var quoted = map[string]bool{"double_quote_scalar": true, "single_quote_scalar": true}

// opened returns the mapping pair a comment sits under, found by walking back
// over whitespace the way the scanner walks forward to find what a comment
// annotates.
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
