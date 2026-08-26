package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestLengthDistribution reports how long documentation actually runs in code
// somebody wrote on purpose, which is what a length threshold has to be set
// against.
func TestLengthDistribution(t *testing.T) {
	roots := os.Getenv("SLOPGUARD_CORPUS")
	if roots == "" {
		t.Skip("set SLOPGUARD_CORPUS to a colon-separated list of repositories")
	}
	counts := map[int]int{}
	total := 0
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
				if c.pragma() {
					continue
				}
				if n := sentences(c.text); n > 0 {
					counts[n]++
					total++
				}
			}
			return nil
		})
	}
	var lengths []int
	for n := range counts {
		lengths = append(lengths, n)
	}
	sort.Ints(lengths)
	seen := 0
	for _, n := range lengths {
		seen += counts[n]
		t.Logf("%3d sentences  %6d comments  %6.2f%% at or below", n, counts[n], 100*float64(seen)/float64(total))
	}
	t.Logf("total %d", total)
}
