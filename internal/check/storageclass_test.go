package check

import (
	"slices"
	"strings"
	"testing"

	"github.com/somaz94/helm-chart-kit/internal/catalog"
	"github.com/somaz94/helm-chart-kit/internal/render"
	"gopkg.in/yaml.v3"
)

// TestEveryOverlayStorageClassIsClassified is what keeps HCK040 honest as the
// overlays grow.
//
// The rule reports a class name only when it is in one of the two lists, so a
// new platform overlay naming a class nobody classified is not a false
// negative that shows up later — it is silence, and silence from a rule reads
// exactly like the chart being fine. This fails instead, at the moment the
// overlay is written, and forces the one call it needs: does this platform
// create the class, or does somebody have to?
func TestEveryOverlayStorageClassIsClassified(t *testing.T) {
	suffixes, err := render.OverlaySuffixes()
	if err != nil {
		t.Fatal(err)
	}
	data := render.Data{ChartName: "demo", Version: "0.1.0", AppVersion: "1.0.0"}

	found := 0
	for _, r := range catalog.Resources() {
		for _, suffix := range suffixes {
			if !render.HasOverlayValues(r.Name, suffix) {
				continue
			}
			raw, _, err := render.ResourceOverlayValues(r.Name, suffix, data)
			if err != nil {
				t.Fatalf("%s/%s: %v", r.Name, suffix, err)
			}
			class := storageClassIn(t, raw)
			if class == "" {
				continue
			}
			found++
			_, provisioned := storageClassNeedsProvisioning[class]
			ships := slices.Contains(storageClassShipsWithThePlatform, class)
			switch {
			case provisioned && ships:
				t.Errorf("%s/%s: %q is in both lists", r.Name, suffix, class)
			case !provisioned && !ships:
				t.Errorf("%s/%s names storage class %q, which neither list classifies — "+
					"add it to storageClassShipsWithThePlatform if %s creates it, "+
					"or to storageClassNeedsProvisioning with what has to happen first",
					r.Name, suffix, class, suffix)
			}
		}
	}
	// A walk that matched nothing would pass every assertion above while
	// proving nothing, which is how this test would rot if the overlays moved.
	if found == 0 {
		t.Fatal("no overlay set a storage class; this test is no longer reading them")
	}
}

// storageClassIn digs persistence.storageClass out of a rendered overlay. The
// key is read from the YAML rather than grepped for, so a class name that
// appears in a comment is not mistaken for one the overlay sets.
func storageClassIn(t *testing.T, raw []byte) string {
	t.Helper()
	var doc struct {
		Persistence struct {
			StorageClass string `yaml:"storageClass"`
		} `yaml:"persistence"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse overlay: %v", err)
	}
	return strings.TrimSpace(doc.Persistence.StorageClass)
}
