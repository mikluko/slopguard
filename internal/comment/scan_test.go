package comment

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mikluko/slopguard/internal/lang"
)

// The cap is what separates reading a write from reading a corpus, so a file
// carrying more comments than [examined] has to come back whole from ScanAll
// and cut from Scan.
func TestScanAllIsUnbounded(t *testing.T) {
	var src strings.Builder
	src.WriteString("package p\n\n")
	const n = examined + 20
	for i := range n {
		fmt.Fprintf(&src, "// comment number %d says something\nvar v%d = %d\n\n", i, i, i)
	}
	whole := []byte(src.String())

	all, release := ScanAll(whole, lang.Go)
	defer release()
	if len(all) != n {
		t.Errorf("ScanAll returned %d of %d comments", len(all), n)
	}

	cut, releaseCut := Scan(whole, lang.Go, []Span{{Start: 0, End: uint(len(whole))}})
	defer releaseCut()
	if len(cut) != examined {
		t.Errorf("Scan returned %d comments, want the cap of %d", len(cut), examined)
	}
}

// Mining labels a comment by the code it sits above, so ScanAll owes the same
// Annotates that the rules read.
func TestScanAllAnnotates(t *testing.T) {
	src := []byte("package p\n\n// the flag is read once at startup\nvar ready = false\n")
	all, release := ScanAll(src, lang.Go)
	defer release()
	if len(all) != 1 {
		t.Fatalf("got %d comments, want 1", len(all))
	}
	if all[0].Annotates == nil {
		t.Fatal("the comment came back with nothing annotated")
	}
	if got := all[0].Annotates.Utf8Text(src); !strings.Contains(got, "ready") {
		t.Errorf("annotated %q, want the declaration below it", got)
	}
}

// Bounding the search must not change what gets blanked: a real action is
// erased, and a `{{` with no closer in its paragraph leaves the comments after
// it readable.
func TestBlankSpares(t *testing.T) {
	src := []byte("# the {{ below is not an action\nkey: value\n\n# replicas: 3\n# image: nginx\n")
	out := blank(src, lang.YAML)
	if !strings.Contains(string(out), "# replicas: 3") {
		t.Errorf("an unpaired {{ erased the comments after it:\n%s", out)
	}
	action := []byte("key: {{ .Values.name }}\n# replicas: 3\n")
	if got := string(blank(action, lang.YAML)); strings.Contains(got, ".Values.name") {
		t.Errorf("a real action was left in place: %q", got)
	} else if len(got) != len(action) {
		t.Errorf("blanking changed the byte offsets: %d against %d", len(got), len(action))
	}
}
