package main

import (
	"github.com/mikluko/slopguard/internal/lang"
	"github.com/mikluko/slopguard/internal/prose"
)

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
