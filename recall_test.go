package main

import (
	"strings"
	"testing"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// The padding rule has no corpus to be measured against, because the shape it
// exists to catch is not in the corpora. Across 4,065 Go standard library files
// and 1,088 first-party Django files it fires zero times, and a search of both
// for the vocabulary it keys on finds every hit inside vendored code. Human
// code, including code nobody would defend, does not pad its documentation this
// way.
//
// So the rows below are written rather than harvested, and a reviewer measured
// exactly what that is worth. Asked to write seven doc comments in the style
// agents actually produce, without having read the word lists, they got zero
// reported. Then they rephrased three of the rows here that do fire, adding one
// ordinary word to each — "both of which are optional", "an in-memory cache" —
// and two of the three went silent.
//
// The number these rows produce is therefore a description of these rows. What
// the rule reliably catches is two shapes: a sentence that only respells its
// signature, and one that only rates the code. What an agent actually writes is
// mostly a third — implementation narrative, "It first opens a transaction,
// then writes the user row" — which introduces real vocabulary and which no
// closure test can see. That shape is the class this repo measured at +0.303 and
// buried, and it stays buried.
//
// A phrase list aimed at the frames agents use was tried and measured: it
// caught five of the reviewer's seven and cost 27 false positives on the Go
// standard library, because the frames belong to the language and not to the
// agent. See the note above [eliminates] in padding.go.
//
// Each row is one declaration and its documentation. `want` is whether the rule
// should report it, judged against the doctrine: a sentence past the first earns
// its place by a precondition, an invariant the caller must hold, a failure
// mode, or a cost the signature cannot show.
var recallRows = []struct {
	name string
	src  string
	want bool
}{
	// --- should fire -------------------------------------------------------

	{
		name: "signature restated, then rated",
		want: true,
		src: `package p
// Verdict is a function that takes a command and returns a verdict about it.
// It is designed to be simple and easy to use. The implementation is
// straightforward and should be self-explanatory to any reader.
func Verdict(command string) string { return "" }`,
	},
	{
		name: "parameter list narrated back",
		want: true,
		src: `package p
// NewClient creates a new client. It takes a timeout parameter and a retries
// parameter. It returns a pointer to a Client object.
func NewClient(timeout int, retries int) *Client { return nil }`,
	},
	{
		name: "usage narrative",
		want: true,
		src: `package p
// Encode encodes the value to bytes. Callers use this function by passing a
// value. The returned bytes can then be used by the caller as needed.
func Encode(value string) []byte { return nil }`,
	},
	{
		name: "second paragraph elaborating the first",
		want: true,
		src: `package p
// Sum adds the numbers together and returns the total.
//
// This helper is provided for convenience. It simply iterates over each of the
// given numbers and adds each number to a running total.
func Sum(numbers []int) int { return 0 }`,
	},
	{
		name: "type doc rating itself",
		want: true,
		src: `package p
// Cache is a cache. It is a simple and efficient implementation that is easy
// to use. The design is intended to be straightforward.
type Cache struct{ entries int }`,
	},

	// --- must not fire -----------------------------------------------------

	{
		name: "every sentence a contract",
		want: false,
		src: `package p
// Parse turns one shell command into a syntax tree. A command that does not
// parse is allowed rather than denied: a parse failure is not evidence of a
// write. The tree is walked once, so a command with N words costs O(N).
// Callers must not hold the returned tree past the next call: it is reused.
// An empty command yields a nil tree and no error.
func Parse(command string) error { return nil }`,
	},
	{
		name: "the conventional Go opener alone",
		want: false,
		src: `package p
// Close closes the file and releases any resources held by it.
func Close(file string) error { return nil }`,
	},
	{
		name: "an invariant spelling only the signature's words",
		want: false,
		src: `package p
// Remove removes the element from the list. The element must not be nil.
func Remove(element *Element, list *List) {}`,
	},
	{
		name: "a cost the signature cannot show",
		want: false,
		src: `package p
// Lookup finds the entry for a key. The table is scanned linearly, so a lookup
// costs O(n) and is not meant for a hot path.
func Lookup(key string, table []string) int { return 0 }`,
	},
	{
		name: "a failure mode",
		want: false,
		src: `package p
// Dial opens a connection to the address. It panics if the address is empty,
// which is a programming error rather than a runtime condition.
func Dial(address string) error { return nil }`,
	},
	{
		name: "an adjective a machine could check",
		want: false,
		src: `package p
// Pool holds reusable buffers. The zero value is ready to use and safe for
// concurrent use by multiple goroutines.
type Pool struct{ buffers int }`,
	},
	{
		name: "prose about something outside the signature",
		want: false,
		src: `package p
// Retry runs the operation again after a delay. The delay doubles each time
// and is capped at one minute, which is what keeps a failing dependency from
// being hammered.
func Retry(operation func() error) error { return nil }`,
	},
}

// TestRecall reports what the padding rule catches and what it costs on the
// written rows. Missing a padded doc is logged rather than failed: the rows are
// constructed, so a recall number taken from them is a description of the rows.
// Reporting documentation that earns its place is a failure, because that is the
// direction this tool cannot afford to be wrong in.
func TestRecall(t *testing.T) {
	caught, total := 0, 0
	for _, row := range recallRows {
		src := []byte(row.src + "\n")
		fired := false
		for _, f := range scan(src, golang, []span{{start: 0, end: uint(len(src))}}) {
			if f.class == "hollow" {
				fired = true
			}
		}
		switch {
		case row.want && fired:
			caught++
			total++
		case row.want && !fired:
			total++
			t.Logf("missed  %s", row.name)
		case !row.want && fired:
			t.Errorf("reported documentation that earns its place: %s\n%s", row.name, row.src)
		}
	}
	t.Logf("caught %d of %d written padded docs; %d earning docs left alone",
		caught, total, len(recallRows)-total)
}

// The seven below were written by a reviewer who had not read the word lists,
// asked for documentation in the style coding agents actually produce. Every one
// is padding by the doctrine. The rule reports none of them, and this test
// records that rather than asserting it: the point is to keep the ceiling
// visible and to fail loudly if a later change claims to have raised it without
// re-measuring precision.
//
// Six of the seven are implementation narrative or free-floating rationale,
// which name real things and so defeat any closure test. The seventh, the
// `Parameters:`/`Returns:` block, is invisible for a different reason worth
// knowing: it carries no sentence-ending punctuation, so it splits into one
// piece and never reaches the rule at all.
func TestKnownCeiling(t *testing.T) {
	agentStyle := []struct{ name, src string }{
		{"is responsible for", `package p
// ValidateEmail validates an email address.
//
// This function is responsible for checking that the provided email address
// conforms to the expected format. It returns a boolean value indicating
// whether the address is considered valid.
func ValidateEmail(address string) bool { return false }`},
		{"implementation narrative", `package p
// SaveUser persists a user to the database.
//
// It first opens a transaction, then writes the user row, and finally commits
// the transaction. This ensures that the write is atomic.
func SaveUser(user string, db string) error { return nil }`},
		{"parameters block", `package p
// FetchUser retrieves a user by ID.
//
// Parameters:
//   - ctx: the context used for cancellation and timeouts
//   - id: the unique identifier of the user to fetch
func FetchUser(ctx string, id string) (string, error) { return "", nil }`},
		{"struct restating its fields", `package p
// ServerConfig holds the configuration for the HTTP server.
//
// It contains the host to bind to and the port to listen on. These values are
// used to configure the underlying server instance at startup.
type ServerConfig struct {
	Host string
	Port int
}`},
		{"interface selling testability", `package p
// Store is an interface for storing and retrieving records.
//
// This interface is designed to be implemented by any backing store. Defining
// it as an interface allows for greater flexibility and makes the calling code
// easier to test with a mock implementation.
type Store interface{ Get(key string) string }`},
		{"entry point narration", `package p
// Handler handles the incoming HTTP request.
//
// This method processes the request and writes an appropriate response to the
// response writer. It is the main entry point for all incoming traffic.
func Handler(w string, r string) {}`},
		{"getter narration", `package p
// GetName returns the name.
//
// This is a simple getter method that returns the name field of the struct.
// It is provided for convenience so that the field can remain unexported.
func GetName(name string) string { return name }`},
	}
	caught := 0
	for _, row := range agentStyle {
		if padsAt(t, row.src) {
			caught++
			t.Logf("now caught: %s", row.name)
		}
	}
	t.Logf("%d of %d agent-style padded docs reported — the documented ceiling", caught, len(agentStyle))
	if caught > 0 {
		t.Logf("a row started firing; re-run the standard library sweep before believing it")
	}
}

// The rule must not decide by topic. Renaming every identifier in a file, and
// every mention of those identifiers in its documentation, changes what the
// comment is about and changes nothing about whether its sentences earn their
// place — so every verdict has to survive it. A rule that moves here is reading
// subject matter, which is the failure that killed the change-event class.
func TestAlphaRenameInvariance(t *testing.T) {
	// Both cases of every name, since a doc mentions in prose what the
	// signature spells in camel case, and renaming only one of the two is not
	// an alpha-rename — it is a doc that has stopped matching its declaration,
	// which the rule is supposed to notice.
	rename := strings.NewReplacer(
		"Verdict", "Adjudicate", "verdict", "adjudicate",
		"command", "directive", "Command", "Directive",
		"NewClient", "BuildHandle", "client", "handle",
		"timeout", "deadline", "retries", "attempts",
		"Encode", "Marshal", "encodes", "marshals", "value", "datum",
		"Sum", "Accumulate", "numbers", "quantities", "number", "quantity",
		"Cache", "Reservoir", "cache", "reservoir", "entries", "slots",
		"Parse", "Interpret", "parse", "interpret", "tree", "graph",
		"Close", "Release", "closes", "releases", "file", "handle",
		"Remove", "Detach", "removes", "detaches",
		"element", "node", "Element", "Node", "list", "chain", "List", "Chain",
		"Lookup", "Seek", "lookup", "seek", "key", "token", "table", "register",
		"Dial", "Connect", "address", "endpoint",
		"Pool", "Reservoir", "buffers", "cells",
		"Retry", "Reattempt", "operation", "procedure", "delay", "pause",
	)
	for _, row := range recallRows {
		t.Run(row.name, func(t *testing.T) {
			before := padsAt(t, row.src)
			after := padsAt(t, rename.Replace(row.src))
			if before != after {
				t.Errorf("renaming changed the verdict: %v then %v\n%s",
					before, after, rename.Replace(row.src))
			}
		})
	}
}

// Neither word list may carry the other. Emptying the one that recognises a
// signature must leave the rule still catching what rates the code, and
// emptying the one that recognises rating must leave it still catching what
// only respells the signature.
func TestListsAreIndependent(t *testing.T) {
	// Nothing here rates anything: every word comes from the signature or is
	// scaffolding. The echo half has to carry this alone.
	echoOnly := `package p
// Client is a type. It is a type that holds a host and a port. The host field
// and the port field are fields of the Client type.
type Client struct {
	host string
	port int
}`
	if !padsAt(t, echoOnly) {
		t.Error("a doc built entirely from its own signature went unreported")
	}
	if hits := rated(t, echoOnly); hits != 0 {
		t.Errorf("nothing here rates anything, but %d sentences were read that way", hits)
	}

	// Nothing here comes from the signature: `Quaternion` and `slerp` appear in
	// neither list, and what makes the sentences hollow is that they only rate.
	// The assessment half has to carry this alone.
	ratingOnly := `package p
// Slerp interpolates between two quaternions.
//
// The implementation is elegant and efficient. It is designed to be
// straightforward and should be easy for any reader to follow.
func Slerp(from Quaternion, to Quaternion, at float64) Quaternion { return from }`
	if !padsAt(t, ratingOnly) {
		t.Error("a doc that only rates the code went unreported")
	}
	if hits := rated(t, ratingOnly); hits < 2 {
		t.Errorf("expected the rating sentences to be read as rating, got %d", hits)
	}
}

// rated counts the sentences of the first doc comment in a source string that
// were reported for rating the code rather than for respelling the signature.
func rated(t *testing.T, source string) int {
	t.Helper()
	src := []byte(source + "\n")
	root := treeOf(t, src)
	for _, c := range group(root, collect(root, golang, false), src) {
		c.annotates = annotated(root, c.nodes[len(c.nodes)-1], src)
		hits := 0
		for _, p := range hollows(c, src) {
			if p.why == "assessment" {
				hits++
			}
		}
		if len(hollows(c, src)) > 0 {
			return hits
		}
	}
	return 0
}

// fires reports whether the padding rule reports a Go source string.
func padsAt(t *testing.T, source string) bool {
	t.Helper()
	src := []byte(source + "\n")
	for _, f := range scan(src, golang, []span{{start: 0, end: uint(len(src))}}) {
		if f.class == "hollow" {
			return true
		}
	}
	return false
}

// treeOf parses Go source and returns its root. The tree is deliberately left
// open: closing it invalidates every node taken out of it.
func treeOf(t *testing.T, src []byte) *tree_sitter.Node {
	t.Helper()
	parser := tree_sitter.NewParser()
	if err := parser.SetLanguage(tree_sitter.NewLanguage(golang.grammar())); err != nil {
		t.Fatal(err)
	}
	tree := parser.Parse(src, nil)
	if tree == nil {
		t.Fatal("source did not parse")
	}
	return tree.RootNode()
}
