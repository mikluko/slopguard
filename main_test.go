package main

import (
	"encoding/json"
	"fmt"
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
// The tests added with that fix all built their findings by hand and called
// [report] directly, so deleting the loop left the suite green and restored the
// false certainty it was written to remove.
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

// Twins are twins whatever their indentation, and an edit that no longer stands
// in the file cannot vouch for anything.
//
// Two holes, both found by reading the code rather than by running it. The twin
// guard counted the raw text literally for one commit while `moved` compared it
// flattened, so a pair differing only in indentation was invisible to the guard
// and identical to `moved`. And neither test noticed a MultiEdit that writes a
// copy of a comment and then deletes it: gone from the file, so no twin; still
// in the payload, so `moved` confirms against it.
func TestReviewWillNotClaimOnEvidenceTheFileNoLongerHolds(t *testing.T) {
	const src = `package p

func a() {
	if cond {
		// deleteTemp(path)
		// closeHandle(h)
	}
}

func b() {
	// deleteTemp(path)
	// closeHandle(h)
}
`
	// The same file with b()'s copy gone, which is what edit three below leaves
	// behind: one flattened occurrence, so the twin guard is silent and only the
	// survival test stands between the write and the claim.
	const undone = `package p

func a() {
	if cond {
		// deleteTemp(path)
		// closeHandle(h)
	}
}

func b() {
	done()
}
`
	for _, c := range []struct {
		name  string
		src   string
		edits [][2]string
	}{
		{
			// The same run at two indentations. Literal counting sees no twin.
			name: "a twin at another indentation is still a twin",
			src:  src,
			edits: [][2]string{
				{"\tdeleteTemp(path)\n\tcloseHandle(h)\n", "\t// deleteTemp(path)\n\t// closeHandle(h)\n"},
			},
		},
		{
			// Edit one writes a copy, edit three deletes it. The copy vouches
			// for a()'s pre-existing comment and is gone before the hook reads
			// the file.
			name: "the edit that would vouch was undone by a later one",
			src:  undone,
			edits: [][2]string{
				{"\tdeleteTemp(path)\n\tcloseHandle(h)\n", "\t// deleteTemp(path)\n\t// closeHandle(h)\n"},
				{"\t// deleteTemp(path)\n\t// closeHandle(h)\n", "\tif cond {\n\t\t// deleteTemp(path)\n\t\t// closeHandle(h)\n\t}\n"},
				{"\t// deleteTemp(path)\n\t// closeHandle(h)\n", "\tdone()\n"},
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(session.MemoryEnv, t.TempDir())
			in := payload{ToolName: "MultiEdit"}
			in.ToolInput.FilePath = file(t, "twins.go", c.src)
			for _, e := range c.edits {
				in.ToolInput.Edits = append(in.ToolInput.Edits, struct {
					OldString string `json:"old_string"`
					NewString string `json:"new_string"`
				}{e[0], e[1]})
			}
			findings := review(in)
			if len(findings) == 0 {
				t.Fatal("the fixture yielded no finding, so this asserts nothing")
			}
			for _, f := range findings {
				if switched(f, in) {
					t.Errorf("line %d is claimed as this write's on evidence the file does not hold", f.Line)
				}
			}
		})
	}
}

// A comment with no twin keeps its claim, which is the other half of the test
// above and the half that was missing.
//
// Blanking every finding's Raw unconditionally passed the whole suite: the twin
// test only proved the loop exists, not that it is narrow, and every other test
// touching the confirmed branch builds its findings by hand and never reaches
// [review]. That mutation would leave the tool unable to say anything with
// certainty from a real hook run, silently.
func TestReviewKeepsTheClaimOnACommentWithNoTwin(t *testing.T) {
	t.Setenv(session.MemoryEnv, t.TempDir())
	const once = `package p

func a() {
	// return v * 3
	return v * 2
}
`
	in := payload{ToolName: "Edit"}
	in.ToolInput.FilePath = file(t, "once.go", once)
	in.ToolInput.OldString = "\treturn v * 3\n\treturn v * 2\n"
	in.ToolInput.NewString = "\t// return v * 3\n\treturn v * 2\n"

	findings := review(in)
	if len(findings) != 1 {
		t.Fatalf("want one finding, got %d", len(findings))
	}
	if findings[0].Raw == "" {
		t.Fatal("a comment with no twin lost its claim to certainty")
	}
	if !switched(findings[0], in) {
		t.Fatal("the transition this write made is not reported as one")
	}
	if !strings.Contains(report("once.go", findings, in), "This write commented out live code") {
		t.Error("the nudge hedges a transition it watched happen")
	}
}

// The row in the table above is built by hand, so something has to check that a
// real Rust doc comment has the shape it assumes.
//
// tree-sitter-rust ends a `///` node at column zero of the row below — the same
// quirk that once made every line of a doc example its own finding — so `Raw`
// ends in a newline and its flattened split carries a trailing empty string.
// Round seventeen added a test for the blank line without checking that Rust
// still produces one, which would have made both vacuous together.
func TestARustDocCommentStillEndsOnTheRowBelow(t *testing.T) {
	t.Setenv(session.MemoryEnv, t.TempDir())
	const src = `fn a() {
    /// delete_temp(path);
    done();
}
`
	in := payload{ToolName: "Edit"}
	in.ToolInput.FilePath = file(t, "a.rs", src)
	in.ToolInput.OldString = "    delete_temp(path);\n    done();\n"
	in.ToolInput.NewString = "    /// delete_temp(path);\n    done();\n"

	findings := review(in)
	if len(findings) != 1 {
		t.Fatalf("want one finding, got %d", len(findings))
	}
	if !strings.HasSuffix(findings[0].Raw, "\n") {
		t.Fatalf("the grammar no longer ends the node on the row below, so the blank-line row above pins nothing: %q", findings[0].Raw)
	}
	if !switched(findings[0], in) {
		t.Fatal("a Rust doc comment this write made lost its claim to certainty")
	}
}

// Guards that hold the tier up and that nothing reached.
//
// Each was free: revert it and the whole suite stayed green, while a real
// payload changed its answer. Two grant a false transition and one costs a true
// one, so they fail in both directions.
func TestTheGuardsUnderTheCertainTier(t *testing.T) {
	t.Run("a pure insertion counts as a replacement", func(t *testing.T) {
		// [attributable] counts replacements that say something, and an
		// insertion says something with an empty before. Counting only the
		// before makes a two-replacement MultiEdit look like one and hands it
		// the unhedged sentence — the shape the tier exists to refuse.
		t.Setenv(session.MemoryEnv, t.TempDir())
		in := payload{ToolName: "MultiEdit"}
		in.ToolInput.FilePath = file(t, "a.go", "package p\n\nfunc a() {\n\tstart()\n\t// deleteTemp(path)\n\tdone()\n}\n")
		for _, e := range [][2]string{
			{"", "\tstart()\n"},
			{"\tdeleteTemp(path)\n\tdone()\n", "\t// deleteTemp(path)\n\tdone()\n"},
		} {
			in.ToolInput.Edits = append(in.ToolInput.Edits, struct {
				OldString string `json:"old_string"`
				NewString string `json:"new_string"`
			}{e[0], e[1]})
		}
		if attributable(in) {
			t.Error("a write carrying an insertion and a replacement passes as one replacement")
		}
		findings := review(in)
		if len(findings) == 0 {
			t.Fatal("the fixture yielded no finding, so this asserts nothing")
		}
		if strings.Contains(report("a.go", findings, in), "This write commented out live code") {
			t.Error("a two-replacement write reached the unhedged sentence")
		}
	})

	// [holds] steps one byte past a match that began mid-line, because the next
	// occurrence can start inside the one just rejected: a run of repeated
	// lines overlaps itself. Striding the run's whole length skips that, and a
	// true transition would lose the tier. Only the function reaches this — end
	// to end, the twin guard refuses the repeated run before `holds` is asked.
	for _, c := range []struct {
		name, text, run string
		want            bool
	}{
		{
			name: "the run begins the text",
			text: "// a\n// b\n",
			run:  "// a\n// b",
			want: true,
		},
		{
			name: "the run begins a later line",
			text: "x()\n// a\n// b\n",
			run:  "// a\n// b",
			want: true,
		},
		{
			name: "the only occurrence begins mid-line",
			text: "qux() // a\n// b\n",
			run:  "// a\n// b",
			want: false,
		},
		{
			name: "a mid-line occurrence overlaps one that begins a line",
			text: "qux() // a\n// a\n// a\n",
			run:  "// a\n// a",
			want: true,
		},
	} {
		t.Run("holds: "+c.name, func(t *testing.T) {
			if got := holds(c.text, c.run); got != c.want {
				t.Fatalf("holds(%q, %q) = %v, want %v", c.text, c.run, got, c.want)
			}
		})
	}

	t.Run("every quoted line is too short to be evidence", func(t *testing.T) {
		// A block comment replaced by two marked delimiters quotes `/*` and
		// `*/` and nothing else. Both are under three characters, so the loop
		// skips them and vouches for nothing; returning true regardless would
		// make a run of pure punctuation a confirmed transition.
		f := rule.Finding{Line: 1, Source: "/*\n*/", Raw: "// /*\n// */"}
		if switched(f, edit("\t/*\n\t*/\n", "\t// /*\n\t// */\n")) {
			t.Error("a run quoting only delimiters is claimed as a transition")
		}
	})
}

// A run written beside its own likeness has a twin, and the guard has to see it.
//
// The write puts the same comment at two indentations; `comment.adjacent` needs
// equal columns, so the run reported is the second written line together with a
// pre-existing third, and [flat] removes exactly the indentation that told them
// apart. The twin guard is the backstop, and it counted with [strings.Count],
// which skips overlapping matches: three identical adjacent lines hold two
// copies of any two-line run and it answered one. So the guard reported no twin
// in the one case where the run's lines are interchangeable.
func TestTheTwinGuardCountsOverlappingCopies(t *testing.T) {
	t.Setenv(session.MemoryEnv, t.TempDir())
	const src = `package p

func a() {
		// deleteTemp(path)
	// deleteTemp(path)
	// deleteTemp(path)
}
`
	in := payload{ToolName: "Edit"}
	in.ToolInput.FilePath = file(t, "a.go", src)
	in.ToolInput.OldString = "\tdeleteTemp(path)\n"
	in.ToolInput.NewString = "\t\t// deleteTemp(path)\n\t// deleteTemp(path)\n"

	findings := review(in)
	if len(findings) == 0 {
		t.Fatal("the fixture yielded no finding, so this asserts nothing")
	}
	for _, f := range findings {
		if !strings.Contains(f.Raw, "\n") {
			continue
		}
		// The run reaches the third line, which the write never wrote.
		if switched(f, in) {
			t.Errorf("line %d claims a run whose last line predates the write: %q", f.Line, f.Raw)
		}
	}
	if strings.Contains(report("a.go", findings, in), "This write commented out live code") {
		t.Error("the nudge claims a transition covering a line the write did not author")
	}
}

// The sentence says the write commented out live code, and it has to have been
// code.
//
// Doubling a marker satisfies every other condition: the write authored the
// line, the line was in the replaced text and is not in the inserted text, and
// tree-sitter emits `comment` as a named node so `// // note` parses as legal
// Go like anything else. Nothing was commented out but a comment. On an
// enumeration of Go payloads this was the most common false claim the tool
// made, and it was the second clause of the tier's sentence rather than the
// authorship clause every earlier round had been about.
func TestTheCertainTierNeedsALineThatWasCode(t *testing.T) {
	for _, c := range []struct {
		name string
		// Empty means Go, which most rows here are.
		file     string
		src      string
		was, now string
		want     bool
	}{
		{
			name: "the write doubled the marker on a comment",
			src:  "package p\n\nfunc a() {\n\tfoo()\n\t// // deleteTemp(path)\n}\n",
			was:  "\tfoo()\n\t// deleteTemp(path)\n",
			now:  "\tfoo()\n\t// // deleteTemp(path)\n",
			want: false,
		},
		{
			name: "the write doubled the marker on prose",
			src:  "package p\n\nfunc a() {\n\tfoo()\n\t// // the caller holds the lock\n}\n",
			was:  "\tfoo()\n\t// the caller holds the lock\n",
			now:  "\tfoo()\n\t// // the caller holds the lock\n",
			want: false,
		},
		{
			// The run mixes a line that was code with one that was a comment,
			// which is what commenting out a block containing a note looks
			// like. One live line is enough, and the sentence is true of it.
			name: "the run holds a line that was code and one that was not",
			src:  "package p\n\nfunc a() {\n\t// deleteTemp(path)\n\t// // the caller holds the lock\n}\n",
			was:  "\tdeleteTemp(path)\n\t// the caller holds the lock\n",
			now:  "\t// deleteTemp(path)\n\t// // the caller holds the lock\n",
			want: true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(session.MemoryEnv, t.TempDir())
			name := c.file
			if name == "" {
				name = "a.go"
			}
			in := payload{ToolName: "Edit"}
			in.ToolInput.FilePath = file(t, name, c.src)
			in.ToolInput.OldString = c.was
			in.ToolInput.NewString = c.now

			findings := review(in)
			if len(findings) == 0 {
				t.Fatal("the fixture yielded no finding, so this asserts nothing")
			}
			got := strings.Contains(report("a.go", findings, in), "This write commented out live code")
			if got != c.want {
				t.Fatalf("unhedged = %v, want %v\n%s", got, c.want, report("a.go", findings, in))
			}
		})
	}
}

