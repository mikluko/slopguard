package rule

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/mikluko/slopguard/internal/comment"
	"github.com/mikluko/slopguard/internal/prose"
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
	// text is the sentence itself, so the nudge can quote what to cut. An
	// ordinal is not addressable: a reader would have to count sentences across
	// a wrapped comment to find the one being talked about.
	text string
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
func hollows(c comment.Comment, src []byte) []padded {
	if c.Trailing || c.Heads {
		return nil
	}
	declaration := documents(c, src)
	if declaration == nil {
		return nil
	}
	pieces := prose.Split(c.Text)
	if len(pieces) < 2 {
		// One sentence is the doctrine's own default and is nobody's business.
		return nil
	}
	spelled := declared(declaration, src)
	var out []padded
	for i, sentence := range pieces {
		// The first sentence is the contract, and Go's convention requires it
		// to open by naming the symbol — "Close closes the File". Reading that
		// one as padding reports the whole standard library, and telling an
		// agent to cut it is telling it to break the convention.
		if i == 0 {
			continue
		}
		if structured(sentence) {
			continue
		}
		words := content(sentence)
		if len(words) < 3 {
			// Too short to be evidence either way: "It is reused." carries one
			// content word, and a rule reading that as padding would take every
			// terse contract in the corpus with it.
			continue
		}
		rating, novel := 0, 0
		for _, word := range words {
			switch {
			case evaluative[word]:
				rating++
			case spelled[word] || scaffold[word]:
			default:
				novel++
			}
		}
		// A sentence spending most of itself rating the code is rating the
		// code, and one domain noun does not redeem it: "The design is intended
		// to be straightforward and easy to extend" carries three ratings and
		// the word "extend". Requiring nothing novel at all would spare it.
		rated := rating >= 2 && novel < rating
		if novel > 0 && !rated {
			// It names something the signature does not. Whether that thing is
			// worth naming is a judgment this rule does not make.
			continue
		}
		why := "echo"
		if rating > 0 {
			why = "assessment"
		}
		// A sentence that rules something out has done its work whatever
		// vocabulary it used: "The element must not be nil" spells only the
		// signature's own words and still constrains every caller. The exemption
		// is not extended to an assessment, because a modal taking an evaluative
		// complement — "should be self-explanatory" — commits to nothing.
		if why == "echo" && eliminates(sentence, spelled) {
			continue
		}
		out = append(out, padded{at: i + 1, why: why, text: sentence})
	}
	// Two, not one. The doctrine puts the burden on the second sentence, so one
	// is what it asks for and one is what was tried: over the standard library
	// it reports 21 comments, and they are contracts — "It returns the element
	// value e.Value.", "On return, data[newpivot] = p". A contract stated
	// entirely in the words of its own signature is ordinary, and the second
	// hollow sentence is what distinguishes a padded comment from a terse one.
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
// opens, so the declaration is the node around it — and there Annotates is
// the next statement of the body, which is the wrong node rather than no node.
//
// A note inside a body documents nothing. What Annotates holds for one is as
// often a closing brace as a statement, and a brace spells nothing, so every
// word of the note reads as already covered.
func documents(c comment.Comment, src []byte) *tree_sitter.Node {
	if c.Doc {
		for at := c.Nodes[0].Parent(); at != nil; at = at.Parent() {
			if definitions[at.Kind()] {
				return at
			}
		}
		return nil
	}
	if c.Buried || c.Annotates == nil {
		return nil
	}
	at := c.Annotates
	// A JavaScript or TypeScript doc sits above `export function f()`, and what
	// follows it is the export rather than the function. Left unwrapped the rule
	// reached 2,423 declarations and missed 39,097.
	if at.Kind() == "export_statement" {
		for i := uint(0); i < at.NamedChildCount(); i++ {
			if child := at.NamedChild(i); definitions[child.Kind()] {
				return child
			}
		}
		return nil
	}
	if !definitions[at.Kind()] {
		return nil
	}
	return at
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
	// Ruby, which had none of its kinds here: 0 of 11,063 documentation
	// comments reached this rule, so a sweep reporting nothing on Ruby was
	// reporting that the rule never ran.
	"method", "singleton_method", "class", "module",
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
			text := n.Utf8Text(src)
			for _, word := range pieces(text) {
				out[strings.TrimSuffix(word, "s")] = true
			}
			// The whole name as well as its pieces. A doc names `ValidateEmail`
			// in prose and the prose is lowercased to one word, while the
			// identifier splits into two, so without this the sentence naming
			// the symbol carries a word its own declaration is holding — and
			// that is the sentence Go convention requires.
			if joined := strings.ToLower(strings.Trim(text, "*&()[]{}, \t")); joined != "" {
				out[strings.TrimSuffix(joined, "s")] = true
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

// A phrase list was tried here and is not coming back. The shape it aimed at is
// the one this rule cannot see — "This function is responsible for validating
// the input" names validation, which the signature does not, so every word test
// reads it as contributing — and matching the frame instead of the words does
// catch it. It also catches the standard library, because the frames are not
// the agent's: "the As method is responsible for setting target" and "In other
// words, the representation must be a bijection" are contracts, and `is used to`
// opens hundreds of them.
//
// Measured, over 4,065 standard-library files: the frames cost 27 false
// positives and a trigger of one hollow sentence rather than two cost 21 more,
// against zero for the rule as it stands. This is the same mistake the repo
// already made once, where a phrase list read `// returns every durable the
// consumer no longer runs` as a change-event marker. A frame does not know who
// its subject is, and that is the whole of the distinction.

// structured reports whether a sentence is not prose at all: a documentation
// tag block, or an example somebody runs.
//
// Every register has these and none of them is a sentence. Javadoc, PHPDoc and
// Doxygen put the return value and the exceptions in `@return` and `@throws`
// sections; a Python docstring puts a worked example after `>>>` and the test
// suite executes it; RDoc writes `#=>`. Read as prose they are hollow every
// time — `@return the user name` carries "return", which is scaffolding, and
// nothing else — and 409 of 410 findings on the JDK were exactly this.
//
// They are machine-readable, which is the category this tool already declines
// to read: a `@return` is no more prose than a `//go:generate` is.
func structured(sentence string) bool {
	text := strings.TrimSpace(sentence)
	for _, opener := range []string{"@", `\`, ">>>", "#=>", "=>", "*", "-", "+"} {
		if strings.HasPrefix(text, opener) {
			return true
		}
	}
	// A tag or a prompt anywhere in it, since a sentence splitter joins a tag
	// block onto whatever prose ran into it.
	for _, marker := range []string{
		"@return", "@param", "@throws", "@see", "@link", "@code", "@since",
		"@deprecated", "@example", "@type", "@property", "@author", "@exception",
		`\return`, `\param`, `\brief`, `\throws`, ">>>", "#=>", ":param:", ":returns:",
		":rtype:", ":raises:", ":ivar:",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// eliminates reports whether a sentence rules out an implementation: a
// negation, an obligation, a condition, a bound, a failure. The scan is over
// the raw sentence rather than its content words, since [content] drops exactly
// the closed-class words this is looking for.
// A word the declaration spells is a mention of that thing and not an operator
// on it. `block`, `error`, `empty` and `once` are all ordinary parameter names,
// and without this a doc silenced itself by naming its own argument: two
// character-identical docs, one taking `value` and one taking `block`, got
// different verdicts.
func eliminates(sentence string, spelled map[string]bool) bool {
	if strings.Contains(sentence, "O(") {
		return true
	}
	for _, word := range strings.Fields(prose.Normalize(sentence)) {
		word = strings.Trim(word, ".,;:()")
		if eliminators[word] && !spelled[strings.TrimSuffix(word, "s")] {
			return true
		}
	}
	return false
}

// scaffold holds the words that any documentation of any symbol can carry
// without saying anything about that symbol. A sentence assembled entirely from
// these and the signature's own words has no subject of its own.
// Every entry has to be a key a lookup can form, and [hollows] forms its keys by
// running [content] over the sentence first. That drops a stopword, drops a word
// under three letters, and trims one trailing "s" — so "process" arrives as
// "proces", "pass" as "pas", and "then" never arrives at all.
//
// Seven entries were dead. Two of them, "proces" and "pas", were made dead by a
// round that read the reader as [eliminates], which stems nothing: it swapped
// the working keys for the spellings and disabled the words it meant to repair.
// "returns" and "afterwards" were dead the same way and older. "then", "each"
// and "new" are stopwords, which no spelling fixes: a sentence's "then" is gone
// before this set is consulted, so the sequence-marker paragraph below promises
// one word more than it delivers.
//
// The suite was green through all of it. [TestEveryScaffoldWordIsAKeyALookupForms]
// is what makes a dead entry loud, since a rule that quietly stops firing is
// this file's characteristic defect.
var scaffold = set(
	"function", "method", "parameter", "argument", "receiver", "return",
	"take", "accept", "value", "result", "implementation", "code", "caller",
	"reader", "user", "about", "use", "usage", "call", "type", "object",
	"instance", "field", "item", "data", "given", "receive", "create", "make",
	"store", "helper", "struct", "pointer", "design", "provide",
	"contain", "hold", "get", "set", "handle", "proces", "iterate", "add",
	"turn", "wrap", "represent", "perform", "work", "way", "thing", "part",
	"need", "want", "allow", "simply", "just", "utility", "wrapper", "loop",
	"over", "convert", "build", "run", "operation", "purpose",
	"pas", "follow", "read", "write", "look", "see", "know", "understand",
	// Sequence markers. They order a narrative and name nothing in it, which is
	// what makes a sentence carrying them and otherwise nothing but the body's
	// own identifiers a walk through the code rather than a claim about it.
	"first", "next", "finally", "lastly", "initially", "afterward",
	"subsequently", "second", "third", "step", "start", "begin", "end",
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
	// Absent on purpose, all three for the same reason: they read like praise
	// and name something a machine checks. `clean` is `path.Clean`. `clear` is
	// the state of a bit and the opposite of ciphertext, and sixteen exported
	// symbols in the standard library are named for it. `trivial` is a C++ type
	// trait — `std::is_trivial_v<T>` — so "the type must be trivial" is a
	// precondition the compiler enforces.
	//
	// A word here costs more than a missed finding. Rating is tested before the
	// signature is, and a rated sentence loses the exemption that spares one
	// ruling something out, so a symbol named `Clear` turned every precondition
	// mentioning it into a finding.
	"simple", "easy", "straightforward", "explanatory", "robust",
	"elegant", "obvious", "designed", "intended", "meant",
	"basically", "essentially", "nice", "convenient", "powerful", "readable",
	"intuitive", "ergonomic", "efficient", "fast", "quick", "better", "best",
	"good", "handy", "useful", "helpful", "neat", "seamless",
)

// eliminators are the closed-class words by which a sentence rules something
// out. A sentence carrying one has constrained a caller whatever else it spells.
var eliminators = set(
	"not", "no", "never", "none", "nothing", "without", "unless", "except",
	"rather", "instead", "otherwise", "if", "when", "whenever", "until",
	"while", "only", "whatever", "wherever", "before", "after", "during",
	"since", "must", "shall", "required", "cannot", "panic", "panics", "nil",
	"error", "errors", "err", "fail", "fails", "failure", "leak", "overflow",
	"null", "nullptr", "none", "undefined", "deadlock", "block", "blocks",
	"truncated", "dropped",
	"ignored", "once", "twice", "exactly", "least", "most", "amortized",
	"zero", "negative", "empty", "invalid", "safe", "unsafe", "concurrent",
	"goroutine", "lock", "held", "reused", "retain", "own",
)
