package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mikluko/slopguard/internal/corpus"
)

// The numbers that decide which comments become the corpus, each pinned from
// both sides.
//
// Every fixture here is built at a literal size, and [TestConstantsAreWhatTheFixturesAssume]
// is what ties those literals to the constants. That indirection is the whole
// point of the file. An audit reverted `sweep` to a hundred thousand, `burst` to
// a hundred, `annotatedFloor` to the value an earlier round had already found
// wrong, and `repeats` to ten, all at once, and the suite stayed green — because
// each fixture was sized as an offset from the constant it was meant to hold. A
// file built with `burst + 1` comments trips any value of `burst`, and
// `len(out) == repeats` is true whatever `repeats` is. Written that way a second
// time, three of these four still failed to pin anything.

// TestConstantsAreWhatTheFixturesAssume fails when a constant moves, which is
// what makes the literal sizes below meaningful. Move a constant deliberately
// and this test says which fixtures have to be rebuilt with it.
func TestConstantsAreWhatTheFixturesAssume(t *testing.T) {
	for _, c := range []struct {
		name string
		got  int
		want int
	}{
		{"sweep", sweep, 15},
		{"burst", burst, 3},
		{"annotatedFloor", annotatedFloor, 40},
		{"repeats", repeats, 2},
		{"endured", endured, 8},
		{"seen", seen, 1},
	} {
		if c.got != c.want {
			t.Errorf("%s is %d, and the fixtures in this file are built for %d: "+
				"rebuild them at the new boundary rather than deleting this line", c.name, c.got, c.want)
		}
	}
}

// commitFiles writes several files in one commit, which is what a codemod looks
// like and what [sweep] is counted in.
func commitFiles(t *testing.T, dir string, files map[string]string) commit {
	t.Helper()
	for path, content := range files {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := corpus.Git(dir, "add", path); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := corpus.Git(dir, "commit", "-q", "-m", "touch many"); err != nil {
		t.Fatal(err)
	}
	head, err := sample(dir, 1)
	if err != nil || len(head) != 1 {
		t.Fatalf("reading HEAD: %v", err)
	}
	return head[0]
}

// annotated is one file carrying a comment above code long enough to clear
// [annotatedFloor], and the same file with the comment gone.
func annotated(n int) (before, after string) {
	return fmt.Sprintf("package p\n\n// step %d explains the run below in words\n%s", n, body),
		fmt.Sprintf("package p\n\n%s", body)
}

// Fourteen files touched is a set of judgements; fifteen is a codemod.
func TestSweepIsBracketed(t *testing.T) {
	for _, c := range []struct {
		name  string
		files int
		want  bool
	}{
		{"fourteen files is judgement", 14, true},
		{"fifteen files is a codemod", 15, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := repoAt(t)
			before := map[string]string{}
			after := map[string]string{}
			for i := range c.files {
				path := fmt.Sprintf("pkg%d/total.go", i)
				before[path], after[path] = annotated(i)
			}
			commitFiles(t, dir, before)
			second := commitFiles(t, dir, after)

			texts := harvested(t, dir, second)
			if got := len(texts) > 0; got != c.want {
				t.Fatalf("%d files touched: harvested %d rows, want any = %v", c.files, len(texts), c.want)
			}
		})
	}
}

// Three comments dropped from one file is three decisions; four is a rewrite.
func TestBurstIsBracketed(t *testing.T) {
	for _, c := range []struct {
		name     string
		comments int
		want     bool
	}{
		{"three is judgement", 3, true},
		{"four is a rewrite", 4, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := repoAt(t)
			var before, after strings.Builder
			before.WriteString("package p\n\n")
			after.WriteString("package p\n\n")
			for i := range c.comments {
				fmt.Fprintf(&before, "// step %d does the next part of the work\nfunc Step%d(items []int) int {\n\treturn len(items) + %d\n}\n\n", i, i, i)
				fmt.Fprintf(&after, "func Step%d(items []int) int {\n\treturn len(items) + %d\n}\n\n", i, i)
			}
			commitFile(t, dir, "steps.go", before.String())
			second := commitFile(t, dir, "steps.go", after.String())

			texts := harvested(t, dir, second)
			if got := len(texts) > 0; got != c.want {
				t.Fatalf("%d comments removed: harvested %d rows, want any = %v", c.comments, len(texts), c.want)
			}
		})
	}
}

// Thirty-eight bytes of annotated code is not enough to check survival against;
// forty is.
//
// The declaration is `var V<padding> = 1`, which is nine bytes plus the padding,
// so the two widths below are the floor of forty minus and plus one.
func TestAnnotatedFloorIsBracketed(t *testing.T) {
	code := func(width int) string {
		return fmt.Sprintf("var %s = 1\n", "V"+strings.Repeat("x", width))
	}
	for _, c := range []struct {
		name  string
		width int
		want  bool
	}{
		{"thirty-eight bytes is under the floor", 29, false},
		{"forty bytes clears it", 31, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := repoAt(t)
			commitFile(t, dir, "v.go", "package p\n\n// this names what the value below is for\n"+code(c.width))
			second := commitFile(t, dir, "v.go", "package p\n\n"+code(c.width))

			texts := harvested(t, dir, second)
			if got := len(texts) > 0; got != c.want {
				t.Fatalf("annotated code of width %d: harvested %d rows, want any = %v", c.width, len(texts), c.want)
			}
		})
	}
}

// Five copies of one wording come out as two.
func TestRepeatsIsBracketed(t *testing.T) {
	rows := make([]corpus.Row, 5)
	for i := range rows {
		rows[i] = corpus.Row{Label: corpus.Deleted, Text: "the same wording every time"}
	}
	if got := len(distinct(rows)); got != 2 {
		t.Fatalf("kept %d copies of one wording, want 2", got)
	}
}
