// Package check validates a chart: it renders it with Helm, then runs the
// house rules over the manifests that come out.
//
// Rendering shells out to the helm binary rather than linking helm as a
// library. That is deliberate — the check then reports what the user's own
// helm does, not what the version this tool happened to vendor would have
// done, and the two diverge exactly where it matters most.
//
// The rules themselves live in rules.go, one entry per HCK id, so that a rule
// is something a chart can name in its .hck.yaml and a reader can look up.
package check

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/somaz94/helm-chart-kit/internal/chart"
	"gopkg.in/yaml.v3"
)

// Severity ranks a finding.
type Severity string

const (
	// Error is a defect: the chart will not render, or will not apply.
	Error Severity = "error"
	// Warn is a practice the house rules call out but that still works.
	Warn Severity = "warn"
	// Info is something true about the chart that is nobody's mistake — a
	// prerequisite it has on the cluster it lands in, which hck can see and
	// cannot check. A warning says the chart is wrong; an info says the chart
	// is fine and something outside it has to be true. Collapsing the two
	// would mean either failing --strict over a chart hck itself generated, or
	// staying silent about a claim that will never bind. Neither --strict nor
	// the exit status reacts to it, and a chart that wants the stronger
	// reading raises it to warn in its own .hck.yaml.
	Info Severity = "info"
)

// Finding is one rule hit.
type Finding struct {
	Severity Severity
	// Rule is the stable identifier, e.g. "HCK002".
	Rule string
	// Where locates the finding: a file, or a rendered object's kind/name.
	Where   string
	Message string
}

// Report is the outcome of a run.
type Report struct {
	Findings []Finding
	// Rendered is the manifest stream helm produced, kept for --print.
	Rendered string
	// Disabled lists the rules the chart turned off, so a clean report can say
	// what it did not look for.
	Disabled []string
}

// count is how many findings came out at one severity. Every count goes
// through it rather than being derived by subtraction: Warns() was once
// "everything that is not an error", which was the same number right up until
// a third severity existed, and would then have quietly counted Info findings
// as warnings and failed --strict on them.
func (r *Report) count(s Severity) int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == s {
			n++
		}
	}
	return n
}

// Errors counts findings that fail the check.
func (r *Report) Errors() int { return r.count(Error) }

// Warns counts advisory findings — the ones --strict fails on.
func (r *Report) Warns() int { return r.count(Warn) }

// Infos counts findings that are notes rather than complaints. They never
// fail a check, at any strictness.
func (r *Report) Infos() int { return r.count(Info) }

// apply runs one rule and records what it found, unless the chart turned it
// off. The severity comes from the rule and the config, never from the check
// itself, so a rule always reports under its own name.
func (r *Report) apply(cfg *Config, rule Rule, run func() []hit) {
	sev, on := cfg.severity(rule)
	if !on {
		return
	}
	for _, h := range run() {
		r.Findings = append(r.Findings, Finding{
			Severity: sev,
			Rule:     rule.ID,
			Where:    h.Where,
			Message:  h.Message,
		})
	}
}

// ErrNoHelm is returned when the helm binary is not on PATH.
var ErrNoHelm = errors.New("helm not found on PATH — hck check renders with your own helm, so it needs one installed")

// Options tunes a run.
type Options struct {
	// ValuesFiles are extra -f arguments. When empty and the chart carries
	// ci/install-values.yaml, that file is used.
	ValuesFiles []string
	// OverlayFiles are appended after ValuesFiles. Unlike an explicit -f they
	// do not suppress the ci/install-values.yaml fallback: a platform overlay
	// says what differs on that platform and nothing about the image tag the
	// chart still requires, so replacing the base with it renders nothing.
	OverlayFiles []string
	// SkipRender runs only the rules that read the chart directory itself.
	SkipRender bool
	// Config is what the chart says about the rules. Nil is the default set.
	Config *Config
}

