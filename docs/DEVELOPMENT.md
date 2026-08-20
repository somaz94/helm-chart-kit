# Development

Guide for building, testing, and contributing to this project.

> 한국어: [DEVELOPMENT-ko.md](DEVELOPMENT-ko.md) · Index: [README.md](../README.md)

<br/>

## Prerequisites

- Go 1.26+
- Make
- `helm` on `PATH` — only for `hck check` and the tests that exercise it; those skip when it is absent

<br/>

## Architecture

```
cmd/cli/            Cobra commands. Each is a constructor, not a package-level
                    variable: cobra binds flags to addresses, so a shared tree
                    would carry flag values from one run into the next.
internal/catalog    What can be generated: resources and presets. Data only.
internal/render     The embedded template set and the renderer.
internal/values     The append-only values.yaml merge.
internal/schema     Assembles values.schema.json from the resource fragments.
internal/docs       values.yaml → Markdown table.
internal/chart      Locating and inspecting a chart directory.
internal/scaffold   Turns a request into a Plan; Apply writes it.
internal/check      Renders via helm, then applies the house rules. The
                    rules are a registry, one entry per HCK id, so a chart can
                    name one in its .hck.yaml.
```

Two decisions carry most of the design:

**Plan and Apply are separate.** `--dry-run` shows exactly what a real run would do rather than an approximation, and the interesting decisions stay testable without touching a disk.

**The values merge is textual, not a YAML round-trip.** Decoding and re-encoding preserves keys and comments but loses blank lines and section banners — so a tool that "just adds a key" would silently reformat a 400-line file. Instead the original bytes are never touched: absent keys are appended, present keys are reported and left alone.

<br/>

## Adding a resource

1. Create `internal/render/templates/resources/<name>/` with `template.yaml.tmpl`, `values.yaml.tmpl` and `schema.json.tmpl`.
2. Add the entry to `resources` in `internal/catalog/catalog.go`, including its `ValuesKeys`.

The generation layer uses `[[ ]]` delimiters, so Helm's own `{{ }}` passes through untouched and the templates stay readable as the Helm files they will become.

`schema.json.tmpl` is a JSON object mapping each top-level values key the resource contributes to the schema for it — the envelope (`$schema`, `title`, `type`, `properties`) is added by `internal/schema`, so the fragment carries only the keys.

Three tests cross-check the catalog and the template tree, so declaring a resource or a key in one place and not the others fails:

- `internal/catalog`: every catalog entry has all three template files
- `internal/render`: every template directory renders, with no delimiter left unsubstituted, and every schema fragment parses as JSON
- `internal/catalog`: `ValuesKeys`, the keys `values.yaml.tmpl` declares, and the keys `schema.json.tmpl` declares are the same list in the same order

That last one is the load-bearing one for the schema. Helm validates values against `values.schema.json` on every render, so a key present in `values.yaml` but missing from the schema does not go unnoticed — it stops the chart installing.

A template that reads another resource's values must use the parenthesised form — `(.Values.autoscaling).enabled`, not `.Values.autoscaling.enabled` — because the other resource may not be in the chart at all. Sprig's `dig` does not work here: `.Values` is a `chartutil.Values`, not a `map[string]interface{}`.

<br/>

## Adding a check rule

Rules live in `internal/check/rules.go`, one entry per `HCK0xx`, and nothing else needs touching — `hck check`, `hck list rules` and a chart's `.hck.yaml` all read the same registry.

```go
{
	ID: "HCK033", Severity: Warn, Scope: ObjectScope,
	Summary: "container mounts the service account token it never uses",
	object: containerRule(func(c map[string]any, kind string) string {
		if ... {
			return "the message the user reads"
		}
		return ""
	}),
},
```

`Scope` decides when the rule runs and what it is handed:

| Scope | Reads | Runs |
|---|---|---|
| `ChartScope` | the chart directory | always, `--no-render` included |
| `RenderScope` | helm's own output | reported by `Run`, no closure |
| `ObjectScope` | one rendered object | once per object |
| `SetScope` | every object together | once, for a combination only wrong as a combination |

