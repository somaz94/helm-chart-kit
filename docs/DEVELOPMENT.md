# Development

Guide for building, testing, and contributing to this project.

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
internal/chart      Locating and inspecting a chart directory.
internal/scaffold   Turns a request into a Plan; Apply writes it.
internal/check      Renders via helm, then applies the house rules.
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
