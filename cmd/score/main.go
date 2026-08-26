// Command score runs a build of the rules over the mined corpus and reports
// what it caught and what it nudged, under the metric in docs/metric.md.
//
// The figures are recall on the comments people deleted and false-positive rate
// on the comments people kept. Precision is deliberately absent: it is a
// function of how many rows of each label the harvest happened to produce, so
// it moves when the mining thresholds move and says nothing about the tool.
//
// Usage:
//
//	score -corpus corpus.jsonl -clones ~/.cache/slopguard-harvest
//
// Every comment is judged by replaying the file it came from through the same
// rules the hook runs, at the line the corpus recorded. Judging the prose alone
// would skip every rule that reads the code beside a comment.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mikluko/slopguard/internal/comment"
	"github.com/mikluko/slopguard/internal/corpus"
	"github.com/mikluko/slopguard/internal/lang"
	"github.com/mikluko/slopguard/internal/rule"
)

// tilts are the threshold offsets the sweep runs at. A positive offset lowers
// every semantic threshold, so the classes fire on more; the structural rules
// carry no threshold and hold still across the whole sweep, which is why the
// curve never reaches the origin.
var tilts = []float64{
	-0.10, -0.06, -0.04, -0.02, 0.00, 0.02, 0.04, 0.06,
	0.08, 0.10, 0.14, 0.18, 0.24, 0.32, 0.45,
}

func main() {
	var (
		corpusPath = flag.String("corpus", "corpus.jsonl", "the mined corpus")
		clones     = flag.String("clones", filepath.Join(os.TempDir(), "slopguard-harvest"), "where the repositories were cloned")
		sweep      = flag.Bool("sweep", true, "trace the curve as well as the shipped operating point")
	)
	flag.Parse()

	if err := run(*corpusPath, *clones, *sweep); err != nil {
		fmt.Fprintln(os.Stderr, "score:", err)
		os.Exit(1)
	}
}

// A verdict is what one build made of one row.
type verdict struct {
	row   corpus.Row
	fired bool
	class string
}

func run(corpusPath, clones string, sweep bool) error {
	rows, err := corpus.Load(corpusPath)
	if err != nil {
		return err
	}
	offsets := []float64{0}
	if sweep {
		offsets = tilts
	}

	judged, missed, err := judge(rows, clones, offsets)
	if err != nil {
		return err
	}

	var deleted, survived int
	for _, v := range judged[0] {
		if v.row.Label == corpus.Deleted {
			deleted++
		} else {
			survived++
		}
	}
	fmt.Printf("# Baseline on the mined corpus\n\n")
	fmt.Printf("%d rows scored: %d deleted, %d survived. %d rows could not be scored.\n\n",
		deleted+survived, deleted, survived, missed)
	if deleted == 0 || survived == 0 {
		return fmt.Errorf("a corpus with only one label cannot be scored")
	}

	shipped := at(offsets, 0)
	fmt.Printf("## The shipped build\n\n")
	report(judged[shipped])

	fmt.Printf("\n## Per class, at the shipped thresholds\n\n")
	perClass(judged[shipped], deleted, survived)

	if sweep {
		fmt.Printf("\n## The curve\n\n")
		curve(judged, offsets, deleted, survived)
	}
	return nil
}

// judge scores every row at every offset, returning one verdict slice per
// offset and how many rows could not be reached.
//
// The file is read and parsed once per offset set rather than once per offset:
// the rules are re-run over the same candidates, which is where the cost is,
// and re-reading the blob would add nothing but latency.
func judge(rows []corpus.Row, clones string, offsets []float64) ([][]verdict, int, error) {
	out := make([][]verdict, len(offsets))
	byRepo := map[string][]corpus.Row{}
	for _, row := range rows {
		byRepo[row.Repo] = append(byRepo[row.Repo], row)
	}
	names := make([]string, 0, len(byRepo))
	for name := range byRepo {
		names = append(names, name)
	}
	sort.Strings(names)

	missed := 0
	for _, name := range names {
		dir := filepath.Join(clones, strings.ReplaceAll(name, "/", "_"))
		store, err := corpus.OpenBlobs(dir)
		if err != nil {
			missed += len(byRepo[name])
			continue
		}
		byFile := map[string][]corpus.Row{}
		for _, row := range byRepo[name] {
			byFile[row.Rev()+"\x00"+row.Path] = append(byFile[row.Rev()+"\x00"+row.Path], row)
		}
		keys := make([]string, 0, len(byFile))
		for key := range byFile {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			rev, path, _ := strings.Cut(key, "\x00")
			found, gone := file(store, rev, path, byFile[key], offsets)
			missed += gone
			for i := range offsets {
				out[i] = append(out[i], found[i]...)
			}
		}
		store.Close()
		fmt.Fprintf(os.Stderr, "scored %s\n", name)
	}
	return out, missed, nil
}

