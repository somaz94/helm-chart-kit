package check

import (
	"encoding/json"
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
	{
		ID: "HCK034", Severity: Warn, Scope: SetScope,
		Summary: "chart creates an Issuer its own Certificate does not use",
		set:     unusedIssuerRule,
	},
	{
		ID: "HCK035", Severity: Warn, Scope: SetScope,
		Summary: "a Service forwards to a container port name nothing declares",
		set:     serviceTargetPortRule,
	},
	{
		ID: "HCK036", Severity: Warn, Scope: SetScope,
		Summary: "a PodDisruptionBudget never allows a voluntary disruption",
		set:     wedgedBudgetRule,
	},
	{
		ID: "HCK037", Severity: Warn, Scope: SetScope,
		Summary: "chart renders two resources answering the same question",
		set:     competingPairsRule,
	},
	{
		ID: "HCK038", Severity: Warn, Scope: SetScope,
		Summary: "a GKE annotation names a config object the chart does not render",
		set:     gkeConfigRefRule,
	},
	{
		ID: "HCK039", Severity: Warn, Scope: SetScope,
		Summary: "chart creates a SecretStore its own ExternalSecret does not use",
		set:     unusedSecretStoreRule,
	},
	{
		ID: "HCK040", Severity: Info, Scope: SetScope,
		Summary: "a claim requests a storage class the platform does not ship",
		set:     unprovisionedStorageClassRule,
	},
}

// wedgedBudgetRule catches a PodDisruptionBudget that allows nothing to be
// evicted, ever.
//
// It applies cleanly and it works, right up until somebody drains a node: the
// eviction API refuses every request, kubectl drain retries until it is
// cancelled, and a rolling cluster upgrade stops on this one pod. The chart is
// fine and the cluster is what breaks, months later, in somebody else's hands.
//
// Both of the values files hck writes already say this — the pdb fragment
// warns about a budget over one replica, and the dev overlay turns the budget
// off for the same reason — and a comment does not run.
func wedgedBudgetRule(objs []object) []hit {
	replicas, known := soleWorkloadReplicas(objs)
	var out []hit
	for _, o := range objs {
		if o.Kind != "PodDisruptionBudget" {
			continue
		}
		// The remedy differs per cause, and telling someone to use
		// maxUnavailable when maxUnavailable is the problem is worse than
		// saying nothing.
		const useMaxUnavailable = "Use maxUnavailable, which keeps working when the replica count moves, or raise the replica count"
		var why, fix string
		switch min, isInt := asCount(o.Spec["minAvailable"]); {
		case isZero(o.Spec["maxUnavailable"]):
			why, fix = fmt.Sprintf("maxUnavailable is %v", o.Spec["maxUnavailable"]),
				"Allow at least one, or drop the budget"
		case str(o.Spec["minAvailable"]) == "100%":
			why, fix = `minAvailable is "100%"`, useMaxUnavailable
		case isInt && known && replicas > 0 && min >= replicas:
			why, fix = fmt.Sprintf("minAvailable is %d and the workload runs %d", min, replicas), useMaxUnavailable
		default:
			continue
		}
		out = append(out, hit{
			Where:   "PodDisruptionBudget/" + o.Name,
			Message: why + ", so no voluntary disruption is ever allowed: a node drain retries until somebody cancels it, and a rolling cluster upgrade stops on this pod. " + fix,
		})
	}
	return out
}

// soleWorkloadReplicas is the replica count of the one workload the chart
// renders, when it declares one.
//
// A Deployment under an HPA does not — the template leaves replicas out so the
// autoscaler owns the number — and a DaemonSet has no such field at all. Both
// are reported as unknown, and minAvailable is then left alone: being quiet
// when unsure is the right way for a warning to be wrong.
func soleWorkloadReplicas(objs []object) (int, bool) {
	found, n := 0, 0
	for _, o := range objs {
		if !workloadKinds[o.Kind] {
			continue
		}
		if r, ok := asCount(o.Spec["replicas"]); ok {
			found++
			n = r
		}
	}
	if found != 1 {
		return 0, false
	}
	return n, true
}

// asCount reads a YAML scalar that should be a number of pods.
func asCount(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	}
	return 0, false
}

// isZero reports whether a budget field allows nothing, written either way
// round: 0 and "0%" mean the same thing to the eviction API.
func isZero(v any) bool {
	if n, ok := asCount(v); ok {
		return n == 0
	}
	return str(v) == "0%"
}