// Run renders the chart and applies every rule.
func Run(c *chart.Chart, opts Options) (*Report, error) {
	cfg := opts.Config
	rep := &Report{Disabled: cfg.Disabled()}

	for _, rule := range rulesIn(ChartScope) {
		rep.apply(cfg, rule, func() []hit { return rule.chart(c) })
	}

	if opts.SkipRender {
		return rep, nil
	}

	helm, err := exec.LookPath("helm")
	if err != nil {
		return nil, ErrNoHelm
	}

	vals := opts.ValuesFiles
	if len(vals) == 0 {
		if ci := filepath.Join(c.Dir, "ci", "install-values.yaml"); fileExists(ci) {
			vals = []string{ci}
		}
	}
	// Overlays layer on top of whatever the base turned out to be, and are
	// appended rather than replacing it: an overlay says what differs on a
	// platform or at a size, and says nothing about the image tag the chart
	// still requires. helm applies -f left to right, so the last one wins.
	vals = append(vals, opts.OverlayFiles...)

	rendered, err := runHelm(helm, append([]string{"template", filepath.Base(c.Dir), c.Dir}, valuesArgs(vals)...))
	if err != nil {
		rep.apply(cfg, mustRule("HCK001"), func() []hit {
			return []hit{{Where: "helm template", Message: firstLines(err.Error(), 12)}}
		})
		return rep, nil
	}
	rep.Rendered = rendered

	// helm lint takes paths only — passing a release name the way template
	// does makes it lint a second, nonexistent chart and always fail.
	rep.apply(cfg, mustRule("HCK002"), func() []hit {
		if _, err := runHelm(helm, append([]string{"lint", c.Dir}, valuesArgs(vals)...)); err != nil {
			return []hit{{Where: "helm lint", Message: firstLines(err.Error(), 12)}}
		}
		return nil
	})

	objs, err := decodeAll(rendered)
	if err != nil {
		return nil, err
	}
	// Object-major, rule-minor: everything wrong with one Deployment is read
	// together, and within it the findings arrive in rule-ID order.
	for _, o := range objs {
		for _, rule := range rulesIn(ObjectScope) {
			rep.apply(cfg, rule, func() []hit { return rule.object(o) })
		}
	}
	for _, rule := range rulesIn(SetScope) {
		rep.apply(cfg, rule, func() []hit { return rule.set(objs) })
	}
	return rep, nil
}

// rulesIn returns the rules of one scope, in ID order.
func rulesIn(scope Scope) []Rule {
	var out []Rule
	for _, r := range rules {
		if r.Scope == scope {
			out = append(out, r)
		}
	}
	return out
}

// mustRule looks up a rule the code itself names. A miss is a mistake in this
// package, not something a user can cause.
func mustRule(id string) Rule {
	r, ok := LookupRule(id)
	if !ok {
		panic("check: no rule " + id)
	}
	return r
}

func nonEmpty(ss ...string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func valuesArgs(valuesFiles []string) []string {
	out := make([]string, 0, len(valuesFiles)*2)
	for _, v := range valuesFiles {
		out = append(out, "-f", v)
	}
	return out
}

func runHelm(helm string, args []string) (string, error) {
	cmd := exec.Command(helm, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Both streams matter: helm template puts everything on stderr, while
		// helm lint puts the per-file findings on stdout and only the
		// "1 chart(s) failed" summary on stderr. Reporting one of the two
		// drops the half that says what is actually wrong.
		msg := strings.TrimSpace(strings.Join(nonEmpty(stdout.String(), stderr.String()), "\n"))
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New(msg)
	}
	return stdout.String(), nil
}

// object is the slice of a rendered manifest the rules read.
type object struct {
	Kind     string
	Name     string
	Spec     map[string]any
	Metadata map[string]any
}

func decodeAll(manifests string) ([]object, error) {
	dec := yaml.NewDecoder(strings.NewReader(manifests))
	var out []object
	for {
		var raw map[string]any
		err := dec.Decode(&raw)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse rendered manifests: %w", err)
		}
		if len(raw) == 0 {
			continue
		}
		o := object{Kind: str(raw["kind"])}
		if md, ok := raw["metadata"].(map[string]any); ok {
			o.Metadata = md
			o.Name = str(md["name"])
		}
		if sp, ok := raw["spec"].(map[string]any); ok {
			o.Spec = sp
		}
		out = append(out, o)
	}
	return out, nil
}

// podSpecOf digs out the pod spec of any workload kind, or reports that the
// object does not carry one.
func podSpecOf(o object) (map[string]any, bool) {
	switch o.Kind {
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job":
		return nested(o.Spec, "template", "spec")
	case "CronJob":
		return nested(o.Spec, "jobTemplate", "spec", "template", "spec")
	case "ScaledJob":
		// KEDA carries a full Job spec inline. Without this case the house
		// rules never reach it, and a pod spec nobody checks is where a
		// missing securityContext lives.
		return nested(o.Spec, "jobTargetRef", "template", "spec")
	case "Pod":
		return o.Spec, o.Spec != nil
	default:
		return nil, false
	}
}

func nested(m map[string]any, keys ...string) (map[string]any, bool) {
	cur := m
	for _, k := range keys {
		next, ok := cur[k].(map[string]any)
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// lastSegment returns the part of an image reference after the final slash,
// so the port in a registry host such as registry:5000 is not mistaken for a
// tag separator.
func lastSegment(image string) string {
	if i := strings.LastIndex(image, "/"); i >= 0 {
		return image[i+1:]
	}
	return image
}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(append(lines[:n], "..."), "\n")
}
