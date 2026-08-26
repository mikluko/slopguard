package rule

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/mikluko/slopguard/internal/comment"
)

// Python is the language where prose parses. `in`, `is`, `not`, `and` and `or`
// are operators, so `# DEBUG=False in production` and `# on_delete=CASCADE is
// deliberate` are both clean parses — the first an assignment, the second an
// assignment too — and both are ordinary comments in a Django project.
//
// What separates them from commented-out code is the shape of the statement
// rather than the fact of parsing. An import, a definition, a return, a raise
// and a call are code and read as nothing else. An assignment is code only
// when what it assigns is a value: `TIMEOUT = 30` is a setting somebody
// commented out, and `on_delete=CASCADE is deliberate` assigns a comparison,
// which is a sentence wearing an equals sign.
func pythonCode(c comment.Comment, root *tree_sitter.Node, body, src []byte) bool {
	found := false
	var walk func(*tree_sitter.Node)
	walk = func(node *tree_sitter.Node) {
		if found {
			return
		}
		switch node.Kind() {
		case "import_statement", "import_from_statement", "function_definition",
			"class_definition", "decorated_definition", "return_statement",
			"raise_statement", "assert_statement", "delete_statement",
			"global_statement", "nonlocal_statement", "with_statement",
			"for_statement", "while_statement", "try_statement", "call":
			found = true
			return
		case "assignment", "augmented_assignment":
			if value := node.ChildByFieldName("right"); value != nil && settable(value.Kind()) {
				found = true
				return
			}
		}
		for i := uint(0); i < node.ChildCount(); i++ {
			walk(node.Child(i))
		}
	}
	walk(root)
	return found
}

// settable reports whether a node kind is a value rather than a claim about
// one.
func settable(kind string) bool {
	switch kind {
	case "integer", "float", "string", "true", "false", "none", "list",
		"dictionary", "set", "tuple", "call", "attribute", "unary_operator",
		"concatenated_string", "list_comprehension", "lambda":
		return true
	}
	return false
}

// dockerCode reports whether a comment is a commented-out instruction.
//
// The grammar alone cannot say: an instruction is a keyword and some words, so
// "copy the buffer before the write" parses as a clean COPY. What separates
// them is the case. Every Dockerfile in the world writes its instructions in
// capitals, and no comment writes prose that way.
func dockerCode(c comment.Comment, root *tree_sitter.Node, body, src []byte) bool {
	head := strings.Fields(string(body))
	if len(head) == 0 || head[0] != strings.ToUpper(head[0]) {
		return false
	}
	found := false
	var walk func(*tree_sitter.Node)
	walk = func(node *tree_sitter.Node) {
		if strings.HasSuffix(node.Kind(), "_instruction") {
			found = true
			return
		}
		for i := uint(0); i < node.ChildCount() && !found; i++ {
			walk(node.Child(i))
		}
	}
	walk(root)
	return found
}
