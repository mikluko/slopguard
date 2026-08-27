package rule

import (
	"strings"
	"testing"

	"github.com/mikluko/slopguard/internal/comment"
	"github.com/mikluko/slopguard/internal/model"
)

// YAML is the language whose prose parses as a value: `# Note: this cluster is
// shared` is a mapping, and so is `# replicas: 3`. Every row here is one of
// those two wearing the other's shape, which is what the rule in yaml.go has to
// tell apart, and what makes a bare parse worth nothing on its own.
var yamlCases = []struct {
	name string
	src  string
	want string
}{
	{
		name: "a comment",
		src: `# Requests are per replica.
replicas: 2
`,
	},
	{
		name: "prose reading as a mapping",
		src: `# note: this cluster is shared
replicas: 2
`,
	},
	{
		name: "config switched off",
		src: `replicas: 2
# resources:
#   limits:
#     cpu: 100m
image: nginx
`,
		want: "commented-out code",
	},
	{
		name: "documentation carrying a constraint",
		src: `# Replica count for the deployment.
# Two is the floor for the pod disruption budget.
# Raising it above four needs a node pool change.
replicas: 2
`,
	},
	{
		// What `helm create` scaffolds. Both of the rule's instructions are
		// wrong here: deleting the block deletes the documentation, and making
		// it real changes what the chart deploys.
		name: "a commented shape under an empty key documents it",
		src: `podSecurityContext: {}
  # fsGroup: 2000
  # runAsNonRoot: true

replicaCount: 1
`,
	},
	{
		name: "the same shape under a key that is not empty is residue",
		src: `podSecurityContext:
  runAsNonRoot: true
  # fsGroup: 2000
  # runAsUser: 1000

replicaCount: 1
`,
		want: "commented-out",
	},
	{
		// The form real charts use, as against the one `helm create` scaffolds:
		// the marker sits flush left and the indentation is inside the comment.
		// Testing the marker's own column exempted none of these and fired 87
		// times across 233 YAML findings.
		name: "a flush-left marker under an empty key documents it",
		src: `# -- hostAliases to add
hostAliases: []
#  - ip: 1.2.3.4
#    hostnames:
#      - domain.tld

replicaCount: 1
`,
		want: "",
	},
	{
		// The majority form in every chart measured, and the one the empty-key
		// exemption misses: the sentences above the block are part of the same
		// run and parse as mapping keys of their own. It is also the example the
		// README prints as exempt, which for one round it was not.
		name: "a setting introduced by prose is being documented",
		src: `image:
  repository: example/app
  ## Optionally specify an array of imagePullSecrets.
  # pullSecrets:
  #   - myRegistrKeySecretName
`,
		want: "",
	},
	{
		// The sentence test reads the line before its dash comes off, so a flag
		// list is configuration however many spaces it holds.
		name: "a commented-out argument list is not prose",
		src: `args:
  # - --log.level=debug --web.enable-lifecycle
  # - --storage.tsdb.retention=15d
  live: true
`,
		want: "commented-out",
	},
	{
		name: "a heading that ends in a colon is still prose",
		src: `# Example proxy configuration:
# proxy_url: http://proxy:3128
proxy_url: ""
`,
		want: "",
	},
	{
		name: "residue carries no sentence",
		src: `image:
  repository: example/app
  # pullSecrets:
  #   - myRegistrKeySecretName
`,
		want: "commented-out",
	},
	{
		name: "an empty key does not excuse a block beside it",
		src: `podSecurityContext: {}

# resources:
#   limits:
#     cpu: 100m

replicaCount: 1
`,
		want: "commented-out",
	},
	{
		name: "prose whose every key is titled",
		src: `# Ref: https://runbooks.example.com/scale-up
# Owner: platform-team
replicas: 2
`,
	},
	{
		name: "a single line of config switched off",
		src: `# replicas: 3
replicas: 5
`,
		want: "commented-out",
	},
	{
		// A `{{- if }}` at column zero otherwise makes the grammar drop every
		// comment in the file, so the same comment would be read in one
		// manifest and invisible in the next.
		name: "a helm template keeps its comments",
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
		name: "a chart values header",
		src: `# Default values for the chart.
image:
  repository: nginx
  tag: "1.27"
`,
	},
}

func TestYAML(t *testing.T) {
	for _, c := range yamlCases {
		t.Run(c.name, func(t *testing.T) {
			if model.Speaks(c.want) {
				skipWithoutRuntime(t)
			}
			findings := scan([]byte(c.src), yaml, []comment.Span{{Start: 0, End: uint(len(c.src))}})
			switch {
			case c.want == "" && len(findings) > 0:
				t.Fatalf("nudged an acceptable comment: %s", findings[0].Reason)
			case c.want == "":
			case len(findings) == 0:
				t.Fatalf("missed a comment that should carry %q", c.want)
			case !strings.Contains(findings[0].Reason, c.want):
				t.Fatalf("reason %q does not carry %q", findings[0].Reason, c.want)
			}
		})
	}
}
