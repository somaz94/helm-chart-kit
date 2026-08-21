// Package catalog is the single source of truth for what hck can generate:
// the resource templates it knows and the presets that group them.
//
// Every entry here must have a matching directory under templates/resources/,
// and that invariant is enforced by a test rather than at runtime — a missing
// template is a build-time mistake, not something a user can trigger.
package catalog

import (
	"slices"
	"strings"
)

// Group is what a resource is for, which is the question somebody adding one
// actually has. The catalog is 32 resources and their names are Kubernetes
// kinds, so an alphabetical list answers "what exists" and nothing else:
// finding the three pieces of a monitoring setup meant already knowing they
// are called servicemonitor, prometheusrule and grafanadashboard.
type Group string

const (
	WorkloadGroup      Group = "workload"
	ExposureGroup      Group = "exposure"
	ScalingGroup       Group = "scaling"
	AccessGroup        Group = "access"
	SecretsGroup       Group = "secrets"
	ObservabilityGroup Group = "observability"
	MeshGroup          Group = "mesh"
	ChartGroup         Group = "chart"
)

// groups are listed in the order a chart is usually built up rather than
// alphabetically: something runs, something reaches it, it scales, it is
// locked down, it is watched. The listing follows this order, so reading it
// top to bottom is roughly the order the decisions come in.
var groups = []struct {
	Name    Group
	Summary string
}{
	{WorkloadGroup, "what runs"},
	{ExposureGroup, "what reaches it"},
	{ScalingGroup, "how many, and what may evict them"},
	{AccessGroup, "identity, permissions and who may connect"},
	{SecretsGroup, "secrets and the certificates that need them"},
	{ObservabilityGroup, "scraping, alerting and dashboards"},
	{MeshGroup, "Istio routing and policy"},
	{ChartGroup, "configuration, storage and the chart's own install test"},
}

// Groups returns every group in build order, with its one-line summary.
func Groups() []struct {
	Name    Group
	Summary string
} {
	return append([]struct {
		Name    Group
		Summary string
	}{}, groups...)
}

// LookupGroup reports whether the name is a group.
func LookupGroup(name string) (Group, bool) {
	for _, g := range groups {
		if string(g.Name) == name {
			return g.Name, true
		}
	}
	return "", false
}

// GroupNames lists every group name, in build order.
func GroupNames() []string {
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		out = append(out, string(g.Name))
	}
	return out
}

// ResourcesInGroup returns the group's resources, sorted by name.
func ResourcesInGroup(g Group) []Resource {
	var out []Resource
	for _, r := range Resources() {
		if r.Group == g {
			out = append(out, r)
		}
	}
	return out
}

// Resource is one generatable Helm template plus the values it introduces.
type Resource struct {
	// Name is the identifier passed to "hck add".
	Name string
	// File is the emitted file name under the chart's templates/ directory.
	File string
	// Summary is the one-line description shown by "hck list resources".
	Summary string
	// APIVersion is the Kubernetes apiVersion the template emits. Shown in
	// listings so a user can tell at a glance whether their cluster has it.
	APIVersion string
	// ValuesKeys are the top-level values.yaml keys this resource
	// contributes, in the order its fragment declares them. The merge itself
	// reads the fragment rather than this list, so the two are kept honest by
	// a test instead: TestValuesKeysMatchTheTemplates compares this against
	// both values.yaml.tmpl and schema.json.tmpl, and every key a resource
	// contributes has to appear in all three.
	ValuesKeys []string
	// Requires names resources that must exist for this one to render. "hck
	// add" reports them rather than pulling them in silently.
	Requires []string
	// Optional marks a resource that depends on a CRD or feature gate the
	// target cluster may not have (Gateway API, Prometheus Operator, ESO).
	Optional bool
	// Group is the purpose this resource serves, and what "hck list
	// resources" sorts it under. Every resource has one — a resource with no
	// group would simply not appear in the listing, which is the one failure
	// nobody would notice, so TestEveryResourceHasAKnownGroup pins it.
	Group Group

	// Workload marks the resource as a chart's primary workload. A preset
	// carries exactly one, and a chart may hold two — guarded so that one
	// renders at a time, which is how a Deployment is swapped for a Rollout.
	// Two rendering at once is what HCK030 reports, over the render rather
	// than over this flag.
	Workload bool
}