// The log has to answer how often the unhedged sentence fires.
//
// It is the tool's only claim made without hedging, and until this test
// nothing wrote down whether it happened — so every argument about whether the
// tier is worth its risk was an argument about a number nobody had. `record`
// itself had no test, so the log's shape was free to change unnoticed too.
func TestTheLogSaysWhetherTheFindingWasConfirmed(t *testing.T) {
	for _, c := range []struct {
		name string
		// The file as the write leaves it, since the hook reads it from disk.
		src      string
		was, now string
		want     bool
	}{
		{
			name: "the write commented the line out",
			src:  "package p\n\nfunc a() {\n\t// deleteTemp(path)\n\tdone()\n}\n",
			was:  "\tdeleteTemp(path)\n\tdone()\n",
			now:  "\t// deleteTemp(path)\n\tdone()\n",
			want: true,
		},
		{
			// An older comment reindented into a new scope by the write that
			// deleted the lines it quotes. The write does touch the comment, so
			// the finding is attributed to it; it did not author it, so the log
			// must not say the tool watched this happen.
			name: "the write only reindented a comment it did not author",
			src:  "package p\n\nfunc a() {\n\tif cond {\n\t\t// deleteTemp(path)\n\t\t// closeHandle(h)\n\t}\n}\n",
			was:  "\t// deleteTemp(path)\n\t// closeHandle(h)\n\tdeleteTemp(path)\n\tcloseHandle(h)\n",
			now:  "\tif cond {\n\t\t// deleteTemp(path)\n\t\t// closeHandle(h)\n\t}\n",
			want: false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(session.MemoryEnv, t.TempDir())
			log := filepath.Join(t.TempDir(), "findings.jsonl")
			t.Setenv(logEnv, log)

			in := payload{ToolName: "Edit"}
			in.ToolInput.FilePath = file(t, "once.go", c.src)
			in.ToolInput.OldString = c.was
			in.ToolInput.NewString = c.now

			findings := review(in)
			if len(findings) != 1 {
				t.Fatalf("want one finding, got %d", len(findings))
			}
			record(in.ToolInput.FilePath, findings, in)

			raw, err := os.ReadFile(log)
			if err != nil {
				t.Fatalf("the log was not written: %v", err)
			}
			var row struct {
				Line      int    `json:"line"`
				Class     string `json:"class"`
				Confirmed bool   `json:"confirmed"`
			}
			if err := json.Unmarshal(raw, &row); err != nil {
				t.Fatalf("the log row does not decode: %v\n%s", err, raw)
			}
			if row.Line == 0 || row.Class == "" {
				t.Errorf("the log row lost a field it used to carry: %s", raw)
			}
			if row.Confirmed != c.want {
				t.Errorf("confirmed = %v, want %v:\n%s", row.Confirmed, c.want, raw)
			}
		})
	}
}

