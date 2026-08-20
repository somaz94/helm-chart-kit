package check

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigFile is the per-chart rule configuration, read from the chart root.
const ConfigFile = ".hck.yaml"

// Config is what a chart says about the house rules:
//
//	rules:
//	  HCK025: off      # this chart wants its CPU limits
//	  HCK023: error    # and will not ship without requests
//
// A rule is either off, or reports at a severity the chart chose instead of
// the default. There is deliberately no way to add a rule here: a rule is code
// with a stable ID, and a chart-local one would report under an ID that means
// something else in the next chart.
type Config struct {
	// Rules maps a rule ID to "off", "warn" or "error".
	Rules map[string]string `yaml:"rules"`
}

// LoadConfig reads .hck.yaml from a chart directory. A chart without one is
// not an error — the defaults are the point, and most charts want them.
func LoadConfig(dir string) (*Config, error) {
	path := filepath.Join(dir, ConfigFile)
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", ConfigFile, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &cfg, nil
}

// Validate refuses a configuration that does not mean anything.
//
// A typo has to be an error rather than a silent no-op: the whole point of
// writing HCK025 down is to stop seeing it, and a misspelling that quietly
// kept reporting would be indistinguishable from the rule being right.
func (c *Config) Validate() error {
	if c == nil {
		return nil
	}
	for _, id := range slices.Sorted(maps.Keys(c.Rules)) {
		r, ok := LookupRule(id)
		if !ok {
			return fmt.Errorf("unknown rule %q (see: hck list rules)", id)
		}
		if r.Locked {
			return fmt.Errorf("%s cannot be configured — a chart that does not render has nothing else worth reporting", id)
		}
		switch setting := c.Rules[id]; setting {
		case "off", string(Warn), string(Error):
		default:
			return fmt.Errorf("%s is set to %q; it takes off, warn or error", id, setting)
		}
	}
	return nil
}

// severity resolves what a rule reports as for this chart, and whether it runs
// at all. A nil Config is a chart that said nothing, which is the default.
func (c *Config) severity(r Rule) (Severity, bool) {
	if c == nil {
		return r.Severity, true
	}
	switch c.Rules[r.ID] {
	case "off":
		return "", false
	case string(Error):
		return Error, true
	case string(Warn):
		return Warn, true
	default:
		return r.Severity, true
	}
}

// Disabled lists the rules this chart turned off, in ID order, so a report can
// say what it did not look for. A check that quietly skips a rule tells the
// reader the chart is clean when nobody asked.
func (c *Config) Disabled() []string {
	if c == nil {
		return nil
	}
	var out []string
	for id, setting := range c.Rules {
		if setting == "off" {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

// Summary describes the configuration in one line, or "" when it says nothing.
func (c *Config) Summary() string {
	if c == nil || len(c.Rules) == 0 {
		return ""
	}
	parts := make([]string, 0, len(c.Rules))
	for _, id := range slices.Sorted(maps.Keys(c.Rules)) {
		parts = append(parts, id+": "+c.Rules[id])
	}
	return strings.Join(parts, ", ")
}
