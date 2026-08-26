package main

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// Counting a doc comment's sentences measures how long it is. What the rule
// asks is whether its sentences earn their length, and the two come apart in
// both directions: three sentences of padding stay under any threshold, and
// nine sentences that each carry a precondition or a cost cross every one.
//
// The doctrine states the test as a deletion: a clause earns its place only if
// removing it changes what the doc guarantees. Restated over the artefact
// rather than over the text, a sentence earns its place when it rules out an
// implementation somebody could otherwise have written. What follows is the
// cheap half of that — not what the sentence rules out, but whether it says
// anything at all that the declaration above it did not already say.
//
// A sentence is hollow when every content word it carries is one of three
// things: a word the signature already spells, a word any documentation of any
// symbol could carry, or a word rating the code rather than constraining it.
// Such a sentence has no subject of its own. It is a set membership test
// between one sentence and one declaration, with no averaging anywhere, which
// is what separates it from the class this repo measured at +0.32 and deleted:
// there is no operation here in which a subject can cancel a distinction.

// padded is a sentence that says nothing the declaration did not, and why.
type padded struct {
	// at is the sentence's position in its comment, one-based, and not a line
	// number. Sentences do not divide onto lines evenly, and naming a line the
	// sentence does not begin on points a reader at the wrong text.
	at int
	// why is "echo" where the sentence only respells the signature, and
	// "assessment" where it rates the code.
	why string
}

// hollowReasons is what each kind of padded sentence is told about itself. The
// nudge names the sentence rather than the comment, because the fix is to cut
// that sentence and not to shorten the whole thing.
var hollowReasons = map[string]string{
	"echo":       "says nothing the signature does not",
	"assessment": "rates the code instead of constraining it",
}

