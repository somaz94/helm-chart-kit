package check

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/somaz94/helm-chart-kit/internal/chart"
)

// Scope is what a rule reads, which is also when it runs: before helm is
// invoked, once per rendered object, or once over the whole rendered set.
type Scope string

const (
	// ChartScope reads the chart directory and needs no render.
	ChartScope Scope = "chart"
	// RenderScope is helm's own verdict, reported under an hck rule ID.
	RenderScope Scope = "render"
	// ObjectScope reads one rendered object.
	ObjectScope Scope = "object"
	// SetScope reads every rendered object together, so it can judge a
	// combination that is only wrong as a combination.
	SetScope Scope = "set"
)

// hit is a rule firing: where, and what to say about it. The ID and severity
// are filled in by the runner from the rule's own declaration, so a rule
// cannot report under somebody else's ID — which is what makes a rule ID
// something a user can configure and trust.
type hit struct {
	Where   string
	Message string
}

// Rule is one house rule.
type Rule struct {
	// ID is the stable identifier, "HCK021". It never changes: it is what a
	// chart's .hck.yaml refers to.
	ID string
	// Severity is what a hit reports as, unless the chart overrides it.
	Severity Severity
	// Scope decides when the rule runs and what it is handed.
	Scope Scope
	// Summary is the one-line description "hck list rules" prints.
	Summary string
	// Locked marks a rule that cannot be turned off. Only HCK001 is: a chart
	// that does not render has nothing else worth reporting, and a check that
	// silently passed one would be worse than no check.
	Locked bool

	// chart, object and set are the three shapes a check takes. Exactly one
	// is set, and it is the one Scope names. RenderScope rules have none —
	// Run reports them from helm's own output.
	chart  func(*chart.Chart) []hit
	object func(object) []hit
	set    func([]object) []hit
}