// Preset is a named set of resources used to seed a new chart, together with
// the answers that set implies.
//
// Platform, Environment, Schema and Docs exist so that "hck init" can ask
// three questions instead of seven. A preset written for one platform knows
// which one, and a preset modelled on a chart that ships a values.schema.json
// knows that too. They are defaults and not decisions: init shows what the
// preset resolved to and lets it be changed, and every flag still overrides.
type Preset struct {
	Name      string
	Summary   string
	Resources []string
	// Platform names the platform overlay this preset is written for, or ""
	// when it is not tied to one.
	Platform string
	// Environment names the environment overlay to start with, or "".
	Environment string
	// Schema asks for a values.schema.json. It stays opt-in either way: what
	// this changes is the default init offers, and the command init prints
	// still spells --schema out.
	Schema bool
	// Docs asks for a values table in README.md.
	Docs bool
}

var resources = []Resource{
	{
		Name:       "deployment",
		Group:      WorkloadGroup,
		Workload:   true,
		File:       "deployment.yaml",
		Summary:    "Stateless workload with probes, resources and securityContext",
		APIVersion: "apps/v1",
		ValuesKeys: []string{"replicaCount", "revisionHistoryLimit", "updateStrategy", "terminationGracePeriodSeconds", "priorityClassName", "image", "imagePullSecrets", "containerPort", "command", "args", "env", "envFrom", "podAnnotations", "podLabels", "podSecurityContext", "securityContext", "livenessProbe", "readinessProbe", "startupProbe", "resources", "nodeSelector", "affinity", "tolerations", "topologySpreadConstraints", "volumes", "volumeMounts"},
		Requires:   []string{"serviceaccount"},
	},
	{
		Name:       "statefulset",
		Group:      WorkloadGroup,
		Workload:   true,
		File:       "statefulset.yaml",
		Summary:    "Stateful workload with volumeClaimTemplates and a headless Service",
		APIVersion: "apps/v1",
		ValuesKeys: []string{"replicaCount", "podManagementPolicy", "updateStrategy", "terminationGracePeriodSeconds", "image", "imagePullSecrets", "containerPort", "env", "podAnnotations", "podLabels", "podSecurityContext", "securityContext", "livenessProbe", "readinessProbe", "resources", "nodeSelector", "affinity", "tolerations", "topologySpreadConstraints", "volumes", "volumeMounts", "persistence"},
		Requires:   []string{"serviceaccount", "service"},
	},
	{
		Name:       "daemonset",
		Group:      WorkloadGroup,
		Workload:   true,
		File:       "daemonset.yaml",
		Summary:    "Node-local workload with host tolerations",
		APIVersion: "apps/v1",
		ValuesKeys: []string{"updateStrategy", "image", "imagePullSecrets", "hostNetwork", "env", "podAnnotations", "podLabels", "podSecurityContext", "securityContext", "resources", "nodeSelector", "volumes", "volumeMounts", "tolerations"},
		Requires:   []string{"serviceaccount"},
	},
	{
		Name:       "cronjob",
		Group:      WorkloadGroup,
		Workload:   true,
		File:       "cronjob.yaml",
		Summary:    "Scheduled job with concurrency policy and history limits",
		APIVersion: "batch/v1",
		ValuesKeys: []string{"cronjob", "image", "imagePullSecrets", "env", "podAnnotations", "podSecurityContext", "securityContext", "resources", "nodeSelector", "tolerations"},
		Requires:   []string{"serviceaccount"},
	},
	{
		Name:       "job",
		Group:      WorkloadGroup,
		File:       "job.yaml",
		Summary:    "One-shot job, optionally gated on a Helm hook",
		APIVersion: "batch/v1",
		ValuesKeys: []string{"job"},
		Requires:   []string{"serviceaccount"},
	},
	{
		Name:       "service",
		Group:      ExposureGroup,
		File:       "service.yaml",
		Summary:    "Service, with headless mode for StatefulSet peers",
		APIVersion: "v1",
		ValuesKeys: []string{"service"},
	},
	{
		Name:       "ingress",
		Group:      ExposureGroup,
		File:       "ingress.yaml",
		Summary:    "Ingress with TLS blocks and per-path pathType",
		APIVersion: "networking.k8s.io/v1",
		ValuesKeys: []string{"ingress"},
		Requires:   []string{"service"},
	},
	{
		Name:       "httproute",
		Group:      ExposureGroup,
		File:       "httproute.yaml",
		Summary:    "Gateway API HTTPRoute — the successor to Ingress",
		APIVersion: "gateway.networking.k8s.io/v1",
		ValuesKeys: []string{"httpRoute"},
		Requires:   []string{"service"},
		Optional:   true,
	},
	{
		Name:       "hpa",
		Group:      ScalingGroup,
		File:       "hpa.yaml",
		Summary:    "HorizontalPodAutoscaler on CPU and memory utilization",
		APIVersion: "autoscaling/v2",
		ValuesKeys: []string{"autoscaling"},
	},
	{
		Name:       "pdb",
		Group:      ScalingGroup,
		File:       "pdb.yaml",
		Summary:    "PodDisruptionBudget guarding voluntary evictions",
		APIVersion: "policy/v1",
		ValuesKeys: []string{"podDisruptionBudget"},
	},
	{
		Name:       "networkpolicy",
		Group:      AccessGroup,
		File:       "networkpolicy.yaml",
		Summary:    "Default-deny NetworkPolicy with explicit ingress and egress rules",
		APIVersion: "networking.k8s.io/v1",
		ValuesKeys: []string{"networkPolicy"},
	},
	{
		Name:       "serviceaccount",
		Group:      AccessGroup,
		File:       "serviceaccount.yaml",
		Summary:    "ServiceAccount with annotations for cloud identity binding",
		APIVersion: "v1",
		ValuesKeys: []string{"serviceAccount"},
	},
	{
		Name:       "rbac",
		Group:      AccessGroup,
		File:       "rbac.yaml",
		Summary:    "Role or ClusterRole plus the matching binding",
		APIVersion: "rbac.authorization.k8s.io/v1",
		ValuesKeys: []string{"rbac"},
		Requires:   []string{"serviceaccount"},
	},
	{
		Name:       "configmap",
		Group:      ChartGroup,
		File:       "configmap.yaml",
		Summary:    "ConfigMap with a checksum annotation hook for rollout on change",
		APIVersion: "v1",
		ValuesKeys: []string{"configMap"},
	},
	{
		Name:       "secret",
		Group:      SecretsGroup,
		File:       "secret.yaml",
		Summary:    "Opaque Secret — values are stringData, never committed",
		APIVersion: "v1",
		ValuesKeys: []string{"secret"},
	},
	{
		Name:       "externalsecret",
		Group:      SecretsGroup,
		File:       "externalsecret.yaml",
		Summary:    "External Secrets Operator ExternalSecret pulling from a SecretStore",
		APIVersion: "external-secrets.io/v1",
		ValuesKeys: []string{"externalSecret"},
		Optional:   true,
	},
	{
		Name:       "pvc",
		Group:      ChartGroup,
		File:       "pvc.yaml",
		Summary:    "Standalone PersistentVolumeClaim with a Helm resource-policy keep",
		APIVersion: "v1",
		ValuesKeys: []string{"persistence"},
	},
	{
		Name:       "servicemonitor",
		Group:      ObservabilityGroup,
		File:       "servicemonitor.yaml",
		Summary:    "Prometheus Operator ServiceMonitor scrape config",
		APIVersion: "monitoring.coreos.com/v1",
		ValuesKeys: []string{"metrics"},
		Requires:   []string{"service"},
		Optional:   true,
	},
	{
		Name:       "prometheusrule",
		Group:      ObservabilityGroup,
		File:       "prometheusrule.yaml",
		Summary:    "Prometheus Operator PrometheusRule alert group",
		APIVersion: "monitoring.coreos.com/v1",
		ValuesKeys: []string{"prometheusRule"},
		Optional:   true,
	},
	{
		Name:       "certificate",
		Group:      SecretsGroup,
		File:       "certificate.yaml",
		Summary:    "cert-manager Certificate feeding a TLS Secret",
		APIVersion: "cert-manager.io/v1",
		ValuesKeys: []string{"certificate"},
		Optional:   true,
	},
	{
		Name:       "podmonitor",
		Group:      ObservabilityGroup,
		File:       "podmonitor.yaml",
		Summary:    "Prometheus Operator PodMonitor, scraping pods without a Service",
		APIVersion: "monitoring.coreos.com/v1",
		ValuesKeys: []string{"podMonitor"},
		Optional:   true,
	},
	{
		Name:       "scaledobject",
		Group:      ScalingGroup,
		File:       "scaledobject.yaml",
		Summary:    "KEDA ScaledObject scaling on queue depth rather than CPU",
		APIVersion: "keda.sh/v1alpha1",
		ValuesKeys: []string{"scaledObject"},
		Optional:   true,
	},
	{
		Name:       "vpa",
		Group:      ScalingGroup,
		File:       "vpa.yaml",
		Summary:    "VerticalPodAutoscaler sizing requests from observed usage",
		APIVersion: "autoscaling.k8s.io/v1",
		ValuesKeys: []string{"verticalPodAutoscaler"},
		Optional:   true,
	},
	{
		Name:       "sealedsecret",
		Group:      SecretsGroup,
		File:       "sealedsecret.yaml",
		Summary:    "Sealed Secret whose ciphertext is safe to commit",
		APIVersion: "bitnami.com/v1alpha1",
		ValuesKeys: []string{"sealedSecret"},
		Optional:   true,
	},
	{
		Name:       "issuer",
		Group:      SecretsGroup,
		File:       "issuer.yaml",
		Summary:    "Namespaced cert-manager Issuer: ACME, CA or self-signed",
		APIVersion: "cert-manager.io/v1",
		ValuesKeys: []string{"issuer"},
		Optional:   true,
	},
	{
		Name:       "referencegrant",
		Group:      ExposureGroup,
		File:       "referencegrant.yaml",
		Summary:    "Gateway API grant letting another namespace reach this Service",
		APIVersion: "gateway.networking.k8s.io/v1beta1",
		ValuesKeys: []string{"referenceGrant"},
		Requires:   []string{"service"},
		Optional:   true,
	},
	{
		Name:       "scaledjob",
		Group:      ScalingGroup,
		File:       "scaledjob.yaml",
		Summary:    "KEDA ScaledJob: one Job per queue item",
		APIVersion: "keda.sh/v1alpha1",
		ValuesKeys: []string{"scaledJob"},
		Requires:   []string{"serviceaccount"},
		Optional:   true,
	},
	{
		Name:       "virtualservice",
		Group:      MeshGroup,
		File:       "virtualservice.yaml",
		Summary:    "Istio VirtualService — routing for a mesh that is not using Gateway API",
		APIVersion: "networking.istio.io/v1",
		ValuesKeys: []string{"virtualService"},
		Requires:   []string{"service"},
		Optional:   true,
	},
	{
		Name:       "destinationrule",
		Group:      MeshGroup,
		File:       "destinationrule.yaml",
		Summary:    "Istio DestinationRule: pool limits, outlier ejection, mTLS mode",
		APIVersion: "networking.istio.io/v1",
		ValuesKeys: []string{"destinationRule"},
		Requires:   []string{"service"},
		Optional:   true,
	},
	{
		Name:       "authorizationpolicy",
		Group:      MeshGroup,
		File:       "authorizationpolicy.yaml",
		Summary:    "Istio AuthorizationPolicy — who may call this workload, at L7",
		APIVersion: "security.istio.io/v1",
		ValuesKeys: []string{"authorizationPolicy"},
		Optional:   true,
	},
	{
		Name:       "grafanadashboard",
		Group:      ObservabilityGroup,
		File:       "grafana-dashboard.yaml",
		Summary:    "Dashboard ConfigMap the Grafana sidecar picks up",
		APIVersion: "v1",
		ValuesKeys: []string{"grafanaDashboard"},
		Optional:   true,
	},
	{
		Name:       "tests",
		Group:      ChartGroup,
		File:       "tests/test-connection.yaml",
		Summary:    "helm test hook that dials the Service",
		APIVersion: "v1",
		ValuesKeys: []string{"tests"},
		Requires:   []string{"service"},
	},
}

