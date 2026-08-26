package main

import (
	"strings"
	"testing"
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
		want: "earn no place",
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
		name: "python commented-out call",
		lang: python,
		src: `def save(user):
    # user.save()
    return user
`,
		want: "commented-out code",
	},
	{
		name: "django settings comment that parses as an assignment",
		lang: python,
		src: `# DEBUG=False in production
DEBUG = True
`,
	},
	{
		name: "django field comment that parses as an assignment",
		lang: python,
		src: `class Order(models.Model):
    # on_delete=CASCADE is deliberate: an order without a customer is meaningless
    customer = models.ForeignKey(Customer, on_delete=models.CASCADE)
`,
	},
	{
		// A docstring sits inside the body it documents rather than above it, so
		// the declaration this is measured against is the node around it.
		name: "python docstring padded past its contract",
		lang: python,
		src: `def double(v):
    """Double v.

    This function takes a value and returns a value. The implementation is
    simple and should be easy to read.
    """
    return v * 2
`,
		want: "earn no place",
	},
	{
		name: "python docstring stating a contract",
		lang: python,
		src: `def save(order):
    """Persist the order, raising when the customer is gone."""
    order.save()
`,
	},
	{
		name: "python coverage pragma",
		lang: python,
		src: `def unreachable():  # pragma: no cover
    raise NotImplementedError
`,
	},
	{
		name: "python migration header",
		lang: python,
		src: `# Generated by Django 4.2.7 on 2024-01-15 10:23
from django.db import migrations
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
		name: "yaml documentation carrying a constraint",
		lang: yaml,
		src: `# Replica count for the deployment.
# Two is the floor for the pod disruption budget.
# Raising it above four needs a node pool change.
replicas: 2
`,
	},
	{
		name: "a commented shape under an empty key documents it",
		lang: yaml,
		src: `podSecurityContext: {}
  # fsGroup: 2000
  # runAsNonRoot: true

replicaCount: 1
`,
	},
	{
		name: "the same shape under a key that is not empty is residue",
		lang: yaml,
		src: `podSecurityContext:
  runAsNonRoot: true
  # fsGroup: 2000
  # runAsUser: 1000

replicaCount: 1
`,
		want: "commented-out",
	},
	{
		name: "an empty key does not excuse a block beside it",
		lang: yaml,
		src: `podSecurityContext: {}

# resources:
#   limits:
#     cpu: 100m

replicaCount: 1
`,
		want: "commented-out",
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
  name: {{ include "chart.fullname" . }}
  # annotations:
  #   kubernetes.io/tls-acme: "true"
{{- end }}
`,
		want: "commented-out",
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
			if judged(c.want) {
				skipWithoutRuntime(t)
			}
			findings := scan([]byte(c.src), c.lang, []span{{start: 0, end: uint(len(c.src))}})
			if c.gap != "" {
				if len(findings) > 0 && strings.Contains(findings[0].reason, c.want) {
					t.Fatalf("this case is marked as a gap and now passes: drop the mark. %s", c.gap)
				}
				t.Skip(c.gap)
			}
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

// judged reports whether a case needs the model to answer. The structural
// rules — commented-out code, the identifier echo, documentation running long —
// answer on any machine, and a case expecting silence has to hold on both.
func judged(want string) bool {
	for _, c := range classes {
		if want != "" && strings.Contains(c.reason, want) {
			return true
		}
	}
	return false
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
	findings := scan([]byte(src), golang, []span{{start: uint(added), end: uint(len(src))}})
	if len(findings) != 1 {
		t.Fatalf("want the one comment inside the added text, got %d", len(findings))
	}
	if !strings.Contains(findings[0].reason, "restates what the code") {
		t.Fatalf("unexpected reason: %s", findings[0].reason)
	}
}
