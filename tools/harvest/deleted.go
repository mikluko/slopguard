package main

import (
	"strconv"
	"strings"
	"time"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/mikluko/slopguard/internal/comment"
	"github.com/mikluko/slopguard/internal/corpus"
	"github.com/mikluko/slopguard/internal/lang"
)

// annotatedFloor is how much code has to sit under a comment before its
// survival can be checked.
//
// Twenty-four bytes of flattened code is `if err != nil { return }` exactly, and
// `from main_app import app`, and `config *bootstrap.Config`. Those recur many
// times in one file, so a membership test on them answers yes whatever the
// commit did: measured on the first corpus, 13.5% of the deleted class had lost
// an occurrence of its own annotated code and passed anyway. The floor is raised
// and the test counts occurrences rather than asking whether the text is present
// at all, which is the half that actually fixes it.
const annotatedFloor = 40

// burst is how many comments one commit may drop from one file before the whole
// file is read as a rewrite rather than as a set of judgements.
//
// A person deleting a comment they think is bad deletes one, or two. A file
// reorganised in one commit loses every comment whose code moved with it, and
// those comments were not judged at all: measured on logrus, six of ten
// harvested deletions came from a single rewrite of one file, and three of the
// six were prose worth keeping.
const burst = 3

// maxFile bounds what is parsed. A megabyte of generated source carries
// thousands of comments that one commit's diff never touched, and parsing it
// costs more than every hand-written file in the same commit together.
const maxFile = 512 * 1024

// A commit is one sampled revision and when it landed.
type commit struct {
	sha  string
	when time.Time
}

// sample returns the most recent commits of dir, merges left out. A merge
// carries its side's changes a second time, and a comment deleted on a branch
// would be harvested once from the branch and once from the merge.
//
// The time is the committer's, not the author's. A rebase or a cherry-pick
// keeps the author's date, so ordering two commits by it can put a comment's
// deletion before the commit that wrote it and report a negative lifetime.
func sample(dir string, want int) ([]commit, error) {
	out, err := corpus.Git(dir, "log", "--no-merges", "-n", strconv.Itoa(want), "--format=%H %ct", "HEAD")
	if err != nil {
		return nil, err
	}
	var all []commit
	for _, line := range corpus.Lines(out) {
		sha, when, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		seconds, err := strconv.ParseInt(when, 10, 64)
		if err != nil {
			continue
		}
		all = append(all, commit{sha: sha, when: time.Unix(seconds, 0).UTC()})
	}
	return all, nil
}

