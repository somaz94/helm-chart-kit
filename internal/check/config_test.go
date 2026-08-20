package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/somaz94/helm-chart-kit/internal/chart"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A chart with no .hck.yaml is the normal case, and has to stay silent.
func TestLoadConfigAbsentIsNotAnError(t *testing.T) {
	cfg, err := LoadConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg != nil {
		t.Errorf("got %+v, want nil", cfg)
	}
}

func TestLoadConfigReadsTheRules(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, "rules:\n  HCK025: off\n  HCK023: error\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(cfg.Rules), 2; got != want {
		t.Fatalf("got %d rules, want %d", got, want)
	}
	if got := cfg.Summary(); got != "HCK023: error, HCK025: off" {
		t.Errorf("Summary() = %q", got)
	}
	if got := cfg.Disabled(); len(got) != 1 || got[0] != "HCK025" {
		t.Errorf("Disabled() = %v", got)
	}
}

// A typo has to be an error. The whole point of writing HCK025 down is to stop
// seeing it, and a misspelling that quietly kept reporting is indistinguishable
// from the rule being right.
func TestLoadConfigRefusesWhatDoesNotMeanAnything(t *testing.T) {
	for name, body := range map[string]string{
		"unknown rule":    "rules:\n  HCK999: off\n",
		"locked rule":     "rules:\n  HCK001: off\n",
		"unknown setting": "rules:\n  HCK025: nope\n",
		"not yaml":        "rules: [\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadConfig(writeConfig(t, body)); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestLoadConfigReportsAnUnreadableFile(t *testing.T) {
	dir := writeConfig(t, "rules:\n")
	if err := os.Chmod(filepath.Join(dir, ConfigFile), 0o000); err != nil {
		t.Skip("cannot make the file unreadable here")
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, ConfigFile), 0o644) })
	if _, err := LoadConfig(dir); err == nil {
		t.Fatal("want an error for an unreadable config")
	}
}

func TestSeverityResolution(t *testing.T) {
	rule := mustRule("HCK023")
	for name, tc := range map[string]struct {
		cfg  *Config
		want Severity
		on   bool
	}{
		"no config":   {nil, Warn, true},
		"unmentioned": {&Config{Rules: map[string]string{"HCK025": "off"}}, Warn, true},
		"off":         {&Config{Rules: map[string]string{"HCK023": "off"}}, "", false},
		"raised":      {&Config{Rules: map[string]string{"HCK023": "error"}}, Error, true},
		"lowered":     {&Config{Rules: map[string]string{"HCK023": "warn"}}, Warn, true},
	} {
		t.Run(name, func(t *testing.T) {
			got, on := tc.cfg.severity(rule)
			if got != tc.want || on != tc.on {
				t.Errorf("severity = (%q, %v), want (%q, %v)", got, on, tc.want, tc.on)
			}
		})
	}
	if s := (*Config)(nil).Summary(); s != "" {
		t.Errorf("nil Summary() = %q", s)
	}
	if d := (*Config)(nil).Disabled(); d != nil {
		t.Errorf("nil Disabled() = %v", d)
	}
}

// A rule turned off must not report, and a rule raised must report as an
// error — both through Run, since that is where the config actually lands.
func TestRunHonoursTheConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Chart.yaml"),
		[]byte("apiVersion: v1\nname: bare\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := chart.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Nothing configured: a chart this bare trips every layout rule.
	rep, err := Run(c, Options{SkipRender: true})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(rep.Findings), 4; got != want {
		t.Fatalf("got %d findings, want %d: %+v", got, want, rep.Findings)
	}

	cfg := &Config{Rules: map[string]string{"HCK011": "off", "HCK012": "error"}}
	rep, err = Run(c, Options{SkipRender: true, Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range rep.Findings {
		if f.Rule == "HCK011" {
			t.Error("HCK011 is off but reported anyway")
		}
		if f.Rule == "HCK012" && f.Severity != Error {
			t.Errorf("HCK012 severity = %q, want %q", f.Severity, Error)
		}
	}
	if got, want := len(rep.Findings), 3; got != want {
		t.Errorf("got %d findings, want %d", got, want)
	}
	if rep.Errors() != 1 {
		t.Errorf("Errors() = %d, want 1", rep.Errors())
	}
	if got := strings.Join(rep.Disabled, ","); got != "HCK011" {
		t.Errorf("Disabled = %q", got)
	}
}

// Every rule has to be listable and looked up by its own ID, or a chart cannot
// name it and "hck list rules" is lying about what runs.
func TestRuleRegistryIsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range Rules() {
		if seen[r.ID] {
			t.Errorf("%s is declared twice", r.ID)
		}
		seen[r.ID] = true
		if r.Summary == "" {
			t.Errorf("%s has no summary", r.ID)
		}
		if r.Severity != Warn && r.Severity != Error {
			t.Errorf("%s has severity %q", r.ID, r.Severity)
		}
		if got, ok := LookupRule(r.ID); !ok || got.ID != r.ID {
			t.Errorf("%s is listed but not found", r.ID)
		}
		// Exactly one check, and it is the one the scope names.
		n := 0
		for _, set := range []bool{r.chart != nil, r.object != nil, r.set != nil} {
			if set {
				n++
			}
		}
		switch r.Scope {
		case RenderScope:
			if n != 0 {
				t.Errorf("%s is render-scope but carries a check", r.ID)
			}
		case ChartScope, ObjectScope, SetScope:
			if n != 1 {
				t.Errorf("%s carries %d checks, want 1", r.ID, n)
			}
		default:
			t.Errorf("%s has scope %q", r.ID, r.Scope)
		}
	}
	if _, ok := LookupRule("HCK999"); ok {
		t.Error("unknown rule reported as found")
	}
}

func TestMustRulePanicsOnAnUnknownID(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("want a panic")
		}
	}()
	mustRule("HCK999")
}