// hollows returns the sentences of a doc comment that earn no place in it.
//
// The verdict is the comment's rather than the sentence's, because Go mandates
// an opening sentence that names the symbol — "Close closes the File" — and
// reading that one alone as hollow reports the whole standard library. A second
// hollow sentence is what says the comment is padded rather than conventional.
func hollows(c comment, src []byte) []padded {
	if c.trailing || c.heads {
		return nil
	}
	declaration := documents(c, src)
	if declaration == nil {
		return nil
	}
	pieces := split(c.text)
	if len(pieces) < 2 {
		// One sentence is the doctrine's own default and is nobody's business.
		return nil
	}
	spelled := declared(declaration, src)
	var out []padded
	for i, sentence := range pieces {
		words := content(sentence)
		if len(words) < 3 {
			// Too short to be evidence either way: "It is reused." carries one
			// content word, and a rule reading that as padding would take every
			// terse contract in the corpus with it.
			continue
		}
		rated := false
		novel := 0
		for _, word := range words {
			switch {
			case evaluative[word]:
				rated = true
			case spelled[word] || scaffold[word]:
			default:
				novel++
			}
		}
		if novel > 0 {
			// It names something the signature does not. Whether that thing is
			// worth naming is a judgment this rule does not make.
			continue
		}
		why := "echo"
		if rated {
			why = "assessment"
		}
		// A sentence that rules something out has done its work whatever
		// vocabulary it used: "The element must not be nil" spells only the
		// signature's own words and still constrains every caller. The exemption
		// is not extended to an assessment, because a modal taking an evaluative
		// complement — "should be self-explanatory" — commits to nothing.
		if why == "echo" && eliminates(sentence) {
			continue
		}
		out = append(out, padded{at: i + 1, why: why})
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

// documents returns the declaration a comment documents, or nil where it
// documents none.
//
// Two shapes, and which one a language uses decides where to look. A comment
// stands above what it documents in Go, C, Java and JavaScript, so the
// declaration is the node after it. A Python docstring sits inside the body it
// opens, so the declaration is the node around it — and there `annotates` is
// the next statement of the body, which is the wrong node rather than no node.
//
// A note inside a body documents nothing. What `annotates` returns for one is
// as often a closing brace as a statement, and a brace spells nothing, so every
// word of the note reads as already covered.
func documents(c comment, src []byte) *tree_sitter.Node {
	if c.doc {
		for at := c.nodes[0].Parent(); at != nil; at = at.Parent() {
			if definitions[at.Kind()] {
				return at
			}
		}
		return nil
	}
	if c.buried || c.annotates == nil || !definitions[c.annotates.Kind()] {
		return nil
	}
	return c.annotates
}

// definitions are the node kinds that declare something a comment can document.
// A comment above a statement is a note about that statement, which the
// restatement rules read; only a declaration has a signature to be measured
// against.
var definitions = set(
	"function_declaration", "method_declaration", "type_declaration",
	"const_declaration", "var_declaration",
	"function_definition", "class_definition", "decorated_definition",
	"function_item", "struct_item", "enum_item", "trait_item", "impl_item",
	"class_declaration", "method_definition", "lexical_declaration",
	"field_declaration", "declaration", "interface_declaration",
	"constructor_declaration", "enum_declaration",
)

// declared returns the words a declaration's signature spells: its name split
// on camel case, its receiver, its parameter names and types, and its results.
// The body is skipped — what a function does inside itself is not something its
// doc is repeating by naming.
func declared(node *tree_sitter.Node, src []byte) map[string]bool {
	out := map[string]bool{}
	var walk func(*tree_sitter.Node)
	walk = func(n *tree_sitter.Node) {
		// Prose under a declaration is not part of what it spells. A struct's
		// fields are its contract and belong here, but the doc comments on
		// those fields are documentation in their own right, and folding them
		// in makes a type's doc read as a restatement of its own fields' docs:
		// `net/http.Response` said "streamed on demand", both words appeared in
		// a field's comment, and the sentence read as saying nothing.
		if strings.Contains(n.Kind(), "comment") {
			return
		}
		if n.ChildCount() == 0 {
			for _, word := range pieces(n.Utf8Text(src)) {
				out[strings.TrimSuffix(word, "s")] = true
			}
			return
		}
		body := n.ChildByFieldName("body")
		for i := uint(0); i < n.ChildCount(); i++ {
			child := n.Child(i)
			if body != nil && child.Equals(*body) {
				continue
			}
			walk(child)
		}
	}
	walk(node)
	return out
}

// eliminates reports whether a sentence rules out an implementation: a
// negation, an obligation, a condition, a bound, a failure. The scan is over
// the raw sentence rather than its content words, since [content] drops exactly
// the closed-class words this is looking for.
func eliminates(sentence string) bool {
	if strings.Contains(sentence, "O(") {
		return true
	}
	for _, word := range strings.Fields(normalize(sentence)) {
		if eliminators[strings.Trim(word, ".,;:()")] {
			return true
		}
	}
	return false
}

// scaffold holds the words that any documentation of any symbol can carry
// without saying anything about that symbol. A sentence assembled entirely from
// these and the signature's own words has no subject of its own.
var scaffold = set(
	"function", "method", "parameter", "argument", "receiver", "return", "returns",
	"take", "accept", "value", "result", "implementation", "code", "caller",
	"reader", "user", "about", "use", "usage", "call", "type", "object",
	"instance", "field", "item", "data", "given", "receive", "create", "make",
	"new", "store", "helper", "struct", "pointer", "design", "provide",
	"contain", "hold", "get", "set", "handle", "process", "iterate", "add",
	"turn", "wrap", "represent", "perform", "work", "way", "thing", "part",
	"need", "want", "allow", "simply", "just", "utility", "wrapper", "loop",
	"over", "each", "convert", "build", "run", "operation", "purpose",
	"pass", "follow", "read", "write", "look", "see", "know", "understand",
)

// evaluative holds the words that rate the artefact or the reader's experience
// of it. The doctrine's list of what earns a second sentence — a precondition,
// an invariant, a failure mode, a cost — admits none of these.
//
// What is deliberately absent is every adjective a machine could check. "Safe",
// "ready", "atomic", "idempotent", "reentrant" and "deterministic" read like
// praise and are contracts: "the zero value is ready to use" is an invariant
// this repo already labels as one.
var evaluative = set(
	"simple", "easy", "straightforward", "explanatory", "clean", "robust",
	"elegant", "obvious", "trivial", "designed", "intended", "meant",
	"basically", "essentially", "nice", "convenient", "powerful", "readable",
	"intuitive", "ergonomic", "efficient", "fast", "quick", "better", "best",
	"good", "handy", "useful", "helpful", "clear", "neat", "seamless",
)

// eliminators are the closed-class words by which a sentence rules something
// out. A sentence carrying one has constrained a caller whatever else it spells.
var eliminators = set(
	"not", "no", "never", "none", "nothing", "without", "unless", "except",
	"rather", "instead", "otherwise", "if", "when", "whenever", "until",
	"while", "only", "whatever", "wherever", "before", "after", "during",
	"since", "must", "shall", "required", "cannot", "panic", "panics", "nil",
	"error", "errors", "err", "fail", "fails", "failure", "leak", "overflow",
	"undefined", "deadlock", "block", "blocks", "truncated", "dropped",
	"ignored", "once", "twice", "exactly", "least", "most", "amortized",
	"zero", "negative", "empty", "invalid", "safe", "unsafe", "concurrent",
	"goroutine", "lock", "held", "reused", "retain", "own",
)
