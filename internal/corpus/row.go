// Package corpus is the mined comment corpus: the row it is made of, the file
// it is stored as, and the git plumbing that both mining it and scoring against
// it need.
//
// What makes this corpus different from the tables in internal/model is where
// its labels come from. Those are hand-assigned class names, so they measure
// whether a fit generalises to unseen comments of a taxonomy this project wrote.
// A row here is labelled by somebody else's act: a comment its own author
// deleted while keeping the code under it, or one left standing through many
// later edits to the same file. Neither label is an opinion held here, which is
// the only reason a number measured against them says anything about the
// taxonomy itself.
package corpus

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"time"
	"unicode"
)

// The two labels a Row carries.
const (
	Deleted  = "deleted"
	Survived = "survived"
)

// A Row is one harvested comment together with the evidence for its label.
type Row struct {
	Repo     string `json:"repo"`
	License  string `json:"license"`
	Path     string `json:"path"`
	Language string `json:"language"`

	// Label is [Deleted] or [Survived].
	Label string `json:"label"`

	// Text is the comment's prose on one line, with the markers stripped, which
	// is the form the model reads and the form mined.go stores.
	Text string `json:"text"`
	// Body keeps the line breaks, for the rules that read a comment as source.
	Body string `json:"body"`
	// Annotates is the code the comment sat above, truncated, which is what a
	// restatement would be restating.
	Annotates string `json:"annotates,omitempty"`

	// Added and Removed are the commits that wrote and deleted the comment.
	// Removed is empty on a survived row.
	Added     string    `json:"added,omitempty"`
	Removed   string    `json:"removed,omitempty"`
	AddedAt   time.Time `json:"added_at,omitzero"`
	RemovedAt time.Time `json:"removed_at,omitzero"`

	// LifetimeDays is how long a deleted comment stood. A short life is the
	// stronger signal: a comment deleted within weeks was judged, one deleted
	// after years was more likely overtaken.
	LifetimeDays float64 `json:"lifetime_days,omitempty"`
	// EditsSince counts the commits that touched the file after a survived
	// comment was written. A comment nobody deleted through many edits is one
	// many readings left alone; a comment in a file nothing touched is not
	// evidence, and rows below the threshold are never emitted.
	EditsSince int `json:"edits_since,omitempty"`

	// Doc, Trailing and Buried carry the structural position, because the rules
	// treat the three differently and a corpus that pools them measures nothing.
	Doc      bool `json:"doc,omitempty"`
	Trailing bool `json:"trailing,omitempty"`
	Buried   bool `json:"buried,omitempty"`

	// Lines is how many lines the comment ran to.
	Lines int `json:"lines"`
	// Line is where the comment starts in the version of the file this row was
	// read from, which [Row.Rev] names.
	//
	// It is here so that scoring can replay the real file through the real
	// rules rather than judging the prose on its own: the restatement rules,
	// `echo` and `leftover` all read the code beside a comment, and a corpus
	// carrying only the text could not exercise them.
	Line uint `json:"line"`
}

// Rev names the revision of the file this row was read from: the commit before
// the deletion, or the current head for a comment still standing.
func (r Row) Rev() string {
	if r.Label == Deleted {
		return r.Removed + "^"
	}
	return "HEAD"
}

// Load reads a corpus file, one JSON row per line.
func Load(path string) ([]Row, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var rows []Row
	reader := bufio.NewScanner(file)
	reader.Buffer(make([]byte, 0, 1<<20), 1<<22)
	for reader.Scan() {
		line := reader.Bytes()
		if len(line) == 0 {
			continue
		}
		var row Row
		if err := json.Unmarshal(line, &row); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, reader.Err()
}

// Flat renders s as one line of single-spaced text, which is how two pieces of
// code are compared for having survived: a reindented body is the same code,
// and a comparison that says otherwise reports every reformatting as a
// deletion.
func Flat(s string) string {
	return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
}

// Prose reports whether s carries enough words to be worth labelling. A one
// word comment is neither contract nor slop, and a corpus full of `// ok` moves
// every centroid toward nothing.
func Prose(s string) bool {
	return len(strings.Fields(s)) >= 3
}

// Truncate cuts s to at most n bytes on a rune boundary, marking the cut.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut] + "…"
}
