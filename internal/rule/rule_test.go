package rule

import (
	"strings"
	"testing"

	"github.com/mikluko/slopguard/internal/comment"
	"github.com/mikluko/slopguard/internal/model"
)

// The table is the specification: a case names the language, the file as
// written, and the phrase the nudge must carry, or "" when the comment stands.
//
// A case marked `gap` is one the tool does not answer today and the table keeps
// anyway: deleting it would remove the only record that the behaviour was ever
// wanted. The reason names what has to change before it passes.
var cases = []struct {
	name string
	lang *language
	src  string
	want string
	gap  string
}{
	{
		// A case arm is where a reader most needs an example of what is being
		// matched, and the example is written in the language being matched.
		name: "a comment opening a case arm names what the arm handles",
		lang: golang,
		src: `package p

func f(kind int) {
	switch kind {
	case 1:
		// clear(m)
		report(kind)
	}
}
`,
	},
	{
		name: "a comment above a case arm names what the arm handles",
		lang: typescript,
		src: `function f(node: Node, parent: Node) {
  switch (parent.type) {
    // no: let NODE = init;
    // yes: let id = NODE;
    case 'VariableDeclarator':
      return parent.init === node
  }
}
`,
	},
	{
		// Further in, there are statements before it, and it is disabling one
		// as readily as any other.
		name: "a comment further inside a case arm is not spared",
		lang: golang,
		src: `package p

func f(kind int) {
	switch kind {
	case 1:
		report(kind)
		// report(kind + 1)
		finish(kind)
	}
}
`,
		want: "commented-out",
	},
	{
		name: "go doc comment",
		lang: golang,
		src: `package p

// double returns v twice over.
func double(v int) int { return v * 2 }
`,
	},
	{
		name: "go doc comment with an earned second sentence",
		lang: golang,
		src: `package p

// double returns v twice over.
// It panics when v overflows.
func double(v int) int { return v * 2 }
`,
	},
	{
		// Nine sentences, each ruling something out. Length was never the
		// question, so this is silence.
		name: "go documentation earning its length",
		lang: golang,
		src: `package p

// double returns v twice over.
// It panics when v overflows.
// Negative values are doubled as they are.
// Zero is returned unchanged.
// The result is never smaller than v for positive v.
// Overflow is checked before the multiply, not after.
// The check costs one comparison.
// A caller that has already bounded v may skip it.
// Nothing here is retained between calls.
func double(v int) int { return v * 2 }
`,
		want: "",
	},
	{
		// Three sentences, two of them saying nothing the signature did not.
		name: "go documentation padded past its contract",
		lang: golang,
		src: `package p

// double returns v twice over.
// This function takes a value and returns a value.
// The implementation is simple and easy to read.
func double(v int) int { return v * 2 }
`,
		want: "padded documentation",
	},
	{
		name: "go comment restating the code",
		lang: golang,
		src: `package p

func double(v int) int {
	// multiply it by two
	return v * 2
}
`,
		want: "restates what the code",
	},
	{
		name: "go comment inside a function pointing outward",
		lang: golang,
		src: `package p

func double(v int) int {
	// the caller has already bounded v, so this cannot overflow
	return v * 2
}
`,
	},
	{
		name: "a change-event comment is left alone, deliberately",
		lang: golang,
		src: `package p

// previously this pointed at the docker hub mirror.
func double(v int) int { return v * 2 }
`,
	},
	{
		name: "go compatibility comment",
		lang: golang,
		src: `package p

// Twice is kept for backwards compatibility.
func Twice(v int) int { return v * 2 }
`,
		want: "by its own history",
		gap:  "compat fires on the bare phrasing and not on this one, which names the symbol first",
	},
	{
		name: "go commented-out code",
		lang: golang,
		src: `package p

// var timeout = 5 * time.Second
func double(v int) int { return v * 2 }
`,
		want: "commented-out code",
	},
	{
		name: "go directive",
		lang: golang,
		src: `package p

//go:generate stringer -type=Kind
func double(v int) int { return v * 2 }
`,
	},
	{
		name: "go deprecation note",
		lang: golang,
		src: `package p

// Deprecated: use Double. v1 callers still bind this name.
func Twice(v int) int { return v * 2 }
`,
	},
	{
		name: "go package doc",
		lang: golang,
		src: `// Package p doubles things.
package p
`,
	},
	{
		name: "typescript comment restating the code",
		lang: typescript,
		src: `const double = (v: number) => {
  // multiply it by two
  return v * 2;
};
`,
		want: "restates what the code",
	},
	{
		name: "typescript eslint pragma",
		lang: typescript,
		src: `// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function double(v: any) {
  return v * 2;
}
`,
	},
	{
		name: "shell comment restating the code",
		lang: bash,
		src: `main() {
  # start the server
  run
}
`,
		want: "restates what the code",
	},
	{
		name: "shell shebang",
		lang: bash,
		src: `#!/usr/bin/env bash
run
`,
	},
	{
		name: "rust comment restating the code",
		lang: rust,
		src: `fn double(v: i32) -> i32 {
    // multiply it by two
    v * 2
}
`,
		want: "restates what the code",
	},
	{
		name: "dockerfile commented-out instruction",
		lang: dockerfile,
		src: `FROM alpine:3.20
# RUN apk add --no-cache curl
RUN apk add --no-cache ca-certificates
`,
		want: "commented-out",
	},
	{
		name: "dockerfile comment carrying a constraint",
		lang: dockerfile,
		src: `FROM alpine:3.20
# the certificates have to land before the build, which fetches over TLS
RUN apk add --no-cache ca-certificates
`,
	},
	{
		name: "makefile comment carrying a constraint",
		lang: makefile,
		src: `# the asset is refit here rather than in build, because it needs the model
test:
	go test ./...
`,
	},
	{
		name: "terraform commented-out attribute",
		lang: hcl,
		src: `resource "aws_instance" "worker" {
  instance_type = "t3.small"
  # instance_type = "t3.micro"
}
`,
		want: "commented-out code",
	},
	{
		name: "terraform comment carrying a constraint",
		lang: hcl,
		src: `resource "aws_instance" "worker" {
  # the smallest plan the node pool will build
  instance_type = "t3.small"
}
`,
	},
}