// The log accumulates, and stays absent when there is nothing to say.
//
// README calls it "a file every finding is appended to". Nothing tested that:
// `O_APPEND` could become `O_TRUNC` with the suite green, leaving a log holding
// only the last write — the shape that makes a week of real use unreadable, and
// unreadable in a way nobody notices until they come to read it.
func TestTheLogAccumulatesAcrossWrites(t *testing.T) {
	t.Setenv(session.MemoryEnv, t.TempDir())
	log := filepath.Join(t.TempDir(), "findings.jsonl")
	t.Setenv(logEnv, log)

	write := func(name, call string) payload {
		in := payload{ToolName: "Edit"}
		in.ToolInput.FilePath = file(t, name, "package p\n\nfunc a() {\n\t// "+call+"\n\tdone()\n}\n")
		in.ToolInput.OldString = "\t" + call + "\n\tdone()\n"
		in.ToolInput.NewString = "\t// " + call + "\n\tdone()\n"
		return in
	}

	// A write with nothing to report must not leave a row, nor create the file
	// where it does not exist: an empty log and no log say different things.
	quiet := payload{ToolName: "Edit"}
	quiet.ToolInput.FilePath = file(t, "quiet.go", "package p\n\nfunc a() {\n\tdone()\n}\n")
	quiet.ToolInput.OldString = "\tstart()\n"
	quiet.ToolInput.NewString = "\tdone()\n"
	record(quiet.ToolInput.FilePath, review(quiet), quiet)
	if _, err := os.Stat(log); err == nil {
		t.Error("a write with no findings created the log")
	}

	for _, c := range []struct{ name, call string }{
		{"one.go", "deleteTemp(path)"},
		{"two.go", "closeHandle(h)"},
	} {
		in := write(c.name, c.call)
		findings := review(in)
		if len(findings) != 1 {
			t.Fatalf("%s: want one finding, got %d", c.name, len(findings))
		}
		record(in.ToolInput.FilePath, findings, in)
	}

	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("the log was not written: %v", err)
	}
	rows := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(rows) != 2 {
		t.Fatalf("want a row per write, got %d:\n%s", len(rows), raw)
	}
	for i, row := range rows {
		var got struct {
			At    string  `json:"at"`
			File  string  `json:"file"`
			Line  int     `json:"line"`
			Class string  `json:"class"`
			Score float64 `json:"score"`
		}
		if err := json.Unmarshal([]byte(row), &got); err != nil {
			t.Fatalf("row %d does not decode: %v", i, err)
		}
		if got.At == "" || got.File == "" || got.Line == 0 || got.Class == "" {
			t.Errorf("row %d is missing a field the log is read by: %s", i, row)
		}
	}
	if rows[0] == rows[1] {
		t.Error("both rows are the same write, so the second did not append")
	}
}

