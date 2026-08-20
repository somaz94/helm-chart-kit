package catalog

import "sort"

// Environment is how hard the same chart is being asked to work: one replica
// and loose limits while someone develops against it, three replicas and a
// disruption budget once it carries traffic.
//
// Orthogonal to Platform. A chart is installed somewhere and at some size, and
// the two overlays stack:
//
//	helm install app . -f values-aws.yaml -f values-prod.yaml
//
// Order matters — the environment goes last, so its replica count wins over a
// platform default rather than the other way round.
type Environment struct {
	// Name is the identifier passed to --env, and the suffix of the generated
	// file: "prod" produces values-prod.yaml.
	Name string
	// Summary is the one-line description shown by "hck env list".
	Summary string
}

var environments = []Environment{
	{Name: "dev", Summary: "One replica, small requests, no budget — cheap and disposable"},
	{Name: "staging", Summary: "Production's shape at a fraction of its size"},
	{Name: "prod", Summary: "Redundant, budgeted, and slow to give up on a pod"},
}

// ValuesFile is the file name an environment overlay takes inside a chart.
func (e Environment) ValuesFile() string { return "values-" + e.Name + ".yaml" }

// Environments returns every known environment, ordered by name.
func Environments() []Environment {
	out := make([]Environment, len(environments))
	copy(out, environments)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// LookupEnvironment finds an environment by name.
func LookupEnvironment(name string) (Environment, bool) {
	for _, e := range environments {
		if e.Name == name {
			return e, true
		}
	}
	return Environment{}, false
}

// EnvironmentNames returns every environment name, ordered.
func EnvironmentNames() []string {
	out := make([]string, 0, len(environments))
	for _, e := range environments {
		out = append(out, e.Name)
	}
	sort.Strings(out)
	return out
}
