package main

import "testing"

// Which language a path is read as, including the files that carry it in their
// name rather than in an extension.
func TestLookup(t *testing.T) {
	for _, c := range []struct {
		path string
		want *language
	}{
		{"internal/store/store.go", golang},
		{"deploy/values.yaml", yaml},
		{"main.tf", hcl},
		{"Dockerfile", dockerfile},
		{"build/Dockerfile", dockerfile},
		{"Dockerfile.dev", dockerfile},
		// An extension that is neither read nor a format of its own is a
		// variant tag, and the file keeps the language its name gives it.
		{"Dockerfile.tmpl", dockerfile},
		{"api.Dockerfile", dockerfile},
		{"Containerfile", dockerfile},
		{"Makefile", makefile},
		{"makefile", makefile},
		{"Makefile.local", makefile},
		{"rules.mk", makefile},
		{"README.md", nil},
		{"Makefile.md", nil},
		{"Dockerfile.json", nil},
		{"Makefile.go", golang},
		{"Jenkinsfile", nil},
		{"LICENSE", nil},
		{"docker", nil},
	} {
		t.Run(c.path, func(t *testing.T) {
			got := lookup(c.path)
			switch {
			case got == nil && c.want != nil:
				t.Fatalf("read as nothing, want %s", c.want.name)
			case got != nil && c.want == nil:
				t.Fatalf("read as %s, want nothing", got.name)
			case got != nil && got != c.want:
				t.Fatalf("read as %s, want %s", got.name, c.want.name)
			}
		})
	}
}
