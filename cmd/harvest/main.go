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
	)
	flag.Parse()

	if err := run(*clones, *out, *only, *commits, *files, *depth); err != nil {
		fmt.Fprintln(os.Stderr, "harvest:", err)
		os.Exit(1)
	}
}

func run(clones, out, only string, commits, files, depth int) error {
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

// fetch clones the repository if it is not there and updates it if it is. A
// bounded depth is enough: the deletions are sampled from recent history, and a
// full clone of every repository in the set costs tens of gigabytes.
func fetch(repo Repo, dir string, depth int) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		_, err := corpus.Git(dir, "fetch", "--quiet", "origin")
		return err
	}
	args := []string{"clone", "--quiet", "--single-branch", "--no-tags"}
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