// The certain tier is a property of the payload, not of the tool that sent it.
//
// Two documents and a doc comment said a MultiEdit never earns the unhedged
// sentence, and the code has never worked that way: [attributable] counts
// replacements. A MultiEdit carrying one is a single-replacement write and gets
// it. Nothing reached the one-replacement array, so the false claim survived
// three rounds of review of the file it was written in.
func TestTheCertainTierCountsReplacementsAndNotToolNames(t *testing.T) {
	const once = `package p

func a() {
	// deleteTemp(path)
	done()
}
`
	pairs := [][2]string{
		{"\tdeleteTemp(path)\n\tdone()\n", "\t// deleteTemp(path)\n\tdone()\n"},
		{"\tunrelated()\n", "\tstillUnrelated()\n"},
	}

	for _, c := range []struct {
		name string
		take int
		want bool
	}{
		{"a MultiEdit carrying one replacement", 1, true},
		{"a MultiEdit carrying two", 2, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(session.MemoryEnv, t.TempDir())
			in := payload{ToolName: "MultiEdit"}
			in.ToolInput.FilePath = file(t, "once.go", once)
			for _, e := range pairs[:c.take] {
				in.ToolInput.Edits = append(in.ToolInput.Edits, struct {
					OldString string `json:"old_string"`
					NewString string `json:"new_string"`
				}{e[0], e[1]})
			}
			findings := review(in)
			if len(findings) != 1 {
				t.Fatalf("want one finding, got %d", len(findings))
			}
			got := strings.Contains(report("once.go", findings, in), "This write commented out live code")
			if got != c.want {
				t.Fatalf("unhedged = %v, want %v", got, c.want)
			}
		})
	}
}

