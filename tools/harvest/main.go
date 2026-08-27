// Command harvest mines public repositories for comments their own authors
// judged, and writes them as a labelled corpus.
//
// The labels are not this project's opinion, which is the whole point of it. A
// comment deleted in a commit that kept the code under it was judged weak by
// the person who wrote it; a comment still standing after many later edits to
// the same file was left alone by everyone who read it since. Both labels are
// somebody else's, recorded with the commits that prove them, so a corpus built
// here measures the tool against a taste it did not come from.
//
// Usage:
//
//	harvest -clones ~/.cache/slopguard-harvest -out corpus.jsonl
//
// Every row carries its repository, licence and commits. Nothing is embedded in
// the binary from here without that provenance travelling with it.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mikluko/slopguard/internal/corpus"
)

func main() {
	var (
		clones  = flag.String("clones", filepath.Join(os.TempDir(), "slopguard-harvest"), "directory the repositories are cloned into")
		out     = flag.String("out", "corpus.jsonl", "where the corpus is written")
		commits = flag.Int("commits", 3000, "how many recent commits of each repository to read for deletions")
		files   = flag.Int("files", 400, "how many files of each repository to read for survivors")
		depth   = flag.Int("depth", 4000, "how much history to clone; zero clones all of it")
		only    = flag.String("only", "", "harvest only the repository with this name")
		keep    = flag.Int("keep", 800, "how many survived rows to take from one repository; zero takes all of them")
	)
	flag.Parse()

	if err := run(*clones, *out, *only, *commits, *files, *depth, *keep); err != nil {
		fmt.Fprintln(os.Stderr, "harvest:", err)
		os.Exit(1)
	}
}

func run(clones, out, only string, commits, files, depth, keep int) error {
	if err := os.MkdirAll(clones, 0o755); err != nil {
		return err
	}
	file, err := os.Create(out)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := json.NewEncoder(file)

	var wrote, dropped int
	for _, repo := range defaults {
		if only != "" && repo.Name != only {
			continue
		}
		started := time.Now()
		dir := filepath.Join(clones, repo.Dir())
		if err := fetch(repo, dir, depth); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", repo.Name, err)
			continue
		}
		rows, err := harvest(dir, repo, commits, files)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", repo.Name, err)
			continue
		}
		rows = expose(dir, balance(distinct(rows), keep))
		var kept int
		for _, row := range rows {
			if !clean(row.Text) {
				dropped++
				continue
			}
			if err := writer.Encode(row); err != nil {
				return err
			}
			kept++
		}
		wrote += kept
		fmt.Fprintf(os.Stderr, "%-42s %5d rows  %s\n", repo.Name, kept, time.Since(started).Round(time.Second))
	}
	fmt.Fprintf(os.Stderr, "\n%d rows written to %s, %d dropped as boilerplate\n", wrote, out, dropped)
	return nil
}

// seen is how many commits have to have touched the code under a survived
// comment, after the comment was written, before it counts as one somebody read
// and left.
//
// One, because the window already opens at the commit that wrote the comment.
// It was two while the count ran over all of history on the comment's own line,
// where the newest commit is the one that wrote the text standing there, so most
// of what it counted happened before the comment existed.
const seen = 1

// expose measures how much attention each survived row's own line has had, and
// drops the rows nobody has been back to.
//
// It runs after the cap rather than before because it costs a git process per
// row. The cap is on the negatives, and the negatives are not the binding
// constraint on this corpus: there are twenty-seven of them per positive, so
// spending accuracy on the abundant side is the right trade.
func expose(dir string, rows []corpus.Row) []corpus.Row {
	kept := make([]corpus.Row, 0, len(rows))
	for _, row := range rows {
		// Both labels, at the revision each was read from. Computing it for
		// survivors alone made the field separate the two classes perfectly, so
		// a one-line rule on it scored recall 1.0 at zero false positives: proof
		// that the negative class was filtered on an axis the positive one was
		// not. A deleted comment's exposure is how many commits touched its line
		// before somebody removed it, which is the same question.
		if row.CodeFrom == 0 || row.Added == "" {
			continue
		}
		edits, err := corpus.LineEdits(dir, row.Added, row.Rev(), row.Path, row.CodeFrom, row.CodeTo)
		if err != nil {
			continue
		}
		row.Exposure = edits
		if row.Label == corpus.Survived && edits < seen {
			continue
		}
		kept = append(kept, row)
	}
	return kept
}

