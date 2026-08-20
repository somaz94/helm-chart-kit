# CLAUDE.md — helm-chart-kit

`hck` scaffolds Helm charts and adds resources to charts that already exist.

<br/>

## Build & Test

```bash
make build           # Build binary → ./bin/hck
make test            # go test ./... -v -race -cover
make cover           # Coverage report
make fmt             # go fmt
make vet             # go vet
golangci-lint run    # Lint (config in .golangci.yml)
```

`helm` must be on `PATH` for `hck check` and the tests that exercise it. Those tests skip when it is absent, so a green run without helm does not mean the render path was covered.

<br/>

## Project Structure

```
cmd/cli/            Cobra commands, one constructor each
internal/catalog/   Resources and presets — data only
internal/render/    Embedded templates + renderer
  templates/chart/      Chart skeleton
  templates/resources/  One directory per resource
internal/values/    Append-only values.yaml merge
internal/schema/    values.schema.json assembly from resource fragments
internal/docs/      values.yaml -> Markdown table
internal/chart/     Chart directory location and inspection
internal/scaffold/  Plan construction and application
internal/check/     helm render + house rules
```

<br/>

## Invariants

These are load-bearing. Breaking one is a defect, not a style choice.

**`values.yaml` is never rewritten.** `internal/values` appends text and nothing else. Do not replace it with a `yaml.Node` round-trip: that preserves keys and comments but eats blank lines and section banners, so every `hck add` would silently reformat the user's file.

