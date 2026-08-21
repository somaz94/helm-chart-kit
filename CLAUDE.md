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

**`hck sync` compares the chart skeleton too, except the two files the author owns.** `scaffold.skeletonDrift` walks `render.ChartFiles()` and compares everything not listed in `skeletonNotOwned` — so a file added to `templates/chart/` is picked up by default, which is the right default and the dangerous one. `Chart.yaml` grows dependencies and maintainers; `values.yaml` is append-only. Comparing either would report drift on every chart that ever grew, and `--write` would delete what hck never wrote. `TestTheSkeletonSetIsADecision` pins the set so adding a skeleton file forces that call, and `TestTheAuthorsFilesAreNotCompared` holds the two exclusions. This gap was real: `.helmignore` gained a line and `_helpers.tpl` is what every template calls into, and neither was ever compared.

**Templates use `[[ ]]`, Helm uses `{{ }}`.** The generation layer's delimiters are set in `internal/render`. A template rendering with a `[[` still in it is caught by `TestEveryCatalogResourceRenders`.

**`//go:embed all:templates`, not `//go:embed templates`.** Without `all:`, embed silently drops every path segment starting with `_` or `.` — which is exactly `templates/chart/templates/_helpers.tpl`.

**Cross-resource values access is parenthesised.** `(.Values.autoscaling).enabled`, never `.Values.autoscaling.enabled`, because the HPA may not be in the chart. Sprig's `dig` fails here: `.Values` is a `chartutil.Values`, not a `map[string]interface{}`.

**A chart carries one workload; it may hold two, and only the render can tell them apart.** `HCK030` counts what helm produced — `Scope: SetScope`, "chart renders more than one primary workload" — and that is the right place to count. `scaffold.noteTwoWorkloads` counts template names over the finished chart and only says so: a `Deployment` beside an Argo `Rollout`, each under its own `.enabled` and pulling one pod template from a shared helper, is a chart that carries two and renders one, and refusing it from the resource names answered a question the names cannot answer. That refusal existed, behind `--force`, and it blocked the ordinary shape along with the accidental one. The values-contention argument it rested on holds for `StatefulSet` beside `Deployment`, where `volumeClaimTemplates` exists on one side only; a delivery-swap pair shares its values keys deliberately, which is the whole design. `--force` no longer touches this on either command — it is the non-empty-directory hatch and nothing else. `TestPlanNewNotesTwoWorkloadsRatherThanRefusing` holds the note, and the CI step guards one workload and asserts `HCK030` goes quiet, which is the claim the whole change rests on.

**A preset answers more than its resource list.** `catalog.Preset` carries `Platform`, `Environment`, `Schema` and `Docs` so that `hck init` can put three questions instead of seven — a preset written for one platform knows which one. They are defaults and not decisions: init prints what the preset resolved to, `Keep those?` takes no for an answer and reopens the four seeded with the preset's own values, and every `hck new` flag still overrides. Every preset but `base` and `base-aws` carries none of them, so the defaults are unchanged. `TestPresetOverlayNamesExist` is what stops a typo here: the name goes straight into `catalog.LookupOverlay`, which fails at run time, and unlike a flag nobody typed it.

**`image.tag` has no `appVersion` fallback.** `_helpers.tpl` calls `fail`. This is why the chart skeleton ships `ci/install-values.yaml` and why `hck check` picks it up by default.

**A pair the chart cannot wire for you is reported, not wired.** `issuer`/`certificate` and `secretstore`/`externalsecret` are the same shape, and `HCK034` and `HCK039` say the same thing about each: the chart made one and its own consumer names something else. Both refs default to a cluster-scoped object — a `ClusterIssuer`, a `ClusterSecretStore` — which is where a shared one lives and is a perfectly good answer, so repointing the ref because the namespaced object appeared would change what an existing chart reads from. That is the line the GKE annotations fall on the other side of: `cloud.google.com/backend-config` has no meaningful default, an unset annotation means no BackendConfig at all, so setting it when the resource is on is unambiguous and the template does it. Where the default means something, report; where it means nothing, wire.

**A finding hck's own output can produce is an info, or the generator is wrong.** `check.Info` is the third severity and the only one `--strict` ignores. It exists because `hck new --platform aws` writes `persistence.storageClass: gp3` — the right class to ask for on EKS, and one EKS does not create — so the chart is correct, the overlay is correct, and the claim binds to nothing until somebody makes the class. A `StorageClass` is cluster-scoped, so hck cannot close the gap by adding the resource: two releases would create the same object and the second `helm install` fails, which is what kept `ClusterSecretStore` and `ClusterPodMonitoring` out of the catalog. `HCK040` reports it instead. As a warning it would mean a chart hck itself generated fails hck's own `--strict`, which contradicts "a chart hck generates must pass hck's own check"; silent, it would be found from a pod that never schedules. `Report.count` is why every tally is of its own severity — `Warns()` was `len(Findings) - Errors()`, the same number until a third severity existed and then quietly the wrong one. `TestTheStorageClassRuleIsANoteRatherThanAComplaint` holds the severity, and the CI step raises `HCK040: warn` to prove a note is still actionable: an info nothing can act on would be decoration.

