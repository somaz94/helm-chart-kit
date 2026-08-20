// Package check validates a chart: it renders it with Helm, then runs the
// house rules over the manifests that come out.
//
// Rendering shells out to the helm binary rather than linking helm as a
// library. That is deliberate — the check then reports what the user's own
// helm does, not what the version this tool happened to vendor would have
// done, and the two diverge exactly where it matters most.
package check

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
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
}

// Errors counts findings that fail the check.
func (r *Report) Errors() int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == Error {
			n++
		}
	}
	return n
}

// Warns counts advisory findings.
func (r *Report) Warns() int { return len(r.Findings) - r.Errors() }

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
}

// Run renders the chart and applies every rule.
func Run(c *chart.Chart, opts Options) (*Report, error) {
	rep := &Report{}
	rep.Findings = append(rep.Findings, chartLayoutRules(c)...)

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
		rep.Findings = append(rep.Findings, Finding{
			Severity: Error,
			Rule:     "HCK001",
			Where:    "helm template",
			Message:  firstLines(err.Error(), 12),
		})
		return rep, nil
	}
	rep.Rendered = rendered

	// helm lint takes paths only — passing a release name the way template
	// does makes it lint a second, nonexistent chart and always fail.
	if _, err := runHelm(helm, append([]string{"lint", c.Dir}, valuesArgs(vals)...)); err != nil {
		rep.Findings = append(rep.Findings, Finding{
			Severity: Warn,
			Rule:     "HCK002",
			Where:    "helm lint",
			Message:  firstLines(err.Error(), 12),
		})
	}

	objs, err := decodeAll(rendered)
	if err != nil {
		return nil, err
	}
	for _, o := range objs {
		rep.Findings = append(rep.Findings, manifestRules(o)...)
	}
	rep.Findings = append(rep.Findings, manifestSetRules(objs)...)
	rep.Findings = append(rep.Findings, scalerRules(objs)...)
	return rep, nil
}

// workloadKinds are the controllers that own a chart's image, resources and
// update strategy. A Job is not one: it is a one-shot task alongside the
// workload, not the thing the chart deploys.
var workloadKinds = map[string]bool{
	"Deployment":  true,
	"StatefulSet": true,
	"DaemonSet":   true,
	"CronJob":     true,
}

// manifestSetRules judges the rendered set as a whole rather than one object
// at a time.
func manifestSetRules(objs []object) []Finding {
	var workloads []string
	for _, o := range objs {
		if workloadKinds[o.Kind] {
			workloads = append(workloads, fmt.Sprintf("%s/%s", o.Kind, o.Name))
		}
	}
	if len(workloads) < 2 {
		return nil
	}
	// Warn rather than Error: hck refuses to generate this, so a chart that
	// has it came from somewhere else, and a multi-workload chart is a
	// defensible thing for someone else to have written. --strict still
	// fails on it, which is what keeps hck's own charts honest.
	return []Finding{{
		Severity: Warn,
		Rule:     "HCK030",
		Where:    "chart",
		Message: fmt.Sprintf(
			"chart renders %d primary workloads (%s); they share image, resources and updateStrategy, so one set of values cannot describe both",
			len(workloads), strings.Join(workloads, ", ")),
	}}
}

// scalerRules catches two controllers driving the same workload's size.
//
// Nothing refuses these at generation time the way a second workload is
// refused: each is a legitimate resource to add, and it is the combination
// that is wrong. They are only visible once the chart renders, which is why
// they live here rather than in scaffold.
func scalerRules(objs []object) []Finding {
	var out []Finding
	var hpa, scaled, vpaAuto []string
	for _, o := range objs {
		switch o.Kind {
		case "HorizontalPodAutoscaler":
			hpa = append(hpa, o.Name)
		case "ScaledObject":
			scaled = append(scaled, o.Name)
		case "VerticalPodAutoscaler":
			// Off and Initial only recommend, and coexist with an HPA. Auto
			// and Recreate evict pods to resize them.
			if mode := vpaUpdateMode(o); mode == "Auto" || mode == "Recreate" {
				vpaAuto = append(vpaAuto, o.Name)
			}
		}
	}
	if len(hpa) > 0 && len(scaled) > 0 {
		out = append(out, Finding{Severity: Warn, Rule: "HCK031", Where: "chart",
			Message: fmt.Sprintf(
				"chart renders both a HorizontalPodAutoscaler (%s) and a KEDA ScaledObject (%s); KEDA creates an HPA of its own, so the two fight over the replica count on every reconcile",
				strings.Join(hpa, ", "), strings.Join(scaled, ", "))})
	}
	if len(hpa) > 0 && len(vpaAuto) > 0 {
		out = append(out, Finding{Severity: Warn, Rule: "HCK032", Where: "chart",
			Message: fmt.Sprintf(
				"chart renders a HorizontalPodAutoscaler (%s) alongside a VerticalPodAutoscaler in an evicting update mode (%s); both read utilization and drive it in opposite directions. Set updateMode to Off or Initial, or scale the two on different resources",
				strings.Join(hpa, ", "), strings.Join(vpaAuto, ", "))})
	}
	return out
}

