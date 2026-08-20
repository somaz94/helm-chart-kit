# Usage

> 한국어: [USAGE-ko.md](USAGE-ko.md) · Index: [README.md](../README.md)

<br/>

## hck init

Create a chart by answering a few questions.

```bash
hck init [chart-name] [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `-d, --dir` | `.` | Parent directory to create the chart in |
| `--defaults` | `false` | Take every default and ask nothing |

Every question corresponds to a flag on `hck new`, and `init` prints the equivalent command when it finishes — the questions are for the first chart, the flags are for every one after it.

Enter takes the default shown in brackets. EOF partway through takes the rest of the defaults, so this is a valid way to drive it:

```bash
printf 'payments-api\nweb\n' | hck init
```

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
| `--schema` | `false` | Also write `values.schema.json` |
| `--schema-strict` | `false` | Write `values.schema.json` and reject undeclared top-level keys |
| `--platform` | — | Platform values overlays to write, comma-separated: `aws`, `gcp`, `azure`, `onprem` |
| `--env` | — | Environment values overlays to write, comma-separated: `dev`, `staging`, `prod` |
| `--dry-run` | `false` | Print what would be written and exit |

```bash
hck new payments-api
hck new billing-worker --preset worker
hck new sessions --preset stateful --with servicemonitor
hck new gateway --preset web --with httproute,prometheusrule --dry-run
hck new payments-api --schema
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
| `--platform` | — | Also apply these platform overlays, comma-separated |
| `--env` | — | Also apply these environment overlays, comma-separated. Applied after `--platform`, so it wins |
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

## hck schema

Assemble the chart's `values.schema.json` from the resources it carries.

```bash
hck schema [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--chart` | `.` | Chart directory |
| `--write` | `false` | Write `values.schema.json` into the chart |
| `--check` | `false` | Fail when the file on disk differs from what would be generated |
| `--strict` | existing file's setting | Reject undeclared top-level keys |

```bash
hck schema                     # print it
hck schema --write             # write it into the chart
hck schema --write --strict    # write it with the top level closed
hck schema --check             # CI gate
```

With no flag the schema goes to stdout and the chart is left alone. `--write` and `--check` are mutually exclusive.

Helm validates the coalesced values against this file on **every** render, so the generated schema is deliberately permissive: objects stay open, and a scalar whose default is empty is typed as the union it really accepts. A schema that is merely incomplete does not document a chart, it breaks one.

`--strict` closes the top level only. Nested objects stay open — the point is to catch a misspelled key, not to model the Kubernetes API — and `global` is always allowed so subchart values keep working.

`--strict` is sticky: once a schema is written with it, `hck schema` and `hck add` both keep it that way. Pass `--strict=false` to drop it deliberately.

<br/>

### Keeping it current

A chart that has a schema must describe every key its `values.yaml` declares, or helm refuses the chart. Two things keep that true:

- `hck add` regenerates the schema whenever it appends to `values.yaml`. A chart with no schema does not get one — the file is opt-in.
- `hck schema --check` rebuilds and compares, so CI catches a `values.yaml` edited by hand without the schema following it.

<br/>

## hck platform

Platform overlays carry the values that differ between one target and another, and nothing else.

```bash
hck platform list                    # known platforms, and which this chart has
hck platform add aws                 # write values-aws.yaml
hck platform add gcp azure           # several at once
hck platform add onprem --dry-run
hck platform add aws --force         # rewrite one that exists
```

| Platform | Covers |
|---|---|
| `aws` | IRSA, ALB ingress, NLB services, gp3 volumes, zone spread, Secrets Manager |
| `gcp` | Workload Identity, GCE ingress, NEG services, pd-balanced volumes, Secret Manager |
| `azure` | Workload Identity including the required pod label, Application Gateway, managed-csi, Key Vault |
| `onprem` | ingress-nginx, MetalLB, a storage class you provide, Vault, private registry pull secrets |

An overlay is not a replacement. Helm reads `values.yaml` first and always, so the generated file carries only the difference:

```bash
helm install payments-api . -f values-aws.yaml
```

Only the resources the chart actually carries contribute. A `worker` chart gets the ServiceAccount annotation and no ingress class, because it has no Ingress; a chart with nothing platform-specific in it gets no file rather than one consisting of a header.

`hck platform add` never overwrites an overlay that exists — edit it freely — unless `--force` is passed.

<br/>

### Checking one

```bash
hck check --platform aws
hck check --platform aws,gcp --strict
```

This renders the chart **with the overlay applied**, which is the only way to find out that it renders at all. The overlay is additive: it layers on top of `ci/install-values.yaml` rather than replacing it, so the chart still gets the image tag it requires.

<br/>

## hck env

Environment overlays carry how hard the chart is being asked to work. Orthogonal to platform, and stacked after it.

```bash
hck env list
hck env add prod
hck env add dev staging
hck env add prod --dry-run
hck env add prod --force
```

| Environment | Shape |
|---|---|
| `dev` | One replica, small requests, no budget, patient liveness probe, CronJobs suspended |
| `staging` | Two replicas, production's ratios at a fraction of its size, HPA and NetworkPolicy on |
| `prod` | Three replicas, `maxUnavailable: 0`, 60s grace, HPA 3–20, PDB, `helm test` off |

```bash
helm install app . -f values-aws.yaml -f values-prod.yaml
```

The environment goes last because `helm` applies `-f` left to right: the replica count prod asks for wins over whatever a platform overlay set. `hck check --platform aws --env prod` passes them to helm in that same order.

<br/>

## hck docs

Turn `values.yaml` into a Markdown table.

```bash
hck docs [flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--chart` | `.` | Chart directory |
| `--write` | `false` | Write the table into the chart's `README.md` |
| `--check` | `false` | Fail when the README's table differs from what would be generated |

```bash
hck docs                       # print it
hck docs --write               # write it into README.md
hck docs --check               # CI gate
```

Descriptions come from the file itself: a comment line opening with `-- ` documents the key below it. A section banner is not a description — only the `-- ` prefix starts one, so the banners that divide `values.yaml` into sections do not leak into the table.

Types and allowed values come from the schema. The chart does not need a committed `values.schema.json`: one is assembled from the resources it carries when the file is absent.

`--write` replaces the block between `<!-- hck:values:start -->` and `<!-- hck:values:end -->`, leaving everything around it untouched. A README with no markers gets them appended under a `## Values` heading; a chart with no README gets one. `--write` and `--check` are mutually exclusive.

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
