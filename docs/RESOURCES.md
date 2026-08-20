# Presets and resources

<br/>

## Presets

| Preset | Resources |
|---|---|
| `web` | serviceaccount, deployment, service, ingress, hpa, pdb, networkpolicy, configmap, tests |
| `worker` | serviceaccount, deployment, pdb, networkpolicy, configmap |
| `cronjob` | serviceaccount, cronjob, configmap |
| `stateful` | serviceaccount, statefulset, service, pdb, networkpolicy, configmap |
| `daemon` | serviceaccount, daemonset, configmap, networkpolicy |

Every preset carries exactly one workload. That is enforced by a test, not a convention: two workloads in one chart contend for the same values keys — `image`, `resources`, `updateStrategy` — with incompatible shapes.

<br/>

## Resources

| Name | apiVersion | Values key | Notes |
|---|---|---|---|
| `deployment` | `apps/v1` | many | Stateless workload. Skips `replicas` while an HPA is enabled. |
| `statefulset` | `apps/v1` | many | Per-replica volumes via `volumeClaimTemplates`. Needs a headless Service. |
| `daemonset` | `apps/v1` | many | Tolerates everything by default, so it reaches control-plane nodes too. |
| `cronjob` | `batch/v1` | `cronjob` | `timeZone` set explicitly; `concurrencyPolicy: Forbid`. |
| `job` | `batch/v1` | `job` | Optionally a Helm hook, for migrations. |
| `service` | `v1` | `service` | `headless: true` for StatefulSet peer DNS. |
| `ingress` | `networking.k8s.io/v1` | `ingress` | Prefer `httproute` where Gateway API is available. |
| `httproute` | `gateway.networking.k8s.io/v1` | `httpRoute` | Needs the Gateway API CRDs. Backend defaults to this chart's Service. |
| `hpa` | `autoscaling/v2` | `autoscaling` | Memory target is null by default — it ratchets up and stays. |
| `pdb` | `policy/v1` | `podDisruptionBudget` | Emits `minAvailable` or `maxUnavailable`, never both. |
| `networkpolicy` | `networking.k8s.io/v1` | `networkPolicy` | Default-deny with a DNS egress rule. |
| `serviceaccount` | `v1` | `serviceAccount` | `automount` off by default. |
| `rbac` | `rbac.authorization.k8s.io/v1` | `rbac` | Namespaced Role unless `clusterScoped`. |
| `configmap` | `v1` | `configMap` | Hashed into a pod annotation, so config changes actually roll. |
| `secret` | `v1` | `secret` | For install-time values only; the release itself is a readable Secret. |
| `externalsecret` | `external-secrets.io/v1` | `externalSecret` | Needs External Secrets Operator. |
| `pvc` | `v1` | `persistence` | `helm.sh/resource-policy: keep` by default. |
| `servicemonitor` | `monitoring.coreos.com/v1` | `metrics` | Needs Prometheus Operator. Set `additionalLabels`, or it is never scraped. |
| `prometheusrule` | `monitoring.coreos.com/v1` | `prometheusRule` | Rules run through `tpl`, so they can scope to the release. |
| `tests` | `v1` | `tests` | `helm test` hook that dials the Service. |

<br/>

## Requirements between resources

`ingress`, `httproute`, `servicemonitor` and `tests` need a `service`. Every workload needs a `serviceaccount`. `rbac` needs one too.

`hck add` reports an unmet requirement and the command that fixes it. It does not add it for you — a chart is a thing you are responsible for, and a tool that quietly adds a Service you did not ask for is harder to reason about than one that tells you.

<br/>

## Adding a resource to hck itself

1. Create `internal/render/templates/resources/<name>/` with `template.yaml.tmpl`, `values.yaml.tmpl` and `schema.json.tmpl`. The generation layer uses `[[ ]]` delimiters so Helm's `{{ }}` passes through untouched.
2. Add the entry to `resources` in `internal/catalog/catalog.go`, including its `ValuesKeys`.

The catalog, the values fragment and the schema fragment are cross-checked against each other by tests, so declaring a resource — or one of its values keys — in only some of them fails the build. See [DEVELOPMENT.md](DEVELOPMENT.md).
