package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The memory is per session, per path, and per key. Nothing here goes through
// the rules: what a key identifies is the caller's, and this package's contract
// is that it hands back the same numbers it was given and no others.
func TestSpokenRemembers(t *testing.T) {
	t.Setenv(MemoryEnv, t.TempDir())

	seen, spoken, record := Spoken("a-session", "double.go")
	if len(seen) != 0 || spoken {
		t.Fatalf("a fresh session remembers nothing, got %d keys and spoken=%v", len(seen), spoken)
	}
	record([]uint64{11, 22})

	seen, spoken, _ = Spoken("a-session", "double.go")
	if !spoken || !seen[11] || !seen[22] {
		t.Errorf("the same session and path should hear it back: %v spoken=%v", seen, spoken)
	}
	if seen, _, _ := Spoken("a-session", "other.go"); seen[11] {
		t.Error("another path in the same session heard it")
	}
	if seen, _, _ := Spoken("another-session", "double.go"); seen[11] {
		t.Error("another session heard it")
	}
}

// Switched off, nothing is written and nothing is read back.
func TestSpokenOff(t *testing.T) {
	t.Setenv(MemoryEnv, "")

	_, _, record := Spoken("a-session", "double.go")
	record([]uint64{11})
	if seen, spoken, _ := Spoken("a-session", "double.go"); len(seen) != 0 || spoken {
		t.Errorf("remembered something with the store off: %v spoken=%v", seen, spoken)
	}
}

// The sweep deletes only what this program wrote. SLOPGUARD_STATE is a path
// somebody types, and a sweep that trusted the directory would empty it.
func TestSweepLeavesOtherFilesAlone(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-48 * time.Hour)
	for _, name := range []string{"notes.md", "config.json", ".hidden", "1chkc1qdadytv"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "2uufoxcnq1d11"), 0o700); err != nil {
		t.Fatal(err)
	}

	sweep(dir)

	left := map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		left[entry.Name()] = true
	}
	for _, name := range []string{"notes.md", "config.json", ".hidden", "2uufoxcnq1d11"} {
		if !left[name] {
			t.Errorf("swept %s, which this program did not write", name)
		}
	}
	if left["1chkc1qdadytv"] {
		t.Error("kept a stale memory of its own")
	}
}
