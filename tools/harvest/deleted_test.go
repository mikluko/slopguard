package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mikluko/slopguard/internal/corpus"
)

// A repository built here stands in for a public one: the harvest reads nothing
// but git, so a handful of commits exercises the same path a clone does.
func repoAt(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "harvest@example.test"},
		{"config", "user.name", "harvest"},
	} {
		if _, err := corpus.Git(dir, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	return dir
}

// commitFile writes path with content and commits it, returning the commit.
func commitFile(t *testing.T, dir, path, content string) commit {
	t.Helper()
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
	if _, err := corpus.Git(dir, "commit", "-q", "-m", "write "+path); err != nil {
		t.Fatal(err)
	}
	head, err := sample(dir, 1)
	if err != nil || len(head) != 1 {
		t.Fatalf("reading HEAD: %v", err)
	}
	return head[0]
}

// body is the function the comments in these tests sit above. It is long enough
// that its text clears [annotatedFloor], which is what lets survival be checked.
const body = `func Total(items []int) int {
	sum := 0
	for _, item := range items {
		sum += item
	}
	return sum
}
`

// harvested runs one commit through the miner and returns the prose it yielded.
func harvested(t *testing.T, dir string, c commit) []string {
	t.Helper()
	store, err := corpus.OpenBlobs(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	rows, err := deleted(dir, Repo{Name: "test/repo", License: "MIT"}, c, store)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, row := range rows {
		texts = append(texts, row.Text)
	}
	return texts
}

// A comment removed while the code under it stays is the label this whole
// harvest exists to collect.
func TestDeletedIsHarvested(t *testing.T) {
	dir := repoAt(t)
	commitFile(t, dir, "total.go", "package p\n\n// loop over the items and add them up\n"+body)
	second := commitFile(t, dir, "total.go", "package p\n\n"+body)

	texts := harvested(t, dir, second)
	if len(texts) != 1 {
		t.Fatalf("got %d rows, want 1: %q", len(texts), texts)
	}
	if !strings.Contains(texts[0], "loop over the items") {
		t.Errorf("harvested %q, want the deleted comment", texts[0])
	}
}

// A comment rewritten in place was cared about, not judged. Measured on logrus,
// this case was most of the harvest before it was excluded, and the prose it
// contributed was mostly prose worth keeping.
func TestRewordedIsNotHarvested(t *testing.T) {
	dir := repoAt(t)
	commitFile(t, dir, "total.go", "package p\n\n// loop over the items and add them up\n"+body)
	second := commitFile(t, dir, "total.go",
		"package p\n\n// Total reports the sum of items, and zero for an empty slice.\n"+body)

	if texts := harvested(t, dir, second); len(texts) != 0 {
		t.Errorf("a reworded comment was harvested as deleted: %q", texts)
	}
}

// A comment that went with its function was swept rather than judged.
func TestSweptIsNotHarvested(t *testing.T) {
	dir := repoAt(t)
	commitFile(t, dir, "total.go", "package p\n\n// loop over the items and add them up\n"+body)
	second := commitFile(t, dir, "total.go", "package p\n\nfunc Other() string { return \"nothing to add up here\" }\n")

	if texts := harvested(t, dir, second); len(texts) != 0 {
		t.Errorf("a comment deleted with its code was harvested: %q", texts)
	}
}

// A rewording where the annotated code grew around it. The survival test
// accepts a substring and the rewrite test demanded exact node text, so the two
// disagreed in one direction: `Contains` passed on the old code while the
// lookup missed on the new, and a comment somebody rewrote came out as deleted.
//
// Python because the shape needs code that grows without a closing delimiter in
// the way, which is what an import list is.
func TestRewordedOverGrownCodeIsNotHarvested(t *testing.T) {
	dir := repoAt(t)
	commitFile(t, dir, "compat.py",
		"# typing.Concatenate and typing.ParamSpec require Python 3.10\nfrom typing_extensions import Concatenate, ParamSpec\n")
	second := commitFile(t, dir, "compat.py",
		"# typing.Concatenate and typing.ParamSpec require Python 3.10\n"+
			"# typing.Self requires Python 3.11\n"+
			"from typing_extensions import Concatenate, ParamSpec, Self\n")

	if texts := harvested(t, dir, second); len(texts) != 0 {
		t.Errorf("a rewording over grown code was harvested as deleted: %q", texts)
	}
}

// Code the commit commented out has not survived it. A block comment keeps the
// code's bytes exactly, so testing against the whole child file finds them
// inside the new comment and reports the code as still standing.
func TestCommentedOutCodeHasNotSurvived(t *testing.T) {
	dir := repoAt(t)
	commitFile(t, dir, "total.go", "package p\n\n// loop over the items and add them up\n"+body)
	second := commitFile(t, dir, "total.go", "package p\n\n/*\n"+body+"*/\n")

	if texts := harvested(t, dir, second); len(texts) != 0 {
		t.Errorf("a comment whose code was commented out was harvested: %q", texts)
	}
}

// A recurring annotated node. Asking whether the code is present in the child
// answers yes on the other instance of it, so a comment whose own subject was
// deleted reads as one whose subject survived. Counting occurrences separates
// them.
func TestRecurringCodeIsNotMistakenForSurvival(t *testing.T) {
	dir := repoAt(t)
	const step = "\tif err := reconcile(ctx, name, opts); err != nil {\n\t\treturn err\n\t}\n"
	commitFile(t, dir, "run.go", "package p\n\nfunc run() error {\n"+
		"\t// the first pass is deliberately separate from the second\n"+step+step+"\treturn nil\n}\n")
	// The annotated statement goes with its comment; its twin below stays.
	second := commitFile(t, dir, "run.go", "package p\n\nfunc run() error {\n"+step+"\treturn nil\n}\n")

	if texts := harvested(t, dir, second); len(texts) != 0 {
		t.Errorf("a comment whose own statement went was harvested on its twin: %q", texts)
	}
}

// One commit stripping every comment from a file is a rewrite, and none of its
// comments was decided on individually.
func TestBurstIsNotHarvested(t *testing.T) {
	dir := repoAt(t)
	var before, after strings.Builder
	before.WriteString("package p\n\n")
	after.WriteString("package p\n\n")
	for i := range burst + 2 {
		fmt.Fprintf(&before, "// step %d does the next part of the work\nfunc Step%d(items []int) int {\n\treturn len(items) + %d\n}\n\n", i, i, i)
		fmt.Fprintf(&after, "func Step%d(items []int) int {\n\treturn len(items) + %d\n}\n\n", i, i)
	}
	commitFile(t, dir, "steps.go", before.String())
	second := commitFile(t, dir, "steps.go", after.String())

	if texts := harvested(t, dir, second); len(texts) != 0 {
		t.Errorf("a file-wide comment strip was harvested: %q", texts)
	}
}
