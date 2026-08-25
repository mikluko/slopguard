package main

import (
	"strings"
	"testing"
)

// The table is the specification: a case names the language, the file as
// written, and the phrase the nudge must carry, or "" when the comment stands.
var cases = []struct {
	name string
	lang *language
	src  string
	want string
}{
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
		name: "go documentation running long",
		lang: golang,
		src: `package p

// double returns v twice over.
// It panics when v overflows.
// The caller usually wants twice instead.
// Negative values are doubled as they are.
// Zero is returned unchanged.
// The result is never smaller than v for positive v.
func double(v int) int { return v * 2 }
`,
		want: "6 sentences",
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
		name: "go change-event comment",
		lang: golang,
		src: `package p

// previously this pointed at the docker hub mirror.
func double(v int) int { return v * 2 }
`,
		want: "change-event explanation",
	},
	{
		name: "go compatibility comment",
		lang: golang,
		src: `package p

// Twice is kept for backwards compatibility.
func Twice(v int) int { return v * 2 }
`,
		want: "by its own history",
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
		name: "python comment restating the code",
		lang: python,
		src: `def double(v):
    # multiply it by two
    return v * 2
`,
		want: "restates what the code",
	},
	{
		name: "python module comment",
		lang: python,
		src: `# Timeouts are in seconds.
TIMEOUT = 5
`,
	},
	{
		name: "python commented-out code",
		lang: python,
		src: `# TIMEOUT = 30
TIMEOUT = 5
`,
		want: "commented-out code",
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
		name: "yaml comment",
		lang: yaml,
		src: `# Requests are per replica.
replicas: 2
`,
	},
	{
		name: "yaml comment reading as a mapping",
		lang: yaml,
		src: `# note: this cluster is shared
replicas: 2
`,
	},
	{
		name: "yaml commented-out config",
		lang: yaml,
		src: `replicas: 2
# resources:
#   limits:
#     cpu: 100m
image: nginx
`,
		want: "commented-out code",
	},
	{
		name: "yaml change-event comment",
		lang: yaml,
		src: `# previously this pointed at the internal mirror
image: ghcr.io/acme/api
`,
		want: "change-event explanation",
	},
	{
		name: "yaml documentation carrying a constraint",
		lang: yaml,
		src: `# Replica count for the deployment.
# Two is the floor for the pod disruption budget.
# Raising it above four needs a node pool change.
replicas: 2
`,
	},
	{
		name: "yaml prose that parses as a mapping",
		lang: yaml,
		src: `# Ref: https://runbooks.example.com/scale-up
# Owner: platform-team
replicas: 2
`,
	},
	{
		name: "yaml single line of commented-out config",
		lang: yaml,
		src: `# replicas: 3
replicas: 5
`,
		want: "commented-out",
	},
	{
		name: "helm template keeps its comments",
		lang: yaml,
		src: `{{- if .Values.ingress.enabled }}
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  # previously this pointed at the docker hub mirror
  name: {{ include "chart.fullname" . }}
{{- end }}
`,
		want: "change-event explanation",
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
	{
		name: "yaml chart values header",
		lang: yaml,
		src: `# Default values for the chart.
image:
  repository: nginx
  tag: "1.27"
`,
	},
}

func TestScan(t *testing.T) {
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			findings := scan([]byte(c.src), c.lang, []span{{start: 0, end: uint(len(c.src))}})
			switch {
			case c.want == "" && len(findings) > 0:
				t.Fatalf("nudged an acceptable comment: %s", findings[0].reason)
			case c.want == "":
				return
			case len(findings) == 0:
				t.Fatalf("missed a comment that should carry %q", c.want)
			case !strings.Contains(findings[0].reason, c.want):
				t.Fatalf("reason %q does not carry %q", findings[0].reason, c.want)
			}
		})
	}
}

// A comment outside the text the tool call wrote is somebody else's problem.
func TestScanIgnoresUntouchedComments(t *testing.T) {
	src := `package p

// double is older than this session and reads however it reads.
func double(v int) int {
	// multiply it by two
	return v * 2
}
`
	added := strings.Index(src, "\t// multiply")
	findings := scan([]byte(src), golang, []span{{start: uint(added), end: uint(len(src))}})
	if len(findings) != 1 {
		t.Fatalf("want the one comment inside the added text, got %d", len(findings))
	}
	if !strings.Contains(findings[0].reason, "restates what the code") {
		t.Fatalf("unexpected reason: %s", findings[0].reason)
	}
}