// rules is the whole rule set, in ID order — which is also the order findings
// come out in, so two runs over the same chart read the same way.
var rules = []Rule{
	{
		ID: "HCK001", Severity: Error, Scope: RenderScope, Locked: true,
		Summary: "chart does not render",
	},
	{
		ID: "HCK002", Severity: Warn, Scope: RenderScope,
		Summary: "helm lint reports a problem",
	},
	{
		ID: "HCK010", Severity: Warn, Scope: ChartScope,
		Summary: "chart has no values.yaml",
		chart: func(c *chart.Chart) []hit {
			if fileExists(c.ValuesPath()) {
				return nil
			}
			return []hit{{Where: "values.yaml",
				Message: "chart has no values.yaml, so nothing about it is configurable"}}
		},
	},
	{
		ID: "HCK011", Severity: Warn, Scope: ChartScope,
		Summary: "chart has no .helmignore",
		chart: func(c *chart.Chart) []hit {
			if fileExists(filepath.Join(c.Dir, ".helmignore")) {
				return nil
			}
			return []hit{{Where: ".helmignore",
				Message: "no .helmignore — packaging will sweep in local files such as ci/ and editor state"}}
		},
	},
	{
		ID: "HCK012", Severity: Warn, Scope: ChartScope,
		Summary: "Chart.yaml apiVersion is not v2",
		chart: func(c *chart.Chart) []hit {
			if c.Meta.APIVersion == "v2" {
				return nil
			}
			return []hit{{Where: "Chart.yaml",
				Message: fmt.Sprintf("apiVersion is %q; v2 is the Helm 3 chart format", c.Meta.APIVersion)}}
		},
	},
	{
		ID: "HCK013", Severity: Warn, Scope: ChartScope,
		Summary: "Chart.yaml has no description",
		chart: func(c *chart.Chart) []hit {
			if c.Meta.Description != "" {
				return nil
			}
			return []hit{{Where: "Chart.yaml",
				Message: "description is empty — it is what a chart repository shows in its index"}}
		},
	},
	{
		ID: "HCK020", Severity: Warn, Scope: ObjectScope,
		Summary: "pod securityContext does not set runAsNonRoot",
		object: podRule(func(podSpec map[string]any) string {
			if sc, ok := podSpec["securityContext"].(map[string]any); !ok || sc["runAsNonRoot"] != true {
				return "pod securityContext does not set runAsNonRoot: true"
			}
			return ""
		}),
	},
	{
		ID: "HCK021", Severity: Error, Scope: ObjectScope,
		Summary: "container image is missing or has no tag",
		object: containerRule(func(c map[string]any, _ string) string {
			image := str(c["image"])
			switch {
			case image == "":
				return "container has no image"
			case !strings.Contains(lastSegment(image), ":"):
				return fmt.Sprintf("image %q has no tag, so it resolves to :latest at pull time", image)
			}
			return ""
		}),
	},
	{
		ID: "HCK022", Severity: Error, Scope: ObjectScope,
		Summary: "container image is tagged :latest",
		object: containerRule(func(c map[string]any, _ string) string {
			if strings.HasSuffix(str(c["image"]), ":latest") {
				return "image tag is :latest — the deployed version becomes unknowable and rollback stops meaning anything"
			}
			return ""
		}),
	},
	{
		ID: "HCK023", Severity: Warn, Scope: ObjectScope,
		Summary: "container declares no resource requests",
		object: containerRule(func(c map[string]any, _ string) string {
			res, _ := c["resources"].(map[string]any)
			if _, ok := res["requests"]; !ok {
				return "no resource requests — the scheduler will treat this as BestEffort and evict it first"
			}
			return ""
		}),
	},
	{
		ID: "HCK024", Severity: Warn, Scope: ObjectScope,
		Summary: "container declares no memory limit",
		object: containerRule(func(c map[string]any, _ string) string {
			res, _ := c["resources"].(map[string]any)
			limits, ok := res["limits"].(map[string]any)
			if !ok {
				return "no memory limit — a leak here takes the whole node down with it"
			}
			if _, ok := limits["memory"]; !ok {
				return "no memory limit — a leak here takes the whole node down with it"
			}
			return ""
		}),
	},
	{
		ID: "HCK025", Severity: Warn, Scope: ObjectScope,
		Summary: "container sets a CPU limit",
		object: containerRule(func(c map[string]any, _ string) string {
			res, _ := c["resources"].(map[string]any)
			limits, ok := res["limits"].(map[string]any)
			if !ok {
				return ""
			}
			// Only once the memory limit HCK024 asks for is there: a container
			// with no limits at all has one thing wrong with it, not two.
			if _, ok := limits["memory"]; !ok {
				return ""
			}
			if _, ok := limits["cpu"]; ok {
				return "CPU limit set — CFS throttling causes latency spikes well before the node is busy; prefer a request with no limit"
			}
			return ""
		}),
	},
	{
		ID: "HCK026", Severity: Warn, Scope: ObjectScope,
		Summary: "container securityContext does not set allowPrivilegeEscalation: false",
		object: containerRule(func(c map[string]any, _ string) string {
			sc, _ := c["securityContext"].(map[string]any)
			if sc["allowPrivilegeEscalation"] != false {
				return "container securityContext does not set allowPrivilegeEscalation: false"
			}
			return ""
		}),
	},
	{
		ID: "HCK027", Severity: Error, Scope: ObjectScope,
		Summary: "container runs privileged",
		object: containerRule(func(c map[string]any, _ string) string {
			sc, _ := c["securityContext"].(map[string]any)
			if sc["privileged"] == true {
				return "container runs privileged"
			}
			return ""
		}),
	},
	{
		ID: "HCK028", Severity: Warn, Scope: ObjectScope,
		Summary: "long-running container has no readiness probe",
		object: containerRule(func(c map[string]any, kind string) string {
			if !servesTraffic(kind) {
				return ""
			}
			if _, ok := c["readinessProbe"]; !ok {
				return "no readiness probe — traffic reaches the pod before it can serve it, on every rollout"
			}
			return ""
		}),
	},
	{
		ID: "HCK029", Severity: Warn, Scope: ObjectScope,
		Summary: "long-running container has no liveness probe",
		object: containerRule(func(c map[string]any, kind string) string {
			if !servesTraffic(kind) {
				return ""
			}
			if _, ok := c["livenessProbe"]; !ok {
				return "no liveness probe"
			}
			return ""
		}),
	},
	{
		ID: "HCK030", Severity: Warn, Scope: SetScope,
		Summary: "chart renders more than one primary workload",
		set:     twoWorkloadsRule,
	},
	{
		ID: "HCK031", Severity: Warn, Scope: SetScope,
		Summary: "chart renders both an HPA and a KEDA ScaledObject",
		set:     hpaAndKedaRule,
	},
	{
		ID: "HCK032", Severity: Warn, Scope: SetScope,
		Summary: "chart renders an HPA alongside an evicting VPA",
		set:     hpaAndVPARule,
	},
	{
		ID: "HCK033", Severity: Warn, Scope: SetScope,
		Summary: "a scaler names a workload the chart does not render",
		set:     danglingScaleTargetRule,
	},
}