**A rule reports on a class name only if it recognises it.** `storageClassShipsWithThePlatform` and `storageClassNeedsProvisioning` between them hold every name hck's own overlays write, and `TestEveryOverlayStorageClassIsClassified` fails on a new overlay naming a class in neither. A name outside both is the user's own, and hck says nothing: it knows nothing about their cluster, and a note about a class it never suggested is a guess — the same quiet-when-unsure line `HCK035` draws over a chart that renders no pod. Two of the four fire and two do not, which is what makes it a rule rather than a banner on every chart that asks for storage.

**Platform is a second axis on a resource, not a kind of group.** A GKE `ManagedCertificate` is a secrets resource that happens to be GKE-only, and `catalog.Resource` carries both — one field would have to give one of those up, the same mistake `catalog.Overlay` avoids by naming its two axes. `Resource.Platform` holds a platform overlay name so the two cannot drift, and `TestResourcePlatformsAreRealPlatforms` is what stops a typo making a resource specific to nothing: never suggested, never filtered, silently unreachable. This is the axis an overlay cannot cover — `values-gcp.yaml` changes what an annotation says and cannot conjure the kind it names, which is why the gcp Ingress overlay carried `networking.gke.io/managed-certificates` commented out with "provision it separately" for as long as it did. Three of the four GKE resources are reached by name from an annotation, so the Ingress and Service templates set it themselves under `(.Values.x).enabled` — a comment promising the chart does that would otherwise be the only thing doing it. `HCK038` reports the annotation naming what the chart does not render, and `HCK037` the chart carrying both halves of a pair. **A group never expands to a platform-only resource**, because `hck add @secrets` on an EKS chart must not put a GKE CRD in it; `groupSkipNote` reports what was left out, since a group silently expanding to less than its listing reads as the group being smaller than it is. Nothing is ever refused over this: hck cannot know what cluster the chart will be installed on.

**Every resource has a group, and the listing iterates groups rather than resources.** `catalog.Group` exists because the catalog is 32 Kubernetes kinds: alphabetical answers "what exists" and leaves "which of these do I want" to somebody who already knows. `hck list resources` walks `Groups()` in build order — what runs, what reaches it, how many, who may connect, secrets, observability, mesh, the chart's own pieces — so a resource whose `Group` is empty or unknown does not appear at all, which is the one failure that reports itself as nothing. `TestEveryResourceHasAKnownGroup` catches the resource pointing at nothing and `TestGroupsCoverEveryResourceExactlyOnce` catches the group quietly dropping one; adding a resource without a group fails both. The `@` in `@observability` is a namespace boundary, not decoration: `scaffold.resolve` strips it and looks the name up among groups, so a group and a resource may share a name without lookup order deciding the winner. A group beside one of its own members resolves once — `resolve` dedups across the expansion, and without that the plan would carry a template twice and the values merge would be asked for the same keys twice.

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

**A hazard written in a comment is not enforced by anything.** `templates/resources/pdb/values.yaml.tmpl` warns that a budget over one replica blocks every node drain, and the dev overlay turns the budget off for the same reason — both correct, both inert. `HCK036` is that knowledge made to run: a `maxUnavailable` of zero, a `minAvailable` at or above the replica count, or `minAvailable: "100%"` allows no eviction ever, so a drain retries until somebody cancels it and a cluster upgrade stops on the pod. The remedy in the message differs by cause; telling someone to use `maxUnavailable` when `maxUnavailable` is the problem is worse than saying nothing. The replica count comes from the one workload that declares one — a Deployment under an HPA and a DaemonSet declare none, and both are left alone.

