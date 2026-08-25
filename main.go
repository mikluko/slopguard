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
		for _, arg := range os.Args[1:] {
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintln(os.Stderr, "usage: slopguard [file ...]   (a hook payload on stdin, or files to judge)")
				return
			}
		}
		sweepFiles(os.Args[1:])
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
func review(in payload) []finding {
	switch in.ToolName {
	case "Write", "Edit", "MultiEdit":
	default:
		return nil
	}
	lang := lookup(in.ToolInput.FilePath)
	if lang == nil {
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
	findings := scan(src, lang, added)
	said, remember := spoken(in.SessionID, in.ToolInput.FilePath)
	kept := findings[:0]
	for _, f := range findings {
		if !said[f.key] {
			kept = append(kept, f)
		}
	}
	findings = kept
	if len(findings) > budget {
		findings = findings[:budget]
	}
	remember(findings)
	return findings
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
func record(path string, findings []finding) {
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
			"line":  f.line,
			"class": f.class,
			"score": f.score,
		})
	}
}

// sweepFiles judges the named files whole and prints what it finds, so that
// the numbers in the README are reproducible from the repository rather than
// from a script in somebody's scratch directory.
func sweepFiles(paths []string) {
	for _, path := range paths {
		lang := lookup(path)
		if lang == nil {
			continue
		}
		src, err := readable(path)
		if err != nil {
			continue
		}
		for _, f := range scan(src, lang, []span{{start: 0, end: uint(len(src))}}) {
			fmt.Printf("%s:%d\t%s\t%.3f\t%s\n", path, f.line, f.class, f.score, f.reason)
		}
	}
}

// written returns the byte ranges of src that this tool call authored: the
// whole file for a Write, and the text each edit put in place otherwise.
//
// An edit is located by its replacement text, which is exact when that text
// occurs once and a guess when it does not. Ambiguity is resolved by the
// surrounding old text where the payload carries it, and bounded by
// [spanBudget] where it does not, because attributing the whole file to a
// one-character edit would nudge the agent for lines it never touched.
func written(src []byte, in payload) []span {
	if in.ToolName == "Write" {
		return []span{{start: 0, end: uint(len(src))}}
	}
	type edit struct{ old, new string }
	edits := []edit{{in.ToolInput.OldString, in.ToolInput.NewString}}
	for _, e := range in.ToolInput.Edits {
		edits = append(edits, edit{e.OldString, e.NewString})
	}
	var out []span
	for _, e := range edits {
		if e.new == "" || len(out) >= spanBudget {
			continue
		}
		found := locate(src, e.new)
		if len(found) > 1 && e.old != "" {
			if narrowed := context(src, found, e); len(narrowed) > 0 {
				found = narrowed
			}
		}
		out = append(out, found...)
	}
	return out
}

// locate returns every occurrence of text in src, up to [spanBudget].
func locate(src []byte, text string) []span {
	var out []span
	needle := []byte(text)
	for offset := 0; len(out) < spanBudget; {
		i := bytes.Index(src[offset:], needle)
		if i < 0 {
			break
		}
		start := offset + i
		out = append(out, span{start: uint(start), end: uint(start + len(needle))})
		offset = start + len(needle)
	}
	return out
}

// context keeps the occurrences whose neighbourhood still holds the unchanged
// halves of the text the edit replaced, which is what tells one occurrence of a
// repeated replacement from another.
func context(src []byte, found []span, e struct{ old, new string }) []span {
	prefix, suffix := shared(e.old, e.new)
	if prefix == 0 && suffix == 0 {
		return nil
	}
	var out []span
	for _, s := range found {
		before := int(s.start) - prefix
		after := int(s.end) + suffix
		if before < 0 || after > len(src) {
			continue
		}
		out = append(out, s)
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

// report is the work handed back to the agent: which lines, which rule, and
// what to do about each. It names the tool and the file, because a nudge that
// does not ask for an edit is read as commentary and answered in prose.
func report(name string, findings []finding, in payload) string {
	var b strings.Builder
	b.WriteString("slopguard read the comments this write added to " + name + ".\n\n")
	for _, f := range findings {
		b.WriteString("  line " + strconv.FormatUint(uint64(f.line), 10) + "  " + f.reason + "\n")
	}
	b.WriteString("\nTake each line with Edit on " + name + " before the next step. " +
		"If the claim still binds the next editor, restate it as the symbol's contract or as a test. " +
		"If it only records this change, cut it and carry it into the commit message. " +
		"What is judged is where the claim lives, not which words carry it, so rewording is not a fix. " +
		"If a line is right where it is, keep it and say so in one line.")
	if rules := anchor(in); rules != "" {
		b.WriteString("\n\nThe rules these come from are in " + rules + ".")
	}
	return b.String()
}

// anchor names the file holding the rules, when the session has one. Citing a
// file the agent cannot open buys nothing.
func anchor(in payload) string {
	if in.CWD == "" {
		return ""
	}
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		if _, err := os.Stat(filepath.Join(in.CWD, name)); err == nil {
			return name
		}
	}
	return ""
}

func summary(name string, findings []finding) string {
	count := strconv.Itoa(len(findings)) + " comment"
	if len(findings) != 1 {
		count += "s"
	}
	return "slopguard: " + count + " in " + name + " to reconsider"
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
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, relative)
}
