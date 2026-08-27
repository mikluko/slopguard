// Command slopguard is a PostToolUse hook for the Write and Edit tools: it
// objects to comments in the text a write just added whose claim belongs
// somewhere else.
//
// It reads the hook payload on stdin and writes a PostToolUse result on stdout.
// A write it does not object to produces no output at all. Given file paths as
// arguments instead, it judges those files and prints what it finds.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mikluko/slopguard/internal/comment"
	"github.com/mikluko/slopguard/internal/lang"
	"github.com/mikluko/slopguard/internal/rule"
	"github.com/mikluko/slopguard/internal/session"
)

const (
	// budget is the number of comments one nudge names. Three ranked lines are
	// one unit of work; a longer list is a report, and a report gets skimmed.
	budget = 3
	// ceiling is the largest file slopguard will read. Above it the write is
	// generated or vendored, and the parse and the embedding both stop being
	// worth their cost.
	ceiling = 2 << 20
	// spanBudget bounds how much of a file one call may claim to have written,
	// across every edit in it. A short replacement such as "}" occurs
	// everywhere, and every extra span costs a pass over every comment.
	spanBudget = 64
	// logEnv names a file every finding is appended to, as one JSON object per
	// line.
	logEnv = "SLOPGUARD_LOG"
)

type payload struct {
	SessionID string `json:"session_id"`
	ToolName  string `json:"tool_name"`
	CWD       string `json:"cwd"`
	ToolInput struct {
		FilePath  string `json:"file_path"`
		Content   string `json:"content"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
		Edits     []struct {
			OldString string `json:"old_string"`
			NewString string `json:"new_string"`
		} `json:"edits"`
	} `json:"tool_input"`
}

func main() {
	// A hook that dies loudly is worse than one that says nothing: exit 2 is
	// how PostToolUse spells "blocking error", so a panic would inject a stack
	// dump into the agent's context in place of a nudge.
	defer func() {
		if recover() != nil {
			os.Exit(0)
		}
	}()
	// Paths instead of a payload: judge those files and print what is found.
	// A flag is neither, and reading stdin for one would hang a hook whose
	// wiring grew an argument by accident.
	if len(os.Args) > 1 {
		var paths []string
		verbose := false
		for _, arg := range os.Args[1:] {
			switch {
			case arg == "-v":
				verbose = true
			case strings.HasPrefix(arg, "-"):
				fmt.Fprintln(os.Stderr, "usage: slopguard [-v] [file ...]   (a hook payload on stdin, or files to judge)")
				return
			default:
				paths = append(paths, arg)
			}
		}
		sweepFiles(paths, verbose)
		return
	}
	var in payload
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		return
	}
	findings := review(in)
	record(in.ToolInput.FilePath, findings)
	if len(findings) == 0 {
		return
	}
	name := display(in)
	json.NewEncoder(os.Stdout).Encode(map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     "PostToolUse",
			"additionalContext": report(name, findings, in),
		},
		"systemMessage": summary(name, findings),
	})
}

// review returns what slopguard objects to in the file the payload just wrote.
// It fails open: an unknown tool, an extension carrying no grammar, a file it
// cannot read, and text it cannot locate all yield nothing.
func review(in payload) []rule.Finding {
	switch in.ToolName {
	case "Write", "Edit", "MultiEdit":
	default:
		return nil
	}
	language := lang.Lookup(in.ToolInput.FilePath)
	if language == nil {
		return nil
	}
	src, err := readable(in.ToolInput.FilePath)
	if err != nil {
		return nil
	}
	added := written(src, in)
	if len(added) == 0 {
		return nil
	}
	findings := rule.Judge(src, language, added)
	said, before, remember := session.Spoken(in.SessionID, in.ToolInput.FilePath)
	repeat = before
	kept := findings[:0]
	for _, f := range findings {
		if !said[f.Key] {
			kept = append(kept, f)
		}
	}
	findings = kept
	if len(findings) > budget {
		findings = findings[:budget]
	}
	// A comment whose text occurs more than once in the file cannot be told from
	// its twin by text, and [switched] compares text. Both copies then satisfy
	// every condition against the one edit that commented out the other, and the
	// pre-existing copy gets the tool's only unhedged sentence. The finding
	// stands; only its claim to certainty goes, which is what an empty Raw
	// means to [switched].
	for i, f := range findings {
		if f.Raw != "" && strings.Count(string(src), f.Raw) > 1 {
			findings[i].Raw = ""
		}
	}
	keys := make([]uint64, len(findings))
	for i, f := range findings {
		keys[i] = f.Key
	}
	remember(keys)
	return findings
}

// switched reports whether this write is what commented the finding out: the
// lines it names were live code in the text the edit replaced.
//
// This is the defect itself rather than evidence for it. Everything else the
// tool does asks whether a comment reads like source, which is a question about
// shape, and shape is where it goes wrong — a compiler sketching what it emits
// and a spec written as an assignment are source-shaped and correct. Whether a
// line was code a moment ago is not a question about shape at all, and the
// answer is already in the payload.
//
// Only an Edit carries one. A Write hands over the whole file with no before, so
// this cannot speak for it and says no rather than guessing.
//
// Every line has to be found. A run that is half old code and half prose is
// somebody writing a note next to what they disabled, and the part this can
// vouch for is not the part being reported.
//
// One edit at a time, and the line has to be gone from that edit's replacement.
// Present-in-the-before is not the claim: a comment quoting a line that is still
// live two lines down was present before and after, and reporting that as a
// transition is the strongest thing this tool says about a comment nobody
// touched. Pooling several edits' before-text is the same error across a file:
// a line edit one deleted would vouch for a comment edit two wrote elsewhere.
func switched(f rule.Finding, in payload) bool {
	type change struct{ was, now string }
	edits := []change{{in.ToolInput.OldString, in.ToolInput.NewString}}
	for _, e := range in.ToolInput.Edits {
		edits = append(edits, change{e.OldString, e.NewString})
	}
	for _, e := range edits {
		if strings.TrimSpace(e.was) == "" {
			continue
		}
		if moved(f, e.was, e.now) {
			return true
		}
	}
	return false
}

// moved reports whether one edit both wrote this comment and stopped its lines
// being code.
//
// Four conditions, and the pair on the raw text is what makes it one edit rather
// than two. The comment as written must appear in what the edit inserted and not
// in what it replaced: present on both sides it is a comment the edit carried
// through untouched, and absent from the replacement it is not this edit's at
// all. Two earlier versions tested only the stripped text and both let a
// deleting edit vouch for a comment it never touched — first by pooling every
// edit's before-text, then by asking merely that the stripped line occur
// somewhere in the replacement, which `defer foo()` satisfies for `foo()`.
//
// The stripped lines carry the other half: each was a whole line of the replaced
// text and is no longer a whole line of the inserted text. Whole lines, because
// `return v * 3` occurs inside `// return v * 3`, so a substring test reports a
// line that was already a comment as one this write commented out — the opposite
// finding.
//
// A block comment whose inner lines carry no marker of their own is not detected
// and cannot be: its content lines are whole lines of the inserted text, so the
// second condition rejects them. That is a miss rather than a false claim.
func moved(f rule.Finding, was, now string) bool {
	if f.Raw == "" || !strings.Contains(now, f.Raw) || strings.Contains(was, f.Raw) {
		return false
	}
	lines := func(text string) map[string]bool {
		out := map[string]bool{}
		for _, line := range strings.Split(text, "\n") {
			out[strings.TrimSpace(line)] = true
		}
		return out
	}
	before, after := lines(was), lines(now)
	found := false
	for _, line := range strings.Split(f.Source, "\n") {
		line = strings.TrimSpace(line)
		// A blank line and a stray delimiter are in every fragment and occur in
		// every file, so neither is evidence that this one moved.
		if len(line) < 3 {
			continue
		}
		if !before[line] || after[line] {
			return false
		}
		found = true
	}
	return found
}

// readable returns the contents of a file worth judging: a regular file, small
// enough that parsing and embedding it are worth their cost. A named pipe under
// a known extension would otherwise block the hook until the harness kills it,
// and a generated file of several megabytes is not what the tool is for.
func readable(path string) ([]byte, error) {
	info, err := os.Stat(path)
	switch {
	case err != nil:
		return nil, err
	case !info.Mode().IsRegular():
		return nil, errNotWorthReading
	case info.Size() > ceiling:
		return nil, errNotWorthReading
	}
	return os.ReadFile(path)
}

var errNotWorthReading = errors.New("not a regular file, or too large to judge")

// record appends what was found to the log, when one is asked for. A week of
// real use is the only honest measure of how often this tool is wrong, and
// nothing else in the process writes it down.
func record(path string, findings []rule.Finding) {
	name := os.Getenv(logEnv)
	if name == "" || len(findings) == 0 {
		return
	}
	handle, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer handle.Close()
	for _, f := range findings {
		json.NewEncoder(handle).Encode(map[string]any{
			"at":    time.Now().UTC().Format(time.RFC3339),
			"file":  path,
			"line":  f.Line,
			"class": f.Class,
			"score": f.Score,
		})
	}
}

// sweepFiles judges the named files whole and prints what it finds, so that
// the numbers in the README are reproducible from the repository rather than
// from a script in somebody's scratch directory.
func sweepFiles(paths []string, verbose bool) {
	for _, path := range paths {
		language := lang.Lookup(path)
		if language == nil {
			continue
		}

		src, err := readable(path)
		if err != nil {
			continue
		}
		var lines [][]byte
		if verbose {
			lines = bytes.Split(src, []byte("\n"))
		}
		for _, f := range rule.Judge(src, language, []comment.Span{{Start: 0, End: uint(len(src))}}) {
			fmt.Printf("%s:%d\t%s\t%.3f\t%s\n", path, f.Line, f.Class, f.Score, f.Reason)
			for _, text := range quoted(lines, f.Line) {
				fmt.Printf("\t| %s\n", text)
			}
		}
	}
}

// quoted returns the comment at a one-based line and the first line of code
// under it, for a sweep being read by someone deciding whether the finding is
// right. It returns nothing when the sweep is not printing them.
//
// A comment several lines long is reported at its first line, so the line after
// that one is usually still the comment. What the reader came for is the code,
// so the run is skipped by the marker it opens with.
func quoted(lines [][]byte, at uint) []string {
	if len(lines) == 0 || at == 0 || int(at) > len(lines) {
		return nil
	}
	head := strings.TrimSpace(string(lines[at-1]))
	out := []string{printable(head)}
	for _, line := range lines[at:] {
		text := strings.TrimSpace(string(line))
		if text == "" || continues(head, text) {
			continue
		}
		out = append(out, printable(text))
		break
	}
	return out
}

// continues reports whether a line is more of the comment that another opens.
func continues(head, text string) bool {
	for _, marker := range []string{"///", "//!", "//", "#", "--", "*", "/*"} {
		if strings.HasPrefix(head, marker) {
			return strings.HasPrefix(text, marker) || strings.HasPrefix(text, "*")
		}
	}
	return false
}

// written returns the byte ranges of src that this tool call authored: the
// whole file for a Write, and the text each edit put in place otherwise.
//
// An edit is located by its replacement text and trimmed to the bytes that
// text changed, so an insertion claims the line it added and not the lines it
// carried along. Where the changed bytes occur more than once, every occurrence
// is claimed: nothing in the payload tells the copies apart, and [spanBudget]
// bounds how many a call may claim.
func written(src []byte, in payload) []comment.Span {
	if in.ToolName == "Write" {
		return []comment.Span{{Start: 0, End: uint(len(src))}}
	}
	type edit struct{ old, new string }
	edits := []edit{{in.ToolInput.OldString, in.ToolInput.NewString}}
	for _, e := range in.ToolInput.Edits {
		edits = append(edits, edit{e.OldString, e.NewString})
	}
	var out []comment.Span
	for _, e := range edits {
		if e.new == "" || len(out) >= spanBudget {
			continue
		}
		prefix, suffix := shared(e.old, e.new)
		for _, s := range locate(src, e.new) {
			if narrowed, ok := narrow(s, prefix, suffix); ok {
				out = append(out, narrowed)
			}
		}
	}
	return out
}

// narrow trims an occurrence to the bytes the edit actually changed: an edit
// that inserts a line into a function leaves the rest of its replacement text
// exactly as it was, and claiming all of it would attribute untouched comments
// to this write. It reports false when nothing changed inside the occurrence,
// which is what a pure deletion looks like from here.
func narrow(s comment.Span, prefix, suffix int) (comment.Span, bool) {
	start := s.Start + uint(prefix)
	end := s.End - uint(suffix)
	if prefix+suffix >= int(s.End-s.Start) || start >= end {
		return comment.Span{}, false
	}
	return comment.Span{Start: start, End: end}, true
}

// locate returns every occurrence of text in src, up to [spanBudget].
func locate(src []byte, text string) []comment.Span {
	var out []comment.Span
	needle := []byte(text)
	for offset := 0; len(out) < spanBudget; {
		i := bytes.Index(src[offset:], needle)
		if i < 0 {
			break
		}
		start := offset + i
		out = append(out, comment.Span{Start: uint(start), End: uint(start + len(needle))})
		offset = start + len(needle)
	}
	return out
}

// shared returns how much of the old text the new text still opens and closes
// with, the part of an edit that did not change.
func shared(old, new string) (prefix, suffix int) {
	for prefix < len(old) && prefix < len(new) && old[prefix] == new[prefix] {
		prefix++
	}
	for suffix < len(old)-prefix && suffix < len(new)-prefix &&
		old[len(old)-1-suffix] == new[len(new)-1-suffix] {
		suffix++
	}
	return prefix, suffix
}

// repeat records whether this session has been nudged before, which is what
// decides between the instruction and its short form.
var repeat bool

// report is the work handed back to the agent: which lines, which rule, and
// what to do about each. It names the tool and the file, because a nudge that
// does not ask for an edit is read as commentary and answered in prose.
func report(name string, findings []rule.Finding, in payload) string {
	var b strings.Builder
	certain := 0
	for _, f := range findings {
		if switched(f, in) {
			certain++
		}
	}
	// A confirmed line is not a heuristic and must not be hedged like one: this
	// write turned that code into a comment, and the tool watched it happen.
	// Hedging it costs the one finding the tool is sure of.
	switch {
	case certain == len(findings):
		b.WriteString("This write commented out live code in " + name + ". Delete it or restore it — git has it.\n\n")
	case certain > 0:
		b.WriteString("This write commented out live code in " + name + ", marked below. The rest is a heuristic " +
			"and much of what it says is wrong.\n\n")
	default:
		// No single rate here. What ships is right six to nine times in ten on
		// application code and about one in four on compilers and spec
		// implementations, and the pooled figure between them is fitted to the
		// sample the exemptions were chosen from. An agent given a number acts
		// on it; the honest thing to hand it is the direction and the shapes.
		b.WriteString("slopguard is a heuristic. Many of its findings are wrong, and in code that implements a " +
			"specification or generates other code, most are. It parsed these comments in " +
			name + " as valid source:\n\n")
	}
	for _, f := range findings {
		mark := "  "
		if certain > 0 && certain < len(findings) && switched(f, in) {
			mark = "* "
		}
		b.WriteString(mark + name + ":" + strconv.FormatUint(uint64(f.Line), 10) + "  " + f.Reason + "\n")
	}
	if certain == len(findings) {
		return b.String()
	}
	// The long form is worth its tokens once. After that the agent has read
	// it, and re-sending it every time is the one place this tool spends
	// context it has not earned. The calibration is not part of that trade: it
	// is what makes the finding safe to act on, so the short form keeps it.
	if repeat {
		b.WriteString("\nAs before: delete it if this write switched off live code, and leave it if it is notation. No reply needed.")
		return b.String()
	}
	b.WriteString("\nPer line: if this write commented out live code, delete it — git has it. " +
		"If it is spec or algebraic notation, a sketch of the code a pass emits, a label naming what a case arm " +
		"handles, or a section heading, then it is right as written: leave it and carry on, no reply needed. " +
		"Those four are the measured false positives. Do not reword a comment to satisfy this, and do not act on a " +
		"comment this write did not author.")
	return b.String()
}

// summary is the line the human sees. It carries the lines rather than only
// their number: the agent is being told to edit something on a signal that is
// often wrong, and the one party able to judge that was being
// handed a count it could not check against anything.
func summary(name string, findings []rule.Finding) string {
	at := make([]string, 0, len(findings))
	for _, f := range findings {
		at = append(at, name+":"+strconv.FormatUint(uint64(f.Line), 10))
	}
	count := strconv.Itoa(len(findings)) + " comment"
	if len(findings) != 1 {
		count += "s"
	}
	return "slopguard: " + count + " to reconsider — " + strings.Join(at, ", ")
}

// display names the file the way the agent addressed it, relative to the
// session's working directory. The name reaches a language model, so anything
// that could carry its own line is dropped.
func display(in payload) string {
	path := in.ToolInput.FilePath
	relative, err := filepath.Rel(in.CWD, path)
	if in.CWD == "" || err != nil || strings.HasPrefix(relative, "..") {
		relative = filepath.Base(path)
	}
	return printable(relative)
}

// printable drops what a terminal or a language model would read as structure
// rather than as text.
func printable(text string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, text)
}