// vpaUpdateMode reads spec.updatePolicy.updateMode, or "" if it is unset.
func vpaUpdateMode(o object) string {
	policy, ok := o.Spec["updatePolicy"].(map[string]any)
	if !ok {
		return ""
	}
	mode, _ := policy["updateMode"].(string)
	return mode
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

// chartLayoutRules covers what can be judged without rendering.
func chartLayoutRules(c *chart.Chart) []Finding {
	var out []Finding
	if !fileExists(c.ValuesPath()) {
		out = append(out, Finding{Severity: Warn, Rule: "HCK010", Where: "values.yaml",
			Message: "chart has no values.yaml, so nothing about it is configurable"})
	}
	if !fileExists(filepath.Join(c.Dir, ".helmignore")) {
		out = append(out, Finding{Severity: Warn, Rule: "HCK011", Where: ".helmignore",
			Message: "no .helmignore — packaging will sweep in local files such as ci/ and editor state"})
	}
	if c.Meta.APIVersion != "v2" {
		out = append(out, Finding{Severity: Warn, Rule: "HCK012", Where: "Chart.yaml",
			Message: fmt.Sprintf("apiVersion is %q; v2 is the Helm 3 chart format", c.Meta.APIVersion)})
	}
	if c.Meta.Description == "" {
		out = append(out, Finding{Severity: Warn, Rule: "HCK013", Where: "Chart.yaml",
			Message: "description is empty — it is what a chart repository shows in its index"})
	}
	return out
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

// manifestRules are the house rules that read a rendered object.
func manifestRules(o object) []Finding {
	podSpec, ok := podSpecOf(o)
	if !ok {
		return nil
	}
	where := fmt.Sprintf("%s/%s", o.Kind, o.Name)
	var out []Finding

	if sc, ok := podSpec["securityContext"].(map[string]any); !ok || sc["runAsNonRoot"] != true {
		out = append(out, Finding{Severity: Warn, Rule: "HCK020", Where: where,
			Message: "pod securityContext does not set runAsNonRoot: true"})
	}

	containers, _ := podSpec["containers"].([]any)
	for _, ci := range containers {
		c, ok := ci.(map[string]any)
		if !ok {
			continue
		}
		name := str(c["name"])
		cw := where + " container=" + name

		image := str(c["image"])
		switch {
		case image == "":
			out = append(out, Finding{Severity: Error, Rule: "HCK021", Where: cw,
				Message: "container has no image"})
		case !strings.Contains(lastSegment(image), ":"):
			out = append(out, Finding{Severity: Error, Rule: "HCK021", Where: cw,
				Message: fmt.Sprintf("image %q has no tag, so it resolves to :latest at pull time", image)})
		case strings.HasSuffix(image, ":latest"):
			out = append(out, Finding{Severity: Error, Rule: "HCK022", Where: cw,
				Message: "image tag is :latest — the deployed version becomes unknowable and rollback stops meaning anything"})
		}

		res, _ := c["resources"].(map[string]any)
		if _, ok := res["requests"]; !ok {
			out = append(out, Finding{Severity: Warn, Rule: "HCK023", Where: cw,
				Message: "no resource requests — the scheduler will treat this as BestEffort and evict it first"})
		}
		if limits, ok := res["limits"].(map[string]any); !ok {
			out = append(out, Finding{Severity: Warn, Rule: "HCK024", Where: cw,
				Message: "no memory limit — a leak here takes the whole node down with it"})
		} else if _, ok := limits["memory"]; !ok {
			out = append(out, Finding{Severity: Warn, Rule: "HCK024", Where: cw,
				Message: "no memory limit — a leak here takes the whole node down with it"})
		} else if _, ok := limits["cpu"]; ok {
			out = append(out, Finding{Severity: Warn, Rule: "HCK025", Where: cw,
				Message: "CPU limit set — CFS throttling causes latency spikes well before the node is busy; prefer a request with no limit"})
		}

		sc, _ := c["securityContext"].(map[string]any)
		if sc["allowPrivilegeEscalation"] != false {
			out = append(out, Finding{Severity: Warn, Rule: "HCK026", Where: cw,
				Message: "container securityContext does not set allowPrivilegeEscalation: false"})
		}
		if sc["privileged"] == true {
			out = append(out, Finding{Severity: Error, Rule: "HCK027", Where: cw,
				Message: "container runs privileged"})
		}

		if o.Kind == "Deployment" || o.Kind == "StatefulSet" {
			if _, ok := c["readinessProbe"]; !ok {
				out = append(out, Finding{Severity: Warn, Rule: "HCK028", Where: cw,
					Message: "no readiness probe — traffic reaches the pod before it can serve it, on every rollout"})
			}
			if _, ok := c["livenessProbe"]; !ok {
				out = append(out, Finding{Severity: Warn, Rule: "HCK029", Where: cw,
					Message: "no liveness probe"})
			}
		}
	}
	return out
}

// podSpecOf digs out the pod spec of any workload kind, or reports that the
// object does not carry one.
func podSpecOf(o object) (map[string]any, bool) {
	switch o.Kind {
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job":
		return nested(o.Spec, "template", "spec")
	case "CronJob":
		return nested(o.Spec, "jobTemplate", "spec", "template", "spec")
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

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(append(lines[:n], "..."), "\n")
}