var presets = []Preset{
	{
		// The two "base" presets are modelled on a pair of house charts that
		// an ApplicationSet repo keeps side by side: one for on-prem, one for
		// EKS. They are the same chart answering a different platform, which
		// is why they are two presets carrying a Platform rather than one
		// preset and a flag to remember.
		//
		// Ingress and HTTPRoute both: the chart they come from guards each on
		// its own .enabled and picks one per install, rather than choosing at
		// build time the way "web" and "gateway" do.
		//
		// Not carried over, for want of a resource to carry them: a
		// certificate-renewal CronJob, an imagePullSecret, and a
		// PersistentVolume to match the claim.
		Name:      "base",
		Summary:   "On-prem house chart: Deployment, Service, Ingress or HTTPRoute, HPA, PVC, Certificate",
		Resources: []string{"serviceaccount", "deployment", "service", "ingress", "httproute", "hpa", "pvc", "certificate", "configmap", "tests"},
		Platform:  "onprem",
		Schema:    true,
		Docs:      true,
	},
	{
		// Not carried over: an Argo Rollout and its AnalysisTemplate, an EFS
		// PersistentVolume, the three extra Ingresses a preview environment
		// wants, and a preview Service. The Rollout is the one worth naming —
		// "hck add" no longer refuses a second workload, so a Deployment and
		// a Rollout guarded against each other is a chart hck can hold; it
		// has no template for the Rollout itself yet.
		Name:      "base-aws",
		Summary:   "EKS house chart: base on AWS, with an ExternalSecret and a PDB, no Certificate",
		Resources: []string{"serviceaccount", "deployment", "service", "ingress", "hpa", "pdb", "externalsecret", "pvc", "configmap", "tests"},
		Platform:  "aws",
		Schema:    true,
		Docs:      true,
	},
	{
		Name:      "web",
		Summary:   "HTTP service: Deployment, Service, Ingress, HPA, PDB, NetworkPolicy",
		Resources: []string{"serviceaccount", "deployment", "service", "ingress", "hpa", "pdb", "networkpolicy", "configmap", "tests"},
	},
	{
		Name:      "worker",
		Summary:   "Queue consumer: Deployment with no Service and no ingress path",
		Resources: []string{"serviceaccount", "deployment", "pdb", "networkpolicy", "configmap"},
	},
	{
		Name:      "cronjob",
		Summary:   "Scheduled task: CronJob, ServiceAccount, ConfigMap",
		Resources: []string{"serviceaccount", "cronjob", "configmap"},
	},
	{
		Name:      "stateful",
		Summary:   "Stateful service: StatefulSet, headless Service, PDB, NetworkPolicy",
		Resources: []string{"serviceaccount", "statefulset", "service", "pdb", "networkpolicy", "configmap"},
	},
	{
		// No PDB: kubectl drain skips DaemonSet pods rather than evicting
		// them, so a budget over them constrains nothing. No tests either —
		// the test hook dials a Service, and a node agent has none.
		Name:      "daemon",
		Summary:   "Node agent: DaemonSet on every node, no Service",
		Resources: []string{"serviceaccount", "daemonset", "configmap", "networkpolicy"},
	},
	{
		// The smallest chart that is worth generating: something running, and
		// something in front of it. Everything else is "hck add", which is
		// the point — a first chart nobody has to read before trusting.
		Name:      "minimal",
		Summary:   "Smallest chart that runs and is reachable: Deployment and Service",
		Resources: []string{"serviceaccount", "deployment", "service"},
	},
	{
		// "web" with the Gateway API in place of the Ingress. The two are the
		// same shape and the same decision, which is why this is a preset
		// rather than a flag: a chart carries one of them, not both.
		Name:      "gateway",
		Summary:   "HTTP service on Gateway API: Deployment, Service, HTTPRoute, HPA, PDB",
		Resources: []string{"serviceaccount", "deployment", "service", "httproute", "hpa", "pdb", "networkpolicy", "configmap", "tests"},
	},
	{
		// No NetworkPolicy: in a mesh, who may call this workload is the
		// AuthorizationPolicy's answer, at L7 and with an identity. A second
		// answer at L3 is a different question with the same name, and the
		// two disagreeing is a debugging session nobody enjoys.
		Name:      "mesh",
		Summary:   "Istio service: Deployment, Service, VirtualService, DestinationRule, AuthorizationPolicy",
		Resources: []string{"serviceaccount", "deployment", "service", "virtualservice", "destinationrule", "authorizationpolicy", "hpa", "pdb", "configmap", "tests"},
	},
	{
		// No HPA: a KEDA ScaledObject owns the replica count, and the two
		// driving it together is what HCK031 reports. A queue consumer scales
		// on queue depth, which is the reason to reach for KEDA at all.
		Name:      "queue",
		Summary:   "KEDA consumer: Deployment scaled on queue depth, no Service",
		Resources: []string{"serviceaccount", "deployment", "scaledobject", "pdb", "networkpolicy", "configmap"},
	},
	{
		Name:      "monitored",
		Summary:   "web, plus a ServiceMonitor, alert rules and a Grafana dashboard",
		Resources: []string{"serviceaccount", "deployment", "service", "ingress", "hpa", "pdb", "networkpolicy", "configmap", "servicemonitor", "prometheusrule", "grafanadashboard", "tests"},
	},
	{
		// A Certificate and no Issuer: the Certificate defaults to a
		// ClusterIssuer, which is where a shared one lives. Shipping the
		// namespaced Issuer alongside it is what HCK034 reports, and "hck add
		// issuer" is there for the chart that genuinely wants its own.
		Name:      "secure",
		Summary:   "web, plus RBAC, a cert-manager Certificate and an ExternalSecret",
		Resources: []string{"serviceaccount", "rbac", "deployment", "service", "ingress", "certificate", "externalsecret", "hpa", "pdb", "networkpolicy", "configmap", "tests"},
	},
}