// An edit is credited with the bytes it changed, not with the whole text it
// replaced. [written] is what decides which comments are attributed to a write
// at all, and the trimming that does it had no test reaching its purpose: the
// shared prefix and the shared suffix could each be dropped with the suite
// green. What survived was one test whose replacement is equal on both sides,
// so its comments sit outside the changed bytes either way.
//
// [narrow]'s rejection is still not covered, and no test can cover it: the span
// it refuses is empty, and an empty span names no comment, so refusing it and
// returning it are indistinguishable from here.
//
// Every row here is a write that leaves the comment exactly where it was and
// touches only the code around it. The write authored none of them.
func TestAWriteIsNotCreditedWithWhatItCarriedThrough(t *testing.T) {
	const src = `package p

func a() {
	// deleteTemp(path)
	setup()
	done()
}
`
	for _, c := range []struct {
		name     string
		was, now string
	}{
		{
			// The comment is the shared prefix: dropping it from the trim makes
			// the span reach back over a line the write only carried along.
			name: "the write appended below a comment it carried through",
			was:  "\t// deleteTemp(path)\n\tsetup()\n",
			now:  "\t// deleteTemp(path)\n\tsetup()\n\tdone()\n",
		},
		{
			// The mirrored case: the comment is the shared suffix.
			name: "the write inserted above a comment it carried through",
			was:  "func a() {\n\t// deleteTemp(path)\n",
			now:  "func a() {\n\tstart()\n\t// deleteTemp(path)\n",
		},
		{
			// A pure deletion, whose replacement is entirely shared prefix, so
			// the trimmed span is empty and the comment above it is nobody's.
			name: "the write deleted the line under a comment it carried through",
			was:  "\t// deleteTemp(path)\n\tsetup()\n",
			now:  "\t// deleteTemp(path)\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(session.MemoryEnv, t.TempDir())
			in := payload{ToolName: "Edit"}
			// The file as the write leaves it, which is what the hook reads.
			in.ToolInput.FilePath = file(t, "a.go", strings.Replace(src, c.was, c.now, 1))
			in.ToolInput.OldString = c.was
			in.ToolInput.NewString = c.now

			if findings := review(in); len(findings) != 0 {
				t.Fatalf("a comment the write only carried through is credited to it: %d findings, first at line %d", len(findings), findings[0].Line)
			}
		})
	}
}

