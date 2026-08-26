package rule

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/mikluko/slopguard/internal/comment"
)

// A tree-sitter grammar is context-free, and Go is not. The grammar accepts
// `f == g` and `y0(x) = 1/sqrt(pi)`, neither of which the compiler does, so a
// rule that reads "parses cleanly" as "is code" reports every equation and
// every relation somebody wrote in a comment as code they switched off.
//
// Two of the three rules below close that gap exactly: an expression statement
// that is not a call, and an assignment to something not assignable, are both
// text the compiler would have refused, so neither was ever code. The third,
// the label, is a heuristic — Go puts a label on any statement — and it is here
// because the shape it catches is how a note announces what follows.

// The per-language halves of the commented-out-code rule, found by language
// name rather than held on the language itself.
//
// A predicate needs the comment, the parse of its text, and the file it came
// from, which is everything the rules already work with. Putting it on the
// language table instead would make that table name those types, and the table
// is what the extraction reads to find comments in the first place — so the
// table would depend on the rules and the rules on the table. Nothing else
// wanted [lang.Language.Name]; this is what it is for.
var (
	evidence = map[string]func(c comment.Comment, parsed *tree_sitter.Node, body, src []byte) bool{
		"python":     pythonCode,
		"yaml":       yamlConfig,
		"dockerfile": dockerCode,
	}
	legal = map[string]func(statements []*tree_sitter.Node, body []byte) bool{
		"go": goLegal,
	}
)

// goLegal reports whether every statement in a fragment could have compiled.
// One that could not is enough to rule the whole fragment out: a run of lines
// is commented out together, so a single equation among them says the run is
// notation rather than code.
func goLegal(statements []*tree_sitter.Node, body []byte) bool {
	for _, node := range statements {
		switch node.Kind() {
		case "expression_statement":
			if !goStatement(node.NamedChild(0), body) {
				return false
			}
		case "assignment_statement":
			if !goAssignable(node.ChildByFieldName("left")) {
				return false
			}
		case "package_clause":
			// Only reachable from the file-scope pass, and only ever prose:
			// "the package clause" is an English phrase that parses as one, and
			// nothing in the standard library commented out a package clause on
			// purpose. A file scope was opened for `func` and `import`, not for
			// this.
			return false
		case "labeled_statement":
			// `match:`, `cond:`, `result:`, `Have:`, `Cases:` — the shape a note
			// takes when it labels what follows. Go puts a label on any
			// statement, so this is a heuristic rather than a rule the language
			// gives: what saves it is that a label opening a run of commented-out
			// code labels a loop or a switch, and a note labels nothing.
			labelled := node.NamedChild(node.NamedChildCount() - 1)
			if labelled == nil || !loops[labelled.Kind()] {
				return false
			}
		}
		// An illegal statement nested inside a legal one rules the run out just
		// the same: `// if x {` over `//     y == z` is an equation somebody
		// wrote down, whatever encloses it.
		if !goLegal(children(node), body) {
			return false
		}
	}
	return true
}

// loops are the statements a label in running code is attached to.
var loops = set("for_statement", "range_clause", "switch_statement",
	"expression_switch_statement", "type_switch_statement", "select_statement")

// children returns a node's named children, for the walk that reaches a nested
// statement.
func children(node *tree_sitter.Node) []*tree_sitter.Node {
	var out []*tree_sitter.Node
	for i := uint(0); i < node.NamedChildCount(); i++ {
		out = append(out, node.NamedChild(i))
	}
	return out
}

// goStatement reports whether an expression is one Go allows to stand alone.
// The spec allows a function or method call and a receive, and excepts the
// built-ins whose whole purpose is the value they return. A conversion is not a
// call at all, whatever it looks like: `int64(x)` on its own line is a claim
// about a value, and no line of running code ever spelled it.
func goStatement(node *tree_sitter.Node, body []byte) bool {
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "call_expression":
		callee := node.ChildByFieldName("function")
		return callee == nil || !dropped[callee.Utf8Text(body)]
	case "unary_expression":
		operator := node.ChildByFieldName("operator")
		return operator != nil && operator.Kind() == "<-"
	}
	return false
}

// dropped names what a call cannot be if its result goes nowhere: a predeclared
// type, which makes the call a conversion rather than a call, and the built-ins
// that exist for the value they return. `len(b)` on its own line is a claim
// about a length, and no line of running code ever spelled it.
//
// A name is all there is to go on, so a package-level function shadowing one of
// these reads as the built-in and its comment goes unreported. That is a trade
// this rule can afford: what a sweep of the standard library turns up is the
// built-ins themselves.
var dropped = set(
	"append", "cap", "complex", "imag", "len", "make", "max", "min", "new", "real",
	"any", "bool", "byte", "complex64", "complex128", "error", "float32", "float64",
	"int", "int8", "int16", "int32", "int64", "rune", "string",
	"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
	// The callee text is the whole selector, so these match as spelled.
	"unsafe.Add", "unsafe.Alignof", "unsafe.Offsetof", "unsafe.Sizeof",
	"unsafe.Slice", "unsafe.SliceData", "unsafe.String", "unsafe.StringData",
)

// goAssignable reports whether the left side of an assignment is something Go
// can assign to. `y0(x) = ...` and `complex(e, f) = n/m` name a function of
// their argument, which is mathematics: the compiler's own words for it are
// "cannot assign to f(x) (neither addressable nor a map index expression)".
func goAssignable(left *tree_sitter.Node) bool {
	return goTarget(left)
}

// goTarget reports whether one expression can be assigned to. The left side of
// an assignment is an expression_list in this grammar even when it holds one
// target, so the list case is the one that runs.
func goTarget(node *tree_sitter.Node) bool {
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "identifier", "selector_expression", "index_expression":
		return true
	case "parenthesized_expression":
		return goTarget(node.NamedChild(0))
	case "unary_expression":
		operator := node.ChildByFieldName("operator")
		return operator != nil && operator.Kind() == "*"
	case "expression_list":
		for i := uint(0); i < node.NamedChildCount(); i++ {
			if !goTarget(node.NamedChild(i)) {
				return false
			}
		}
		return node.NamedChildCount() > 0
	}
	return false
}
