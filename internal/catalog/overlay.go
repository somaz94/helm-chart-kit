package catalog

// Axis is the question an overlay answers. Platform says where the chart is
// installed, environment says how hard it is being asked to work, and the two
// are orthogonal: a chart is installed somewhere and at some size.
//
// One type for both because they are one mechanism. Both produce
// values-<name>.yaml, both read templates/resources/<r>/values-<name>.yaml.tmpl
// through the same builder, and both share one file-name space inside a chart.
// The two were separate types for a while and the code that consumed them was
// written twice, once per axis, which is how "hck env add --help" ended up
// listing platforms.
type Axis string

const (
	// PlatformAxis is where the chart runs: EKS, GKE, AKS, self-managed.
	PlatformAxis Axis = "platform"
	// EnvironmentAxis is how hard it works: dev, staging, prod.
	EnvironmentAxis Axis = "environment"
)

// Command is the hck subcommand that manages an axis. It is not the axis name:
// the noun is "environment" but nobody wants to type it.
func (a Axis) Command() string {
	if a == EnvironmentAxis {
		return "env"
	}
	return "platform"
}

// Overlay is one value of an axis — one generated values-<name>.yaml.
//
// The generated file is an overlay, not a replacement. Helm reads values.yaml
// first and always, so the overlay carries only what is actually different —
// which is also what makes it readable.
type Overlay struct {
	// Axis is which question this overlay answers.
	Axis Axis
	// Name is the identifier passed to --platform or --env, and the suffix of
	// the generated file: "aws" produces values-aws.yaml.
	Name string
	// Summary is the one-line description shown by "hck platform list".
	Summary string
	// Needs names the things the overlay expects to be installed already.
	// Reported, never installed. Empty on the environment axis: a size is
	// something you ask for, not something the cluster has to provide.
	Needs []string
}

// ValuesFile is the file name an overlay takes inside a chart.
//
// A sibling of values.yaml rather than a values/ subdirectory: Helm reads
// values.yaml first and unconditionally, so a "base" file next to the overlays
// would duplicate it and invite edits that go nowhere.
func (o Overlay) ValuesFile() string { return "values-" + o.Name + ".yaml" }

func (o Overlay) entryName() string { return o.Name }

// A platform overlay says how something is wired — an annotation, a class, a
// store reference. It must never set a key ending in ".enabled".
//
// Enabling and disabling belongs to the environment axis, because the two
// stack and the last -f wins: a platform overlay switching a NetworkPolicy off
// and a prod overlay switching it on give different answers depending on the
// order they are passed in, which is not a decision anybody made.
// TestPlatformOverlaysDoNotToggle enforces it. Where a platform genuinely
// cannot support something, that belongs in Needs, where it is said once.
var overlays = []Overlay{
	{
		Axis:    PlatformAxis,
		Name:    "aws",
		Summary: "EKS: IRSA, ALB ingress, NLB services, gp3 volumes",
		Needs: []string{
			"AWS Load Balancer Controller",
			"EBS CSI driver, and a gp3 StorageClass — EKS ships gp2 only",
			"the VPC CNI network policy controller, if networkPolicy is enabled",
		},
	},
	{
		Axis:    PlatformAxis,
		Name:    "gcp",
		Summary: "GKE: Workload Identity, GCE ingress, NEG services, pd-balanced volumes",
		Needs: []string{
			"GKE Ingress controller",
			"a Prometheus Operator, if metrics are enabled — Google Managed Prometheus reads PodMonitoring, not ServiceMonitor",
		},
	},
	{
		Axis:    PlatformAxis,
		Name:    "azure",
		Summary: "AKS: Workload Identity, Application Gateway ingress, managed-csi volumes",
		Needs:   []string{"Workload Identity webhook", "Application Gateway Ingress Controller"},
	},
	{
		Axis:    PlatformAxis,
		Name:    "onprem",
		Summary: "Self-managed: ingress-nginx, MetalLB, a storage class you provide",
		Needs: []string{
			"ingress-nginx",
			"MetalLB",
			"a default StorageClass",
			"metrics-server, if autoscaling is enabled — without it an HPA reports <unknown> and never scales",
		},
	},
	{
		Axis:    EnvironmentAxis,
		Name:    "dev",
		Summary: "One replica, small requests, no budget — cheap and disposable",
	},
	{
		Axis:    EnvironmentAxis,
		Name:    "staging",
		Summary: "Production's shape at a fraction of its size",
	},
	{
		Axis:    EnvironmentAxis,
		Name:    "prod",
		Summary: "Redundant, budgeted, and slow to give up on a pod",
	},
}

// Overlays returns every overlay on one axis, ordered by name.
func Overlays(axis Axis) []Overlay {
	out := make([]Overlay, 0, len(overlays))
	for _, o := range overlays {
		if o.Axis == axis {
			out = append(out, o)
		}
	}
	return sorted(out)
}

// AllOverlays returns every overlay on both axes, ordered by name. Both axes
// write into one file-name space, so the checks that guard that space have to
// see them together.
func AllOverlays() []Overlay { return sorted(overlays) }

// LookupOverlay finds an overlay by name on one axis. The axis is part of the
// lookup rather than a filter afterwards: "--platform prod" names a real
// overlay on the wrong axis, and reporting it as unknown is the honest answer.
func LookupOverlay(axis Axis, name string) (Overlay, bool) {
	return lookup(Overlays(axis), name)
}

// OverlayNames returns the names on one axis, ordered.
func OverlayNames(axis Axis) []string { return names(Overlays(axis)) }
