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

// A comment with an identical twin elsewhere in the file cannot be claimed as
// this write's, because the only thing distinguishing the two is position and
// the transition test compares text.
//
// Through [review] rather than [report], which is where the blanking happens.
// Three tests were added with that fix and all three built their findings by
// hand, so deleting the loop left the suite green and restored the false
// certainty it was written to remove.
func TestReviewWillNotClaimACommentWithATwin(t *testing.T) {
	t.Setenv(session.MemoryEnv, t.TempDir())
	const twice = `package p

func a() {
	// return v * 3
	return v * 2
}

func b() {
	// return v * 3
	return v * 2
}
`
	in := payload{ToolName: "Edit"}
	in.ToolInput.FilePath = file(t, "twice.go", twice)
	in.ToolInput.OldString = "\treturn v * 3\n\treturn v * 2\n}\n"
	in.ToolInput.NewString = "\t// return v * 3\n\treturn v * 2\n}\n"

	findings := review(in)
	if len(findings) == 0 {
		t.Fatal("the fixture yielded no finding, so this asserts nothing")
	}
	for _, f := range findings {
		if f.Raw != "" {
			t.Errorf("line %d keeps its claim to certainty though its text occurs twice", f.Line)
		}
		if switched(f, in) {
			t.Errorf("line %d is reported as this write's transition", f.Line)
		}
	}
	// The opening sentence, not the per-line instruction, which asks the agent
	// to consider whether this write commented something out and is present in
	// every form of the nudge.
	if strings.Contains(report("twice.go", findings, in), "This write commented out live code") {
		t.Error("the nudge claims a transition it cannot attribute")
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

// Commenting code out is a thing that happens to a file, not a property of the
// text left behind, and an Edit carries the before. Where the payload shows the
// lines were live a moment ago the tool is not guessing and does not say it is.
//
// This is the one channel that cannot fire on the shapes hand adjudication keeps
// finding: a compiler sketching what it emits and a spec step written as an
// assignment were never live code in this write.
func TestSwitchedReadsTheEditRatherThanTheComment(t *testing.T) {
	const live = "func double(v int) int {\n\treturn v * 3\n}"
	const off = "func double(v int) int {\n\t// return v * 3\n}"

	for _, c := range []struct {
		name string
		in   payload
		want bool
	}{
		{
			name: "the edit turned this line into a comment",
			in:   edit(live, off),
			want: true,
		},
		{
			name: "the line was already a comment before the edit",
			in:   edit("func double(v int) int {\n\t// return v * 3\n\treturn v * 2\n}", off),
			want: false,
		},
		{
			name: "a Write carries no before to read",
			in:   payload{ToolName: "Write"},
			want: false,
		},
		{
			name: "notation that was never code here",
			in:   edit("func lsh(x, s uint) uint {\n\treturn x << s\n}", "// z = x << s\nfunc lsh(x, s uint) uint {\n\treturn x << s\n}"),
			want: false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := rule.Finding{Line: 2, Source: "return v * 3", Raw: "// return v * 3"}
			if c.name == "notation that was never code here" {
				f.Source, f.Raw = "z = x << s", "// z = x << s"
			}
			if got := switched(f, c.in); got != c.want {
				t.Fatalf("switched = %v, want %v", got, c.want)
			}
		})
	}
}

// A confirmed transition is not hedged. Calling a line the tool watched being
// commented out a guess spends the one finding it is certain of to protect the
// ones it is not.
func TestReportDoesNotHedgeWhatItWatchedHappen(t *testing.T) {
	in := edit("func double(v int) int {\n\treturn v * 3\n}", "func double(v int) int {\n\t// return v * 3\n}")
	findings := []rule.Finding{{
		Line: 2, Reason: "commented-out code: delete it, or make it real",
		Source: "return v * 3", Raw: "// return v * 3",
	}}

	repeat = false
	out := report("double.go", findings, in)
	if strings.Contains(out, "heuristic") || strings.Contains(out, "wrong") {
		t.Errorf("hedged a confirmed transition:\n%s", out)
	}
	for _, want := range []string{"commented out live code", "git has it", "double.go:2"} {
		if !strings.Contains(out, want) {
			t.Errorf("the nudge does not carry %q:\n%s", want, out)
		}
	}
}

// A mixed report hedges the findings it is not sure of and marks the ones it is.
//
// Both halves were unpinned: replacing the branch with an unhedged sentence, and
// neutering the marker that says which line is confirmed, each left the suite
// green. The marker is the only thing distinguishing the two kinds of claim in
// one message.
func TestReportMarksTheConfirmedFindingAmongGuesses(t *testing.T) {
	in := edit("\tfoo()\n\tbar()\n", "\t// foo()\n\tbar()\n")
	findings := []rule.Finding{
		{Line: 1, Reason: "commented-out code: delete it, or make it real", Source: "foo()", Raw: "// foo()"},
		{Line: 9, Reason: "commented-out code: delete it, or make it real", Source: "qux()", Raw: "// qux()"},
	}

	repeat = false
	out := report("m.go", findings, in)
	if !strings.Contains(out, "* m.go:1") {
		t.Errorf("the confirmed finding is not marked:\n%s", out)
	}
	if strings.Contains(out, "* m.go:9") {
		t.Errorf("an unconfirmed finding is marked as confirmed:\n%s", out)
	}
	if !strings.Contains(out, "wrong") {
		t.Errorf("the unconfirmed findings are not hedged:\n%s", out)
	}
}

// The nudge states that it is often wrong and never how often. The pooled rate
// is fitted to the sample the exemptions were chosen from, so quoting it hands
// an agent a number the method section retracts.
//
// Asserting the presence of "wrong" does not pin this: the wording it replaced
// contained that word too, and restoring it verbatim passed the suite. Nor does
// a list of the phrasings already used — a first version listed five and passed
// a nudge carrying "four in ten", "a third" and "40 to 75 percent".
//
// So the test is on the shape of a rate rather than on its wording: no digit, no
// percent sign, and none of the words English counts small fractions with. A
// finding's line number is the one number the nudge may carry, so it is stripped
// before the check.
func TestReportQuotesNoRate(t *testing.T) {
	findings := []rule.Finding{{Line: 42, Reason: "commented-out code: delete it, or make it real"}}
	fractions := []string{
		"half", "third", "quarter", "fifth", "sixth", "seventh", "eighth", "ninth", "tenth",
		"percent", "%", "most of the time", "of the time",
	}
	for _, certain := range []bool{false, true} {
		for _, repeated := range []bool{false, true} {
			repeat = repeated
			out := report("store.go", findings, edit("x", "y"))
			if certain {
				out = report("store.go", findings, payload{})
			}
			bare := strings.ReplaceAll(out, "store.go:42", "")
			for _, digit := range "0123456789" {
				if strings.ContainsRune(bare, digit) {
					t.Errorf("the nudge carries a digit:\n%s", out)
					break
				}
			}
			for _, word := range fractions {
				if strings.Contains(strings.ToLower(bare), word) {
					t.Errorf("the nudge quotes a rate (%q):\n%s", word, out)
				}
			}
		}
	}
	repeat = false
}

// edit builds the payload an Edit hands the hook.
func edit(old, new string) payload {
	in := payload{ToolName: "Edit"}
	in.ToolInput.OldString = old
	in.ToolInput.NewString = new
	return in
}

// The edit that deleted the code has to be the edit that wrote the comment.
//
// Each of these fails a mutation the suite used to pass: dropping the
// absent-from-the-replacement test, dropping the still-present-as-a-comment
// test, and pooling every edit's before-text. The first version of this fix
// removed the pooling and kept the disjunction, which is the same defect wearing
// a loop — one edit deleting `foo()` in one function still vouched for another
// edit writing `// foo()` in a second, through the one message that carries no
// hedge.
func TestSwitchedPairsTheEditThatDeletedWithTheEditThatCommented(t *testing.T) {
	multi := func(pairs ...[2]string) payload {
		in := payload{ToolName: "MultiEdit"}
		for _, p := range pairs {
			in.ToolInput.Edits = append(in.ToolInput.Edits, struct {
				OldString string `json:"old_string"`
				NewString string `json:"new_string"`
			}{p[0], p[1]})
		}
		return in
	}

	for _, c := range []struct {
		name string
		in   payload
		want bool
	}{
		{
			name: "one edit deleted it and the same edit commented it",
			in:   multi([2]string{"\tfoo()\n\tbar()\n", "\t// foo()\n\tbar()\n"}),
			want: true,
		},
		{
			// The shape the previous fix still reported: edit one deletes a
			// live call, edit two writes the comment somewhere else entirely.
			name: "one edit deleted it and a different edit wrote the comment",
			in: multi(
				[2]string{"\tfoo()\n\tbar()\n", "\tbar()\n"},
				[2]string{"\tqux()\n", "\tqux()\n\t// foo()\n"},
			),
			want: false,
		},
		{
			name: "the line is still live in the replacement",
			in:   edit("\tfoo()\n\tbar()\n", "\t// foo()\n\tfoo()\n\tbar()\n"),
			want: false,
		},
		{
			// The deleting edit keeps the text inside a longer live line, which
			// a substring test on the stripped form accepts.
			name: "the deleting edit refactored the call rather than commenting it",
			in: multi(
				[2]string{"\tfoo()\n\tdefer foo()\n", "\tdefer foo()\n"},
				[2]string{"\tqux()\n", "\tqux()\n\t// foo()\n"},
			),
			want: false,
		},
		{
			// One edit, and the comment is older than it: the edit rewrote
			// `foo()` into `defer foo()` and carried the comment through
			// untouched. Only the raw text tells this from a transition.
			name: "the edit carried an older comment through unchanged",
			in: edit(
				"\ta()\n\t// foo()\n\tfoo()\n\tbar()\n",
				"\tz()\n\t// foo()\n\tdefer foo()\n\tbar()\n",
			),
			want: false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := rule.Finding{Line: 1, Source: "foo()", Raw: "// foo()"}
			if got := switched(f, c.in); got != c.want {
				t.Fatalf("switched = %v, want %v", got, c.want)
			}
		})
	}
}

// The text handed to the agent is the tool's whole interface to it, and nothing
// asserted anything about it: the payload could be rewritten end to end with the
// suite green. What it must carry follows from the measurement rather than from
// taste. Much of what the tool says is wrong, so a nudge that does not say so is
// asking an agent to act on a guess, and one that does not offer a free way to
// decline makes deleting a correct comment the cheapest way out.
//
// The calibration is asserted as a word rather than a rate on purpose: the
// pooled figure is fitted to the sample the exemptions were chosen from, so the
// nudge must not quote it, and a test demanding a number would put it back.
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
				"wrong",
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