// file scores the rows belonging to one revision of one path.
func file(store *corpus.Blobs, rev, path string, rows []corpus.Row, offsets []float64) ([][]verdict, int) {
	out := make([][]verdict, len(offsets))
	src, err := store.Read(rev + ":" + path)
	if err != nil || len(src) == 0 {
		return out, len(rows)
	}
	language := lang.Lookup(path)
	if language == nil {
		return out, len(rows)
	}
	all, release := comment.ScanAll(src, language)
	defer release()

	// Only the comments the corpus recorded are judged. The rest of the file's
	// comments would be embedded for nothing, and on a corpus this size that is
	// most of the run.
	wanted := map[uint]corpus.Row{}
	for _, row := range rows {
		wanted[row.Line] = row
	}
	var candidates []comment.Comment
	for _, one := range all {
		if _, ok := wanted[one.Line]; ok {
			candidates = append(candidates, one)
		}
	}
	if len(candidates) == 0 {
		return out, len(rows)
	}

	for i, offset := range offsets {
		flagged := map[uint]rule.Finding{}
		for _, finding := range rule.WeighAt(candidates, language, src, offset) {
			flagged[finding.Line] = finding
		}
		for _, one := range candidates {
			row := wanted[one.Line]
			finding, fired := flagged[one.Line]
			out[i] = append(out[i], verdict{row: row, fired: fired, class: finding.Class})
		}
	}
	return out, len(rows) - len(candidates)
}

// rates returns recall on the deleted rows and false-positive rate on the
// survived ones.
func rates(verdicts []verdict) (recall, fpr float64, caught, nudged int) {
	var deleted, survived int
	for _, v := range verdicts {
		if v.row.Label == corpus.Deleted {
			deleted++
			if v.fired {
				caught++
			}
			continue
		}
		survived++
		if v.fired {
			nudged++
		}
	}
	if deleted > 0 {
		recall = float64(caught) / float64(deleted)
	}
	if survived > 0 {
		fpr = float64(nudged) / float64(survived)
	}
	return recall, fpr, caught, nudged
}

func report(verdicts []verdict) {
	recall, fpr, caught, nudged := rates(verdicts)
	fmt.Printf("| figure | value |\n|---|---|\n")
	fmt.Printf("| recall on deleted | %.3f (%d caught) |\n", recall, caught)
	fmt.Printf("| false-positive rate on survived | %.3f (%d nudged) |\n", fpr, nudged)
}

func perClass(verdicts []verdict, deleted, survived int) {
	caught := map[string]int{}
	nudged := map[string]int{}
	for _, v := range verdicts {
		if !v.fired {
			continue
		}
		if v.row.Label == corpus.Deleted {
			caught[v.class]++
		} else {
			nudged[v.class]++
		}
	}
	classes := map[string]bool{}
	for name := range caught {
		classes[name] = true
	}
	for name := range nudged {
		classes[name] = true
	}
	names := make([]string, 0, len(classes))
	for name := range classes {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Printf("| class | recall on deleted | FPR on survived | caught | nudged |\n|---|---|---|---|---|\n")
	for _, name := range names {
		fmt.Printf("| %s | %.3f | %.3f | %d | %d |\n", name,
			float64(caught[name])/float64(deleted),
			float64(nudged[name])/float64(survived),
			caught[name], nudged[name])
	}
}

func curve(judged [][]verdict, offsets []float64, deleted, survived int) {
	type point struct {
		offset, recall, fpr float64
	}
	var points []point
	fmt.Printf("| offset | recall | FPR |\n|---|---|---|\n")
	for i, offset := range offsets {
		recall, fpr, _, _ := rates(judged[i])
		points = append(points, point{offset, recall, fpr})
		fmt.Printf("| %+.2f | %.3f | %.4f |\n", offset, recall, fpr)
	}
	sort.Slice(points, func(a, b int) bool { return points[a].fpr < points[b].fpr })

	fmt.Printf("\nRecall at the operating points the metric names:\n\n")
	for _, target := range []float64{0.01, 0.02, 0.05} {
		best, found := 0.0, false
		for _, p := range points {
			if p.fpr <= target && p.recall > best {
				best, found = p.recall, true
			}
		}
		if !found {
			fmt.Printf("- FPR %.2f: not reachable; the structural rules alone nudge more than this.\n", target)
			continue
		}
		fmt.Printf("- FPR %.2f: recall %.3f\n", target, best)
	}

	// Partial AUC over the region the hook could actually be shipped in,
	// normalised so that 1.0 is perfect over that region alone.
	area, previous := 0.0, point{}
	for i, p := range points {
		if i > 0 && p.fpr <= 0.05 {
			area += (p.fpr - previous.fpr) * (p.recall + previous.recall) / 2
		}
		if p.fpr > 0.05 {
			break
		}
		previous = p
	}
	fmt.Printf("\nPartial AUC over FPR in [0, 0.05], normalised: %.3f\n", area/0.05)
}

// at returns the index of the offset equal to want.
func at(offsets []float64, want float64) int {
	for i, offset := range offsets {
		if offset == want {
			return i
		}
	}
	return 0
}