// repeats is how many times one wording may come out of a single repository.
//
// A comment written once and copied into every file is one person's decision,
// not many, and it arrives with the weight of many. Measured on the first
// corpus, 9.3% of all rows were a text repeated at least three times inside one
// repository: `# use clap_builder as clap;` alone was 308 rows, 38.6% of clap's
// survived class, and one date-fns pragma was 52 rows of the deleted class.
const repeats = 2

// distinct caps how often one wording may repeat within a repository, keeping
// the earliest occurrences.
func distinct(rows []corpus.Row) []corpus.Row {
	seen := map[string]int{}
	kept := make([]corpus.Row, 0, len(rows))
	for _, row := range rows {
		key := row.Label + "\x00" + corpus.Flat(row.Text)
		if seen[key] >= repeats {
			continue
		}
		seen[key]++
		kept = append(kept, row)
	}
	return kept
}

// balance caps what one repository contributes to each label.
//
// Both sides, and that is a correction. Deleted rows went uncapped on the
// grounds that they are scarce everywhere, which stopped being true once the
// mining rules were repaired: tokio then supplied 28% of the whole positive
// class, and the tool catches one of its 250. Leaving it out moved the tool's
// measured lift from 1.25 to 1.58, which is a corpus deciding the answer.
//
// The survived side needs it for the older reason. Rust documents every public
// item with `///`, so before any cap tokio produced 14,646 survived rows against
// grpc-go's 2,600. That ratio is a fact about the language rather than the code.
func balance(rows []corpus.Row, keep int) []corpus.Row {
	if keep <= 0 {
		return rows
	}
	var deleted, survived []corpus.Row
	for _, row := range rows {
		if row.Label == corpus.Deleted {
			deleted = append(deleted, row)
			continue
		}
		survived = append(survived, row)
	}
	// The positive cap is a quarter of the negative one, which keeps the ratio
	// a repository contributes near the corpus's own.
	if positives := keep / 4; len(deleted) > positives {
		taken := make([]corpus.Row, 0, positives)
		for i := range positives {
			taken = append(taken, deleted[i*len(deleted)/positives])
		}
		deleted = taken
	}
	if len(survived) <= keep {
		return append(deleted, survived...)
	}
	// Indexed rather than stepped. `len/keep` is 1 for any repository yielding
	// between one and two times the cap, which took the first `keep` rows in
	// file order and stopped: bbolt's eight hundred ended at `tx.go` and seven
	// parsable files after it contributed nothing.
	out := deleted
	for i := range keep {
		out = append(out, survived[i*len(survived)/keep])
	}
	return out
}

// fetch clones the repository if it is not there and updates it if it is. A
// bounded depth is enough: the deletions are sampled from recent history, and a
// full clone of every repository in the set costs tens of gigabytes.
func fetch(repo Repo, dir string, depth int) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		_, err := corpus.Git(dir, "fetch", "--quiet", "origin")
		return err
	}
	// No template directory. A mining clone is read-only, so the sample hooks
	// git copies in are dead weight, and an environment that refuses to let
	// anything write a git hook refuses the whole clone over them.
	args := []string{"clone", "--quiet", "--single-branch", "--no-tags", "--template="}
	if depth > 0 {
		args = append(args, "--depth", strconv.Itoa(depth))
	}
	args = append(args, repo.URL(), dir)
	_, err := corpus.Git(".", args...)
	return err
}

// harvest reads one repository both ways and returns every row it yields.
func harvest(dir string, repo Repo, commits, files int) ([]corpus.Row, error) {
	store, err := corpus.OpenBlobs(dir)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	var rows []corpus.Row

	history, err := sample(dir, commits)
	if err != nil {
		return nil, err
	}
	for _, c := range history {
		found, err := deleted(dir, repo, c, store)
		if err != nil {
			return nil, err
		}
		rows = append(rows, found...)
	}

	paths, err := readable(dir)
	if err != nil {
		return nil, err
	}
	step := stride(len(paths), files)
	for i := 0; i < len(paths); i += step {
		found, err := survived(dir, repo, paths[i], store)
		if err != nil {
			return nil, err
		}
		rows = append(rows, found...)
	}
	return rows, nil
}
