# helm-chart-kit

[![CI](https://github.com/somaz94/helm-chart-kit/actions/workflows/ci.yml/badge.svg)](https://github.com/somaz94/helm-chart-kit/actions/workflows/ci.yml)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Latest Tag](https://img.shields.io/github/v/tag/somaz94/helm-chart-kit)](https://github.com/somaz94/helm-chart-kit/tags)
[![Top Language](https://img.shields.io/github/languages/top/somaz94/helm-chart-kit)](https://github.com/somaz94/helm-chart-kit)

`hck` — a single-binary chart scaffolder that keeps working on a chart after it exists.

> For detailed documentation, see the [docs/](docs/) folder:
>
> [Usage](docs/USAGE.md) |
> [Resources](docs/RESOURCES.md) |
> [Development](docs/DEVELOPMENT.md)
>
> 한국어: [README-ko.md](README-ko.md) |
> [사용법](docs/USAGE-ko.md) |
> [리소스](docs/RESOURCES-ko.md) |
> [개발](docs/DEVELOPMENT-ko.md)

<br/>

## The problem it solves

`helm create` writes six files and leaves. Every chart that actually ships then grows a PodDisruptionBudget, a NetworkPolicy, a ServiceMonitor, an HTTPRoute — and each one arrives by copy-paste from whichever chart someone remembered having it.

The paste is not the expensive part. The expensive part is `values.yaml`: the new keys have to go in, they have to be documented, and the four hundred lines already there have to survive the edit. So the copy happens by hand, the comments do not come along, and the sections drift apart between charts.

`hck add` does that step:

```bash
$ hck add servicemonitor
updated payments-api

  +  templates/servicemonitor.yaml
  ~  values.yaml

  values.yaml wrote: metrics

  note: servicemonitor needs a CRD the cluster may not have (monitoring.coreos.com/v1)
```

`values.yaml` is appended to, never rewritten. The bytes that were there before are byte-for-byte the bytes that are there after — banners, blank lines, trailing comments and all — because the merge works on text and only ever adds. A key that already exists is reported and left alone.

<br/>

## Why not just `helm create`?

| | `helm create` | `hck` |
|---|---|---|
| **Resources it can emit** | 6, fixed | 29, composable |
| **Adding one to an existing chart** | — | `hck add <resource>` |
| **`values.yaml` on add** | — | Appended, existing bytes untouched |
| **Documented values** | Sparse | Every key carries the reason it exists |
| **Gateway API, ServiceMonitor, ExternalSecret** | — | Yes |
| **Presets** | One shape | `web`, `worker`, `cronjob`, `stateful`, `daemon` |
| **Platform values** | None | `aws`, `gcp`, `azure`, `onprem` overlays |
| **Environment values** | None | `dev`, `staging`, `prod` overlays |
| **Custom starters** | `--starter`, whole-chart only | Per-resource, composable |
| **Validation** | `helm lint` | `helm template` + `helm lint` + house rules |
| **Image tag** | Falls back to `appVersion` | Required — the render fails without one |
| **Second workload in one chart** | Allowed | Refused, with the reason |
| **Idempotent** | Overwrites | Never overwrites; reports and stops |

<br/>

## Install

```bash
# Homebrew (macOS and Linux)
brew install --cask somaz94/tap/helm-chart-kit

# Scoop (Windows)
scoop bucket add somaz94 https://github.com/somaz94/scoop-bucket
scoop install helm-chart-kit

# curl
curl -fsSL https://raw.githubusercontent.com/somaz94/helm-chart-kit/main/scripts/install.sh | bash

# go install
go install github.com/somaz94/helm-chart-kit/cmd@latest
```

`hck new` and `hck add` have no runtime dependencies. `hck check` shells out to `helm`, deliberately: it then reports what your own helm does rather than what a vendored copy would have done.

<br/>

## Quick start

Three commands cover most of it:

```bash
hck new payments-api      # create a chart
hck add servicemonitor    # add to one that exists
hck check                 # render it and apply the house rules
```

Not sure which flags you want? `hck init` asks instead, and prints the
equivalent command when it is done:

```bash
hck init payments-api
```

Everything below is opt-in. A chart that never asks for a JSON Schema, a
values table or a platform overlay does not get one — so if the three commands
above are all you need, you are done.

<details>
<summary>The rest of the surface, at a glance</summary>

```bash
# Presets decide what a new chart starts with
hck new billing-worker --preset worker     # no Service, no ingress path
hck new sessions --preset stateful         # StatefulSet with per-replica volumes
hck new log-shipper --preset daemon        # DaemonSet on every node

# Add several at once, or see what would happen first
hck add pdb networkpolicy
hck add httproute --dry-run

# Check against real values, or fail on warnings too
hck check -f values/prod.yaml --strict

# Platform overlays: only what differs on EKS / GKE / AKS / self-managed
hck new payments-api --platform aws
hck platform add gcp azure
hck check --platform aws                   # check it AS INSTALLED there

# Environment overlays: how hard it is being asked to work
hck new payments-api --env dev,prod
hck env add staging
hck check --platform aws --env prod        # both axes at once

# Describe the values with a JSON Schema, or document them as a table
hck schema --write
hck docs --write
hck schema --check && hck docs --check     # CI gates

# See what is available
hck list
```

</details>

<br/>

## `hck init`

The flags below are worth learning, but not on the first chart:

```console
$ hck init payments-api
Chart name? [payments-api]
  presets:
    cronjob   Scheduled task: CronJob, ServiceAccount, ConfigMap
    daemon    Node agent: DaemonSet on every node, no Service
    stateful  Stateful service: StatefulSet, headless Service, PDB, NetworkPolicy
    web       HTTP service: Deployment, Service, Ingress, HPA, PDB, NetworkPolicy
    worker    Queue consumer: Deployment with no Service and no ingress path
Preset? [web]
Extra resources? (comma-separated) [none] servicemonitor
Platform overlays? [none] aws
Environment overlays? [none] dev,prod
Write values.schema.json? [y/N] y
Write a values table into README.md? [y/N] y

created ./payments-api (preset web)
...

The same thing without the questions:
  hck new payments-api --with servicemonitor --platform aws --env dev,prod --schema && hck docs --chart payments-api --write
```

It prints the equivalent command because the questions are for the first
chart and the flags are for every one after it. That equivalence is not a
claim: `TestInitPrintsAWorkingEquivalent` runs the printed command and
compares the two trees file by file.

`--defaults` asks nothing, and an early EOF takes the remaining defaults — so
a heredoc that answers the first two questions is a valid way to drive it.

<br/>

## What a chart starts with

```
payments-api/
├── Chart.yaml
├── .helmignore
├── values.yaml                       # one documented section per resource
├── ci/
│   └── install-values.yaml           # the overrides that make it render; .helmignore keeps it out of the package
└── templates/
    ├── _helpers.tpl
    ├── NOTES.txt
    ├── configmap.yaml
    ├── deployment.yaml
    ├── hpa.yaml
    ├── ingress.yaml
    ├── networkpolicy.yaml
    ├── pdb.yaml
    ├── service.yaml
    ├── serviceaccount.yaml
    └── tests/
        └── test-connection.yaml
```

It passes `hck check` on the first run, with no findings.

`--schema` adds a `values.schema.json` beside `values.yaml`.

<br/>

## `values.schema.json`

Helm validates a chart's values against `values.schema.json` on every render,
which cuts both ways: a good schema turns a typo into an error message, and a
merely incomplete one turns a working values file into a failed release. That
is why `hck` generates the file rather than asking you to write it, and why it
is opt-in:

```bash
hck new payments-api --schema      # scaffold with a schema
hck schema --write                 # add one to a chart that already has none
hck schema                         # print it without writing
hck schema --check                 # fail if the file on disk is stale
```

Once a chart has one, `hck add` keeps it in step — a resource that contributes
values keys contributes the schema for them in the same run.

The generated schema is deliberately permissive. Objects stay open, and a
scalar whose default is empty is typed as the union it really accepts, so
`service.nodePort` takes a string or an integer rather than only the empty
string it ships with. What it does constrain is what Kubernetes itself
constrains: `image.pullPolicy` is one of three values, `service.type` one of
four, `cronjob.concurrencyPolicy` one of three.

`--strict` closes the top level, so an undeclared top-level key is an error
instead of a value that silently does nothing:

```bash
hck new payments-api --schema-strict
hck schema --write --strict
```

```console
$ helm template rel ./payments-api --set replicaCont=3
Error: values don't meet the specifications of the schema(s) in the following chart(s):
payments-api:
- (root): Additional property replicaCont is not allowed
```

Nested objects stay open even then — the point is to catch a misspelled key,
not to model the Kubernetes API. `global` is always allowed, so subchart values
still work.

`hck schema --check` is the CI gate: it rebuilds the schema and fails if the
committed file differs, which catches a `values.yaml` edited by hand without
the schema following it.

<br/>

## Platform overlays

The same chart wants different values on EKS, GKE, AKS and a self-managed
cluster — an IAM role annotation here, an ingress class there, a storage class
somewhere else. `hck` generates those as overlays:

```bash
hck new payments-api --platform aws          # scaffold with values-aws.yaml
hck platform add gcp azure                   # add to a chart you already have
hck platform list                            # what exists, and what this chart has
```

An overlay is **not a replacement**. Helm reads `values.yaml` first and always,
so the file carries only what is different:

```yaml
# values-aws.yaml
serviceAccount:
  annotations:
    # -- IAM role the pods assume, via IRSA.
    eks.amazonaws.com/role-arn: arn:aws:iam::000000000000:role/payments-api

ingress:
  className: alb
  annotations:
    alb.ingress.kubernetes.io/target-type: ip
```

```bash
helm install payments-api . -f values-aws.yaml
```

Only the resources the chart actually has contribute: a `worker` chart gets the
ServiceAccount annotation and no ingress class, because it has no Ingress. A
chart with nothing platform-specific in it gets no file at all rather than one
consisting of a header.

**`hck check --platform aws` renders the chart with the overlay applied.** An
overlay that does not render is worse than no overlay — it looks like
configuration right up until someone installs with it.

| Platform | Covers |
|---|---|
| `aws` | IRSA, ALB ingress, NLB services, gp3 volumes, zone spread, Secrets Manager |
| `gcp` | Workload Identity, GCE ingress, NEG services, pd-balanced volumes, Secret Manager |
| `azure` | Workload Identity (including the pod label everyone forgets), Application Gateway, managed-csi, Key Vault |
| `onprem` | ingress-nginx, MetalLB, a storage class you provide, Vault, private registry pull secrets |

<br/>

## Environment overlays

Platform says *where*; environment says *how hard*. The two are orthogonal and
they stack:

```bash
helm install app . -f values-aws.yaml -f values-prod.yaml
#                     ↑ EKS              ↑ three replicas, a budget, strict probes
```

Environment goes **last**, because `helm` applies `-f` left to right: the size
prod asks for wins over whatever the platform overlay happened to set.

| Environment | Shape |
|---|---|
| `dev` | One replica, small requests, no budget, patient liveness probe — a pod stopped in a debugger is not an unhealthy pod |
| `staging` | Two replicas, production's ratios at a fraction of its size, HPA and NetworkPolicy on so both are exercised before production depends on them |
| `prod` | Three replicas, `maxUnavailable: 0` rollouts, 60s termination grace, HPA 3–20 scaling up fast and down slowly, PDB, `helm test` off |

```bash
hck new payments-api --env dev,prod
hck env add staging
hck env list
hck check --env prod --strict
```

<br/>

## `hck docs`

`values.yaml` already documents itself — every key carries a `-- ` comment, the
helm-docs convention. `hck docs` reads those back out and renders a table:

```console
$ hck docs
| Key | Type | Default | Description |
|---|---|---|---|
| `replicaCount` | int | `1` | Number of pod replicas. Ignored when autoscaling.enabled is true. |
| `image.pullPolicy` | string | `IfNotPresent` | Image pull policy. One of: `Always`, `IfNotPresent`, `Never`. |
| `service.nodePort` | string/int/null | `""` | Fixed node port. Only read when type is NodePort. |
```

The types and the allowed values come from the schema, because `values.yaml`
cannot express them: a key defaulting to `""` says nothing about whether it
takes a string or a number, and nothing at all about the four values the API
accepts. The chart does not need a committed `values.schema.json` for this —
one is assembled on the fly when the file is absent.

`--write` replaces the block between two markers in the chart's `README.md`
and leaves everything around it alone:

```markdown
<!-- hck:values:start -->
... generated table ...
<!-- hck:values:end -->
```

A chart with no README gets one. `hck docs --check` is the CI gate.

<br/>

## The opinions baked in

A generated chart is not neutral. These are the calls it makes, and why.

**`image.tag` is required.** No `appVersion` fallback. A chart that silently deploys whatever `appVersion` happens to say is how a stale image reaches production; the render fails instead.

**No CPU limit, always a memory limit.** CFS throttling on a CPU limit causes latency spikes long before the node is busy. Memory is incompressible, so it is capped.

**Restricted Pod Security Standard by default.** `runAsNonRoot`, `readOnlyRootFilesystem`, all capabilities dropped. Loosen it deliberately rather than starting permissive.

**NetworkPolicy is default-deny, with DNS opened.** Both `policyTypes` are rendered with empty rule lists, which denies everything; the `kube-system` DNS egress rule is on by default because without it every hostname-based rule silently fails.

**One workload per chart.** `hck add statefulset` into a chart that has a Deployment is refused, and so is `hck new --preset web --with daemonset`: the two contend for the same values keys with incompatible shapes, so the result renders but does not apply. `hck check` reports it as `HCK030` for a chart that has it already.

**`serviceAccount.automount` is off.** The token is a credential. Turn it on when the workload actually calls the API server.

<br/>

## The house rules `hck check` enforces

| Rule | Severity | What it catches |
|---|---|---|
| `HCK001` | error | The chart does not render |
| `HCK002` | warn | `helm lint` findings |
| `HCK010`–`HCK013` | warn | Missing `values.yaml`, `.helmignore`, wrong `apiVersion`, empty description |
| `HCK020` | warn | Pod does not set `runAsNonRoot` |
| `HCK021` | error | Container has no image, or an untagged one |
| `HCK022` | error | Image tag is `:latest` |
| `HCK023` | warn | No resource requests |
| `HCK024` | warn | No memory limit |
| `HCK025` | warn | CPU limit set |
| `HCK026` | warn | `allowPrivilegeEscalation` not disabled |
| `HCK027` | error | Container runs privileged |
| `HCK028`–`HCK029` | warn | No readiness or liveness probe on a long-running workload |
| `HCK030` | warn | Chart renders more than one primary workload |
| `HCK031` | warn | HPA and a KEDA ScaledObject both scale the workload |
| `HCK032` | warn | HPA alongside a VPA in an evicting update mode |

Warnings pass by default; `--strict` fails on them.

<br/>

## License

[Apache 2.0](LICENSE)