// scaleTargetField is where each controller names the workload it sizes.
// Every one of them is a {kind, name} pair, and every one of them is inert if
// nothing by that name renders.
var scaleTargetField = map[string]string{
	"HorizontalPodAutoscaler": "scaleTargetRef",
	"ScaledObject":            "scaleTargetRef",
	"VerticalPodAutoscaler":   "targetRef",
}

// danglingScaleTargetRule catches a scaler pointed at something that is not
// there.
//
// This is the quietest way a chart can be wrong. Nothing fails: the chart
// renders, helm installs it, and the controller reports "I cannot find that"
// in a status nobody reads while the workload never scales. Two ordinary
// things produce it — "hck add hpa" against a chart whose workload is not the
// kind the scaler defaults to, and removing a workload out from under one.
func danglingScaleTargetRule(objs []object) []hit {
	rendered := map[string]bool{}
	var workloads []string
	for _, o := range objs {
		rendered[o.Kind+"/"+o.Name] = true
		if workloadKinds[o.Kind] {
			workloads = append(workloads, o.Kind+"/"+o.Name)
		}
	}

	var out []hit
	for _, o := range objs {
		field, ok := scaleTargetField[o.Kind]
		if !ok {
			continue
		}
		ref, ok := o.Spec[field].(map[string]any)
		if !ok {
			continue
		}
		kind, name := str(ref["kind"]), str(ref["name"])
		if kind == "" || name == "" || rendered[kind+"/"+name] {
			continue
		}
		// Naming what the chart does render is most of the answer: the reader
		// sees the mismatch instead of being told to go looking for it.
		instead := "and it renders no workload at all"
		if len(workloads) > 0 {
			instead = "it renders " + strings.Join(workloads, ", ")
		}
		out = append(out, hit{
			Where: fmt.Sprintf("%s/%s", o.Kind, o.Name),
			Message: fmt.Sprintf(
				"%s names %s/%s, which this chart does not render — %s. Nothing fails: the controller reports that it cannot find the target in its own status, and the workload never scales",
				field, kind, name, instead),
		})
	}
	return out
}

// Rules returns every house rule, in ID order.
func Rules() []Rule { return slices.Clone(rules) }

// LookupRule finds a rule by ID.
func LookupRule(id string) (Rule, bool) {
	for _, r := range rules {
		if r.ID == id {
			return r, true
		}
	}
	return Rule{}, false
}

// servesTraffic reports whether a kind runs indefinitely and is expected to
// answer while it does. A Job is not one: it runs to completion, and probing
// it would only interrupt work that is finishing on its own.
func servesTraffic(kind string) bool { return kind == "Deployment" || kind == "StatefulSet" }

// podRule lifts a check over one pod spec into an object-scope rule, so the
// rule itself says what is wrong and nothing about how the pod was reached.
// An object carrying no pod spec is not the rule's problem.
func podRule(check func(podSpec map[string]any) string) func(object) []hit {
	return func(o object) []hit {
		podSpec, ok := podSpecOf(o)
		if !ok {
			return nil
		}
		msg := check(podSpec)
		if msg == "" {
			return nil
		}
		return []hit{{Where: fmt.Sprintf("%s/%s", o.Kind, o.Name), Message: msg}}
	}
}

