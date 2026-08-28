package rule

import (
	"slices"
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
		// An Ansible role spells the same idiom flush left, which is where a
		// sweep of geerlingguy's roles produced a false positive.
		name: "a menu flush left under an empty list documents it",
		src: `redis_disabled_commands: []
#  - FLUSHDB
#  - FLUSHALL
#  - KEYS

redis_maxmemory: 0
`,
	},
	{
		name: "a commented shape under an empty list documents it",
		src: `tls: []
  # - secretName: chart-example-tls
  #   hosts:
  #     - chart-example.local

replicaCount: 1
`,
	},
	{
		name: "a commented shape under a key with no value documents it",
		src: `podSecurityContext:
  # fsGroup: 2000
  # runAsNonRoot: true

replicaCount: 1
`,
	},
	{
		name: "a commented shape under an empty string documents it",
		src: `nodeSelector: ""
  # kubernetes.io/os: linux
  # kubernetes.io/arch: amd64

replicaCount: 1
`,
	},
	{
		name: "a block dedented past the empty key above it is residue",
		src: `image:
  pullSecrets: []
# resources:
#   limits:
#     cpu: 100m

replicaCount: 1
`,
		want: "commented-out",
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

// TestHelmScaffold holds the carve-out's boundary against the file it was drawn
// from. The scaffold puts eight documented-but-unset keys and two settings
// commented under keys that already carry values in one file, so a carve-out
// widened past the idiom shows up here as a line that stopped being reported
// rather than as a row nobody thought to add.
func TestHelmScaffold(t *testing.T) {
	var lines []uint
	for _, finding := range scan([]byte(helmScaffold), yaml, whole([]byte(helmScaffold))) {
		lines = append(lines, finding.Line)
	}
	slices.Sort(lines)
	if want := []uint{88, 142}; !slices.Equal(lines, want) {
		t.Fatalf("reported lines %v, want %v", lines, want)
	}
}

// helmScaffold is `helm create` v4.2.4's values.yaml verbatim. Verbatim is the
// point: the two lines the test names are line numbers in it, and trimming it
// to the interesting keys would move them.
const helmScaffold = `# Default values for demo.
# This is a YAML-formatted file.
# Declare variables to be passed into your templates.

# This will set the replicaset count more information can be found here: https://kubernetes.io/docs/concepts/workloads/controllers/replicaset/
replicaCount: 1

# This sets the container image more information can be found here: https://kubernetes.io/docs/concepts/containers/images/
image:
  repository: nginx
  # This sets the pull policy for images.
  pullPolicy: IfNotPresent
  # Overrides the image tag whose default is the chart appVersion.
  tag: ""

# This is for the secrets for pulling an image from a private repository more information can be found here: https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/
imagePullSecrets: []
# This is to override the chart name.
nameOverride: ""
fullnameOverride: ""

# This section builds out the service account more information can be found here: https://kubernetes.io/docs/concepts/security/service-accounts/
serviceAccount:
  # Specifies whether a service account should be created.
  create: true
  # Automatically mount a ServiceAccount's API credentials?
  automount: true
  # Annotations to add to the service account.
  annotations: {}
  # The name of the service account to use.
  # If not set and create is true, a name is generated using the fullname template.
  name: ""

# This is for setting Kubernetes Annotations to a Pod.
# For more information checkout: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/
podAnnotations: {}
# This is for setting Kubernetes Labels to a Pod.
# For more information checkout: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
podLabels: {}

podSecurityContext: {}
  # fsGroup: 2000

securityContext: {}
  # capabilities:
  #   drop:
  #   - ALL
  # readOnlyRootFilesystem: true
  # runAsNonRoot: true
  # runAsUser: 1000

# This is for setting up a service more information can be found here: https://kubernetes.io/docs/concepts/services-networking/service/
service:
  # This sets the service type more information can be found here: https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services-service-types
  type: ClusterIP
  # This sets the ports more information can be found here: https://kubernetes.io/docs/concepts/services-networking/service/#field-spec-ports
  port: 80

# This block is for setting up the ingress for more information can be found here: https://kubernetes.io/docs/concepts/services-networking/ingress/
ingress:
  enabled: false
  className: ""
  annotations: {}
    # kubernetes.io/ingress.class: nginx
    # kubernetes.io/tls-acme: "true"
  hosts:
    - host: chart-example.local
      paths:
        - path: /
          pathType: ImplementationSpecific
  tls: []
    # - secretName: chart-example-tls
    #   hosts:
    #     - chart-example.local

# -- Expose the service via gateway-api HTTPRoute
# Requires Gateway API resources and suitable controller installed within the cluster
# (see: https://gateway-api.sigs.k8s.io/guides/)
httpRoute:
  # HTTPRoute enabled.
  enabled: false
  # HTTPRoute annotations.
  annotations: {}
  # Which Gateways this Route is attached to.
  parentRefs:
  - name: gateway
    sectionName: http
    # namespace: default
  # Hostnames matching HTTP header.
  hostnames:
  - chart-example.local
  # List of rules and filters applied.
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /headers
  #   filters:
  #   - type: RequestHeaderModifier
  #     requestHeaderModifier:
  #       set:
  #       - name: My-Overwrite-Header
  #         value: this-is-the-only-value
  #       remove:
  #       - User-Agent
  # - matches:
  #   - path:
  #       type: PathPrefix
  #       value: /echo
  #     headers:
  #     - name: version
  #       value: v2

resources: {}
  # We usually recommend not to specify default resources and to leave this as a conscious
  # choice for the user. This also increases chances charts run on environments with little
  # resources, such as Minikube. If you do want to specify resources, uncomment the following
  # lines, adjust them as necessary, and remove the curly braces after 'resources:'.
  # limits:
  #   cpu: 100m
  #   memory: 128Mi
  # requests:
  #   cpu: 100m
  #   memory: 128Mi

# This is to setup the liveness and readiness probes more information can be found here: https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
livenessProbe:
  httpGet:
    path: /
    port: http
readinessProbe:
  httpGet:
    path: /
    port: http

# This section is for setting up autoscaling more information can be found here: https://kubernetes.io/docs/concepts/workloads/autoscaling/
autoscaling:
  enabled: false
  minReplicas: 1
  maxReplicas: 100
  targetCPUUtilizationPercentage: 80
  # targetMemoryUtilizationPercentage: 80

# Additional volumes on the output Deployment definition.
volumes: []
  # - name: foo
  #   secret:
  #     secretName: mysecret
  #     optional: false

# Additional volumeMounts on the output Deployment definition.
volumeMounts: []
  # - name: foo
  #   mountPath: "/etc/foo"
  #   readOnly: true

nodeSelector: {}

tolerations: []

affinity: {}
`