// A write is credited with every copy of what it changed, because nothing in
// the payload tells the copies apart. `replace_all` is the shape that makes it
// real rather than theoretical, and [locate] could return the first occurrence
// alone with the suite green.
func TestAWriteClaimsEveryCopyOfWhatItChanged(t *testing.T) {
	t.Setenv(session.MemoryEnv, t.TempDir())
	const both = `package p

func first() {
	// deleteTemp(path)
	done()
}

func second() {
	// deleteTemp(path)
	done()
}
`
	in := payload{ToolName: "Edit"}
	in.ToolInput.FilePath = file(t, "both.go", both)
	in.ToolInput.OldString = "\tdeleteTemp(path)\n"
	in.ToolInput.NewString = "\t// deleteTemp(path)\n"

	findings := review(in)
	if len(findings) != 2 {
		t.Fatalf("want a finding for each copy the write could have changed, got %d", len(findings))
	}
	if findings[0].Line == findings[1].Line {
		t.Fatalf("both findings name line %d, so only one copy was claimed", findings[0].Line)
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
	fractions := []string{
		"half", "third", "quarter", "fifth", "sixth", "seventh", "eighth", "ninth", "tenth",
		"percent", "%", "most of the time", "of the time",
	}
	// A ratio spelled in words has a rate's shape and none of the tokens above.
	// This is an enumeration of joins rather than a test of shape, and it says so
	// because a previous version of this comment claimed otherwise: a reviewer
	// got eleven phrasings past it, "nine times out of ten" among them. The list
	// below is what those eleven taught it.
	//
	// **It leaks, and no count of the leaks belongs here.** Two revisions of this
	// note have named a closed set of remaining holes — one, then four — and a
	// reviewer found more each time: unnamed numbers in either position, plural
	// or unlisted nouns, an article between the halves, a hyphen or colon
	// instead of a join word, "per cent" spelled apart, fractions past a tenth.
	// The list catches what it has been taught and nothing else, and enumerating
	// the rest is the same mistake at one remove. What would close it is parsing
	// the sentence, not a longer list.
	// The previous list dropped "one in" while the commit adding it claimed to
	// have verified against "four in ten", which it passed — and "one in four on
	// compilers" is the phrasing in `report`'s default branch and in AGENTS.md, so it
	// is the likeliest thing to leak. Built as pairs rather than banned outright
	// because the nudge legitimately says "those four are the measured false
	// positives".
	numbers := []string{
		"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten",
		"twenty", "fifty", "hundred", "once", "twice",
	}
	joins := []string{
		"%s in %s", "%s in every %s", "%s out of %s", "%s out of every %s",
		"%s times in %s", "%s times out of %s", "%s of %s", "%s of every %s",
		"%s case in %s", "%s time in %s", "%s finding in %s",
	}
	for _, a := range numbers {
		for _, b := range numbers {
			for _, shape := range joins {
				fractions = append(fractions, fmt.Sprintf(shape, a, b))
			}
		}
	}
	// One finding this write demonstrably commented out and one it did not, so
	// the three branches of the report are all reached. A first version passed
	// `payload{}` and an edit matching nothing, which hedges either way, so it
	// certified every branch while exercising one.
	confirmed := rule.Finding{Line: 42, Reason: "commented-out code", Source: "foo()", Raw: "// foo()"}
	guessed := rule.Finding{Line: 43, Reason: "commented-out code", Source: "qux()", Raw: "// qux()"}
	commenting := edit("\tfoo()\n", "\t// foo()\n")

	for _, c := range []struct {
		name     string
		findings []rule.Finding
		in       payload
	}{
		{"every finding confirmed", []rule.Finding{confirmed}, commenting},
		{"some confirmed", []rule.Finding{confirmed, guessed}, commenting},
		{"none confirmed", []rule.Finding{guessed}, payload{}},
	} {
		for _, repeated := range []bool{false, true} {
			t.Run(c.name, func(t *testing.T) {
				repeat = repeated
				t.Cleanup(func() { repeat = false })
				out := report("store.go", c.findings, c.in)
				bare := strings.NewReplacer("store.go:42", "", "store.go:43", "").Replace(out)
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
			})
		}
	}
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
		// raw and text override the single-line fixture, for the rows whose
		// shape only a multi-line run can carry.
		raw  string
		text string
	}{
		{
			name: "one edit deleted it and the same edit commented it",
			in:   multi([2]string{"\tfoo()\n\tbar()\n", "\t// foo()\n\tbar()\n"}),
			want: true,
		},
		{
			// A run commented out at one indentation and reported at another,
			// which is what an edit inserting into a nested scope produces. This
			// is the only row with a multi-line Raw that wants a yes, and
			// without it either side of the flattening could be dropped with the
			// suite green — which would silence the confirmed claim on every run
			// of more than one line, and silently, since the invariant sweeps
			// take no payload. Its `Raw` carries indentation *between* its lines
			// because that is what a real run carries; written flush, it pinned
			// `flat(now)` alone and `flat(f.Raw)` stayed revertible.
			name: "a run commented out across two lines is confirmed",
			raw:  "// foo()\n\t// bar()",
			text: "foo()\nbar()",
			in: edit(
				"\tfoo()\n\tbar()\n",
				"\tif cond {\n\t\t// foo()\n\t\t// bar()\n\t}\n",
			),
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
			// The same carried-through comment, reindented by the edit that
			// carried it. Only a run reaches this: `Raw` holds the indentation
			// *between* a run's lines, and a one-line comment has none, so the
			// row that used the shared single-line fixture passed with and
			// without the flattening it was written for.
			name: "the edit reindented an older comment while deleting what it quotes",
			raw:  "// foo()\n\t\t// bar()",
			text: "foo()\nbar()",
			in: edit(
				"\t// foo()\n\t// bar()\n\tfoo()\n\tbar()\n",
				"\tif cond {\n\t\t// foo()\n\t\t// bar()\n\t}\n",
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
		{
			// tree-sitter-rust ends a `///` node at column zero of the row
			// below, so `Raw` ends in a newline and its flattened split has a
			// trailing empty string. Asking `before` about that line answers
			// yes for any replacement that ends in one, which is nearly all of
			// them: the round that added the line-wise test took the certain
			// tier away from every Rust doc comment, and the suite stayed green.
			name: "a doc comment whose node ends on the row below is still confirmed",
			raw:  "/// foo()\n",
			text: "foo()",
			in:   edit("\tfoo()\n\tbar()\n", "\t/// foo()\n\tbar()\n"),
			want: true,
		},
		{
			// The comment stood at the end of a live line the edit deleted, so
			// the run it now forms is text the replacement already held. Whether
			// the write authored it or promoted a trailing comment to its own
			// line cannot be told apart, and only the whole-run test refuses it:
			// no line of the run was ever a whole line of what was replaced.
			name: "the comment already stood at the end of a line the edit deleted",
			in:   edit("\tfoo()\n\tbar() // foo()\n", "\t// foo()\n"),
			want: false,
		},
		{
			// The run stood in the replaced text too, differing only in the
			// indentation between its lines, which is the miss the doc claims in
			// the safe direction. Reached through neither other test: no line of
			// the run is a whole line of what was replaced, so the line-wise
			// test passes it, and only flattening the replaced text finds it.
			name: "the run stood in the replaced text at another indentation",
			raw:  "// foo()\n\t// bar()",
			text: "foo()\nbar()",
			in: edit(
				"\tfoo()\n\tbar()\n\tqux() // foo()\n\t// bar() zap()\n",
				"\tif cond {\n\t\t// foo()\n\t\t// bar()\n\t}\n",
			),
			want: false,
		},
		{
			// A trailing comment on a line the write kept supplies the run's
			// first line, so a plain substring test finds in the replacement a
			// run the replacement never wrote as one. The second line stood
			// before the write, and the unhedged sentence covered it.
			name: "a trailing comment supplies the first line of the run",
			raw:  "// foo()\n\t// foo()",
			text: "foo()\nfoo()",
			in: edit(
				"\tqux() // foo()\n\tfoo()\n",
				"\tqux() // foo()\n\t// foo()\n",
			),
			want: false,
		},
		{
			// The quoted lines include a bare `}`, which the file still holds
			// live below the block. Skipping lines too short to be evidence is
			// what keeps a commented-out block confirmed; without it the brace
			// is found in the replacement and the tier goes, which is the Rust
			// failure in another costume.
			name: "a commented-out block whose closing brace the file still holds",
			raw:  "// if cond {\n\t// \tfoo()\n\t// }",
			text: "if cond {\nfoo()\n}",
			in: edit(
				"\tif cond {\n\t\tfoo()\n\t}\n}\n",
				"\t// if cond {\n\t// \tfoo()\n\t// }\n}\n",
			),
			want: true,
		},
		{
			// Two comments that were already comments, made adjacent for the
			// first time by one edit deleting the live code between them. The
			// run is genuinely new — it stood nowhere before — so a whole-run
			// carry-through test passes it, and each line the finding quotes was
			// a whole line of the replacement and is not one of the insertion.
			// Every condition met, and the write authored none of the comment.
			name: "one edit deleted the code between two comments that already existed",
			raw:  "// foo()\n\t// bar()",
			text: "foo()\nbar()",
			in: edit(
				"\t// foo()\n\tfoo()\n\t// bar()\n\tbar()\n",
				"\t// foo()\n\t// bar()\n",
			),
			want: false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := rule.Finding{Line: 1, Source: "foo()", Raw: "// foo()"}
			if c.raw != "" {
				f.Raw, f.Source = c.raw, c.text
			}
			// A row asking for a run has to get one. Both override rows answer
			// the same way when they fall back to the shared single-line
			// fixture, so without this the plumbing could be deleted and they
			// would pass on the wrong shape — the vacuous-fixture defect this
			// table has now hit twice.
			if strings.Contains(c.raw, "\n") && !strings.Contains(f.Raw, "\n") {
				t.Fatalf("this row is about a multi-line run and got %q", f.Raw)
			}
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
