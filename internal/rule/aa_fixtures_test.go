package rule

import (
	"testing"

	"github.com/mikluko/slopguard/internal/comment"
	"github.com/mikluko/slopguard/internal/lang"
	"github.com/mikluko/slopguard/internal/model"
	"github.com/mikluko/slopguard/internal/prose"
)

// What every test file here reads: the skip for a machine with no model, the
// leaf-package functions under short names, and the language table's entries
// under the names the spec tables give them. The prefix sorts it above the
// tests, which is for the reader rather than the compiler: a file of fixtures
// asserts nothing.

// scan runs the whole pipeline over one file, which is what a spec table hands
// it. The table's case is a file that was just written, so the span is all of
// it.
//
// Every class is on, the wider set included. These tables are the specification
// of what each rule recognises, and a rule being off by default is a shipping
// decision rather than a statement about what it reads. Turning the wider set
// off here would silently stop testing three of the five.
//
// Asked for by argument rather than by environment. Setting the variable here
// left it set for whatever ran next and cleared one the caller had exported, and
// no race detector reports either: the environment is behind its own lock, so
// what breaks is the test's meaning rather than its memory.
func scan(src []byte, language *lang.Language, added []comment.Span) []Finding {
	candidates, release := comment.Scan(src, language, added)
	defer release()
	return weigh(candidates, language, src, 0, "", true)
}

// shipped runs the pipeline as it ships, with the wider set off, which is what
// [scan] deliberately does not do.
func shipped(src []byte, language *lang.Language, added []comment.Span) []Finding {
	candidates, release := comment.Scan(src, language, added)
	defer release()
	return weigh(candidates, language, src, 0, "", false)
}

// whole is the span covering a source file entire.
func whole(src []byte) []comment.Span {
	return []comment.Span{{Start: 0, End: uint(len(src))}}
}

// skipWithoutRuntime skips a test that needs the model, for either reason it can
// be absent: no library to load, or the semantic pass switched off.
//
// Every rule here has to hold without the model — that is a supported way to run
// this tool rather than a degraded one — so a test that needs it skips rather
// than fails.
func skipWithoutRuntime(t *testing.T) {
	t.Helper()
	if why := model.Absent(); why != "" {
		t.Skip(why)
	}
}

// The handful of leaf-package functions the tests call directly, under the
// names they had when everything was one package. A test that walks a corpus
// reads better for them, and keeping them here means the spec tables did not
// have to change when the packages did.
var (
	lookup    = lang.Lookup
	split     = prose.Split
	sentences = prose.Sentences
	notice    = prose.Notice
)

// language is the table's type under the name the spec tables give the column.
// An alias rather than a definition: it is the same type, so a test can hand one
// straight to scan.
type language = lang.Language

// The language table's entries under short names, for the spec tables that name
// one language per row. `golang` reads better than `lang.Go` down a column of
// thirty cases, and this is the only place the two spellings meet.
var (
	golang     = lang.Go
	python     = lang.Python
	javascript = lang.JavaScript
	typescript = lang.TypeScript
	tsx        = lang.TSX
	rust       = lang.Rust
	bash       = lang.Bash
	clang      = lang.C
	cpp        = lang.CPP
	java       = lang.Java
	ruby       = lang.Ruby
	yaml       = lang.YAML
	hcl        = lang.HCL
	dockerfile = lang.Dockerfile
	makefile   = lang.Makefile
	php        = lang.PHP
)