// entry is anything the catalog indexes by name. Resources, presets and
// overlays are three lists asked the same three questions — list them, find
// one, name them all — and the answers below are those questions written once
// instead of once per list.
type entry interface {
	entryName() string
}

func (r Resource) entryName() string { return r.Name }
func (p Preset) entryName() string   { return p.Name }

// sorted copies a catalog list and orders it by name. The copy is the point:
// the package variables are the source of truth and a caller that sorts in
// place would reorder them for everyone else.
func sorted[T entry](items []T) []T {
	out := slices.Clone(items)
	slices.SortFunc(out, func(a, b T) int { return strings.Compare(a.entryName(), b.entryName()) })
	return out
}

// lookup finds one entry by name.
func lookup[T entry](items []T, name string) (T, bool) {
	for _, it := range items {
		if it.entryName() == name {
			return it, true
		}
	}
	var zero T
	return zero, false
}

// names lists every entry's name, ordered.
func names[T entry](items []T) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.entryName())
	}
	slices.Sort(out)
	return out
}

// Resources returns every known resource, ordered by name.
func Resources() []Resource { return sorted(resources) }

// Presets returns every known preset, ordered by name.
func Presets() []Preset { return sorted(presets) }

// LookupResource finds a resource by name.
func LookupResource(name string) (Resource, bool) { return lookup(resources, name) }

// LookupPreset finds a preset by name.
func LookupPreset(name string) (Preset, bool) { return lookup(presets, name) }

// ResourceNames returns every resource name, ordered.
func ResourceNames() []string { return names(resources) }

// PresetNames returns every preset name, ordered.
func PresetNames() []string { return names(presets) }
