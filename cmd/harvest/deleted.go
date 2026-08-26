package main

import (
	"strconv"
	"strings"
	"time"

	"github.com/mikluko/slopguard/internal/comment"
	"github.com/mikluko/slopguard/internal/corpus"
	"github.com/mikluko/slopguard/internal/lang"
)

// annotatedFloor is how much code has to sit under a comment before its
// survival can be checked. A three-character node occurs everywhere in a file,
// so a shorter one would report the code as surviving whatever the commit did.
const annotatedFloor = 24

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
var excluded = []string{
	"vendor/", "node_modules/", "third_party/", "thirdparty/", "external/",
	"testdata/", "fixtures/", "/generated/", ".pb.go", "_pb2.py", ".min.js",
	"dist/", "build/",
}

// skip reports whether path is one this harvest does not read.
func skip(path string) bool {
	lower := strings.ToLower(path)
	for _, segment := range excluded {
		if strings.Contains(lower, segment) {
			return true
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
		if len(before) == 0 || len(after) == 0 || len(before) > maxFile {
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

	held := make(map[string]int, len(current))
	// documented is the code that still carries a comment of some kind after the
	// commit, keyed the same way survival is checked.
	//
	// It is what separates a deletion from a rewording, and without it the
	// harvest is worthless: a doc comment edited to add a link or to be
	// reflowed has prose that is gone from the child and code that survives,
	// so it satisfies every other test here while being evidence that somebody
	// cared about the comment rather than that they judged it.
	documented := make(map[string]bool, len(current))
	for _, one := range current {
		held[corpus.Flat(one.Text)]++
		if one.Annotates != nil {
			documented[corpus.Flat(one.Annotates.Utf8Text(after))] = true
		}
	}
	survived := corpus.Flat(string(after))

	var rows []corpus.Row
	for _, one := range old {
		text := corpus.Flat(one.Text)
		if held[text] > 0 {
			held[text]--
			continue
		}
		if !corpus.Prose(text) || one.Annotates == nil {
			continue
		}
		code := corpus.Flat(one.Annotates.Utf8Text(before))
		if len(code) < annotatedFloor || !strings.Contains(survived, code) {
			continue
		}
		if documented[code] {
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
		}
		if born, ok := corpus.BlameLine(dir, c.sha+"^", path, one.Line); ok {
			row.Added, row.AddedAt = born.SHA, born.When
			row.LifetimeDays = c.when.Sub(born.When).Hours() / 24
		}
		rows = append(rows, row)
	}
	if len(rows) > burst {
		return nil
	}
	return rows
}
