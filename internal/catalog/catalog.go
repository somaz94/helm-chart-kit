// Package catalog is the single source of truth for what hck can generate:
// the resource templates it knows and the presets that group them.
//
// Every entry here must have a matching directory under templates/resources/,
// and that invariant is enforced by a test rather than at runtime — a missing
// template is a build-time mistake, not something a user can trigger.
package catalog

import "sort"

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
	// reads the fragment rather than this list, so nothing at runtime would
	// notice the two disagreeing — TestValuesKeysMatchTheTemplates compares
	// them instead.
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
		ValuesKeys: []string{"replicaCount", "podManagementPolicy", "updateStrategy", "terminationGracePeriodSeconds", "image", "imagePullSecrets", "containerPort", "env", "podAnnotations", "podLabels", "podSecurityContext", "securityContext", "livenessProbe", "readinessProbe", "resources", "nodeSelector", "affinity", "tolerations", "volumes", "volumeMounts", "persistence"},
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
}

// Resources returns every known resource, ordered by name.
func Resources() []Resource {
	out := make([]Resource, len(resources))
	copy(out, resources)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Presets returns every known preset, ordered by name.
func Presets() []Preset {
	out := make([]Preset, len(presets))
	copy(out, presets)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// LookupResource finds a resource by name.
func LookupResource(name string) (Resource, bool) {
	for _, r := range resources {
		if r.Name == name {
			return r, true
		}
	}
	return Resource{}, false
}

// LookupPreset finds a preset by name.
func LookupPreset(name string) (Preset, bool) {
	for _, p := range presets {
		if p.Name == name {
			return p, true
		}
	}
	return Preset{}, false
}

// ResourceNames returns every resource name, ordered.
func ResourceNames() []string {
	out := make([]string, 0, len(resources))
	for _, r := range resources {
		out = append(out, r.Name)
	}
	sort.Strings(out)
	return out
}

// PresetNames returns every preset name, ordered.
func PresetNames() []string {
	out := make([]string, 0, len(presets))
	for _, p := range presets {
		out = append(out, p.Name)
	}
	sort.Strings(out)
	return out
}