func TestScan(t *testing.T) {
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if judged(c.want) {
				skipWithoutRuntime(t)
			}
			findings := scan([]byte(c.src), c.lang, []comment.Span{{Start: 0, End: uint(len(c.src))}})
			if c.gap != "" {
				if len(findings) > 0 && strings.Contains(findings[0].Reason, c.want) {
					t.Fatalf("this case is marked as a gap and now passes: drop the mark. %s", c.gap)
				}
				t.Skip(c.gap)
			}
			switch {
			case c.want == "" && len(findings) > 0:
				t.Fatalf("nudged an acceptable comment: %s", findings[0].Reason)
			case c.want == "":
				return
			case len(findings) == 0:
				t.Fatalf("missed a comment that should carry %q", c.want)
			case !strings.Contains(findings[0].Reason, c.want):
				t.Fatalf("reason %q does not carry %q", findings[0].Reason, c.want)
			}
		})
	}
}

// judged reports whether a case needs the model to answer. The structural
// rules — commented-out code, the identifier echo, documentation running long —
// answer on any machine, and a case expecting silence has to hold on both.
func judged(want string) bool {
	return model.Speaks(want)
}

// Every kind in [labels] is one a grammar in the table actually emits, and every
// grammar that has switch arms has its kinds here.
//
// Neither invariant sweep touches most of these, so nothing measured them:
// `case_expression` sat in the set for a round while no grammar emitted it, and
// PHP's `default_statement` was missing for the same round, both invisible
// because the mined corpus holds no PHP. The fixtures below reach the rule
// through the real scanner, so a kind that is misspelled or gone from a grammar
// fails here rather than silently exempting nothing.
func TestEveryLabelIsAKindItsGrammarEmits(t *testing.T) {
	for _, c := range []struct {
		name string
		lang *language
		src  string
	}{
		{"go expression case", golang, "package p\n\nfunc f(k int) {\n\tswitch k {\n\t// step(k)\n\tcase 1:\n\t\treport(k)\n\t}\n}\n"},
		{"typescript switch case", typescript, "function f(k: number) {\n  switch (k) {\n    // step(k)\n    case 1:\n      report(k)\n  }\n}\n"},
		{"python case clause", python, "def f(k):\n    match k:\n        # step(k)\n        case 1:\n            report(k)\n"},
		{"rust match arm", rust, "fn f(k: i32) {\n    match k {\n        // step(k);\n        1 => report(k),\n        _ => (),\n    }\n}\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := []byte(c.src)
			candidates, release := comment.ScanAll(src, c.lang)
			defer release()
			if len(candidates) == 0 {
				t.Fatal("the fixture yielded no comment, so it tests nothing")
			}
			var seen []string
			for _, one := range candidates {
				if one.Annotates != nil {
					seen = append(seen, one.Annotates.Kind())
				}
				if parent := one.Nodes[0].Parent(); parent != nil {
					seen = append(seen, parent.Kind())
				}
			}
			for _, kind := range seen {
				if labels[kind] {
					return
				}
			}
			t.Fatalf("no kind around this comment is in labels; the grammar emits %q", seen)
		})
	}
}

