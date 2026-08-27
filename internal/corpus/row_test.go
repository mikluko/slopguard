package corpus

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// A row names the revision it was read from, and the two labels name different
// ones: a deletion is read from the commit before it happened, a survivor from
// the head.
func TestRevNamesTheVersionRead(t *testing.T) {
	deleted := Row{Label: Deleted, Removed: "abc123"}
	if got, want := deleted.Rev(), "abc123^"; got != want {
		t.Errorf("Rev() = %q, want %q", got, want)
	}
	if got, want := (Row{Label: Survived}).Rev(), "HEAD"; got != want {
		t.Errorf("Rev() = %q, want %q", got, want)
	}
}

// The corpus travels compressed, so Load has to read both spellings and hand
// back the same rows.
func TestLoadReadsPlainAndGzip(t *testing.T) {
	rows := []Row{
		{Repo: "a/b", Label: Deleted, Text: "start write transactions", Line: 12, Removed: "abc"},
		{Repo: "a/b", Label: Survived, Text: "the caller must hold the lock", Line: 40, Exposure: 9},
	}
	var encoded []byte
	for _, row := range rows {
		line, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		encoded = append(encoded, line...)
		encoded = append(encoded, '\n')
	}

	dir := t.TempDir()
	plain := filepath.Join(dir, "corpus.jsonl")
	if err := os.WriteFile(plain, encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	zipped := filepath.Join(dir, "corpus.jsonl.gz")
	file, err := os.Create(zipped)
	if err != nil {
		t.Fatal(err)
	}
	writer := gzip.NewWriter(file)
	if _, err := writer.Write(encoded); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	file.Close()

	for _, path := range []string{plain, zipped} {
		got, err := Load(path)
		if err != nil {
			t.Fatalf("Load(%s): %v", filepath.Base(path), err)
		}
		if len(got) != len(rows) {
			t.Fatalf("Load(%s) returned %d rows, want %d", filepath.Base(path), len(got), len(rows))
		}
		for i := range rows {
			if got[i].Text != rows[i].Text || got[i].Line != rows[i].Line || got[i].Label != rows[i].Label {
				t.Errorf("Load(%s) row %d = %+v, want %+v", filepath.Base(path), i, got[i], rows[i])
			}
		}
	}
}

// Line is what lets scoring find the comment again, so it has to survive the
// round trip even at zero rather than being dropped as empty.
func TestLineSurvivesAtZero(t *testing.T) {
	line, err := json.Marshal(Row{Label: Survived, Line: 0})
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(line, &back); err != nil {
		t.Fatal(err)
	}
	if _, ok := back["line"]; !ok {
		t.Error("line was dropped from the encoding, which is how a corpus scores nothing and looks fine")
	}
}

func TestFlatAndProse(t *testing.T) {
	if got, want := Flat("  a\n\tb   c\n"), "a b c"; got != want {
		t.Errorf("Flat() = %q, want %q", got, want)
	}
	if Prose("ok") {
		t.Error("a two word comment counts as prose")
	}
	if !Prose("the caller must hold the lock") {
		t.Error("a sentence does not count as prose")
	}
}

// Truncate cuts on a rune boundary, which is what keeps a corpus of other
// people's comments from carrying broken UTF-8.
func TestTruncateKeepsRunesWhole(t *testing.T) {
	if got, want := Truncate("abc", 10), "abc"; got != want {
		t.Errorf("Truncate() = %q, want %q", got, want)
	}
	got := Truncate("aaaé", 4)
	if got != "aaa…" {
		t.Errorf("Truncate() = %q, want the multibyte rune dropped whole", got)
	}
}