// containerRule does the same for a check over one container, and is handed
// the object's kind because a probe means something different on a Deployment
// than on a Job.
func containerRule(check func(c map[string]any, kind string) string) func(object) []hit {
	return func(o object) []hit {
		podSpec, ok := podSpecOf(o)
		if !ok {
			return nil
		}
		containers, _ := podSpec["containers"].([]any)
		var out []hit
		for _, ci := range containers {
			c, ok := ci.(map[string]any)
			if !ok {
				continue
			}
			msg := check(c, o.Kind)
			if msg == "" {
				continue
			}
			out = append(out, hit{
				Where:   fmt.Sprintf("%s/%s container=%s", o.Kind, o.Name, str(c["name"])),
				Message: msg,
			})
		}
		return out
	}
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

// twoWorkloadsRule judges the rendered set as a whole rather than one object
// at a time.
//
// Warn rather than Error: hck refuses to generate this, so a chart that has it
// came from somewhere else, and a multi-workload chart is a defensible thing
// for someone else to have written. --strict still fails on it, which is what
// keeps hck's own charts honest.
func twoWorkloadsRule(objs []object) []hit {
	var workloads []string
	for _, o := range objs {
		if workloadKinds[o.Kind] {
			workloads = append(workloads, fmt.Sprintf("%s/%s", o.Kind, o.Name))
		}
	}
	if len(workloads) < 2 {
		return nil
	}
	return []hit{{Where: "chart", Message: fmt.Sprintf(
		"chart renders %d primary workloads (%s); they share image, resources and updateStrategy, so one set of values cannot describe both",
		len(workloads), strings.Join(workloads, ", "))}}
}

// hpaAndKedaRule and hpaAndVPARule catch two controllers driving the same
// workload's size.
//
// Nothing refuses these at generation time the way a second workload is
// refused: each is a legitimate resource to add, and it is the combination
// that is wrong. They are only visible once the chart renders, which is why
// they live here rather than in scaffold.
func hpaAndKedaRule(objs []object) []hit {
	hpa, scaled := namesOfKind(objs, "HorizontalPodAutoscaler"), namesOfKind(objs, "ScaledObject")
	if len(hpa) == 0 || len(scaled) == 0 {
		return nil
	}
	return []hit{{Where: "chart", Message: fmt.Sprintf(
		"chart renders both a HorizontalPodAutoscaler (%s) and a KEDA ScaledObject (%s); KEDA creates an HPA of its own, so the two fight over the replica count on every reconcile",
		strings.Join(hpa, ", "), strings.Join(scaled, ", "))}}
}

func hpaAndVPARule(objs []object) []hit {
	hpa := namesOfKind(objs, "HorizontalPodAutoscaler")
	if len(hpa) == 0 {
		return nil
	}
	var evicting []string
	for _, o := range objs {
		// Off and Initial only recommend, and coexist with an HPA. Auto and
		// Recreate evict pods to resize them.
		if o.Kind == "VerticalPodAutoscaler" {
			if mode := vpaUpdateMode(o); mode == "Auto" || mode == "Recreate" {
				evicting = append(evicting, o.Name)
			}
		}
	}
	if len(evicting) == 0 {
		return nil
	}
	return []hit{{Where: "chart", Message: fmt.Sprintf(
		"chart renders a HorizontalPodAutoscaler (%s) alongside a VerticalPodAutoscaler in an evicting update mode (%s); both read utilization and drive it in opposite directions. Set updateMode to Off or Initial, or scale the two on different resources",
		strings.Join(hpa, ", "), strings.Join(evicting, ", "))}}
}

func namesOfKind(objs []object, kind string) []string {
	var out []string
	for _, o := range objs {
		if o.Kind == kind {
			out = append(out, o.Name)
		}
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

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
