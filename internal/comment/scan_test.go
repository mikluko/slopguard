package comment

import (
	"strings"
	"testing"

	"github.com/mikluko/slopguard/internal/lang"
)

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
