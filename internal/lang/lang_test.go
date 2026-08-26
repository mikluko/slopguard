package lang

import "testing"

// Which language a path is read as, including the files that carry it in their
// name rather than in an extension.
func TestLookup(t *testing.T) {
	for _, c := range []struct {
		path string
		want *Language
	}{
		{"internal/store/store.go", Go},
		{"deploy/values.yaml", YAML},
		{"main.tf", HCL},
		{"Dockerfile", Dockerfile},
		{"build/Dockerfile", Dockerfile},
		{"Dockerfile.dev", Dockerfile},
		// An extension that is neither read nor a format of its own is a
		// variant tag, and the file keeps the language its name gives it.
		{"Dockerfile.tmpl", Dockerfile},
		{"api.Dockerfile", Dockerfile},
		{"Containerfile", Dockerfile},
		{"Makefile", Makefile},
		{"makefile", Makefile},
		{"Makefile.local", Makefile},
		{"rules.mk", Makefile},
		{"README.md", nil},
		{"Makefile.md", nil},
		{"Dockerfile.json", nil},
		{"Makefile.go", Go},
		{"Jenkinsfile", nil},
		{"LICENSE", nil},
		{"docker", nil},
	} {
		t.Run(c.path, func(t *testing.T) {
			got := Lookup(c.path)
			switch {
			case got == nil && c.want != nil:
				t.Fatalf("read as nothing, want %s", c.want.Name)
			case got != nil && c.want == nil:
				t.Fatalf("read as %s, want nothing", got.Name)
			case got != nil && got != c.want:
				t.Fatalf("read as %s, want %s", got.Name, c.want.Name)
			}
		})
	}
}

// Every language names the node kinds it reads out of its own grammar, and a
// name that grammar does not use is a rule that silently never fires. This is
// the only place that pairing is checked.
func TestNodeKindsExist(t *testing.T) {
	for _, l := range byName {
		t.Run(l.Name, func(t *testing.T) {
			if len(l.Comments) == 0 {
				t.Error("no comment kind named, so no comment is ever found")
			}
			if l.Grammar == nil {
				t.Fatal("no grammar")
			}
		})
	}
	seen := map[*Language]bool{}
	for _, l := range byExtension {
		if seen[l] {
			continue
		}
		seen[l] = true
		t.Run(l.Name, func(t *testing.T) {
			if len(l.Comments) == 0 {
				t.Error("no comment kind named, so no comment is ever found")
			}
			if l.Grammar == nil {
				t.Fatal("no grammar")
			}
		})
	}
}
