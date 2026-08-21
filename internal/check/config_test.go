package check

import (
	"os"
	"path/filepath"
	"slices"
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
		if r.Severity != Info && r.Severity != Warn && r.Severity != Error {
			t.Errorf("%s has severity %q", r.ID, r.Severity)
		}
		// A locked rule cannot be configured, so its declared severity is the
		// only one it ever reports at. An info that cannot be raised would be
		// a finding nothing can ever act on.
		if r.Locked && r.Severity == Info {
			t.Errorf("%s is locked and an info, so nothing can ever make it fail", r.ID)
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

// The wildcard is what a chart says when it wants the house rules mostly out
// of the way, and a named ID beats it in both directions — off under a warn
// wildcard, and on under an off one.
func TestWildcardRuleAppliesToEveryConfigurableRule(t *testing.T) {
	cfg := &Config{Rules: map[string]string{
		WildcardRule: "off",
		"HCK021":     "error",
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("a wildcard is a legal key: %v", err)
	}

	sev, on := cfg.severity(mustRule("HCK021"))
	if !on || sev != Error {
		t.Errorf("HCK021 = %q/%v, want error/true — a named ID beats the wildcard", sev, on)
	}
	if _, on := cfg.severity(mustRule("HCK023")); on {
		t.Error("HCK023 is not named and the wildcard is off, so it should not run")
	}
	// A locked rule is out of the wildcard's reach: a chart that does not
	// render reporting clean is not something a "*" should be able to do.
	if _, on := cfg.severity(mustRule("HCK001")); !on {
		t.Error("the wildcard reached HCK001")
	}

	disabled := cfg.Disabled()
	if slices.Contains(disabled, "HCK021") {
		t.Errorf("HCK021 is on, but the report lists it as skipped: %v", disabled)
	}
	if slices.Contains(disabled, "HCK001") {
		t.Errorf("HCK001 cannot be turned off, but the report lists it as skipped: %v", disabled)
	}
	if !slices.Contains(disabled, "HCK023") {
		t.Errorf("HCK023 was turned off by the wildcard and the report does not say so: %v", disabled)
	}
	// The whole point of the line is that a clean report still says what it
	// did not look for, so every rule the wildcard reached has to be in it.
	if want := len(Rules()) - 2; len(disabled) != want {
		t.Errorf("%d rules reported as skipped, want %d", len(disabled), want)
	}
}

// --off is the same thing said for one run. It layers over the chart's file
// rather than replacing it, and it refuses the same things the file does.
func TestTurnOff(t *testing.T) {
	base := &Config{Rules: map[string]string{"HCK023": "error"}}

	cfg, err := base.TurnOff([]string{"HCK025", "hck011"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"HCK025", "HCK011"} {
		if _, on := cfg.severity(mustRule(id)); on {
			t.Errorf("%s was turned off and still runs", id)
		}
	}
	if sev, _ := cfg.severity(mustRule("HCK023")); sev != Error {
		t.Errorf("HCK023 = %q, want the chart's own error — --off layers, it does not replace", sev)
	}
	if _, ok := base.Rules["HCK025"]; ok {
		t.Error("TurnOff edited the chart's own config in place")
	}

	if _, err := base.TurnOff([]string{"HCK999"}); err == nil {
		t.Error("a misspelled --off has to be an error, not a rule that quietly kept reporting")
	}
	if _, err := base.TurnOff([]string{"HCK001"}); err == nil {
		t.Error("--off HCK001 should be refused the same way the file refuses it")
	}

	// Nothing to turn off leaves the caller's config exactly as it was, nil
	// included: a chart that said nothing still means the defaults.
	var absent *Config
	if got, err := absent.TurnOff(nil); err != nil || got != nil {
		t.Errorf("got %v, %v; want the nil config back untouched", got, err)
	}
}

// info is a severity a chart can name, in both directions. Raising HCK040 is
// how a team that owns its cluster says the storage class is their job and
// they want --strict to stop them; lowering a rule to info is how a chart
// keeps seeing a finding it disagrees with without being failed by it, which
// is the half "off" cannot express.
func TestInfoIsASeverityAChartCanSet(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  *Config
		rule string
		want Severity
	}{
		{"the default", nil, "HCK040", Info},
		{"raised to warn", &Config{Rules: map[string]string{"HCK040": "warn"}}, "HCK040", Warn},
		{"raised to error", &Config{Rules: map[string]string{"HCK040": "error"}}, "HCK040", Error},
		{"a warning lowered to info", &Config{Rules: map[string]string{"HCK023": "info"}}, "HCK023", Info},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, ok := LookupRule(tc.rule)
			if !ok {
				t.Fatalf("no %s", tc.rule)
			}
			sev, on := tc.cfg.severity(r)
			if !on || sev != tc.want {
				t.Errorf("severity = %q/%v, want %q/true", sev, on, tc.want)
			}
		})
	}
}

// A severity the config does not know has to be refused rather than silently
// falling through to the rule's default, and the error has to list what is
// actually accepted — "it takes off, warn or error" sent a reader looking for
// a setting that exists.
func TestValidateAcceptsInfoAndNamesItWhenRefusing(t *testing.T) {
	if err := (&Config{Rules: map[string]string{"HCK040": "info"}}).Validate(); err != nil {
		t.Errorf("info refused: %v", err)
	}
	err := (&Config{Rules: map[string]string{"HCK040": "note"}}).Validate()
	if err == nil {
		t.Fatal("want an error for an unknown severity")
	}
	if !strings.Contains(err.Error(), "info") {
		t.Errorf("the error does not offer info: %v", err)
	}
}
