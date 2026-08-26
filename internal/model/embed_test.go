package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// Every reason the semantic pass can be unavailable, and the wording each one
// answers with. A test elsewhere in this repo skips on a non-empty answer, so
// what matters is that the answer is empty exactly when the pass can run.
func TestAbsent(t *testing.T) {
	stub := filepath.Join(t.TempDir(), "libonnxruntime.so")
	if err := os.WriteFile(stub, []byte("not really a library"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("switched off", func(t *testing.T) {
		t.Setenv(disableEnv, "1")
		t.Setenv(libraryPathEnv, stub)
		if why := Absent(); !strings.Contains(why, disableEnv) {
			t.Errorf("Absent() = %q, want it to name %s", why, disableEnv)
		}
	})
	t.Run("no library where it was pointed", func(t *testing.T) {
		t.Setenv(disableEnv, "")
		missing := filepath.Join(t.TempDir(), "nothing.so")
		t.Setenv(libraryPathEnv, missing)
		why := Absent()
		if !strings.Contains(why, missing) {
			t.Errorf("Absent() = %q, want it to name the path it looked at", why)
		}
	})
	t.Run("a library is there", func(t *testing.T) {
		t.Setenv(disableEnv, "")
		t.Setenv(libraryPathEnv, stub)
		if why := Absent(); why != "" {
			t.Errorf("Absent() = %q, want it to say the pass can run", why)
		}
	})
}

// The override wins over the search, and a search that finds nothing still
// names somewhere, so the failure says where it looked.
func TestLibrary(t *testing.T) {
	t.Setenv(libraryPathEnv, "/somewhere/else/libonnxruntime.so")
	if got := library(); got != "/somewhere/else/libonnxruntime.so" {
		t.Errorf("library() = %q, want the override", got)
	}
	t.Setenv(libraryPathEnv, "")
	if got := library(); got == "" {
		t.Error("library() named nowhere, so a failure would name nothing")
	}
}

// The tokenizer is quadratic in its input, so the cut has to happen. It also
// has to leave valid UTF-8: a comment cut through a multi-byte rune reaches the
// tokenizer as a replacement character and reads as something nobody wrote.
func TestClip(t *testing.T) {
	if got := clip("short"); got != "short" {
		t.Errorf("clip shortened text that was already short: %q", got)
	}
	// Three-byte runes, so the budget lands mid-rune whatever its value.
	long := strings.Repeat("私", budgetBytes)
	got := clip(long)
	switch {
	case len(got) > budgetBytes:
		t.Errorf("clip left %d bytes, over the budget of %d", len(got), budgetBytes)
	case !utf8.ValidString(got):
		t.Error("clip cut through a rune")
	case len(got) < budgetBytes-utf8.UTFMax:
		t.Errorf("clip took %d bytes, further under the budget than one rune", len(got))
	}
}

// The head shipped in assets/ has to have been fitted from the corpus this
// binary carries. A binary whose asset went stale judges along a direction that
// no longer answers to the labels beside it, and nothing about that is visible
// at runtime — which is what the fingerprint is for.
func TestHeadMatchesItsCorpus(t *testing.T) {
	if _, ok := DecodeHead(HeadBytes, Fingerprint()); !ok {
		t.Fatal("assets/head.bin was fitted from a corpus this binary no longer carries: go test -update")
	}
	if _, ok := DecodeHead(HeadBytes, Fingerprint()+1); ok {
		t.Error("a head decoded under the wrong fingerprint, so the check catches nothing")
	}
}
