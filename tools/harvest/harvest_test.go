package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/mikluko/slopguard/internal/corpus"
)

// Every knob below shaped a published corpus while nothing held it. Three review
// rounds found four of them wrong, each time by reading the data rather than by
// running the suite.

// A repository's survived rows are capped, and taken across the whole list
// rather than off its front. `len/keep` is one for any repository yielding
// between one and two times the cap, which took the first eight hundred in file
// order and left seven parsable files contributing nothing.
func TestBalanceSpreadsAcrossTheList(t *testing.T) {
	var rows []corpus.Row
	for i := range 150 {
		rows = append(rows, corpus.Row{Label: corpus.Survived, Text: "survived " + strconv.Itoa(i)})
	}
	for i := range 40 {
		rows = append(rows, corpus.Row{Label: corpus.Deleted, Text: "deleted " + strconv.Itoa(i)})
	}

	out := balance(rows, 100)
	var survived, deleted []corpus.Row
	for _, row := range out {
		if row.Label == corpus.Survived {
			survived = append(survived, row)
			continue
		}
		deleted = append(deleted, row)
	}
	if len(survived) != 100 {
		t.Errorf("kept %d survived rows, want the cap of 100", len(survived))
	}
	// The positive cap is a quarter of the negative one.
	if len(deleted) != 25 {
		t.Errorf("kept %d deleted rows, want 25", len(deleted))
	}
	// Off the front, the last survived row taken would be "survived 99".
	if survived[len(survived)-1].Text == "survived 99" {
		t.Error("the cap took the first hundred in order rather than a spread")
	}
}

// Nothing is dropped when a repository is under the cap.
func TestBalanceKeepsEverythingUnderTheCap(t *testing.T) {
	rows := []corpus.Row{
		{Label: corpus.Survived, Text: "one"},
		{Label: corpus.Deleted, Text: "two"},
	}
	if out := balance(rows, 100); len(out) != 2 {
		t.Errorf("kept %d of 2 rows under the cap", len(out))
	}
}

// One wording may repeat twice per repository. A comment written once and copied
// into every file is one person's decision arriving with the weight of many:
// 9.3% of a shipped corpus was a text repeated three or more times, one line of
// it 308 rows.
func TestDistinctCapsARepeatedWording(t *testing.T) {
	var rows []corpus.Row
	for range 10 {
		rows = append(rows, corpus.Row{Label: corpus.Survived, Text: "use clap_builder as clap"})
	}
	rows = append(rows, corpus.Row{Label: corpus.Survived, Text: "something else entirely"})

	out := distinct(rows)
	if len(out) != repeats+1 {
		t.Errorf("kept %d rows, want %d repeats plus the singleton", len(out), repeats)
	}
}

// The two labels are counted apart, so a wording appearing on both sides is not
// starved on the second one.
func TestDistinctCountsTheLabelsApart(t *testing.T) {
	rows := []corpus.Row{
		{Label: corpus.Survived, Text: "the caller must hold the lock"},
		{Label: corpus.Survived, Text: "the caller must hold the lock"},
		{Label: corpus.Deleted, Text: "the caller must hold the lock"},
	}
	if out := distinct(rows); len(out) != 3 {
		t.Errorf("kept %d of 3 rows, want the deleted one counted apart", len(out))
	}
}

// Path segments, not substrings. Matching substrings excluded tokio's whole
// `tests-build/` tree on the strength of `build/`, and `packages` excluded
// vuejs/core's entire source, taking TypeScript from 799 rows to one.
func TestSkipMatchesSegments(t *testing.T) {
	for _, path := range []string{
		"vendor/github.com/x/y.go",
		"node_modules/left-pad/index.js",
		"src/testdata/fixture.py",
		"pkg/generated/api.go",
		"api/service.pb.go",
	} {
		if !skip(path) {
			t.Errorf("skip(%q) = false, want it excluded", path)
		}
	}
	for _, path := range []string{
		"tests-build/ui.rs",
		"packages/runtime-core/src/index.ts",
		"internal/rebuilder/plan.go",
		"src/vendored_names.py",
	} {
		if skip(path) {
			t.Errorf("skip(%q) = true, want it read", path)
		}
	}
}

// A stale clone still harvests, because every row is pinned to a commit. A fetch
// failure used to skip the repository outright, which lost ten of twenty-four on
// a machine whose git rewrites https to ssh, and every Rust, Java and Ruby row
// with them.
func TestFetchKeepsAStaleClone(t *testing.T) {
	dir := repoAt(t)
	commitFile(t, dir, "total.go", "package p\n\n// the bound is deliberately eight\n"+body)
	// No remote at all, so the fetch cannot succeed.
	if err := fetch(Repo{Name: "test/repo"}, dir, 0); err != nil {
		t.Errorf("a clone with no reachable remote was refused: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Errorf("the clone went away: %v", err)
	}
}

// The exposure window opens at the commit that wrote the comment and counts what
// touched the code below it afterwards. Counted over all history it was mostly
// churn from before the comment existed: 98.4% of survivors had nothing touch
// their line after their own text was written.
func TestLineEditsCountsWhatCameAfter(t *testing.T) {
	dir := repoAt(t)
	const shape = "package p\n\n// the retry bound is deliberately three\nfunc run(n int) int {\n\treturn n + %d\n}\n"

	written := commitFile(t, dir, "run.go", "package p\n\n// the retry bound is deliberately three\nfunc run(n int) int {\n\treturn n + 0\n}\n")
	for i := 1; i <= 3; i++ {
		commitFile(t, dir, "run.go", sprintf(shape, i))
	}

	// Lines 4 to 6 are the function; the comment on line 3 is left alone.
	edits, err := corpus.LineEdits(dir, written.sha, "HEAD", "run.go", 4, 6)
	if err != nil {
		t.Fatal(err)
	}
	if edits != 3 {
		t.Errorf("counted %d edits after the comment was written, want 3", edits)
	}

	// The comment's own line had nothing happen to it after it was written,
	// which is the number the old instrument reported as exposure.
	quiet, err := corpus.LineEdits(dir, written.sha, "HEAD", "run.go", 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	if quiet != 0 {
		t.Errorf("the comment's own line moved %d times, want 0", quiet)
	}
}

// sprintf keeps the format call out of the table above.
func sprintf(format string, a ...any) string {
	return fmt.Sprintf(format, a...)
}