// touched returns the paths one commit modified, leaving out what this harvest
// cannot read or should not read: files in a language with no grammar, and the
// vendored, generated and third-party trees where the comments are somebody
// else's again.
//
// Only modifications count. An added file has no earlier version to have
// deleted a comment from, and a deleted file took its comments with it, which
// is a sweep rather than a judgement.
func touched(dir, sha string) ([]string, error) {
	out, err := corpus.Git(dir, "diff-tree", "--no-commit-id", "--name-status", "-r", sha)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range corpus.Lines(out) {
		status, path, ok := strings.Cut(line, "\t")
		if !ok || status != "M" || skip(path) || lang.Lookup(path) == nil {
			continue
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// excluded names the path segments whose contents were written elsewhere, so a
// comment found under one is not evidence about the repository that carries it.
// `packages` is deliberately absent. Adding it excluded vuejs/core's entire
// source tree, 569 of its 705 files, and took TypeScript survivors from 799 to
// one, in a change whose message said only that matching moved from substrings
// to segments.
var excluded = []string{
	"vendor", "node_modules", "third_party", "thirdparty", "external",
	"testdata", "fixtures", "generated", "dist", "build",
	"outputs", "site-packages",
}

// suffixes name generated files by their extension rather than by where they sit.
var suffixes = []string{".pb.go", "_pb2.py", ".min.js", ".pb.cc", "_generated.go"}

// skip reports whether path is one this harvest does not read.
//
// Matched a segment at a time. Matching a substring excluded tokio's whole
// `tests-build/` tree on the strength of `build/`, and a leading slash on one
// entry and not the others meant a top-level `generated/` was read while a
// nested one was not.
func skip(path string) bool {
	lower := strings.ToLower(path)
	for _, suffix := range suffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	for segment := range strings.SplitSeq(lower, "/") {
		for _, name := range excluded {
			if segment == name {
				return true
			}
		}
	}
	return false
}

// deleted returns the comments this commit removed while leaving the code they
// annotated in place.
//
// The pair of parses is what makes the label trustworthy. A comment gone from
// the child is not yet evidence, because deleting a function deletes its doc
// comment too and says nothing about the comment; requiring the annotated code
// to still be there is what separates a judgement from a sweep.
func deleted(dir string, repo Repo, c commit, store *corpus.Blobs) ([]corpus.Row, error) {
	paths, err := touched(dir, c.sha)
	if err != nil {
		// A commit whose parent is outside a shallow clone cannot be read, and
		// that is the boundary rather than a failure.
		return nil, nil
	}
	var rows []corpus.Row
	for _, path := range paths {
		language := lang.Lookup(path)
		before, err := store.Read(c.sha + "^:" + path)
		if err != nil {
			return nil, err
		}
		after, err := store.Read(c.sha + ":" + path)
		if err != nil {
			return nil, err
		}
		if len(before) == 0 || len(after) == 0 || len(before) > maxFile || len(after) > maxFile {
			continue
		}
		rows = append(rows, gone(dir, repo, c, path, language, before, after)...)
	}
	return rows, nil
}

// gone compares one file's two versions and returns the rows for the comments
// that left it.
func gone(dir string, repo Repo, c commit, path string, language *lang.Language, before, after []byte) []corpus.Row {
	old, releaseOld := comment.ScanAll(before, language)
	defer releaseOld()
	if len(old) == 0 {
		return nil
	}
	current, releaseNew := comment.ScanAll(after, language)
	defer releaseNew()

	// held counts the comment nodes the child still carries, keyed one node at a
	// time rather than one run at a time.
	//
	// A run is what [comment.ScanAll] groups, so keying on it means adding a
	// single line beside an existing comment changes the key of the comment
	// nobody touched, and that untouched comment reads as deleted. Measured on
	// the first corpus, 7.2% of the deleted class was still present in the child
	// verbatim, and this was how.
	held := make(map[string]int, len(current))
	// documented is the code that still carries a comment of some kind after the
	// commit.
	//
	// It is what separates a deletion from a rewording, and without it the
	// harvest is worthless: a doc comment edited to add a link or to be
	// reflowed has prose that is gone from the child and code that survives,
	// so it satisfies every other test here while being evidence that somebody
	// cared about the comment rather than that they judged it.
	// It is a list rather than a set because it has to answer the same question
	// the survival test asks, and that test accepts a substring. Keyed for exact
	// equality it disagreed with survival in one direction only: when the
	// annotated code grew, `Contains` passed on the old text while the lookup
	// missed on the new, so a rewording came out as a deletion.
	var documented []string
	for _, one := range current {
		for _, node := range one.Nodes {
			held[corpus.Flat(node.Utf8Text(after))]++
		}
		if one.Annotates != nil {
			documented = append(documented, corpus.Flat(one.Annotates.Utf8Text(after)))
		}
	}
	// The haystacks are each version's code with every comment blanked. Code a
	// commit commented out is still present as bytes, so testing against the
	// whole file reports it as having survived the very commit that switched it
	// off. The needle is read out of the same blanked bytes, so that a node
	// carrying a comment inside it is still comparable.
	blanked := bare(before, old)
	survived := corpus.Flat(string(bare(after, current)))
	previous := corpus.Flat(string(blanked))

	var dropped int
	var rows []corpus.Row
	for _, one := range old {
		if standing(one, before, held) {
			continue
		}
		dropped++
		text := corpus.Flat(one.Text)
		if !corpus.Prose(text) || one.Annotates == nil {
			continue
		}
		code := spans(blanked, one.Annotates)
		if len(code) < annotatedFloor {
			continue
		}
		// A needle absent from its own parent means the test cannot run, and a
		// row it never ran on carries no established label.
		stood := strings.Count(previous, code)
		if stood == 0 {
			continue
		}
		// Occurrences rather than membership: a short node recurs, so asking
		// whether the text is present answers yes on a different instance of it.
		if strings.Count(survived, code) < stood {
			continue
		}
		if rewritten(documented, code) {
			continue
		}
		row := corpus.Row{
			Repo:      repo.Name,
			License:   repo.License,
			Path:      path,
			Language:  language.Name,
			Label:     corpus.Deleted,
			Text:      one.Text,
			Body:      one.Body,
			Annotates: corpus.Truncate(code, 400),
			Removed:   c.sha,
			RemovedAt: c.when,
			Doc:       one.Doc,
			Trailing:  one.Trailing,
			Buried:    one.Buried,
			Lines:     len(one.Nodes),
			Line:      one.Line,
			CodeFrom:  one.Annotates.StartPosition().Row + 1,
			CodeTo:    one.Annotates.EndPosition().Row + 1,
		}
		if born, ok := corpus.BlameLine(dir, c.sha+"^", path, one.Line); ok {
			row.Added, row.AddedAt = born.SHA, born.When
			row.LifetimeDays = c.when.Sub(born.When).Hours() / 24
			row.Dated = true
		}
		rows = append(rows, row)
	}
	// Counted over every comment that left the file, not over the rows that
	// survived the other filters. Gating on the rows lets a mass refactor
	// contribute up to [burst] of them however many comments it actually swept:
	// measured on the first corpus, 21% of contributing file-commits had dropped
	// more than three comments, one of them 399.
	if dropped > burst {
		return nil
	}
	return rows
}

// rewritten reports whether some comment in the child still documents this
// code, allowing for the code having grown or shrunk around it.
func rewritten(documented []string, code string) bool {
	for _, still := range documented {
		if strings.Contains(still, code) || strings.Contains(code, still) {
			return true
		}
	}
	return false
}

// standing reports whether every node of a comment is still in the child, and
// consumes the ones it matched so that two identical comments need two of them.
func standing(one comment.Comment, src []byte, held map[string]int) bool {
	var matched []string
	for _, node := range one.Nodes {
		text := corpus.Flat(node.Utf8Text(src))
		if held[text] == 0 {
			for _, back := range matched {
				held[back]++
			}
			return false
		}
		held[text]--
		matched = append(matched, text)
	}
	return true
}

// bare returns src with every comment blanked, so that what is left is the code
// alone. Whitespace of the same width replaces the comment, which keeps every
// byte offset addressing the file it came from.
func bare(src []byte, comments []comment.Comment) []byte {
	out := append([]byte(nil), src...)
	for _, one := range comments {
		for _, node := range one.Nodes {
			for i := node.StartByte(); i < node.EndByte() && i < uint(len(out)); i++ {
				if out[i] != '\n' {
					out[i] = ' '
				}
			}
		}
	}
	return out
}

// spans returns the flattened text of one node, read out of a source whose
// comments have already been blanked.
//
// Reading it out of the raw source instead is what made the survival test
// vacuous: an annotated node that carries a comment inside it produced a needle
// that could not occur in either haystack, both counts came out zero, and
// `0 < 0` admitted the row without the test ever being evaluated. Measured on
// the shipped corpus that was 70.1% of the positive class, and 87% of its Rust.
func spans(blanked []byte, node *tree_sitter.Node) string {
	start, end := node.StartByte(), node.EndByte()
	if end > uint(len(blanked)) {
		end = uint(len(blanked))
	}
	if start >= end {
		return ""
	}
	return corpus.Flat(string(blanked[start:end]))
}
