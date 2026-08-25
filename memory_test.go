package main

import (
	"os"
	"strings"
	"testing"
)

const restated = `package p

func double(v int) int {
	// multiply it by two
	return v * 2
}
`

// A comment named once is not named again in the same session, however the
// file moves under it.
func TestMemorySilencesARepeat(t *testing.T) {
	skipWithoutRuntime(t)
	t.Setenv(memoryEnv, t.TempDir())

	in := payload{SessionID: "a-session", ToolName: "Write"}
	in.ToolInput.FilePath = file(t, "double.go", restated)
	if findings := review(in); len(findings) != 1 {
		t.Fatalf("want one finding on the first write, got %d", len(findings))
	}
	if findings := review(in); len(findings) != 0 {
		t.Fatalf("want silence on the second, got %q", findings[0].reason)
	}
}

// The memory is per session and per comment, not per file: another session
// hears it, and so does another comment in the same file.
func TestMemoryIsNarrow(t *testing.T) {
	skipWithoutRuntime(t)
	t.Setenv(memoryEnv, t.TempDir())

	path := file(t, "double.go", restated)
	first := payload{SessionID: "a-session", ToolName: "Write"}
	first.ToolInput.FilePath = path
	review(first)

	second := payload{SessionID: "another-session", ToolName: "Write"}
	second.ToolInput.FilePath = path
	if findings := review(second); len(findings) != 1 {
		t.Fatalf("another session should hear it, got %d findings", len(findings))
	}

	moved := strings.Replace(restated, "// multiply it by two", "// close the connection", 1)
	if err := os.WriteFile(path, []byte(moved), 0o644); err != nil {
		t.Fatal(err)
	}
	if findings := review(first); len(findings) != 1 {
		t.Fatalf("a different comment should be heard, got %d findings", len(findings))
	}
}

// Nothing is remembered when the store is switched off.
func TestMemoryOff(t *testing.T) {
	skipWithoutRuntime(t)
	t.Setenv(memoryEnv, "")

	in := payload{SessionID: "a-session", ToolName: "Write"}
	in.ToolInput.FilePath = file(t, "double.go", restated)
	if findings := review(in); len(findings) != 1 {
		t.Fatalf("want one finding, got %d", len(findings))
	}
	if findings := review(in); len(findings) != 1 {
		t.Fatalf("want the same finding again, got %d", len(findings))
	}
}
