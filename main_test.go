package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mikluko/slopguard/internal/session"
)

// The fixture is commented-out code rather than a restatement, because these
// tests are about the plumbing around a finding and should exercise the
// configuration that ships. `echo` and the semantic pass are behind
// [rule.Wider] and off by default.
const source = `package p

func double(v int) int {
	// return v * 3
	return v * 2
}
`

// A Write is reviewed whole: every comment in the file is new.
func TestReviewWrite(t *testing.T) {
	skipWithoutRuntime(t)
	t.Setenv(session.MemoryEnv, t.TempDir())
	in := payload{ToolName: "Write"}
	in.ToolInput.FilePath = file(t, "double.go", source)
	in.ToolInput.Content = source
	findings := review(in)
	if len(findings) != 1 {
		t.Fatalf("want one finding, got %d", len(findings))
	}
	if findings[0].Line != 4 {
		t.Fatalf("want the comment on line 4, got line %d", findings[0].Line)
	}
}

// An Edit is reviewed at the text it inserted, in the context of the file it
// landed in.
func TestReviewEdit(t *testing.T) {
	skipWithoutRuntime(t)
	t.Setenv(session.MemoryEnv, t.TempDir())
	in := payload{ToolName: "Edit"}
	in.ToolInput.FilePath = file(t, "double.go", source)
	in.ToolInput.NewString = "\t// return v * 3\n\treturn v * 2"
	findings := review(in)
	if len(findings) != 1 {
		t.Fatalf("want one finding, got %d", len(findings))
	}
	if !strings.Contains(findings[0].Reason, "commented-out code") {
		t.Fatalf("unexpected reason: %s", findings[0].Reason)
	}
}

// An edit is credited with the bytes it changed, not with everything its
// replacement text happens to match. The same three lines appear twice here,
// and only the copy the edit inserted a comment into is this write's.
func TestReviewCreditsOnlyWhatChanged(t *testing.T) {
	skipWithoutRuntime(t)
	t.Setenv(session.MemoryEnv, t.TempDir())

	const both = `package p

func first(v int) int {
	// close the connection
	return v * 2
}

func second(v int) int {
	// close the connection
	return v * 2
}
`
	in := payload{SessionID: "one", ToolName: "Edit"}
	in.ToolInput.FilePath = file(t, "twice.go", both)
	in.ToolInput.OldString = "\treturn v * 2\n}\n\nfunc second"
	in.ToolInput.NewString = "\treturn v * 2\n}\n\nfunc second"

	if findings := review(in); len(findings) != 0 {
		t.Fatalf("an edit that changed nothing claimed %d comments, at line %d",
			len(findings), findings[0].Line)
	}
}

// Everything slopguard has no business judging passes in silence.
func TestReviewFailsOpen(t *testing.T) {
	prose := file(t, "notes.md", "# Notes\n\nno longer true, previously it was.\n")
	sourceFile := file(t, "double.go", source)
	for _, c := range []struct {
		name string
		in   func(*payload)
	}{
		{"another tool", func(in *payload) { in.ToolName = "Bash"; in.ToolInput.FilePath = sourceFile }},
		{"a format with no grammar", func(in *payload) { in.ToolName = "Write"; in.ToolInput.FilePath = prose }},
		{"a file that is gone", func(in *payload) { in.ToolName = "Write"; in.ToolInput.FilePath = sourceFile + ".missing" }},
		{"text that is not in the file", func(in *payload) {
			in.ToolName = "Edit"
			in.ToolInput.FilePath = sourceFile
			in.ToolInput.NewString = "// a line that was never written\n"
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var in payload
			c.in(&in)
			if findings := review(in); len(findings) != 0 {
				t.Fatalf("want silence, got %q", findings[0].Reason)
			}
		})
	}
}

// A comment named once is not named again in the same session, however the
// file moves under it. What the memory is and where it lives is
// [session]'s; what is asserted here is that the command consults it.
func TestReviewSilencesARepeat(t *testing.T) {
	skipWithoutRuntime(t)
	t.Setenv(session.MemoryEnv, t.TempDir())

	in := payload{SessionID: "a-session", ToolName: "Write"}
	in.ToolInput.FilePath = file(t, "double.go", source)
	if findings := review(in); len(findings) != 1 {
		t.Fatalf("want one finding on the first write, got %d", len(findings))
	}
	if findings := review(in); len(findings) != 0 {
		t.Fatalf("want silence on the second, got %q", findings[0].Reason)
	}
}

// The memory is per session and per comment, not per file: another session
// hears it, and so does another comment in the same file.
func TestReviewMemoryIsNarrow(t *testing.T) {
	skipWithoutRuntime(t)
	t.Setenv(session.MemoryEnv, t.TempDir())

	path := file(t, "double.go", source)
	first := payload{SessionID: "a-session", ToolName: "Write"}
	first.ToolInput.FilePath = path
	review(first)

	second := payload{SessionID: "another-session", ToolName: "Write"}
	second.ToolInput.FilePath = path
	if findings := review(second); len(findings) != 1 {
		t.Fatalf("another session should hear it, got %d findings", len(findings))
	}

	moved := strings.Replace(source, "// return v * 3", "// return v * 4", 1)
	if err := os.WriteFile(path, []byte(moved), 0o644); err != nil {
		t.Fatal(err)
	}
	if findings := review(first); len(findings) != 1 {
		t.Fatalf("a different comment should be heard, got %d findings", len(findings))
	}
}

// Nothing is remembered when the store is switched off.
func TestReviewMemoryOff(t *testing.T) {
	skipWithoutRuntime(t)
	t.Setenv(session.MemoryEnv, "")

	in := payload{SessionID: "a-session", ToolName: "Write"}
	in.ToolInput.FilePath = file(t, "double.go", source)
	if findings := review(in); len(findings) != 1 {
		t.Fatalf("want one finding, got %d", len(findings))
	}
	if findings := review(in); len(findings) != 1 {
		t.Fatalf("want the same finding again, got %d", len(findings))
	}
}

func file(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
