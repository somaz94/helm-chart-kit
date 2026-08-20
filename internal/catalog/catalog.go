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
	// Workload marks the resource as a chart's primary workload. A chart
	// carries exactly one: they contend for the same values keys — image,
	// resources, updateStrategy — with incompatible shapes, so a second one
	// would render but not apply.
	Workload bool
}

// Preset is a named set of resources used to seed a new chart.
type Preset struct {
	Name      string
	Summary   string
	Resources []string
}

var resources = []Resource{
	{
		Name:       "deployment",
		Workload:   true,
		File:       "deployment.yaml",
		Summary:    "Stateless workload with probes, resources and securityContext",
		APIVersion: "apps/v1",
		ValuesKeys: []string{"replicaCount", "revisionHistoryLimit", "updateStrategy", "terminationGracePeriodSeconds", "priorityClassName", "image", "imagePullSecrets", "containerPort", "command", "args", "env", "envFrom", "podAnnotations", "podLabels", "podSecurityContext", "securityContext", "livenessProbe", "readinessProbe", "startupProbe", "resources", "nodeSelector", "affinity", "tolerations", "topologySpreadConstraints", "volumes", "volumeMounts"},
		Requires:   []string{"serviceaccount"},
	},
	{
		Name:       "statefulset",
		Workload:   true,
		File:       "statefulset.yaml",
		Summary:    "Stateful workload with volumeClaimTemplates and a headless Service",
		APIVersion: "apps/v1",
		ValuesKeys: []string{"replicaCount", "podManagementPolicy", "updateStrategy", "terminationGracePeriodSeconds", "image", "imagePullSecrets", "containerPort", "env", "podAnnotations", "podLabels", "podSecurityContext", "securityContext", "livenessProbe", "readinessProbe", "resources", "nodeSelector", "affinity", "tolerations", "topologySpreadConstraints", "volumes", "volumeMounts", "persistence"},
		Requires:   []string{"serviceaccount", "service"},
	},
	{
		Name:       "daemonset",
		Workload:   true,
		File:       "daemonset.yaml",
		Summary:    "Node-local workload with host tolerations",
		APIVersion: "apps/v1",
		ValuesKeys: []string{"updateStrategy", "image", "imagePullSecrets", "hostNetwork", "env", "podAnnotations", "podLabels", "podSecurityContext", "securityContext", "resources", "nodeSelector", "volumes", "volumeMounts", "tolerations"},
		Requires:   []string{"serviceaccount"},
	},
	{
		Name:       "cronjob",
		Workload:   true,
		File:       "cronjob.yaml",
		Summary:    "Scheduled job with concurrency policy and history limits",
		APIVersion: "batch/v1",
		ValuesKeys: []string{"cronjob", "image", "imagePullSecrets", "env", "podAnnotations", "podSecurityContext", "securityContext", "resources", "nodeSelector", "tolerations"},
		Requires:   []string{"serviceaccount"},
	},
	{
		Name:       "job",
		File:       "job.yaml",
		Summary:    "One-shot job, optionally gated on a Helm hook",
		APIVersion: "batch/v1",
		ValuesKeys: []string{"job"},
		Requires:   []string{"serviceaccount"},
	},
	{
		Name:       "service",
		File:       "service.yaml",
		Summary:    "Service, with headless mode for StatefulSet peers",
		APIVersion: "v1",
		ValuesKeys: []string{"service"},
	},
	{
		Name:       "ingress",
		File:       "ingress.yaml",
		Summary:    "Ingress with TLS blocks and per-path pathType",
		APIVersion: "networking.k8s.io/v1",
		ValuesKeys: []string{"ingress"},
		Requires:   []string{"service"},
	},
	{
		Name:       "httproute",
		File:       "httproute.yaml",
		Summary:    "Gateway API HTTPRoute — the successor to Ingress",
		APIVersion: "gateway.networking.k8s.io/v1",
		ValuesKeys: []string{"httpRoute"},
		Requires:   []string{"service"},
		Optional:   true,
	},
	{
		Name:       "hpa",
		File:       "hpa.yaml",
		Summary:    "HorizontalPodAutoscaler on CPU and memory utilization",
		APIVersion: "autoscaling/v2",
		ValuesKeys: []string{"autoscaling"},
	},
	{
		Name:       "pdb",
		File:       "pdb.yaml",
		Summary:    "PodDisruptionBudget guarding voluntary evictions",
		APIVersion: "policy/v1",
		ValuesKeys: []string{"podDisruptionBudget"},
	},
	{
		Name:       "networkpolicy",
		File:       "networkpolicy.yaml",
		Summary:    "Default-deny NetworkPolicy with explicit ingress and egress rules",
		APIVersion: "networking.k8s.io/v1",
		ValuesKeys: []string{"networkPolicy"},
	},
	{
		Name:       "serviceaccount",
		File:       "serviceaccount.yaml",
		Summary:    "ServiceAccount with annotations for cloud identity binding",
		APIVersion: "v1",
		ValuesKeys: []string{"serviceAccount"},
	},
	{
		Name:       "rbac",
		File:       "rbac.yaml",
		Summary:    "Role or ClusterRole plus the matching binding",
		APIVersion: "rbac.authorization.k8s.io/v1",
		ValuesKeys: []string{"rbac"},
		Requires:   []string{"serviceaccount"},
	},
	{
		Name:       "configmap",
		File:       "configmap.yaml",
		Summary:    "ConfigMap with a checksum annotation hook for rollout on change",
		APIVersion: "v1",
		ValuesKeys: []string{"configMap"},
	},
	{
		Name:       "secret",
		File:       "secret.yaml",
		Summary:    "Opaque Secret — values are stringData, never committed",
		APIVersion: "v1",
		ValuesKeys: []string{"secret"},
	},
	{
		Name:       "externalsecret",
		File:       "externalsecret.yaml",
		Summary:    "External Secrets Operator ExternalSecret pulling from a SecretStore",
		APIVersion: "external-secrets.io/v1",
		ValuesKeys: []string{"externalSecret"},
		Optional:   true,
	},
	{
		Name:       "pvc",
		File:       "pvc.yaml",
		Summary:    "Standalone PersistentVolumeClaim with a Helm resource-policy keep",
		APIVersion: "v1",
		ValuesKeys: []string{"persistence"},
	},
	{
		Name:       "servicemonitor",
		File:       "servicemonitor.yaml",
		Summary:    "Prometheus Operator ServiceMonitor scrape config",
		APIVersion: "monitoring.coreos.com/v1",
		ValuesKeys: []string{"metrics"},
		Requires:   []string{"service"},
		Optional:   true,
	},
	{
		Name:       "prometheusrule",
		File:       "prometheusrule.yaml",
		Summary:    "Prometheus Operator PrometheusRule alert group",
		APIVersion: "monitoring.coreos.com/v1",
		ValuesKeys: []string{"prometheusRule"},
		Optional:   true,
	},
	{
		Name:       "certificate",
		File:       "certificate.yaml",
		Summary:    "cert-manager Certificate feeding a TLS Secret",
		APIVersion: "cert-manager.io/v1",
		ValuesKeys: []string{"certificate"},
		Optional:   true,
	},
	{
		Name:       "podmonitor",
		File:       "podmonitor.yaml",
		Summary:    "Prometheus Operator PodMonitor, scraping pods without a Service",
		APIVersion: "monitoring.coreos.com/v1",
		ValuesKeys: []string{"podMonitor"},
		Optional:   true,
	},
	{
		Name:       "scaledobject",
		File:       "scaledobject.yaml",
		Summary:    "KEDA ScaledObject scaling on queue depth rather than CPU",
		APIVersion: "keda.sh/v1alpha1",
		ValuesKeys: []string{"scaledObject"},
		Optional:   true,
	},
	{
		Name:       "vpa",
		File:       "vpa.yaml",
		Summary:    "VerticalPodAutoscaler sizing requests from observed usage",
		APIVersion: "autoscaling.k8s.io/v1",
		ValuesKeys: []string{"verticalPodAutoscaler"},
		Optional:   true,
	},
	{
		Name:       "sealedsecret",
		File:       "sealedsecret.yaml",
		Summary:    "Sealed Secret whose ciphertext is safe to commit",
		APIVersion: "bitnami.com/v1alpha1",
		ValuesKeys: []string{"sealedSecret"},
		Optional:   true,
	},
	{
		Name:       "issuer",
		File:       "issuer.yaml",
		Summary:    "Namespaced cert-manager Issuer: ACME, CA or self-signed",
		APIVersion: "cert-manager.io/v1",
		ValuesKeys: []string{"issuer"},
		Optional:   true,
	},
	{
		Name:       "referencegrant",
		File:       "referencegrant.yaml",
		Summary:    "Gateway API grant letting another namespace reach this Service",
		APIVersion: "gateway.networking.k8s.io/v1beta1",
		ValuesKeys: []string{"referenceGrant"},
		Requires:   []string{"service"},
		Optional:   true,
	},
	{
		Name:       "scaledjob",
		File:       "scaledjob.yaml",
		Summary:    "KEDA ScaledJob: one Job per queue item",
		APIVersion: "keda.sh/v1alpha1",
		ValuesKeys: []string{"scaledJob"},
		Requires:   []string{"serviceaccount"},
		Optional:   true,
	},
	{
		Name:       "virtualservice",
		File:       "virtualservice.yaml",
		Summary:    "Istio VirtualService — routing for a mesh that is not using Gateway API",
		APIVersion: "networking.istio.io/v1",
		ValuesKeys: []string{"virtualService"},
		Requires:   []string{"service"},
		Optional:   true,
	},
	{
		Name:       "destinationrule",
		File:       "destinationrule.yaml",
		Summary:    "Istio DestinationRule: pool limits, outlier ejection, mTLS mode",
		APIVersion: "networking.istio.io/v1",
		ValuesKeys: []string{"destinationRule"},
		Requires:   []string{"service"},
		Optional:   true,
	},
	{
		Name:       "authorizationpolicy",
		File:       "authorizationpolicy.yaml",
		Summary:    "Istio AuthorizationPolicy — who may call this workload, at L7",
		APIVersion: "security.istio.io/v1",
		ValuesKeys: []string{"authorizationPolicy"},
		Optional:   true,
	},
	{
		Name:       "grafanadashboard",
		File:       "grafana-dashboard.yaml",
		Summary:    "Dashboard ConfigMap the Grafana sidecar picks up",
		APIVersion: "v1",
		ValuesKeys: []string{"grafanaDashboard"},
		Optional:   true,
	},
	{
		Name:       "tests",
		File:       "tests/test-connection.yaml",
		Summary:    "helm test hook that dials the Service",
		APIVersion: "v1",
		ValuesKeys: []string{"tests"},
		Requires:   []string{"service"},
	},
}

var presets = []Preset{
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
