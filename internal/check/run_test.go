package check

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/somaz94/helm-chart-kit/internal/chart"
)

// writeRenderableChart builds the smallest chart that helm will render.
func writeRenderableChart(t *testing.T, template string) *chart.Chart {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "Chart.yaml"),
		"apiVersion: v2\nname: demo\ndescription: demo\nversion: 0.1.0\nappVersion: \"1.0.0\"\n")
	mustWrite(t, filepath.Join(dir, "values.yaml"), "tag: \"1.2.3\"\n")
	mustWrite(t, filepath.Join(dir, ".helmignore"), "ci/\n")
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "templates", "workload.yaml"), template)
	c, err := chart.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func requireHelm(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not on PATH")
	}
}

const cleanDeployment = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
spec:
  selector:
    matchLabels:
      app: demo
  template:
    metadata:
      labels:
        app: demo
    spec:
      securityContext:
        runAsNonRoot: true
      containers:
        - name: app
          image: ghcr.io/acme/app:{{ .Values.tag }}
          livenessProbe:
            httpGet: {path: /, port: 8080}
          readinessProbe:
            httpGet: {path: /, port: 8080}
          resources:
            requests: {cpu: 10m, memory: 16Mi}
            limits: {memory: 32Mi}
          securityContext:
            allowPrivilegeEscalation: false
`

func TestRunClean(t *testing.T) {
	requireHelm(t)
	rep, err := Run(writeRenderableChart(t, cleanDeployment), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Errors() != 0 || rep.Warns() != 0 {
		t.Fatalf("clean chart produced findings: %+v", rep.Findings)
	}
	if !strings.Contains(rep.Rendered, "kind: Deployment") {
		t.Error("rendered manifests were not captured")
	}
}

// A template that does not render must surface as HCK001 with helm's own
// message, not as a Go error the user cannot act on.
func TestRunReportsRenderFailure(t *testing.T) {
	requireHelm(t)
	rep, err := Run(writeRenderableChart(t, "{{ .Values.missing.deeper }}\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Errors() == 0 {
		t.Fatal("a broken template produced no error finding")
	}
	if rep.Findings[len(rep.Findings)-1].Rule != "HCK001" {
		t.Fatalf("unexpected findings: %+v", rep.Findings)
	}
}

// ci/install-values.yaml is picked up automatically, which is what makes a
// chart with a required image tag checkable at all.
func TestRunUsesCIValuesByDefault(t *testing.T) {
	requireHelm(t)
	c := writeRenderableChart(t, `apiVersion: v1
kind: ConfigMap
metadata:
  name: demo
data:
  tag: {{ required "tag is required" .Values.tag | quote }}
`)
	mustWrite(t, c.ValuesPath(), "tag: \"\"\n")
	if err := os.MkdirAll(filepath.Join(c.Dir, "ci"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(c.Dir, "ci", "install-values.yaml"), "tag: \"9.9.9\"\n")

	rep, err := Run(c, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Errors() != 0 {
		t.Fatalf("ci/install-values.yaml was not applied: %+v", rep.Findings)
	}
	if !strings.Contains(rep.Rendered, "9.9.9") {
		t.Error("the CI values did not reach the render")
	}
}

func TestRunExplicitValuesFile(t *testing.T) {
	requireHelm(t)
	c := writeRenderableChart(t, "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: demo\ndata:\n  tag: {{ .Values.tag | quote }}\n")
	vf := filepath.Join(t.TempDir(), "over.yaml")
	mustWrite(t, vf, "tag: \"from-flag\"\n")

	rep, err := Run(c, Options{ValuesFiles: []string{vf}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rep.Rendered, "from-flag") {
		t.Error("-f values file was not applied")
	}
}

// SkipRender must not need helm at all.
func TestRunSkipRender(t *testing.T) {
	c := writeRenderableChart(t, cleanDeployment)
	rep, err := Run(c, Options{SkipRender: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Rendered != "" {
		t.Error("SkipRender still rendered")
	}
}

func TestRunWithoutHelmOnPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := Run(writeRenderableChart(t, cleanDeployment), Options{})
	if err == nil || !strings.Contains(err.Error(), "helm not found") {
		t.Fatalf("got %v, want ErrNoHelm", err)
	}
}
