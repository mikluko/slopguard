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
	"regexp"
	"sort"
	"strings"

	"github.com/mikluko/slopguard/internal/comment"
	"github.com/mikluko/slopguard/internal/corpus"
	"github.com/mikluko/slopguard/internal/lang"
	"github.com/mikluko/slopguard/internal/model"
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
		dump       = flag.String("dump", "", "print every row this class fired on, with its label; \"all\" prints them for every class")
		maxLife    = flag.Float64("maxlife", 0, "keep only deletions this many days old or younger; zero keeps all of them")
		markers    = flag.Bool("markers", true, "count deletions of TODO, FIXME, XXX and HACK comments")
		matched    = flag.Bool("matched", false, "keep only deletions whose annotated code somebody came back to, as the survived rows require")
	)
	flag.Parse()

	if err := run(*corpusPath, *clones, *sweep, *dump, *maxLife, *markers, *matched); err != nil {
		fmt.Fprintln(os.Stderr, "score:", err)
		os.Exit(1)
	}
}

// recent drops the deletions older than days, leaving the survived rows alone.
//
// It exists because how long a comment stood before somebody removed it is the
// best single predictor of whether the removal was a judgement about the
// comment. Hand-reading forty deletions spread across the whole corpus, about
// half were good comments removed for other reasons: JSDoc blocks that moved to
// a generator, pragmas, contract notes carried off by a restructuring. Reading
// thirty from the rows deleted within ninety days, that share fell to between
// one in ten and one in five, and what remained were TODOs, FIXMEs,
// commented-out lines and step narration.
//
// The reason is that an old comment and its code drift apart, so an old
// deletion is more often a consequence of the code changing than a verdict on
// the prose. A young one is somebody reading what was just written and taking
// it out.
func recent(rows []corpus.Row, days float64) []corpus.Row {
	if days <= 0 {
		return rows
	}
	kept := make([]corpus.Row, 0, len(rows))
	for _, row := range rows {
		// Dated rather than a positive lifetime: a comment written and removed
		// inside one committer-second has a lifetime of zero, and testing the
		// number discarded exactly the rows this filter exists to keep.
		if row.Label == corpus.Deleted && (!row.Dated || row.LifetimeDays > days) {
			continue
		}
		kept = append(kept, row)
	}
	return kept
}

// A verdict is what one build made of one row.
type verdict struct {
	row   corpus.Row
	fired bool
	class string
}

// misses counts the rows that could not be scored, by why.
//
// The count is reported rather than merely subtracted. A corpus mined by one
// build of the harvester and scored by another can fail to line up completely,
// which reads as a clean run over nothing at all unless the reasons are printed
// beside the total.
type misses struct {
	repo      int
	blob      int
	language  int
	unmatched int
}

func (m misses) total() int { return m.repo + m.blob + m.language + m.unmatched }

func (m misses) String() string {
	return fmt.Sprintf("%d unreachable clone, %d missing blob, %d unknown language, %d no comment at the recorded line",
		m.repo, m.blob, m.language, m.unmatched)
}

// marked matches the debt conventions Google's C++ and Java guides mandate.
// Their deletion is the debt being repaid rather than a verdict on the comment,
// and they are 14.9% of young deletions against 0.90% of survivors, so counting
// them as recall the tool failed to earn flatters a one-line regex into beating
// the whole pipeline.
var marked = regexp.MustCompile(`\b(TODO|FIXME|XXX|HACK)\b`)

