# Usage

<br/>

## hck new

Create a chart directory seeded with a preset's resources.

```bash
hck new <chart-name> [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `-p, --preset` | `web` | Resource set to seed with |
| `-d, --dir` | `.` | Parent directory to create the chart in |
| `--with` | — | Extra resources on top of the preset, comma-separated or repeated |
| `--description` | `A Helm chart for <name>` | `Chart.yaml` description |
| `--version` | `0.1.0` | Chart version |
| `--app-version` | `1.0.0` | Version of the application the chart deploys |
| `--dry-run` | `false` | Print what would be written and exit |

```bash
hck new payments-api
hck new billing-worker --preset worker
hck new sessions --preset stateful --with servicemonitor
hck new gateway --preset web --with httproute,prometheusrule --dry-run
```

The chart name must be a lowercase DNS label — Helm's own constraint — and the target directory must be empty or absent.

<br/>

## hck add

Add resource templates to a chart that already exists.

```bash
hck add <resource>... [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--chart` | `.` | Chart directory; parent directories are searched for `Chart.yaml` |
| `--dry-run` | `false` | Print what would be written and exit |
| `--force` | `false` | Overwrite existing templates and allow a second workload |

```bash
hck add servicemonitor
hck add pdb networkpolicy
hck add externalsecret --chart ./charts/payments-api
```

Like `git`, it walks up: run it from anywhere inside a chart.

<br/>

### What add guarantees

**Nothing is overwritten.** A template file that already exists is reported and skipped. A `values.yaml` key that already exists is reported and left as it is.

**`values.yaml` is appended to, not rewritten.** The merge is textual and append-only, so everything already in the file survives byte-for-byte: section banners, blank lines, trailing comments, key order. Round-tripping the document through a YAML decoder and encoder would preserve the keys and lose the rest, which is why that is not what happens here.

**A key's documentation travels with it.** The comment block above a key is part of that key's block, so it lands in `values.yaml` alongside it.

**Repeating a command is a no-op.** `hck add pdb` twice writes once and then reports `nothing to do`.

**Requirements are reported, not pulled in.** Adding `ingress` to a chart with no Service prints what is missing and the command that fixes it, rather than quietly adding a Service you did not ask for.

<br/>

## hck check

Render the chart with your own `helm`, then apply the house rules to the manifests that come out.

```bash
hck check [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--chart` | `.` | Chart directory |
| `-f, --values` | — | Values files passed to helm; repeatable |
| `--strict` | `false` | Fail on warnings as well as errors |
| `--print` | `false` | Print the rendered manifests |
| `--no-render` | `false` | Skip helm; run only the rules that read the chart directory |

```bash
hck check
hck check --chart ./charts/payments-api
hck check -f values/prod.yaml --strict
hck check --no-render          # no helm needed
```

When no `-f` is given and the chart has `ci/install-values.yaml`, that file is used. A chart whose `image.tag` is required cannot render on its defaults — which is the point of requiring one — so the CI values file is what makes it checkable at all.

`hck check` exits non-zero on any error finding, or on any finding at all under `--strict`.

<br/>

## hck list

```bash
hck list              # presets and resources
hck list presets
hck list resources
```

Resources marked `[crd]` need a CRD or feature gate the target cluster may not have.

<br/>

## Shell completion

```bash
hck completion zsh  > "${fpath[1]}/_hck"
hck completion bash > /etc/bash_completion.d/hck
hck completion fish > ~/.config/fish/completions/hck.fish
```

Completion knows the preset and resource names, so `hck add <TAB>` lists what can be added.
