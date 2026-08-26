package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// A length threshold asks how many sentences a comment has. What the rule it
// serves asks is which of them earn their place, and that is a judgment about
// one sentence against the declaration it documents and against the sentences
// beside it. Neither question can be answered from a count, and answering the
// second one needs the material this harvests: every documentation sentence in
// a corpus, with the declaration it sits above and its position in the run.
//
// It is a harvester and not a measurement. What it writes is labelled by hand
// afterwards, the way corpus.go and mined.go were.

// harvested is one documentation sentence, ready to be labelled.
type harvested struct {
	Path string `json:"path"`
	Line uint   `json:"line"`
	// At is the sentence's position in its comment and Of how many there are,
	// since the doctrine spends the first sentence on the contract and asks a
	// different question of every one after it.
	At int `json:"at"`
	Of int `json:"of"`
	// Decl is the first line of the declaration the comment documents, which is
	// what a sentence restating the signature would be restating.
	Decl     string `json:"decl"`
	Sentence string `json:"sentence"`
}

// TestHarvestSentences writes every documentation sentence in a corpus to a
// file as one JSON object per line.
func TestHarvestSentences(t *testing.T) {
	roots := os.Getenv("SLOPGUARD_CORPUS")
	into := os.Getenv("SLOPGUARD_HARVEST")
	if roots == "" || into == "" {
		t.Skip("set SLOPGUARD_CORPUS to repositories and SLOPGUARD_HARVEST to an output file")
	}
	file, err := os.Create(into)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)

	rows, comments := 0, 0
	for _, root := range strings.Split(roots, ":") {
		filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || strings.Contains(path, "/.git/") {
				return nil
			}
			lang := lookup(path)
			if lang == nil {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil || len(src) > ceiling {
				return nil
			}
			parser := tree_sitter.NewParser()
			defer parser.Close()
			if parser.SetLanguage(tree_sitter.NewLanguage(lang.Grammar())) != nil {
				return nil
			}
			tree := parser.Parse(blank(src, lang), nil)
			if tree == nil {
				return nil
			}
			defer tree.Close()
			root := tree.RootNode()
			for _, c := range group(root, collect(root, lang, false), src) {
				if c.pragma() || c.trailing || notice(c.body) {
					continue
				}
				// Documentation, not a note inside a body: the length rule
				// reaches a comment heading a file or standing above a
				// declaration, and those are the two this has to judge.
				c.annotates = annotated(root, c.nodes[len(c.nodes)-1], src)
				if c.buried && !c.doc {
					continue
				}
				pieces := split(c.text)
				if len(pieces) == 0 {
					continue
				}
				comments++
				decl := ""
				if c.annotates != nil {
					decl = firstLine(c.annotates.Utf8Text(src))
				}
				for i, sentence := range pieces {
					if err := encoder.Encode(harvested{
						Path:     path,
						Line:     c.line,
						At:       i + 1,
						Of:       len(pieces),
						Decl:     decl,
						Sentence: sentence,
					}); err != nil {
						return err
					}
					rows++
				}
			}
			return nil
		})
	}
	t.Logf("%d sentences from %d documentation comments -> %s", rows, comments, into)
}

// firstLine returns a declaration's signature: what it says before its body.
func firstLine(text string) string {
	if at := strings.IndexByte(text, '\n'); at >= 0 {
		text = text[:at]
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text), "{"))
}