A rule returns messages, never findings: the ID and severity are attached by the runner from the rule's own declaration, so a rule cannot report under somebody else's ID. That is what makes an ID something a chart can name and a user can trust.

`podRule` and `containerRule` lift a check over a pod spec or a container into an object-scope rule, so the rule says what is wrong and nothing about how the pod was reached.

Two things are fixed once a rule ships:

- **The ID never changes.** It is what a chart's `.hck.yaml` refers to, and reusing a retired one silently turns a rule back on somewhere.
- **The default severity is the rule's own judgement**, not a chart's. A chart that disagrees says so in its `.hck.yaml`; `TestRuleRegistryIsWellFormed` checks the declaration is coherent.

<br/>

## The overlay axes

Platform (`aws`/`gcp`/`azure`/`onprem`) and environment (`dev`/`staging`/`prod`) share one mechanism: fragments live at `templates/resources/<name>/values-<suffix>.yaml.tmpl` and `scaffold.buildOverlay` assembles them.

**The two axes must not both claim a key.** A platform says how something is wired — an annotation, a class, a store reference. An environment says what is on and how big. Both become `-f` arguments, so a key both set is resolved by argument order rather than by intent. Two tests enforce it:

- `TestPlatformOverlaysDoNotToggle` — no `*.enabled` in a platform overlay
- `TestOverlayOrderDoesNotChangeTheRender` — all 12 pairs rendered both ways

Where a platform genuinely cannot support something, that belongs in `Overlay.Needs`.

<br/>

## Build

```bash
make build           # Build binary → ./bin/hck
make clean           # Remove build artifacts
make install         # Install to /usr/local/bin
```

<br/>

## Testing

```bash
make test            # Run unit tests (alias)
make test-unit       # go test ./... -v -race -cover
make cover           # Generate coverage report
make cover-html      # Open coverage report in browser
```

<br/>

## Code Quality

```bash
make fmt             # Format code (go fmt)
make vet             # Run go vet
```

<br/>

## Workflow

```bash
make check-gh        # Verify gh CLI is installed and authenticated
make branch name=feature-name   # Create feature branch from main
make pr title="feat: add feature"   # Test → push → create PR (auto-generates body)
```

`make pr` automatically:
1. Runs `go test ./... -race -cover` and `go vet`
2. Pushes the branch to origin
3. Generates PR body from commit history (categorized by `feat:`, `fix:`, `test:`, `docs:`)
4. Detects changed test packages and builds a test plan checklist
5. Creates the PR via `gh pr create`

<br/>

## CI/CD Workflows

| Workflow | Trigger | Description |
|----------|---------|-------------|
| `ci.yml` | push (main), PR, dispatch | Unit tests → Build → Version verify |
| `release.yml` | tag push `v*` | GoReleaser (binaries + Homebrew + Scoop) |
| `changelog-generator.yml` | after release, PR merge | Auto-generate CHANGELOG.md |
| `contributors.yml` | after changelog | Auto-generate CONTRIBUTORS.md |
| `stale-issues.yml` | daily cron | Auto-close stale issues |
| `dependabot-auto-merge.yml` | PR (dependabot) | Auto-merge minor/patch updates |
| `issue-greeting.yml` | issue opened | Welcome message |

<br/>

### Workflow Chain

```
tag push v* → Create release (GoReleaser)
                └→ Generate changelog
                      └→ Generate Contributors
```

<br/>

## Conventions

- **Commits**: Conventional Commits (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `ci:`, `chore:`)
- **Secrets**: `PAT_TOKEN` (cross-repo ops), `GITHUB_TOKEN` (releases)
- **paths-ignore**: `.github/workflows/**`, `**/*.md`
- **Docs**: English is the source, `-ko.md` is the translation. Editing one means editing its pair in the same change
