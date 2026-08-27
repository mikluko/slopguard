package corpus

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Git runs one git command in dir and returns its standard output. A command
// that fails returns the error with git's own diagnostics attached, since the
// exit status alone never says which object was missing.
func Git(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errs bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errs
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errs.String()))
	}
	return out.Bytes(), nil
}

// Lines splits git's output into its non-empty lines.
func Lines(out []byte) []string {
	var kept []string
	for line := range strings.SplitSeq(string(out), "\n") {
		if line != "" {
			kept = append(kept, line)
		}
	}
	return kept
}

// A Blobs is a `git cat-file --batch` process held open across lookups.
//
// Mining reads two blobs for every changed file of every commit sampled, and
// spawning git once per blob costs several times what parsing the blob costs.
// The process is stateful, so a caller must not read from two goroutines at
// once.
type Blobs struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	out *bufio.Reader
}

// OpenBlobs starts the reader against the repository in dir. Close ends it.
func OpenBlobs(dir string) (*Blobs, error) {
	cmd := exec.Command("git", "cat-file", "--batch")
	cmd.Dir = dir
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Blobs{cmd: cmd, in: in, out: bufio.NewReaderSize(out, 1<<20)}, nil
}

// Read returns the contents of rev, which is a revision and path joined by a
// colon. An object the repository does not have returns nil and no error: a
// path absent from a commit is an ordinary answer here, not a failure.
func (b *Blobs) Read(rev string) ([]byte, error) {
	// A newline would be read as two requests and leave the reader permanently
	// one answer behind, which pairs every later file with the wrong revision
	// and looks like nothing at all downstream.
	if strings.ContainsAny(rev, "\n\r") {
		return nil, fmt.Errorf("rev %q contains a newline", rev)
	}
	if _, err := io.WriteString(b.in, rev+"\n"); err != nil {
		return nil, err
	}
	header, err := b.out.ReadString('\n')
	if err != nil {
		return nil, err
	}
	header = strings.TrimSuffix(header, "\n")
	// The refusals are suffixes of the echoed request, so they are tested before
	// the header is split: a path with a space in it splits into more fields
	// than a header has and the size parse then fails on a fragment of the name.
	for _, refusal := range []string{" missing", " ambiguous"} {
		if strings.HasSuffix(header, refusal) {
			return nil, nil
		}
	}
	fields := strings.Fields(header)
	if len(fields) != 3 {
		return nil, fmt.Errorf("cat-file header %q", header)
	}
	size, err := strconv.Atoi(fields[2])
	if err != nil {
		return nil, fmt.Errorf("cat-file header %q: %w", header, err)
	}
	content := make([]byte, size)
	if _, err := io.ReadFull(b.out, content); err != nil {
		return nil, err
	}
	// The batch format writes a newline after every object's bytes.
	if _, err := b.out.Discard(1); err != nil {
		return nil, err
	}
	// A rev naming a tree or a tag answers with a valid header and bytes that
	// are not source, and handing those to a parser is worse than refusing
	// them. The bytes are read first regardless: leaving them in the pipe is
	// the desynchronisation this function exists to avoid.
	if fields[1] != "blob" {
		return nil, nil
	}
	return content, nil
}

// Close ends the reader process.
func (b *Blobs) Close() error {
	b.in.Close()
	return b.cmd.Wait()
}

// A Blamed line is the commit that last wrote it and when that commit landed.
// The time is the committer's, so that two commits compare in the order history
// actually has them rather than in the order they were first written.
type Blamed struct {
	SHA  string
	When time.Time
}

// Blame returns one entry per line of path at rev, indexed from zero. An empty
// rev blames the working tree.
//
// It reads `--line-porcelain`, which repeats every header on every line, so a
// block is read as it arrives rather than against a table of earlier blocks.
// A block runs from its header to the tab-prefixed line carrying the source,
// which is what closes it.
func Blame(dir, rev, path string) ([]Blamed, error) {
	args := []string{"blame", "--line-porcelain"}
	if rev != "" {
		args = append(args, rev)
	}
	args = append(args, "--", path)
	out, err := Git(dir, args...)
	if err != nil {
		return nil, err
	}
	var all []Blamed
	var current Blamed
	for line := range strings.SplitSeq(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "\t"):
			all = append(all, current)
			current = Blamed{}
		case strings.HasPrefix(line, "committer-time "):
			seconds, err := strconv.ParseInt(strings.TrimPrefix(line, "committer-time "), 10, 64)
			if err == nil {
				current.When = time.Unix(seconds, 0).UTC()
			}
		case len(line) >= 40 && IsHex(line[:40]) && current.SHA == "":
			current.SHA = line[:40]
		}
	}
	return all, nil
}

// BlameLine returns the commit that last wrote one line of path at rev.
func BlameLine(dir, rev, path string, line uint) (Blamed, bool) {
	out, err := Git(dir, "blame", "--line-porcelain",
		"-L", fmt.Sprintf("%d,%d", line, line), rev, "--", path)
	if err != nil {
		return Blamed{}, false
	}
	var one Blamed
	for text := range strings.SplitSeq(string(out), "\n") {
		switch {
		case strings.HasPrefix(text, "committer-time "):
			seconds, err := strconv.ParseInt(strings.TrimPrefix(text, "committer-time "), 10, 64)
			if err == nil {
				one.When = time.Unix(seconds, 0).UTC()
			}
		case len(text) >= 40 && IsHex(text[:40]) && one.SHA == "":
			one.SHA = text[:40]
		}
	}
	if one.SHA == "" || one.When.IsZero() {
		return Blamed{}, false
	}
	return one, true
}

// LineEdits counts the commits that touched one line of path, following the
// line back through the renames and shifts that moved it.
//
// It is what separates a comment somebody read and left from one that merely
// sat in a file other people were editing elsewhere. Counting commits to the
// file answers the second question while claiming to answer the first: a
// comment recorded at seventy-two file edits can have exactly one commit that
// ever touched its own line, and most do, because a survived comment sits at a
// median line of 359 while a file's churn is somewhere else entirely.
//
// One git process per line, so it is affordable only over a sample. Merges are
// left out for the reason they are left out everywhere else here.
func LineEdits(dir, rev, path string, line uint) (int, error) {
	out, err := Git(dir, "log", "--no-merges", "--format=%H",
		"-L", fmt.Sprintf("%d,%d:%s", line, line, path), rev)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, text := range Lines(out) {
		if len(text) == 40 && IsHex(text) {
			count++
		}
	}
	return count, nil
}

// IsHex reports whether s is all lowercase hexadecimal, which is how a header
// line is told from a field whose name happens to be forty characters long.
func IsHex(s string) bool {
	for i := range len(s) {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
