package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/somaz94/helm-chart-kit/internal/chart"
)

func TestLastSegment(t *testing.T) {
	// The port in a registry host must not be mistaken for a tag separator.
	for in, want := range map[string]string{
		"nginx":                        "nginx",
		"library/nginx:1.0":            "nginx:1.0",
		"registry:5000/team/app":       "app",
		"registry:5000/team/app:1.2.3": "app:1.2.3",
	} {
		if got := lastSegment(in); got != want {
			t.Errorf("lastSegment(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDecodeAll(t *testing.T) {
	objs, err := decodeAll(`---
apiVersion: v1
kind: Service
metadata:
  name: a
spec:
  type: ClusterIP
---
# a document with nothing in it
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: b
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 2 {
		t.Fatalf("got %d objects, want 2", len(objs))
	}
	if objs[0].Kind != "Service" || objs[0].Name != "a" {
		t.Errorf("unexpected first object: %+v", objs[0])
	}
	if objs[1].Kind != "ConfigMap" || objs[1].Name != "b" {
		t.Errorf("unexpected second object: %+v", objs[1])
	}
}

func TestDecodeAllRejectsGarbage(t *testing.T) {
	if _, err := decodeAll("kind: [\n"); err == nil {
		t.Error("want an error for malformed YAML")
	}
}

func TestPodSpecOf(t *testing.T) {
	deploy := object{Kind: "Deployment", Spec: map[string]any{
		"template": map[string]any{"spec": map[string]any{"containers": []any{}}},
	}}
	if _, ok := podSpecOf(deploy); !ok {
		t.Error("Deployment pod spec not found")
	}

	cron := object{Kind: "CronJob", Spec: map[string]any{
		"jobTemplate": map[string]any{"spec": map[string]any{
			"template": map[string]any{"spec": map[string]any{"containers": []any{}}},
		}},
	}}
	if _, ok := podSpecOf(cron); !ok {
		t.Error("CronJob pod spec not found — it nests one level deeper than the rest")
	}

	if _, ok := podSpecOf(object{Kind: "Service", Spec: map[string]any{}}); ok {
		t.Error("Service reported as carrying a pod spec")
	}
	if _, ok := podSpecOf(object{Kind: "Deployment", Spec: map[string]any{}}); ok {
		t.Error("a Deployment with no template reported as carrying a pod spec")
	}
}

// deploymentWith builds a minimal Deployment around one container.
func deploymentWith(container map[string]any, podSecurityContext map[string]any) object {
	return object{
		Kind: "Deployment", Name: "demo",
		Spec: map[string]any{"template": map[string]any{"spec": map[string]any{
			"securityContext": podSecurityContext,
			"containers":      []any{container},
		}}},
	}
}

func findingsByRule(t *testing.T, o object) map[string]Finding {
	t.Helper()
	out := map[string]Finding{}
	for _, f := range objectRules(o) {
		out[f.Rule] = f
	}
	return out
}

func TestManifestRulesCleanContainer(t *testing.T) {
	got := findingsByRule(t, deploymentWith(map[string]any{
		"name":  "app",
		"image": "ghcr.io/acme/app:1.2.3",
		"resources": map[string]any{
			"requests": map[string]any{"cpu": "100m", "memory": "128Mi"},
			"limits":   map[string]any{"memory": "256Mi"},
		},
		"securityContext": map[string]any{"allowPrivilegeEscalation": false},
		"livenessProbe":   map[string]any{},
		"readinessProbe":  map[string]any{},
	}, map[string]any{"runAsNonRoot": true}))

	if len(got) != 0 {
		t.Fatalf("a clean Deployment produced findings: %v", got)
	}
}

func TestManifestRulesCatchesTheUsualDefects(t *testing.T) {
	got := findingsByRule(t, deploymentWith(map[string]any{
		"name":            "app",
		"image":           "acme/app:latest",
		"resources":       map[string]any{"limits": map[string]any{"cpu": "1", "memory": "1Gi"}},
		"securityContext": map[string]any{"privileged": true},
	}, nil))

	for _, rule := range []string{
		"HCK020", // runAsNonRoot unset
		"HCK022", // :latest
		"HCK023", // no requests
		"HCK025", // cpu limit
		"HCK026", // allowPrivilegeEscalation unset
		"HCK027", // privileged
		"HCK028", // no readiness probe
		"HCK029", // no liveness probe
	} {
		if _, ok := got[rule]; !ok {
			t.Errorf("%s did not fire", rule)
		}
	}
	if got["HCK027"].Severity != Error {
		t.Error("a privileged container should be an error, not a warning")
	}
}

func TestManifestRulesUntaggedImage(t *testing.T) {
	got := findingsByRule(t, deploymentWith(map[string]any{"name": "app", "image": "registry:5000/acme/app"}, nil))
	f, ok := got["HCK021"]
	if !ok {
		t.Fatal("HCK021 did not fire for an untagged image")
	}
	if f.Severity != Error {
		t.Error("an untagged image should be an error")
	}
}

func TestManifestRulesMissingImage(t *testing.T) {
	got := findingsByRule(t, deploymentWith(map[string]any{"name": "app"}, nil))
	if got["HCK021"].Severity != Error {
		t.Error("a container with no image should be an error")
	}
}

// Probe rules apply to long-running workloads only; a Job that exits is not
// expected to answer a readiness probe.
func TestManifestRulesSkipProbesForJobs(t *testing.T) {
	job := object{Kind: "Job", Name: "migrate", Spec: map[string]any{
		"template": map[string]any{"spec": map[string]any{
			"securityContext": map[string]any{"runAsNonRoot": true},
			"containers": []any{map[string]any{
				"name":  "migrate",
				"image": "acme/app:1.0",
				"resources": map[string]any{
					"requests": map[string]any{"cpu": "10m"},
					"limits":   map[string]any{"memory": "64Mi"},
				},
				"securityContext": map[string]any{"allowPrivilegeEscalation": false},
			}},
		}},
	}}
	for _, f := range objectRules(job) {
		if f.Rule == "HCK028" || f.Rule == "HCK029" {
			t.Errorf("probe rule %s fired on a Job", f.Rule)
		}
	}
}

func TestChartLayoutRules(t *testing.T) {
	dir := t.TempDir()
	meta := "apiVersion: v1\nname: demo\nversion: 0.1.0\n"
	if err := os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := chart.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, f := range chartRules(c) {
		got[f.Rule] = true
	}
	for _, rule := range []string{"HCK010", "HCK011", "HCK012", "HCK013"} {
		if !got[rule] {
			t.Errorf("%s did not fire", rule)
		}
	}
}

func TestReportCounts(t *testing.T) {
	r := &Report{Findings: []Finding{
		{Severity: Error}, {Severity: Warn}, {Severity: Warn},
	}}
	if r.Errors() != 1 || r.Warns() != 2 {
		t.Fatalf("Errors=%d Warns=%d, want 1 and 2", r.Errors(), r.Warns())
	}
}

func TestFirstLines(t *testing.T) {
	if got := firstLines("a\nb\nc", 2); !strings.HasSuffix(got, "...") {
		t.Errorf("long input was not truncated: %q", got)
	}
	if got := firstLines("a\nb", 5); got != "a\nb" {
		t.Errorf("short input was altered: %q", got)
	}
}

func TestPodSpecOfPod(t *testing.T) {
	spec := map[string]any{"containers": []any{}}
	got, ok := podSpecOf(object{Kind: "Pod", Spec: spec})
	if !ok || got == nil {
		t.Fatal("a bare Pod carries its own pod spec")
	}
	if _, ok := podSpecOf(object{Kind: "Pod"}); ok {
		t.Error("a Pod with no spec reported as carrying one")
	}
}

func TestManifestRulesIgnoresMalformedContainers(t *testing.T) {
	o := object{Kind: "Deployment", Name: "demo", Spec: map[string]any{
		"template": map[string]any{"spec": map[string]any{
			"securityContext": map[string]any{"runAsNonRoot": true},
			"containers":      []any{"not-a-map"},
		}},
	}}
	if got := objectRules(o); len(got) != 0 {
		t.Fatalf("a non-map container produced findings: %v", got)
	}
}

func TestNonEmpty(t *testing.T) {
	got := nonEmpty("", "  ", "a", "\nb\n")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("got %v", got)
	}
}

func TestValuesArgs(t *testing.T) {
	got := valuesArgs([]string{"a.yaml", "b.yaml"})
	want := []string{"-f", "a.yaml", "-f", "b.yaml"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v, want %v", got, want)
	}
	if len(valuesArgs(nil)) != 0 {
		t.Error("nil input should yield no args")
	}
}

func TestManifestSetRulesSingleWorkloadIsClean(t *testing.T) {
	for _, objs := range [][]object{
		nil,
		{{Kind: "Deployment", Name: "a"}},
		// A Job is a one-shot task alongside the workload, not one itself.
		{{Kind: "Deployment", Name: "a"}, {Kind: "Job", Name: "migrate"}},
		{{Kind: "Service", Name: "a"}, {Kind: "ConfigMap", Name: "a"}},
	} {
		if got := setRules(objs); len(got) != 0 {
			t.Errorf("%v produced findings: %+v", objs, got)
		}
	}
}

func TestManifestSetRulesFlagsTwoWorkloads(t *testing.T) {
	for _, tc := range [][]object{
		{{Kind: "Deployment", Name: "a"}, {Kind: "DaemonSet", Name: "b"}},
		{{Kind: "StatefulSet", Name: "a"}, {Kind: "CronJob", Name: "b"}},
		{{Kind: "Deployment", Name: "a"}, {Kind: "Deployment", Name: "b"}},
	} {
		got := setRules(tc)
		if len(got) != 1 {
			t.Fatalf("%v produced %d findings, want 1", tc, len(got))
		}
		if got[0].Rule != "HCK030" {
			t.Errorf("rule = %q, want HCK030", got[0].Rule)
		}
		// Warn, not Error: hck refuses to generate this, but a chart written
		// elsewhere is allowed to be odd. --strict is what fails on it.
		if got[0].Severity != Warn {
			t.Errorf("severity = %q, want %q", got[0].Severity, Warn)
		}
		for _, o := range tc {
			if !strings.Contains(got[0].Message, o.Kind+"/"+o.Name) {
				t.Errorf("message does not name %s/%s: %s", o.Kind, o.Name, got[0].Message)
			}
		}
	}
}

func TestScalerRulesQuietOnLegitimateCombinations(t *testing.T) {
	off := func(mode string) object {
		return object{Kind: "VerticalPodAutoscaler", Name: "a",
			Spec: map[string]any{"updatePolicy": map[string]any{"updateMode": mode}}}
	}
	for name, objs := range map[string][]object{
		"nothing":              nil,
		"hpa alone":            {{Kind: "HorizontalPodAutoscaler", Name: "a"}},
		"keda alone":           {{Kind: "ScaledObject", Name: "a"}},
		"vpa alone in Auto":    {off("Auto")},
		"hpa with vpa Off":     {{Kind: "HorizontalPodAutoscaler", Name: "a"}, off("Off")},
		"hpa with vpa Initial": {{Kind: "HorizontalPodAutoscaler", Name: "a"}, off("Initial")},
		"hpa with vpa unset":   {{Kind: "HorizontalPodAutoscaler", Name: "a"}, {Kind: "VerticalPodAutoscaler", Name: "a"}},
		"keda with vpa Auto":   {{Kind: "ScaledObject", Name: "a"}, off("Auto")},
	} {
		t.Run(name, func(t *testing.T) {
			if got := setRules(objs); len(got) != 0 {
				t.Errorf("produced findings: %+v", got)
			}
		})
	}
}

func TestScalerRulesFlagsHPAWithKEDA(t *testing.T) {
	got := setRules([]object{
		{Kind: "HorizontalPodAutoscaler", Name: "a"},
		{Kind: "ScaledObject", Name: "b"},
	})
	if len(got) != 1 || got[0].Rule != "HCK031" {
		t.Fatalf("got %+v, want one HCK031", got)
	}
	if got[0].Severity != Warn {
		t.Errorf("severity = %q", got[0].Severity)
	}
}

// Only the evicting modes conflict; Off and Initial merely recommend.
func TestScalerRulesFlagsHPAWithEvictingVPA(t *testing.T) {
	for _, mode := range []string{"Auto", "Recreate"} {
		t.Run(mode, func(t *testing.T) {
			got := setRules([]object{
				{Kind: "HorizontalPodAutoscaler", Name: "a"},
				{Kind: "VerticalPodAutoscaler", Name: "b",
					Spec: map[string]any{"updatePolicy": map[string]any{"updateMode": mode}}},
			})
			if len(got) != 1 || got[0].Rule != "HCK032" {
				t.Fatalf("got %+v, want one HCK032", got)
			}
		})
	}
}

func TestVPAUpdateModeHandlesAMalformedSpec(t *testing.T) {
	for name, o := range map[string]object{
		"no spec":           {Kind: "VerticalPodAutoscaler"},
		"policy not a map":  {Spec: map[string]any{"updatePolicy": "Auto"}},
		"mode not a string": {Spec: map[string]any{"updatePolicy": map[string]any{"updateMode": 3}}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := vpaUpdateMode(o); got != "" {
				t.Errorf("got %q, want empty", got)
			}
		})
	}
}

// A pod spec the rules cannot reach is a pod spec nobody checks. KEDA's
// ScaledJob carries one inline, three levels down.
func TestPodSpecOfReachesEveryInlinePodSpec(t *testing.T) {
	spec := map[string]any{"containers": []any{map[string]any{"name": "app"}}}
	for name, o := range map[string]object{
		"Deployment":  {Kind: "Deployment", Spec: map[string]any{"template": map[string]any{"spec": spec}}},
		"StatefulSet": {Kind: "StatefulSet", Spec: map[string]any{"template": map[string]any{"spec": spec}}},
		"DaemonSet":   {Kind: "DaemonSet", Spec: map[string]any{"template": map[string]any{"spec": spec}}},
		"Job":         {Kind: "Job", Spec: map[string]any{"template": map[string]any{"spec": spec}}},
		"Pod":         {Kind: "Pod", Spec: spec},
		"CronJob": {Kind: "CronJob", Spec: map[string]any{
			"jobTemplate": map[string]any{"spec": map[string]any{"template": map[string]any{"spec": spec}}}}},
		"ScaledJob": {Kind: "ScaledJob", Spec: map[string]any{
			"jobTargetRef": map[string]any{"template": map[string]any{"spec": spec}}}},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := podSpecOf(o)
			if !ok {
				t.Fatal("no pod spec found")
			}
			if _, ok := got["containers"]; !ok {
				t.Errorf("found the wrong map: %v", got)
			}
		})
	}
	if _, ok := podSpecOf(object{Kind: "Service"}); ok {
		t.Error("a Service reported a pod spec")
	}
}

// The three helpers below run one scope's rules the way Run does, so a test
// exercises the rule through the same path a real check takes rather than
// calling the closure inside it.

func chartRules(c *chart.Chart) []Finding {
	rep := &Report{}
	for _, rule := range rulesIn(ChartScope) {
		rep.apply(nil, rule, func() []hit { return rule.chart(c) })
	}
	return rep.Findings
}

func objectRules(o object) []Finding {
	rep := &Report{}
	for _, rule := range rulesIn(ObjectScope) {
		rep.apply(nil, rule, func() []hit { return rule.object(o) })
	}
	return rep.Findings
}

func setRules(objs []object) []Finding {
	rep := &Report{}
	for _, rule := range rulesIn(SetScope) {
		rep.apply(nil, rule, func() []hit { return rule.set(objs) })
	}
	return rep.Findings
}

// The quietest way a chart can be wrong: the scaler renders, helm installs it,
// and the controller reports that it cannot find its target in a status nobody
// reads.
func TestDanglingScaleTargetIsReported(t *testing.T) {
	hpa := func(kind, name string) object {
		return object{Kind: "HorizontalPodAutoscaler", Name: "app", Spec: map[string]any{
			"scaleTargetRef": map[string]any{"kind": kind, "name": name},
		}}
	}
	deployment := object{Kind: "Deployment", Name: "app", Spec: map[string]any{
		"template": map[string]any{"spec": map[string]any{}},
	}}
	statefulset := object{Kind: "StatefulSet", Name: "app", Spec: map[string]any{
		"template": map[string]any{"spec": map[string]any{}},
	}}

	t.Run("target is there", func(t *testing.T) {
		if got := findRule(setRules([]object{deployment, hpa("Deployment", "app")}), "HCK033"); got != nil {
			t.Errorf("fired on a scaler whose target renders: %v", got)
		}
	})
	t.Run("wrong kind", func(t *testing.T) {
		got := findRule(setRules([]object{statefulset, hpa("Deployment", "app")}), "HCK033")
		if got == nil {
			t.Fatal("an HPA aimed at a kind the chart does not render was not reported")
		}
		// The chart's own workload is named, so the mismatch is the message
		// rather than something the reader has to go and look up.
		if !strings.Contains(got.Message, "StatefulSet/app") {
			t.Errorf("the message does not say what the chart renders: %q", got.Message)
		}
	})
	t.Run("no workload at all", func(t *testing.T) {
		got := findRule(setRules([]object{hpa("Deployment", "app")}), "HCK033")
		if got == nil {
			t.Fatal("an HPA in a chart with no workload was not reported")
		}
		if !strings.Contains(got.Message, "no workload at all") {
			t.Errorf("got %q", got.Message)
		}
	})
	t.Run("every scaler kind is covered", func(t *testing.T) {
		for _, o := range []object{
			{Kind: "ScaledObject", Name: "app", Spec: map[string]any{
				"scaleTargetRef": map[string]any{"kind": "Deployment", "name": "gone"}}},
			{Kind: "VerticalPodAutoscaler", Name: "app", Spec: map[string]any{
				"targetRef": map[string]any{"kind": "Deployment", "name": "gone"}}},
		} {
			if findRule(setRules([]object{deployment, o}), "HCK033") == nil {
				t.Errorf("%s aimed at a missing target was not reported", o.Kind)
			}
		}
	})
	t.Run("a reference that says nothing is not a finding", func(t *testing.T) {
		for _, spec := range []map[string]any{
			{},
			{"scaleTargetRef": "not a map"},
			{"scaleTargetRef": map[string]any{"name": "app"}},
			{"scaleTargetRef": map[string]any{"kind": "Deployment"}},
		} {
			o := object{Kind: "HorizontalPodAutoscaler", Name: "app", Spec: spec}
			if got := findRule(setRules([]object{deployment, o}), "HCK033"); got != nil {
				t.Errorf("fired on %v: %v", spec, got)
			}
		}
	})
}

// findRule returns the first finding for a rule, or nil.
func findRule(findings []Finding, id string) *Finding {
	for i, f := range findings {
		if f.Rule == id {
			return &findings[i]
		}
	}
	return nil
}

func issuer(name string) object { return object{Kind: "Issuer", Name: name} }

func certificate(issuerKind, issuerName string) object {
	return object{Kind: "Certificate", Name: "app", Spec: map[string]any{
		"issuerRef": map[string]any{"kind": issuerKind, "name": issuerName},
	}}
}

// "hck add certificate issuer" leaves the two unconnected: the Certificate
// keeps its default ClusterIssuer and the Issuer the chart just created is
// used by nothing. Both apply cleanly, so nothing says so.
func TestUnusedIssuerIsReported(t *testing.T) {
	t.Run("wired to the chart's own issuer", func(t *testing.T) {
		if got := findRule(setRules([]object{issuer("app"), certificate("Issuer", "app")}), "HCK034"); got != nil {
			t.Errorf("fired on a Certificate that uses the chart's Issuer: %v", got)
		}
	})
	t.Run("pointed somewhere else", func(t *testing.T) {
		got := findRule(setRules([]object{issuer("app"), certificate("ClusterIssuer", "letsencrypt-prod")}), "HCK034")
		if got == nil {
			t.Fatal("an Issuer nothing uses was not reported")
		}
		if !strings.Contains(got.Message, "ClusterIssuer/letsencrypt-prod") {
			t.Errorf("the message does not say where the Certificate points: %q", got.Message)
		}
	})
	t.Run("an issuer other releases use is left alone", func(t *testing.T) {
		if got := findRule(setRules([]object{issuer("app")}), "HCK034"); got != nil {
			t.Errorf("fired on a chart with no Certificate at all: %v", got)
		}
	})
	t.Run("a certificate on its own is left alone", func(t *testing.T) {
		if got := findRule(setRules([]object{certificate("ClusterIssuer", "letsencrypt-prod")}), "HCK034"); got != nil {
			t.Errorf("fired on a chart with no Issuer: %v", got)
		}
	})
}

func service(selector map[string]any, ports ...map[string]any) object {
	as := make([]any, 0, len(ports))
	for _, p := range ports {
		as = append(as, p)
	}
	return object{Kind: "Service", Name: "app", Spec: map[string]any{
		"selector": selector,
		"ports":    as,
	}}
}

func workloadWithPorts(kind string, portNames ...string) object {
	ports := make([]any, 0, len(portNames))
	for _, n := range portNames {
		ports = append(ports, map[string]any{"name": n, "containerPort": 8080})
	}
	container := map[string]any{"name": "app", "image": "app:1"}
	if len(ports) > 0 {
		container["ports"] = ports
	}
	return object{Kind: kind, Name: "app", Spec: map[string]any{
		"template": map[string]any{"spec": map[string]any{
			"containers": []any{container},
		}},
	}}
}

// A named targetPort is the right way to write a Service, and it is silent
// when it is wrong: the endpoints exist, the name resolves to nothing, and
// every connection is refused.
func TestServiceTargetPortIsChecked(t *testing.T) {
	selector := map[string]any{"app.kubernetes.io/name": "app"}
	named := map[string]any{"name": "http", "targetPort": "http"}

	t.Run("the container declares it", func(t *testing.T) {
		objs := []object{workloadWithPorts("Deployment", "http"), service(selector, named)}
		if got := findRule(setRules(objs), "HCK035"); got != nil {
			t.Errorf("fired on a Service whose target port exists: %v", got)
		}
	})
	t.Run("nothing declares it", func(t *testing.T) {
		// What "hck add service" against a daemon chart produces: the
		// DaemonSet template declares no container port at all.
		objs := []object{workloadWithPorts("DaemonSet"), service(selector, named)}
		got := findRule(setRules(objs), "HCK035")
		if got == nil {
			t.Fatal("a Service forwarding to a name nothing declares was not reported")
		}
		if !strings.Contains(got.Message, `"http"`) {
			t.Errorf("got %q", got.Message)
		}
	})
	t.Run("a number is not a name", func(t *testing.T) {
		objs := []object{workloadWithPorts("DaemonSet"), service(selector, map[string]any{"name": "http", "targetPort": 8080})}
		if got := findRule(setRules(objs), "HCK035"); got != nil {
			t.Errorf("fired on a numeric targetPort: %v", got)
		}
	})
	t.Run("a Service that does not choose its endpoints", func(t *testing.T) {
		objs := []object{workloadWithPorts("DaemonSet"), service(nil, named)}
		if got := findRule(setRules(objs), "HCK035"); got != nil {
			t.Errorf("fired on a Service with no selector: %v", got)
		}
	})
	t.Run("a chart that renders no pod at all", func(t *testing.T) {
		// The Service is for somebody else's pods, and this says nothing.
		if got := findRule(setRules([]object{service(selector, named)}), "HCK035"); got != nil {
			t.Errorf("fired on a chart with no workload: %v", got)
		}
	})
	t.Run("every port is checked, not just the first", func(t *testing.T) {
		objs := []object{
			workloadWithPorts("Deployment", "http"),
			service(selector, named, map[string]any{"name": "metrics", "targetPort": "metrics"}),
		}
		got := findRule(setRules(objs), "HCK035")
		if got == nil {
			t.Fatal("a second port forwarding to nothing was not reported")
		}
		if !strings.Contains(got.Message, "metrics") {
			t.Errorf("got %q", got.Message)
		}
	})
}

// A container with limits but no memory limit has one thing wrong with it, not
// two: HCK024 says the memory limit is missing, and HCK025 stays quiet rather
// than also complaining about the CPU limit that is there.
func TestACpuLimitWithoutAMemoryLimitIsOneFinding(t *testing.T) {
	got := findingsByRule(t, deploymentWith(map[string]any{
		"name":      "app",
		"image":     "ghcr.io/acme/app:1.2.3",
		"resources": map[string]any{"requests": map[string]any{"cpu": "100m"}, "limits": map[string]any{"cpu": "1"}},
	}, nil))
	if f, ok := got["HCK024"]; !ok {
		t.Error("HCK024 did not fire on limits with no memory")
	} else if !strings.Contains(f.Message, "memory limit") {
		t.Errorf("got %q", f.Message)
	}
	if f, ok := got["HCK025"]; ok {
		t.Errorf("HCK025 also fired: %q", f.Message)
	}
}

// The rendered manifests come from helm and are trusted to be YAML, not to be
// shaped the way a rule expects. A malformed entry is skipped rather than
// panicking the whole check.
func TestServiceTargetPortRuleIgnoresMalformedInput(t *testing.T) {
	selector := map[string]any{"app.kubernetes.io/name": "app"}
	workload := object{Kind: "Deployment", Name: "app", Spec: map[string]any{
		"template": map[string]any{"spec": map[string]any{
			"containers": []any{
				"not a container",
				map[string]any{"name": "app", "ports": []any{"not a port", map[string]any{"name": "http"}}},
			},
		}},
	}}
	svc := object{Kind: "Service", Name: "app", Spec: map[string]any{
		"selector": selector,
		"ports":    []any{"not a port", map[string]any{"name": "http", "targetPort": "http"}},
	}}
	if got := findRule(setRules([]object{workload, svc}), "HCK035"); got != nil {
		t.Errorf("fired despite the container declaring http: %v", got)
	}

	// And the name really is being read out of that malformed list, rather
	// than the rule giving up on it.
	svc.Spec["ports"] = []any{map[string]any{"name": "metrics", "targetPort": "metrics"}}
	if got := findRule(setRules([]object{workload, svc}), "HCK035"); got == nil {
		t.Error("a port nothing declares was not reported")
	}
}

func budget(spec map[string]any) object {
	return object{Kind: "PodDisruptionBudget", Name: "app", Spec: spec}
}

func workloadWithReplicas(kind string, replicas any) object {
	spec := map[string]any{"template": map[string]any{"spec": map[string]any{}}}
	if replicas != nil {
		spec["replicas"] = replicas
	}
	return object{Kind: kind, Name: "app", Spec: spec}
}

// A budget that allows nothing applies cleanly and works, right up until
// somebody drains a node. Both values files hck writes warn about it in a
// comment, and a comment does not run.
func TestWedgedBudgetIsReported(t *testing.T) {
	deploy3 := workloadWithReplicas("Deployment", 3)

	t.Run("hck's own default is quiet", func(t *testing.T) {
		objs := []object{deploy3, budget(map[string]any{"maxUnavailable": 1})}
		if got := findRule(setRules(objs), "HCK036"); got != nil {
			t.Errorf("fired on maxUnavailable: 1 over 3 replicas: %v", got)
		}
	})
	t.Run("minAvailable below the replica count is fine", func(t *testing.T) {
		objs := []object{deploy3, budget(map[string]any{"minAvailable": 2})}
		if got := findRule(setRules(objs), "HCK036"); got != nil {
			t.Errorf("fired on minAvailable: 2 over 3 replicas: %v", got)
		}
	})
	t.Run("maxUnavailable zero", func(t *testing.T) {
		for _, zero := range []any{0, "0%"} {
			objs := []object{deploy3, budget(map[string]any{"maxUnavailable": zero})}
			got := findRule(setRules(objs), "HCK036")
			if got == nil {
				t.Fatalf("maxUnavailable %v was not reported", zero)
			}
			// Telling someone to use maxUnavailable when maxUnavailable is
			// the problem would be worse than saying nothing.
			if strings.Contains(got.Message, "Use maxUnavailable") {
				t.Errorf("the remedy contradicts the cause: %q", got.Message)
			}
		}
	})
	t.Run("minAvailable at or above the replica count", func(t *testing.T) {
		for _, tc := range []struct {
			replicas, min int
		}{{1, 1}, {3, 3}, {3, 5}} {
			objs := []object{workloadWithReplicas("Deployment", tc.replicas), budget(map[string]any{"minAvailable": tc.min})}
			if got := findRule(setRules(objs), "HCK036"); got == nil {
				t.Errorf("minAvailable %d over %d replicas was not reported", tc.min, tc.replicas)
			}
		}
	})
	t.Run("minAvailable 100 percent", func(t *testing.T) {
		objs := []object{deploy3, budget(map[string]any{"minAvailable": "100%"})}
		if got := findRule(setRules(objs), "HCK036"); got == nil {
			t.Error(`minAvailable "100%" was not reported`)
		}
		// A percentage that leaves room is not a wedge.
		objs = []object{deploy3, budget(map[string]any{"minAvailable": "50%"})}
		if got := findRule(setRules(objs), "HCK036"); got != nil {
			t.Errorf(`fired on minAvailable "50%%": %v`, got)
		}
	})
	t.Run("quiet when the replica count is not knowable", func(t *testing.T) {
		// A Deployment under an HPA leaves replicas out so the autoscaler owns
		// it, and a DaemonSet has no such field at all.
		for _, w := range []object{
			workloadWithReplicas("Deployment", nil),
			workloadWithReplicas("DaemonSet", nil),
		} {
			objs := []object{w, budget(map[string]any{"minAvailable": 1})}
			if got := findRule(setRules(objs), "HCK036"); got != nil {
				t.Errorf("fired without knowing the replica count (%s): %v", w.Kind, got)
			}
		}
		// And a chart with no workload at all says nothing.
		if got := findRule(setRules([]object{budget(map[string]any{"minAvailable": 1})}), "HCK036"); got != nil {
			t.Errorf("fired on a chart with no workload: %v", got)
		}
	})
	t.Run("a budget over zero replicas is not a wedge", func(t *testing.T) {
		objs := []object{workloadWithReplicas("Deployment", 0), budget(map[string]any{"minAvailable": 0})}
		if got := findRule(setRules(objs), "HCK036"); got != nil {
			t.Errorf("fired on a workload scaled to zero: %v", got)
		}
	})
}
