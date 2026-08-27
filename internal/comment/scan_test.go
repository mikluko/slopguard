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

// A grammar may end a line comment at column zero of the row below, and
// tree-sitter-rust does. Read literally that makes every `///` multi-line, so no
// run groups, and a rule judging one line at a time reads the body of a doc
// example as commented-out code.
//
// This is the regression for that. Reverting [written] to the node's own end row
// leaves the rest of the suite green while putting 1,524 false positives back
// across 1,254 Rust files, so the fixture is the only thing holding it.
func TestRustDocRunGroups(t *testing.T) {
	src := []byte("/// Returns the sum of the slice.\n" +
		"///\n" +
		"/// # Examples\n" +
		"///\n" +
		"/// ```\n" +
		"/// let total = add_up(&[1, 2, 3]);\n" +
		"/// assert_eq!(total, 6);\n" +
		"/// ```\n" +
		"pub fn add_up(values: &[i32]) -> i32 {\n\tvalues.iter().sum()\n}\n")

	found, release := ScanAll(src, lang.Rust)
	defer release()
	if len(found) != 1 {
		lines := make([]uint, len(found))
		for i, one := range found {
			lines[i] = one.Line
		}
		t.Fatalf("the doc run came back as %d comments starting at %v, want one", len(found), lines)
	}
	if !strings.Contains(found[0].Body, "assert_eq!") {
		t.Errorf("the grouped comment lost the example body: %q", found[0].Body)
	}
}

// The same grammar, one comment per declaration rather than a run: two `///`
// blocks separated by code must not merge into one.
func TestRustDocRunsStayApart(t *testing.T) {
	src := []byte("/// The first thing.\npub fn one() {}\n\n/// The second thing.\npub fn two() {}\n")
	found, release := ScanAll(src, lang.Rust)
	defer release()
	if len(found) != 2 {
		t.Fatalf("got %d comments, want two", len(found))
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
