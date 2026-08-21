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
| **Resources it can emit** | 6, fixed | 36, composable |
| **Adding one to an existing chart** | — | `hck add <resource>` |
| **`values.yaml` on add** | — | Appended, existing bytes untouched |
| **Documented values** | Sparse | Every key carries the reason it exists |
| **Gateway API, Istio, ServiceMonitor, ExternalSecret** | — | Yes |
| **Presets** | One shape | 11: `web`, `gateway`, `mesh`, `worker`, `queue`, `stateful`, `daemon`, `cronjob`, `monitored`, `secure`, `minimal` |
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
hck new edge --preset gateway              # HTTPRoute instead of Ingress
hck new checkout --preset mesh             # Istio routing, policy and mTLS
hck new mailer --preset queue              # scaled by KEDA on queue depth
hck new payments --preset monitored        # web, plus the Prometheus stack
hck new tokens --preset secure             # web, plus RBAC, TLS and ESO
hck new tiny --preset minimal              # a Deployment and a Service

# Add several at once, or see what would happen first
hck add pdb networkpolicy
hck add httproute --dry-run

# Take one back out. Templates only — values.yaml is never rewritten
hck remove ingress
hck remove hpa pdb --dry-run

# See which templates no longer match what hck generates
hck sync
hck sync --check                           # CI gate
hck sync --write deployment                # take hck's version

# Check against real values, or fail on warnings too
hck check -f values/prod.yaml --strict
hck check --off HCK025                     # this chart wants its CPU limits

# Add a whole purpose at once — "@name" stands for a group's members
hck add @observability                     # servicemonitor, rules, dashboard
hck list resources                         # 32 resources, grouped by purpose

# Two workload templates guarded so one renders at a time: built and noted,
# not refused. HCK030 reports the chart that renders both
hck new blue-green --preset web --with daemonset

# The escape hatch: a directory that is not empty. Files already there are
# left alone, values.yaml first among them
hck new recovered --preset web --force

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
hck list rules                             # the check rules, and their IDs
```

</details>

<br/>

## `hck init`

The flags below are worth learning, but not on the first chart:

```console
$ hck init payments-api
Chart name? [payments-api]
  presets:
    base      On-prem house chart: Deployment, Service, Ingress or HTTPRoute, HPA, PVC, Certificate
    base-aws  EKS house chart: base on AWS, with an ExternalSecret and a PDB, no Certificate
    cronjob   Scheduled task: CronJob, ServiceAccount, ConfigMap
    daemon    Node agent: DaemonSet on every node, no Service
    gateway   HTTP service on Gateway API: Deployment, Service, HTTPRoute, HPA, PDB
    mesh      Istio service: Deployment, Service, VirtualService, DestinationRule, AuthorizationPolicy
    minimal   Smallest chart that runs and is reachable: Deployment and Service
    monitored web, plus a ServiceMonitor, alert rules and a Grafana dashboard
    queue     KEDA consumer: Deployment scaled on queue depth, no Service
    secure    web, plus RBAC, a cert-manager Certificate and an ExternalSecret
    stateful  Stateful service: StatefulSet, headless Service, PDB, NetworkPolicy
    web       HTTP service: Deployment, Service, Ingress, HPA, PDB, NetworkPolicy
    worker    Queue consumer: Deployment with no Service and no ingress path
Preset? [web] base
Extra resources? (comma-separated) [none] servicemonitor

  base also decided:
    platform overlays: onprem
    environment overlays: none
    values.schema.json: yes
    values table in README.md: yes

Keep those? [Y/n]

created ./payments-api (preset base)
...

The same thing without the questions:
  hck new payments-api --preset base --with servicemonitor --platform onprem --schema && hck docs --chart payments-api --write
```

Three questions, because the preset answers the other four. Answering `n` to
the last one opens them, each seeded with what the preset chose.

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
| `HCK033` | warn | A scaler names a workload the chart does not render |
| `HCK034` | warn | Chart creates an Issuer its own Certificate does not use |
| `HCK035` | warn | A Service forwards to a container port name nothing declares |
| `HCK037` | warn | Chart renders two resources answering the same question |
| `HCK038` | warn | A GKE annotation names a config object the chart does not render |
| `HCK036` | warn | A PodDisruptionBudget that never allows a voluntary disruption |

Warnings pass by default; `--strict` fails on them. `hck list rules` prints the same table with the wording `hck check` uses.

A chart that disagrees with a rule says so in its own `.hck.yaml`, next to `Chart.yaml`:

```yaml
rules:
  HCK025: off      # this chart wants its CPU limits
  HCK023: error    # and will not ship without requests
```

Every rule takes `off`, `warn` or `error`. An ID that does not exist is an error rather than a silent no-op — the whole point of writing `HCK025` down is to stop seeing it, and a misspelling that quietly kept reporting would be indistinguishable from the rule being right. `HCK001` is the exception: a chart that does not render has nothing else worth reporting, so it cannot be configured. What a chart turned off is printed with the findings, because a clean report over a chart with half the rules off says less than it looks like it does.

For CI, `--format json` reports the same run as a document with an `ok` field matching the exit status:

```bash
hck check --format json --strict
```

<br/>

## License

[Apache 2.0](LICENSE)
