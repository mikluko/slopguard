package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mikluko/slopguard/internal/corpus"
)

// stood runs one path through the survivor miner and returns the prose it
// yielded.
func stood(t *testing.T, dir, path string) []string {
	t.Helper()
	store, err := corpus.OpenBlobs(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	rows, err := survived(dir, Repo{Name: "test/repo", License: "MIT"}, path, store)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, row := range rows {
		texts = append(texts, row.Text)
	}
	return texts
}

// churn commits n further edits to path, each one touching the file without
// disturbing the comment at the top of it.
func churn(t *testing.T, dir, path, head string, n int) {
	t.Helper()
	for i := range n {
		commitFile(t, dir, path, fmt.Sprintf("%s\nfunc Edit%d() int { return %d }\n", head, i, i))
	}
}

// A survivor whose annotated code nobody has been back to is not one anybody
// read and left, and [expose] drops it.
//
// This gate defines the negative class alongside `endured`, and nothing called
// [expose] until this test: reverting `seen` to zero failed only the assertion
// that the constant is what the fixtures assume, which proves the literal and
// not the behaviour. The two fixtures differ in one thing, whether a later
// commit touched the annotated lines or only the lines after them.
func TestSeenIsBracketed(t *testing.T) {
	const comment = "// this explains what the total below is counted for\n"

	for _, c := range []struct {
		name    string
		touched bool
		want    bool
	}{
		{"nobody went back to the code", false, false},
		{"somebody edited the code under it", true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := repoAt(t)
			head := "package p\n\n" + comment + body
			commitFile(t, dir, "total.go", head)
			churn(t, dir, "total.go", head, 8)
			if c.touched {
				// The annotated function itself, so LineEdits counts it.
				grown := "package p\n\n" + comment +
					"func Total(items []int) int {\n\tsum := 1\n\tfor _, item := range items {\n\t\tsum += item\n\t}\n\treturn sum\n}\n"
				commitFile(t, dir, "total.go", grown)
			}

			store, err := corpus.OpenBlobs(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			rows, err := survived(dir, Repo{Name: "test/repo", License: "MIT"}, "total.go", store)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) == 0 {
				t.Fatal("the fixture yielded no survivor to expose")
			}
			if got := len(expose(dir, rows)) > 0; got != c.want {
				t.Fatalf("annotated code touched = %v: kept %v, want %v", c.touched, got, c.want)
			}
		})
	}
}

// A comment is a survivor once [endured] later commits have touched its file
// with the comment still in front of whoever wrote them. Seven is not enough
// and eight is, which is the boundary [TestConstantsAreWhatTheFixturesAssume]
// ties to the constant.
//
// The survived side had no test at all before this one. Both gates that define
// the negative class lived in code nothing exercised.
func TestEnduredIsBracketed(t *testing.T) {
	head := "package p\n\n// this explains what the total below is counted for\n" + body

	for _, c := range []struct {
		name   string
		edits  int
		expect bool
	}{
		{"seven later commits is not survival", 7, false},
		{"eight later commits is", 8, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := repoAt(t)
			commitFile(t, dir, "total.go", head)
			churn(t, dir, "total.go", head, c.edits)

			texts := stood(t, dir, "total.go")
			if got := len(texts) > 0; got != c.expect {
				t.Fatalf("%d later commits: harvested %d survivors, want any = %v", c.edits, len(texts), c.expect)
			}
		})
	}
}

// A survivor's comment carries the prose, and the code it annotates is stored
// with the comments blanked out.
//
// Stored raw on this side and blanked on the other, `annotates` was two
// different measurements wearing one name: comment markers appeared in 25.2% of
// survived rows and 4.3% of deleted ones, which is a label anything reading the
// field could separate on.
func TestSurvivedBlanksTheCommentsInItsAnnotatedCode(t *testing.T) {
	head := "package p\n\n// this explains what the total below is counted for\n" +
		"func Total(items []int) int {\n\t// add them up as we go along here\n\tsum := 0\n\tfor _, item := range items {\n\t\tsum += item\n\t}\n\treturn sum\n}\n"

	dir := repoAt(t)
	commitFile(t, dir, "total.go", head)
	churn(t, dir, "total.go", head, 8)

	store, err := corpus.OpenBlobs(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	rows, err := survived(dir, Repo{Name: "test/repo", License: "MIT"}, "total.go", store)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("no survivor to read")
	}
	for _, row := range rows {
		if got := row.Annotates; strings.Contains(got, "//") || strings.Contains(got, "add them up") {
			t.Errorf("annotated code carries a comment: %q", got)
		}
	}
}
