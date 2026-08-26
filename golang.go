package main

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// A tree-sitter grammar is context-free, and Go is not. The grammar accepts
// `f == g` and `y0(x) = 1/sqrt(pi)`, neither of which the compiler does, so a
// rule that reads "parses cleanly" as "is code" reports every equation and
// every relation somebody wrote in a comment as code they switched off.
//
// What follows is the gap: three shapes the grammar admits and the language
// refuses. A fragment carrying one of them was never compiled, so it is not
// code anybody commented out, whatever it parses as.

// goLegal reports whether every statement in a fragment could have compiled.
// One that could not is enough to rule the whole fragment out: a run of lines
// is commented out together, so a single equation among them says the run is
// notation rather than code.
func goLegal(statements []*tree_sitter.Node, body []byte) bool {
	for _, node := range statements {
		switch node.Kind() {
		case "expression_statement":
			if !goStatement(node.NamedChild(0)) {
				return false
			}
		case "assignment_statement":
			if !goAssignable(node.ChildByFieldName("left"), body) {
				return false
			}
		case "labeled_statement":
			// `match:`, `cond:`, `result:`, `Have:` — the shape a note takes
			// when it labels what follows. Go has labels, but a run of code
			// commented out of a function does not usually open on one, and the
			// rewrite-rule DSLs in the compiler are written this way throughout.
			return false
		}
	}
	return true
}

// goStatement reports whether an expression is one Go allows to stand alone.
// The spec allows a call and a receive and nothing else, so a comparison or an
// arithmetic expression on its own line is a claim about values rather than a
// line that once ran.
func goStatement(node *tree_sitter.Node) bool {
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "call_expression":
		return true
	case "unary_expression":
		return node.ChildByFieldName("operator").Kind() == "<-"
	}
	return false
}

// goAssignable reports whether the left side of an assignment is something Go
// can assign to. `y0(x) = ...` and `complex(e, f) = n/m` name a function of
// their argument, which is mathematics: the compiler's own words for it are
// "cannot assign to f(x) (neither addressable nor a map index expression)".
func goAssignable(left *tree_sitter.Node, body []byte) bool {
	if left == nil {
		return false
	}
	for i := uint(0); i < left.NamedChildCount(); i++ {
		if !goTarget(left.NamedChild(i)) {
			return false
		}
	}
	// A single target is the node itself rather than a list under it.
	return left.NamedChildCount() > 0 || goTarget(left)
}

// goTarget reports whether one expression can be assigned to.
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
		return node.ChildByFieldName("operator").Kind() == "*"
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