// What ships is `leftover` alone. Every other class is behind [Wider], and this
// is the only place that says so: the spec tables all run with the wider set on,
// because they specify what a rule reads rather than whether it is switched on.
//
// Each source here is a fixture another test asserts does fire.
func TestShippedRunsLeftoverAlone(t *testing.T) {
	// Python, because this has to hold on a machine with no ONNX Runtime and
	// the Go fixture beside it is answered by the model: a comment heading a run
	// of statements is not a structural echo, deliberately.
	echoed := `def f(user):
    # save the user profile
    save_user_profile(user)
`
	padded := `package p

// Double returns the value twice over. This function takes a value and returns
// a value. The implementation is simple and easy to read.
func Double(value int) int { return value * 2 }
`
	commented := `package p

func f() {
	// total := 0
	// for _, item := range items {
	// 	total += item
	// }
	_ = 0
}
`
	for _, c := range []struct {
		name string
		lang *language
		src  string
		want string
	}{
		{"echo is off", python, echoed, ""},
		{"hollow is off", golang, padded, ""},
		{"leftover still fires", golang, commented, "leftover"},
	} {
		t.Run(c.name, func(t *testing.T) {
			found := shipped([]byte(c.src), c.lang, whole([]byte(c.src)))
			switch {
			case c.want == "" && len(found) > 0:
				t.Fatalf("the default said something: %s (%s)", found[0].Class, found[0].Reason)
			case c.want == "":
			case len(found) == 0:
				t.Fatalf("the default went silent on %s", c.want)
			case found[0].Class != c.want:
				t.Fatalf("want class %q, got %q", c.want, found[0].Class)
			}
		})
	}
	// The same two sources under the wider set, so that a fixture quietly
	// ceasing to fire cannot pass this test by going silent everywhere.
	for _, c := range []struct {
		name string
		lang *language
		src  string
		want string
	}{
		{"echo", python, echoed, "echo"},
		{"hollow", golang, padded, "hollow"},
	} {
		t.Run(c.name+" fires when asked for", func(t *testing.T) {
			found := scan([]byte(c.src), c.lang, whole([]byte(c.src)))
			if len(found) == 0 || found[0].Class != c.want {
				t.Fatalf("want %s under the wider set, got %v", c.want, found)
			}
		})
	}
}

// Wider is read as a boolean, so the negative form turns it off. Read as mere
// presence, `SLOPGUARD_WIDER=0` switched the wider set on.
func TestWiderReadsItsValue(t *testing.T) {
	for value, want := range map[string]bool{
		"1": true, "true": true, "TRUE": true,
		"0": false, "false": false, "": false, "off": false,
	} {
		t.Setenv(widerEnv, value)
		if got := Wider(); got != want {
			t.Errorf("SLOPGUARD_WIDER=%q: want %v, got %v", value, want, got)
		}
	}
}

// A comment outside the text the tool call wrote is somebody else's problem.
func TestScanIgnoresUntouchedComments(t *testing.T) {
	skipWithoutRuntime(t)
	src := `package p

// double is older than this session and reads however it reads.
func double(v int) int {
	// multiply it by two
	return v * 2
}
`
	added := strings.Index(src, "\t// multiply")
	findings := scan([]byte(src), golang, []comment.Span{{Start: uint(added), End: uint(len(src))}})
	if len(findings) != 1 {
		t.Fatalf("want the one comment inside the added text, got %d", len(findings))
	}
	if !strings.Contains(findings[0].Reason, "restates what the code") {
		t.Fatalf("unexpected reason: %s", findings[0].Reason)
	}
}