**A reference by name to something the chart does not provide is the quietest way a chart can be wrong, and each one is a rule.** Everything applies, nothing errors, and the symptom is a controller writing "I cannot find that" into a status nobody reads. `HCK033` (a scaler's target), `HCK034` (a Certificate's issuer, when the chart made an Issuer and pointed past it) and `HCK035` (a Service's named `targetPort`) are the three hck's own output could produce. Each names what the chart *does* render, so the mismatch is the message rather than something to go and look up. `HCK035` collects port names across every pod spec instead of matching the Service selector, and says nothing at all when the chart renders no pod: quiet-when-unsure is the right way for a warning to be wrong.

**A scaler points at the chart's own workload, and `hck check` says so when it does not.** `render.Data.WorkloadKind` is resolved from the finished chart — `PlanAdd` unions what is arriving with what is already there, because a scaler added beside a StatefulSet has to name the StatefulSet and the arriving list does not mention it. `workloadKindByName` is hand-written and cross-checked against what each workload template actually emits by `TestWorkloadKindsMatchTheTemplates`. Where the scaler cannot target the chart's kind at all — an HPA over a DaemonSet — the default stays inside the schema's enum and `HCK033` reports that the target is not there. This was a live defect: `hck add hpa` against a stateful chart shipped an HPA aimed at `Deployment/x` while the chart rendered `StatefulSet/x`, and nothing said so.

**A check rule is a registry entry, and its ID is permanent.** `internal/check/rules.go` holds one entry per `HCK0xx`; `hck check`, `hck list rules` and a chart's `.hck.yaml` all read it. A rule returns messages, never `Finding`s — the runner attaches the ID and severity from the rule's own declaration, so a rule cannot report under somebody else's ID. That is the whole basis for a chart naming one: reusing a retired ID silently turns a rule back on in a chart that turned the old one off. `TestRuleRegistryIsWellFormed` checks each declaration carries exactly the one check its `Scope` names.

**A rule a chart turned off is still reported as turned off.** `check.Report.Disabled` is printed with the findings and carried in `--format json`. A clean report over a chart with half the rules off says less than it looks like it does, and the difference between "nothing is wrong" and "nobody asked" has to survive into CI. An unknown rule ID in `.hck.yaml` is an error rather than a no-op for the same reason, and `HCK001` cannot be configured at all.

**The two ways to turn a rule off resolve through one path, so neither can skip that report.** `check.WildcardRule` (`"*"` in `.hck.yaml`) and `hck check --off` both end up in `Config.Rules`, and `Config.Disabled` resolves rule by rule through `severity` rather than reading the map — with a wildcard in play the map says `"*"` and the reader needs the IDs, and an explicit severity beside a wildcard `off` means that one is still on. `Config.TurnOff` copies rather than mutating: a `--off` that edited the chart's own config in place would be indistinguishable, one frame later, from the chart having asked for it. Neither reaches a locked rule.

**`hck new --force` into a directory fills it in and writes over nothing.** `scaffold.keepWhatIsThere` turns every `Create` over a file already on disk into a `Skip`, and clears `ValuesAdded`/`ValuesSkipped` when `values.yaml` is one of them — the merge ran, and a plan that still named the keys would be describing a file it is not writing. Overwriting was the obvious reading of `--force` and it is the wrong one: it takes `values.yaml` with it, and `values.yaml` is never rewritten. That makes the flag a way to recover a chart that lost a template, not a way to extend one — `hck add` appends, `hck new --force` does not. `TestPlanNewForceKeepsWhatIsAlreadyThere` applies the plan and compares the bytes.

**Every preset renders clean with its own resources switched on, not just on its defaults.** Every optional resource defaults to off, so `TestCheckRendersTheGeneratedChart` passes a `mesh` or `queue` preset without rendering the VirtualService or the ScaledObject that is the reason to pick it. `TestEveryPresetRendersWithItsResourcesOn` filters `cmd/cli/testdata/enable-all.yaml` down to the keys that preset's own `values.yaml` declares and renders that. The filtering is the load-bearing part: setting a key a chart never declared is a user typo, not a preset, and one of them — `configMap.enabled` on a chart with no ConfigMap — fails the render through the Deployment's checksum `include`.

**A preset is shaped as much by what it leaves out.** `mesh` carries no NetworkPolicy: who may call this workload is the AuthorizationPolicy's answer at L7 and with an identity, and the same question answered again at L3 is two answers. `queue` carries no HPA, because the ScaledObject owns the replica count and `HCK031` reports the pair. `secure` carries a Certificate and no Issuer, because the Certificate defaults to a ClusterIssuer and `HCK034` reports the unwired pair. Each omission is a rule the check would otherwise fire on, which is why they are comments in `internal/catalog/catalog.go` rather than something to rediscover.

**A CI step asserts on `--format json` or on the exit status, never on the text report.** `hck check`'s human-readable output is written for people and its wording is not a contract — renaming one label, `off in .hck.yaml:` to `not checked:`, turned `ci.yml` red with nothing about the behaviour having changed, and `ci.yml` is under `paths-ignore`, so the break did not surface until the next code commit. The field names are the interface — `jsonReport` for `check`, `jsonSync` for `sync`, `jsonListing` for `list` — and the workflow reads them through `jq`. `hck list` and `hck sync` grew a `--format json` because the workflow had to parse their tables otherwise, and one of those parsers duly broke: grouping the catalog moved the listing's indentation from two spaces to four and took three `awk` patterns with it. One exception remains, deliberately: helm's own rendered output (`--print`), which are the chart's strings rather than hck's. The "The JSON contract" step pins the field names, so removing one fails there rather than in whatever reads it next.

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
