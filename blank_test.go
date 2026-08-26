package main

import (
	"strings"
	"testing"
	"time"
)

// A `{{` in prose has no closer, so the search for one has to be bounded or it
// runs to the end of the file — once per paragraph, which is quadratic. The
// shape that costs the most is one unpaired opener per paragraph, since every
// one of them starts a fresh scan.
func TestBlankIsLinear(t *testing.T) {
	build := func(paragraphs int) []byte {
		var b strings.Builder
		for i := 0; i < paragraphs; i++ {
			b.WriteString("# the {{ here opens nothing and never closes\n")
			b.WriteString("key: value\n\n")
		}
		return []byte(b.String())
	}
	small, large := build(2000), build(8000)

	elapsed := func(src []byte) time.Duration {
		start := time.Now()
		blank(src, yaml)
		return time.Since(start)
	}
	// Warm the allocator so the first call does not pay for both.
	elapsed(small)

	quick, slow := elapsed(small), elapsed(large)
	// Four times the input. Linear is about 4, the quadratic this replaced was
	// about 16, and the bound is loose enough that a shared machine does not
	// fail it.
	if slow > 8*quick+2*time.Millisecond {
		t.Errorf("4x the input cost %v against %v, which is not linear", slow, quick)
	}
	t.Logf("%d paragraphs %v, %d paragraphs %v", 2000, quick, 8000, slow)
}

// Bounding the search must not change what gets blanked: a real action is
// erased, and a `{{` with no closer in its paragraph leaves the comments after
// it readable.
func TestBlankSpares(t *testing.T) {
	src := []byte("# the {{ below is not an action\nkey: value\n\n# replicas: 3\n# image: nginx\n")
	out := blank(src, yaml)
	if !strings.Contains(string(out), "# replicas: 3") {
		t.Errorf("an unpaired {{ erased the comments after it:\n%s", out)
	}
	action := []byte("key: {{ .Values.name }}\n# replicas: 3\n")
	if got := string(blank(action, yaml)); strings.Contains(got, ".Values.name") {
		t.Errorf("a real action was left in place: %q", got)
	} else if len(got) != len(action) {
		t.Errorf("blanking changed the byte offsets: %d against %d", len(got), len(action))
	}
}
