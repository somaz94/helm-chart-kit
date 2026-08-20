package catalog

import "sort"

// Platform is a target the chart is installed onto. What differs between them
// is values, not templates: the same ServiceAccount carries an IAM role ARN on
// EKS, a Google service account on GKE, and a client ID on AKS.
//
// The generated file is an overlay, not a replacement. Helm reads values.yaml
// first and always, so the overlay carries only what is actually different —
// which is also what makes it readable.
type Platform struct {
	// Name is the identifier passed to --platform, and the suffix of the
	// generated file: "aws" produces values-aws.yaml.
	Name string
	// Summary is the one-line description shown by "hck list platforms".
	Summary string
	// Needs names the things the platform expects to be installed already.
	// Reported, never installed.
	Needs []string
}

var platforms = []Platform{
	{
		Name:    "aws",
		Summary: "EKS: IRSA, ALB ingress, NLB services, gp3 volumes",
		Needs:   []string{"AWS Load Balancer Controller", "EBS CSI driver"},
	},
	{
		Name:    "gcp",
		Summary: "GKE: Workload Identity, GCE ingress, NEG services, pd-balanced volumes",
		Needs:   []string{"GKE Ingress controller"},
	},
	{
		Name:    "azure",
		Summary: "AKS: Workload Identity, Application Gateway ingress, managed-csi volumes",
		Needs:   []string{"Workload Identity webhook", "Application Gateway Ingress Controller"},
	},
	{
		Name:    "onprem",
		Summary: "Self-managed: ingress-nginx, MetalLB, a storage class you provide",
		Needs:   []string{"ingress-nginx", "MetalLB", "a default StorageClass"},
	},
}

// ValuesFile is the file name a platform's overlay takes inside a chart.
//
// A sibling of values.yaml rather than a values/ subdirectory: Helm reads
// values.yaml first and unconditionally, so a "base" file next to the overlays
// would duplicate it and invite edits that go nowhere.
func (p Platform) ValuesFile() string { return "values-" + p.Name + ".yaml" }

// Platforms returns every known platform, ordered by name.
func Platforms() []Platform {
	out := make([]Platform, len(platforms))
	copy(out, platforms)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// LookupPlatform finds a platform by name.
func LookupPlatform(name string) (Platform, bool) {
	for _, p := range platforms {
		if p.Name == name {
			return p, true
		}
	}
	return Platform{}, false
}

// PlatformNames returns every platform name, ordered.
func PlatformNames() []string {
	out := make([]string, 0, len(platforms))
	for _, p := range platforms {
		out = append(out, p.Name)
	}
	sort.Strings(out)
	return out
}
