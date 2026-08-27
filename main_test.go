package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mikluko/slopguard/internal/rule"
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

// The text handed to the agent is the tool's whole interface to it, and nothing
// asserted anything about it: the payload could be rewritten end to end with the
// suite green. What it must carry follows from the measurement rather than from
// taste. About half of these findings are wrong, so a nudge that does not say so
// is asking an agent to act on a coin flip, and one that does not offer a free
// way to decline makes deleting a correct comment the cheapest way out.
func TestReportCarriesWhatMakesItSafeToActOn(t *testing.T) {
	findings := []rule.Finding{{Line: 42, Reason: "commented-out code: delete it, or make it real"}}

	for _, c := range []struct {
		name   string
		repeat bool
	}{
		{"the long form", false},
		{"the short form", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			repeat = c.repeat
			t.Cleanup(func() { repeat = false })
			out := report("store.go", findings, payload{CWD: t.TempDir()})

			for _, want := range []string{
				// The calibration, a way to decline that costs nothing, and a
				// location the agent's own tools can open.
				"half",
				"leave it",
				"store.go:42",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("the nudge does not carry %q:\n%s", want, out)
				}
			}
			// The three remedies this used to name belong to classes that are
			// off by default, so on the one class that ships they were advice
			// for a defect the finding is not.
			for _, gone := range []string{"commit message", "as a test", "belong elsewhere"} {
				if strings.Contains(out, gone) {
					t.Errorf("the nudge still offers %q, which no shipped class earns:\n%s", gone, out)
				}
			}
		})
	}
}

// The nudge does not cite a file for its authority. `anchor` named AGENTS.md or
// CLAUDE.md on nothing but the file existing in the working directory, so the
// tool attributed its own rules to a document it had never read, in the half of
// cases where it was wrong as much as in the half where it was right.
func TestReportClaimsNoAuthorityItDoesNotHave(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# unrelated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repeat = false
	out := report("store.go", []rule.Finding{{Line: 1, Reason: "commented-out code"}}, payload{CWD: dir})
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		if strings.Contains(out, name) {
			t.Errorf("the nudge cites %s as the source of its rules:\n%s", name, out)
		}
	}
}

// The human gets the lines, not only how many there were. They are the party
// able to tell a true finding from a false one, and a bare count gave them
// nothing to check it against.
func TestSummaryNamesTheLines(t *testing.T) {
	out := summary("store.go", []rule.Finding{{Line: 42}, {Line: 88}})
	for _, want := range []string{"store.go:42", "store.go:88", "2 comments"} {
		if !strings.Contains(out, want) {
			t.Errorf("the human's line does not carry %q: %s", want, out)
		}
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
