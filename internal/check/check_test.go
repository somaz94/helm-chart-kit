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

func rules(t *testing.T, o object) map[string]Finding {
	t.Helper()
	out := map[string]Finding{}
	for _, f := range manifestRules(o) {
		out[f.Rule] = f
	}
	return out
}

func TestManifestRulesCleanContainer(t *testing.T) {
	got := rules(t, deploymentWith(map[string]any{
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
	got := rules(t, deploymentWith(map[string]any{
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
	got := rules(t, deploymentWith(map[string]any{"name": "app", "image": "registry:5000/acme/app"}, nil))
	f, ok := got["HCK021"]
	if !ok {
		t.Fatal("HCK021 did not fire for an untagged image")
	}
	if f.Severity != Error {
		t.Error("an untagged image should be an error")
	}
}

func TestManifestRulesMissingImage(t *testing.T) {
	got := rules(t, deploymentWith(map[string]any{"name": "app"}, nil))
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
	for _, f := range manifestRules(job) {
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
	for _, f := range chartLayoutRules(c) {
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
	if got := manifestRules(o); len(got) != 0 {
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
		if got := manifestSetRules(objs); len(got) != 0 {
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
		got := manifestSetRules(tc)
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
			if got := scalerRules(objs); len(got) != 0 {
				t.Errorf("produced findings: %+v", got)
			}
		})
	}
}

func TestScalerRulesFlagsHPAWithKEDA(t *testing.T) {
	got := scalerRules([]object{
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
			got := scalerRules([]object{
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