**Removal deletes templates and nothing else.** `scaffold.PlanRemove` emits `Delete` entries and a `ValuesOrphaned` list; it never touches `values.yaml` or `values.schema.json`. Two removals are refused without `--force`, and both guard something invisible: one another present resource `Requires` (the chart renders and does not work, and nothing says so until helm runs), and one whose file is `scaffold.Edited` (a template that differs from what hck generates is somebody's work, and a mistyped name should not delete it). A key another present resource also declares is not orphaned — `persistence` belongs to the StatefulSet as much as to the PVC.

**`hck sync` cannot tell a local edit from an hck template that moved on.** Both are simply not the bytes `render.ResourceTemplate` produces now, and `scaffold.Drift` reports exactly that much. This is why the default is a report, why `--write` takes resource names, and why `--write` with neither names nor `--all` is an error rather than a guess. `Unreadable` is a third state on purpose: reporting an unreadable file as edited would invite `--write` to overwrite it.

**Templates use `[[ ]]`, Helm uses `{{ }}`.** The generation layer's delimiters are set in `internal/render`. A template rendering with a `[[` still in it is caught by `TestEveryCatalogResourceRenders`.

**`//go:embed all:templates`, not `//go:embed templates`.** Without `all:`, embed silently drops every path segment starting with `_` or `.` — which is exactly `templates/chart/templates/_helpers.tpl`.

**Cross-resource values access is parenthesised.** `(.Values.autoscaling).enabled`, never `.Values.autoscaling.enabled`, because the HPA may not be in the chart. Sprig's `dig` fails here: `.Values` is a `chartutil.Values`, not a `map[string]interface{}`.

**One workload per chart.** Enforced in `scaffold.checkSingleWorkload`, which counts the finished chart rather than what is arriving — two workloads in one `hck add`, or a `--with` on top of a preset's own, are the same defect as one landing next to one already there. `PlanNew` and `PlanAdd` both call it; `hck check` reports an existing chart as `HCK030`. Two workloads contend for `image`, `resources` and `updateStrategy` with incompatible shapes, and once the chart has a `values.schema.json` they are worse than that: the schema resolves the contested key by canonical order and `values.yaml` resolves it by merge order, so the two pick different owners and helm rejects values the workload in the chart accepts.

**`image.tag` has no `appVersion` fallback.** `_helpers.tpl` calls `fail`. This is why the chart skeleton ships `ci/install-values.yaml` and why `hck check` picks it up by default.

**The catalog and the template tree are cross-checked both ways.** `internal/catalog` walks catalog → templates; `internal/render` walks templates → catalog. Adding a resource to only one fails one of the pair.

**A resource's values keys are declared in three places and must agree.** `catalog.ValuesKeys`, the top-level keys of `values.yaml.tmpl`, and the top-level keys of `schema.json.tmpl` — same list, same order, enforced by `TestValuesKeysMatchTheTemplates`. This is not bookkeeping: Helm validates values against `values.schema.json` on every render, so a key in `values.yaml` that the schema does not describe stops the chart installing.

**The generated schema is permissive on purpose.** Objects stay open; a scalar whose default is empty is typed as the union it actually accepts (`service.nodePort` takes string or integer). An incomplete schema is worse than none — it rejects values the templates handle fine. `--strict` closes the top level only, never a nested object, and `global` stays allowed so subcharts work.

**Overlays are one mechanism on two axes.** One `catalog.Overlay` type carries both: `catalog.PlatformAxis` (where) and `catalog.EnvironmentAxis` (how hard). Both produce `values-<name>.yaml` and both read `templates/resources/<name>/values-<suffix>.yaml.tmpl` through `scaffold.buildOverlay`. They share one file-name space, so `TestPlatformAndEnvironmentNamesDoNotCollide` guards it. Environment is passed to helm after platform: `-f` applies left to right and the size has to win.

**The two overlay axes must not both claim a key.** A platform overlay says how something is wired — an annotation, a class, a store reference. An environment overlay decides what is on and how big. Both become `-f` arguments, so a key both set is resolved by argument order rather than intent: "aws says no NetworkPolicy, prod says yes" rendered differently depending on which came last. `TestPlatformOverlaysDoNotToggle` forbids `*.enabled` in a platform overlay, and `TestOverlayOrderDoesNotChangeTheRender` renders all 12 pairs both ways. Where a platform genuinely cannot support something, that belongs in `Overlay.Needs`.

**Every optional resource defaults to off, so a chart that carries them renders none of them.** A `hck check` over such a chart reports "no findings" while proving nothing — 25 preset×resource combinations passed that way. `TestEveryResourceRendersWhenEnabled` turns them all on from `cmd/cli/testdata/enable-all.yaml` and asserts each one appears in the output. Its dashboard body deliberately contains Grafana's own `{{pod}}` legend syntax, which is what broke the first `grafanadashboard` template.

**An overlay must change the render, not merely be accepted.** The first version passed `OverlayFiles` into `check.Options` and never appended them to the helm command line. Every check still passed — a check that renders the base chart renders it fine — and the CI step asserting "20 combinations ok" was green the whole time. `TestRunAppliesOverlayFiles` and the CI `grep` for a value only the overlay supplies are what make that failure visible.

**A platform overlay is additive, never a replacement.** `internal/catalog/overlay.go` declares both axes; `templates/resources/<name>/values-<platform>.yaml.tmpl` carries only what differs there. Helm reads `values.yaml` first and always, so an overlay that repeats a base value says nothing — `TestPlatformOverlaysDifferFromTheBase` fails on it. `check.Options.OverlayFiles` exists for the same reason: passing an overlay as a plain `-f` would suppress the `ci/install-values.yaml` fallback and the chart would stop rendering for want of an image tag.

**An overlay may only set keys its resource owns.** `TestPlatformOverlayKeysBelongToTheResource` compares against `catalog.ValuesKeys`. Without it an overlay can set a key no template reads — dead configuration that looks live. This is what caught `topologySpreadConstraints` being absent from the StatefulSet.

**The values table is delimited, not owned.** `hck docs --write` replaces only what sits between `<!-- hck:values:start -->` and `<!-- hck:values:end -->`. Everything else in the README belongs to whoever wrote it, and `TestReplaceKeepsEverythingOutsideTheMarkers` plus a CI step both pin that.

**A `-- ` prefix is what makes a comment a description.** `internal/docs` reads `values.yaml` through `yaml.Node` head comments, but only a line opening `-- ` starts one. Without that rule the section banners — the `# ====` blocks — would each be attributed to whatever key happened to follow them.

**A check rule is a registry entry, and its ID is permanent.** `internal/check/rules.go` holds one entry per `HCK0xx`; `hck check`, `hck list rules` and a chart's `.hck.yaml` all read it. A rule returns messages, never `Finding`s — the runner attaches the ID and severity from the rule's own declaration, so a rule cannot report under somebody else's ID. That is the whole basis for a chart naming one: reusing a retired ID silently turns a rule back on in a chart that turned the old one off. `TestRuleRegistryIsWellFormed` checks each declaration carries exactly the one check its `Scope` names.

**A rule a chart turned off is still reported as turned off.** `check.Report.Disabled` is printed with the findings and carried in `--format json`. A clean report over a chart with half the rules off says less than it looks like it does, and the difference between "nothing is wrong" and "nobody asked" has to survive into CI. An unknown rule ID in `.hck.yaml` is an error rather than a no-op for the same reason, and `HCK001` cannot be configured at all.

**`values.schema.json` is opt-in and generated, never hand-edited.** `hck new` writes one only under `--schema`; `hck add` regenerates one that exists and never introduces one that does not. Unlike `values.yaml`, it is rebuilt whole — it is an artifact, not a document someone maintains.

<br/>

## Workflow After Code Changes

1. **Tests first** — add or update tests, run `make test`. Coverage target is 90%+ excluding `cmd/main.go`.
2. **Lint** — `golangci-lint run` must be clean.
3. **End to end** — `hck new` every preset and `hck check` each one; a chart hck generates must pass hck's own check with no findings. `TestCheckRendersTheGeneratedChart` does this, and skips without helm. `TestHelmAcceptsTheGeneratedSchema` does the same for `--schema` and `--schema-strict`: only helm can prove the generated schema does not reject a chart it should accept.
4. **Docs** — update `README.md` and `docs/` when the command surface or the resource catalog changes.

<br/>

## Conventions

- Commits: Conventional Commits, single line, English, no `Co-Authored-By`
- Code comments: English
- Documentation: English is the source; every doc has a `-ko.md` Korean pair. `<br/>` between heading sections
- **Editing one half of a doc pair means editing the other in the same change.** The pairs are `README.md` ↔ `README-ko.md` and `docs/{USAGE,RESOURCES,DEVELOPMENT}.md` ↔ their `-ko` siblings. Headings, code blocks, table rows and factual values (resource counts, HCK rule IDs, flag names and defaults) must match exactly — only prose differs. This overrides the global PrivateWork English-only rule, at the repo owner's request
- Command examples, table cells holding identifiers, and `-- ` comments quoted from generated files stay in English in the Korean half too: they depict what the tool actually writes, and a reader who follows the Korean doc has to find the same text in the file
- Do not edit `.goreleaser.yml`, `.github/workflows/release.yml` or `CHANGELOG.md` without asking — they are the release pipeline