// unusedIssuerRule catches a chart that creates an Issuer and then does not
// use it.
//
// "hck add certificate issuer" produces exactly this. The Certificate keeps
// its default issuerRef — a ClusterIssuer that lives outside the chart — and
// the namespaced Issuer the chart just created is referenced by nothing. Both
// objects apply cleanly, so the only symptom is a Certificate waiting on an
// issuer somebody else was supposed to provide.
//
// A chart with an Issuer and no Certificate is left alone: an issuer other
// releases use is a reasonable thing to ship on its own.
func unusedIssuerRule(objs []object) []hit {
	used := map[string]bool{}
	var pointsAt []string
	for _, o := range objs {
		if o.Kind != "Certificate" {
			continue
		}
		ref, _ := o.Spec["issuerRef"].(map[string]any)
		kind, name := str(ref["kind"]), str(ref["name"])
		if kind == "Issuer" {
			used[name] = true
		}
		if ref := kind + "/" + name; kind != "" && name != "" && !slices.Contains(pointsAt, ref) {
			pointsAt = append(pointsAt, ref)
		}
	}
	if len(pointsAt) == 0 {
		return nil
	}
	var out []hit
	for _, o := range objs {
		if o.Kind != "Issuer" || used[o.Name] {
			continue
		}
		out = append(out, hit{
			Where: "Issuer/" + o.Name,
			Message: fmt.Sprintf(
				"nothing in this chart uses this Issuer — its Certificate names %s instead, which the chart does not provide. Point certificate.issuerRef at it, or drop one of the two",
				strings.Join(pointsAt, ", ")),
		})
	}
	return out
}