func run(corpusPath, clones string, sweep bool, dump string, maxLife float64, markers, matched bool) error {
	rows, err := corpus.Load(corpusPath)
	if err != nil {
		return err
	}
	// Without the model the semantic classes fall back to a phrase list that
	// ignores the bias entirely, so every offset scores the same and the sweep
	// prints a flat line as though it were a curve. Say so at the top rather
	// than letting a reader take the tables at face value.
	absent := model.Absent()
	if absent != "" {
		fmt.Printf("> **The semantic pass did not run: %s.**\n"+
			"> Only the structural rules are measured below, and the sweep is flat by construction.\n\n", absent)
	}
	rows = recent(rows, maxLife)
	if matched {
		// The survived label requires that somebody came back to the annotated
		// code and left the comment standing. Nothing requires it of a deletion,
		// so `exposure == 0` holds on part of the positive class and on no
		// negative at all, and a rule reading it would separate the two at no
		// cost. Requiring it of both makes the classes answer one question:
		// somebody returned to this code, and then they did or did not remove
		// the comment. It costs most of the positives, which is why it is a
		// check on the headline rather than the headline.
		kept := rows[:0]
		dropped := 0
		for _, row := range rows {
			if row.Label == corpus.Deleted && row.Exposure == 0 {
				dropped++
				continue
			}
			kept = append(kept, row)
		}
		rows = kept
		fmt.Printf("%d deletions nobody came back to left out.\n\n", dropped)
	}
	if !markers {
		kept := rows[:0]
		dropped := 0
		for _, row := range rows {
			if row.Label == corpus.Deleted && marked.MatchString(row.Text) {
				dropped++
				continue
			}
			kept = append(kept, row)
		}
		rows = kept
		fmt.Printf("%d marker deletions left out.\n\n", dropped)
	}
	offsets := []float64{0}
	if sweep {
		offsets = tilts
	}

	judged, lost, err := judge(rows, clones, offsets, "")
	if err != nil {
		return err
	}
	// A corpus mined before the harvester recorded line numbers scores nothing
	// and looks like a clean run over an empty set, which is the one failure
	// here that is invisible in the output.
	if lost.unmatched == len(rows) {
		return fmt.Errorf("no row matched a comment at its recorded line: %d rows, all unmatched — regrow the corpus with the current harvester", len(rows))
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
	fmt.Printf("%d rows scored: %d deleted, %d survived.\n\n%d rows could not be scored: %s.\n\n",
		deleted+survived, deleted, survived, lost.total(), lost)
	if deleted == 0 || survived == 0 {
		return fmt.Errorf("a corpus with only one label cannot be scored")
	}

	shipped := at(offsets, 0)
	fmt.Printf("## The shipped build\n\n")
	report(judged[shipped])

	fmt.Printf("\n## Per class, as a share of one run\n\n")
	perClass(judged[shipped], deleted, survived)

	fmt.Printf("\n## Per class, each measured with the others off\n\n")
	alone(rows, clones, deleted, survived)

	if sweep {
		fmt.Printf("\n## The curve\n\n")
		curve(judged, offsets, deleted, survived)
	}
	if dump != "" {
		fmt.Printf("\n## What fired\n\n")
		fired(judged[shipped], dump)
	}
	return nil
}

// fired prints the rows one class nudged, label first, so that a false-positive
// rate can be read as text rather than trusted as a number. The label is what
// makes it worth printing: a class nudging a comment somebody kept is where the
// noise in the corpus and the noise in the rule are hardest to tell apart, and
// only reading them separates the two.
func fired(verdicts []verdict, want string) {
	for _, v := range verdicts {
		if !v.fired || (want != "all" && v.class != want) {
			continue
		}
		text := v.row.Text
		if len(text) > 96 {
			text = corpus.Truncate(text, 96)
		}
		fmt.Printf("%-9s %-9s %s:%d\t%s\n", v.row.Label, v.class, v.row.Path, v.row.Line, text)
	}
}

// judge scores every row at every offset, returning one verdict slice per
// offset and how many rows could not be reached.
//
// The file is read and parsed once per offset set rather than once per offset:
// the rules are re-run over the same candidates, which is where the cost is,
// and re-reading the blob would add nothing but latency.
func judge(rows []corpus.Row, clones string, offsets []float64, only string) ([][]verdict, misses, error) {
	out := make([][]verdict, len(offsets))
	var lost misses
	byRepo := map[string][]corpus.Row{}
	for _, row := range rows {
		byRepo[row.Repo] = append(byRepo[row.Repo], row)
	}
	names := make([]string, 0, len(byRepo))
	for name := range byRepo {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		dir := filepath.Join(clones, strings.ReplaceAll(name, "/", "_"))
		store, err := corpus.OpenBlobs(dir)
		if err != nil {
			lost.repo += len(byRepo[name])
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
			found := file(store, rev, path, byFile[key], offsets, &lost, only)
			for i := range offsets {
				out[i] = append(out[i], found[i]...)
			}
		}
		store.Close()
		fmt.Fprintf(os.Stderr, "scored %s\n", name)
	}
	return out, lost, nil
}

// file scores the rows belonging to one revision of one path.
func file(store *corpus.Blobs, rev, path string, rows []corpus.Row, offsets []float64, lost *misses, only string) [][]verdict {
	out := make([][]verdict, len(offsets))
	src, err := store.Read(rev + ":" + path)
	if err != nil || len(src) == 0 {
		lost.blob += len(rows)
		return out
	}
	language := lang.Lookup(path)
	if language == nil {
		lost.language += len(rows)
		return out
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
	// Counted over the rows rather than over the comments matched: a trailing
	// comment can share a start line with the one above it, so two candidates
	// can answer for one row and a difference of lengths goes negative.
	var candidates []comment.Comment
	found := make(map[uint]bool, len(wanted))
	for _, one := range all {
		if _, ok := wanted[one.Line]; ok {
			candidates = append(candidates, one)
			found[one.Line] = true
		}
	}
	for _, row := range rows {
		if !found[row.Line] {
			lost.unmatched++
		}
	}
	if len(candidates) == 0 {
		return out
	}

	for i, offset := range offsets {
		flagged := map[uint]rule.Finding{}
		for _, finding := range rule.WeighOnly(candidates, language, src, offset, only) {
			// Keep the strongest finding on a line rather than the last, since
			// a trailing comment can share a start line with the one above it.
			if held, taken := flagged[finding.Line]; taken && held.Score >= finding.Score {
				continue
			}
			flagged[finding.Line] = finding
		}
		// One verdict per row, not per comment. Two comments can share a start
		// line, and iterating the comments then credited that row's label twice
		// and moved the denominator off the corpus.
		for _, row := range rows {
			if !found[row.Line] {
				continue
			}
			finding, fired := flagged[row.Line]
			out[i] = append(out[i], verdict{row: row, fired: fired, class: finding.Class})
		}
	}
	return out
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

// classes are the rules a run can be restricted to, in the order `inspect`
// tries them.
var classes = []string{"leftover", "echo", "hollow", "tautology", "compat"}

// alone reports each class measured with the others switched off, which is what
// the class costs rather than what it adds on top of whatever ran before it.
//
// The table beside this one is a partition: a comment gets one verdict and the
// rules run in a fixed precedence, so `tautology` only ever sees what `leftover`
// and `echo` declined. Read as a ranking that understates every rule but the
// first, and it has been read that way in this repository more than once.
func alone(rows []corpus.Row, clones string, deleted, survived int) {
	fmt.Printf("| class | recall on deleted | FPR on survived | caught | nudged |\n|---|---|---|---|---|\n")
	for _, name := range classes {
		judged, _, err := judge(rows, clones, []float64{0}, name)
		if err != nil || len(judged) == 0 {
			continue
		}
		recall, fpr, caught, nudged := rates(judged[0])
		fmt.Printf("| %s | %.3f | %.3f | %d | %d |\n", name, recall, fpr, caught, nudged)
	}
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
	//
	// The walk starts at the origin and interpolates to the right-hand edge. It
	// used to skip the first trapezoid, which on a curve whose lowest point sits
	// at FPR 0.044 discarded 88% of the width and reported a tool that beats
	// chance as scoring below a coin flip.
	const edge = 0.05
	area, previous := 0.0, point{}
	for _, p := range points {
		if p.fpr > edge {
			if previous.fpr < edge {
				across := (edge - previous.fpr) / (p.fpr - previous.fpr)
				at := previous.recall + across*(p.recall-previous.recall)
				area += (edge - previous.fpr) * (previous.recall + at) / 2
			}
			previous = point{fpr: edge}
			break
		}
		area += (p.fpr - previous.fpr) * (p.recall + previous.recall) / 2
		previous = p
	}
	// A curve that never reaches the edge holds its last recall out to it.
	if previous.fpr < edge {
		area += (edge - previous.fpr) * previous.recall
	}
	fmt.Printf("\nPartial AUC over FPR in [0, %.2f], normalised: %.3f (a random rule scores %.3f)\n",
		edge, area/edge, edge/2)
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