// serviceTargetPortRule catches a Service forwarding to a port name no
// container declares.
//
// A named targetPort is the right way to write a Service — it survives the
// container port moving — and it is silent when it is wrong. The endpoints
// exist, the name resolves to nothing, and every connection is refused with
// nothing anywhere reporting a failure. "hck add service" against a DaemonSet
// or CronJob chart produces it: neither of those templates declares a
// container port at all.
//
// The names are collected across every pod spec the chart renders rather than
// matched against the Service selector. A chart carries one workload, and
// being quiet when unsure is the right way for a warning to be wrong — a
// chart that renders no pod at all has a Service for somebody else's pods,
// and this says nothing about it.
func serviceTargetPortRule(objs []object) []hit {
	declared := map[string]bool{}
	pods := 0
	for _, o := range objs {
		spec, ok := podSpecOf(o)
		if !ok {
			continue
		}
		pods++
		containers, _ := spec["containers"].([]any)
		for _, ci := range containers {
			c, ok := ci.(map[string]any)
			if !ok {
				continue
			}
			ports, _ := c["ports"].([]any)
			for _, pi := range ports {
				p, ok := pi.(map[string]any)
				if !ok {
					continue
				}
				if name := str(p["name"]); name != "" {
					declared[name] = true
				}
			}
		}
	}
	if pods == 0 {
		return nil
	}

	var out []hit
	for _, o := range objs {
		if o.Kind != "Service" {
			continue
		}
		// No selector means the Service does not choose its own endpoints.
		if sel, _ := o.Spec["selector"].(map[string]any); len(sel) == 0 {
			continue
		}
		ports, _ := o.Spec["ports"].([]any)
		for _, pi := range ports {
			p, ok := pi.(map[string]any)
			if !ok {
				continue
			}
			// Only a named targetPort can dangle. A number is forwarded
			// whether or not anything is listening on it.
			target := str(p["targetPort"])
			if target == "" || declared[target] {
				continue
			}
			out = append(out, hit{
				Where: "Service/" + o.Name,
				Message: fmt.Sprintf(
					"port %q forwards to a container port named %q, which no container in this chart declares. The endpoints exist and the name resolves to nothing, so every connection is refused without anything reporting a failure",
					str(p["name"]), target),
			})
		}
	}
	return out
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

// competingPairsRule reports two resources that answer one question.
//
// The platform axis makes this reachable in a way it was not before: a chart
// can now carry a cert-manager Certificate and a GKE ManagedCertificate, or a
// ServiceMonitor and a PodMonitoring, and both pairs apply cleanly. Nothing
// errors. One TLS secret is simply never used, or one scrape is billed twice
// and the dashboards disagree about which is authoritative.
//
// Same shape as HCK031 and HCK032, and listed beside them for that reason:
// two controllers with one job, chosen by whichever was reconciled last.
func competingPairsRule(objs []object) []hit {
	pairs := []struct{ a, b, why string }{
		{"Certificate", "ManagedCertificate",
			"both terminate TLS for this chart, and only the one the Ingress references is used — the other is a certificate nobody serves"},
		{"ServiceMonitor", "PodMonitoring",
			"Prometheus Operator reads the first and Google Managed Prometheus the second; a cluster running both scrapes the workload twice"},
	}
	var out []hit
	for _, p := range pairs {
		as, bs := namesOfKind(objs, p.a), namesOfKind(objs, p.b)
		if len(as) == 0 || len(bs) == 0 {
			continue
		}
		out = append(out, hit{Where: "chart", Message: fmt.Sprintf(
			"chart renders both a %s (%s) and a %s (%s); %s",
			p.a, strings.Join(as, ", "), p.b, strings.Join(bs, ", "), p.why)})
	}
	return out
}

// gkeConfigRefRule reports a GKE annotation naming a BackendConfig or
// FrontendConfig the chart does not render.
//
// These are referenced by name and by nothing else, so a name that resolves to
// nothing is the quiet failure this file keeps coming back to: everything
// applies, and GKE falls back to a health check on "/" or serves plaintext on
// port 80. The chart renders, installs, and is wrong in a way only the load
// balancer knows about.
//
// Says nothing when the chart renders no object of that kind AND sets no such
// annotation — and nothing at all about a name pointing outside the chart is
// impossible to distinguish from a typo, so this reports only what it can see.
func gkeConfigRefRule(objs []object) []hit {
	present := map[string]map[string]bool{
		"BackendConfig":  {},
		"FrontendConfig": {},
	}
	for _, o := range objs {
		if set, ok := present[o.Kind]; ok {
			set[o.Name] = true
		}
	}
	refs := []struct{ annotation, kind string }{
		{"cloud.google.com/backend-config", "BackendConfig"},
		{"networking.gke.io/v1beta1.FrontendConfig", "FrontendConfig"},
	}
	var out []hit
	for _, o := range objs {
		annotations, _ := o.Metadata["annotations"].(map[string]any)
		for _, r := range refs {
			raw, ok := annotations[r.annotation].(string)
			if !ok || raw == "" {
				continue
			}
			for _, name := range gkeRefNames(raw) {
				if present[r.kind][name] {
					continue
				}
				have := "none"
				if n := sortedKeys(present[r.kind]); len(n) > 0 {
					have = strings.Join(n, ", ")
				}
				out = append(out, hit{
					Where: o.Kind + "/" + o.Name,
					Message: fmt.Sprintf(
						"%s names %s %q, which this chart does not render (it renders: %s). The annotation applies and GKE silently falls back to its own defaults",
						r.annotation, r.kind, name, have),
				})
			}
		}
	}
	return out
}

// gkeRefNames pulls the names out of a backend-config annotation, which is
// JSON ({"default":"x"} or {"ports":{"80":"x"}}), and out of a FrontendConfig
// annotation, which is a bare name.
func gkeRefNames(raw string) []string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "{") {
		return []string{raw}
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		// Unparseable is not the same as dangling, and guessing at it would
		// report a name nobody wrote.
		return nil
	}
	var out []string
	for _, v := range doc {
		switch t := v.(type) {
		case string:
			out = append(out, t)
		case map[string]any:
			for _, vv := range t {
				if name, ok := vv.(string); ok {
					out = append(out, name)
				}
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// sortedKeys is the set as a sorted slice, for a message that has to be the
// same on every run.
func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// unusedSecretStoreRule reports a SecretStore nothing in the chart reads.
//
// The same shape as HCK034 and for the same reason: both halves apply, the
// ExternalSecret keeps reading whatever ClusterSecretStore it names, and the
// SecretStore beside it is an object with credentials in it that nothing uses.
// Nothing errors, and the chart looks like it owns its own credentials path
// when it does not.
//
// Only a "SecretStore" kind in the ref counts as using it. A ClusterSecretStore
// reference legitimately points outside the chart — that is what a shared store
// is — so it is reported as what the ExternalSecret names instead, never as a
// dangling reference.
func unusedSecretStoreRule(objs []object) []hit {
	used := map[string]bool{}
	var pointsAt []string
	for _, o := range objs {
		if o.Kind != "ExternalSecret" {
			continue
		}
		ref, _ := o.Spec["secretStoreRef"].(map[string]any)
		kind, name := str(ref["kind"]), str(ref["name"])
		if kind == "SecretStore" {
			used[name] = true
		}
		if r := kind + "/" + name; kind != "" && name != "" && !slices.Contains(pointsAt, r) {
			pointsAt = append(pointsAt, r)
		}
	}
	if len(pointsAt) == 0 {
		return nil
	}
	var out []hit
	for _, o := range objs {
		if o.Kind != "SecretStore" || used[o.Name] {
			continue
		}
		out = append(out, hit{
			Where: "SecretStore/" + o.Name,
			Message: fmt.Sprintf(
				"nothing in this chart uses this SecretStore — its ExternalSecret names %s instead, which the chart does not provide. Point externalSecret.secretStoreRef at it, or drop one of the two",
				strings.Join(pointsAt, ", ")),
		})
	}
	return out
}

// storageClassShipsWithThePlatform is every class name one of hck's own
// platform overlays writes that the platform creates for you. Nothing to
// report: a claim against it binds on a cluster nobody has touched.
var storageClassShipsWithThePlatform = []string{
	"premium-rwo",         // GKE, alongside standard and standard-rwo
	"managed-csi-premium", // AKS, alongside default, managed-csi and azurefile
}

// storageClassNeedsProvisioning is every class name one of hck's own platform
// overlays writes that the platform does not create, mapped to what has to
// happen first. These are the two halves of one list, and
// TestEveryOverlayStorageClassIsClassified holds them to it: a new overlay
// naming a class nobody classified fails there rather than being silently
// unreportable.
//
// A class name outside both lists is the user's own, and hck says nothing
// about it. It knows nothing about their cluster, and a note about a class it
// never suggested would be a guess — the same quiet-when-unsure line HCK035
// draws when the chart renders no pod.
var storageClassNeedsProvisioning = map[string]string{
	"gp3": "EKS ships gp2 and no gp3 — create one against the EBS CSI driver first",
	"local-path": "local-path comes from local-path-provisioner, which k3s ships and a stock cluster does not — install it, " +
		"or point persistence.storageClass at storage this cluster already has",
}

// unprovisionedStorageClassRule reports a claim whose storage class hck itself
// suggested and hck itself cannot create.
//
// It is an info rather than a warning, and the difference is the whole point.
// Nothing here is a mistake: gp3 is the right class to ask for on EKS, the aws
// overlay is right to say so, and a chart is not wrong for naming it. What is
// true is that the chart has a prerequisite outside itself — the class has to
// exist — and if it does not, helm install succeeds, the PersistentVolumeClaim
// stays Pending, and the pod never schedules. No controller writes that
// anywhere a person is looking.
//
// The chart cannot close the gap either: a StorageClass is cluster-scoped, so
// a chart creating one collides with the next release that does the same, for
// the same reason ClusterSecretStore and ClusterPodMonitoring stayed out of
// the catalog. Reporting is the whole of what hck can do here, which is why
// this is the rule and not a resource.
func unprovisionedStorageClassRule(objs []object) []hit {
	var out []hit
	note := func(where, class string) {
		remedy, ok := storageClassNeedsProvisioning[class]
		if !ok {
			return
		}
		out = append(out, hit{
			Where: where,
			Message: fmt.Sprintf(
				"requests storage class %q, which this chart does not create and the platform does not ship. %s. Until then the claim stays Pending and the pod never schedules, and nothing reports a failure",
				class, remedy),
		})
	}
	for _, o := range objs {
		switch o.Kind {
		case "PersistentVolumeClaim":
			note("PersistentVolumeClaim/"+o.Name, str(o.Spec["storageClassName"]))
		case "StatefulSet":
			templates, _ := o.Spec["volumeClaimTemplates"].([]any)
			for _, ti := range templates {
				t, ok := ti.(map[string]any)
				if !ok {
					continue
				}
				spec, ok := t["spec"].(map[string]any)
				if !ok {
					continue
				}
				where := "StatefulSet/" + o.Name
				if md, ok := t["metadata"].(map[string]any); ok {
					if n := str(md["name"]); n != "" {
						where += " volumeClaimTemplate=" + n
					}
				}
				note(where, str(spec["storageClassName"]))
			}
		}
	}
	return out
}
