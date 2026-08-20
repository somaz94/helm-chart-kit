# Presets and resources

> 한국어: [RESOURCES-ko.md](RESOURCES-ko.md) · Index: [README.md](../README.md)

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
| `hpa` | `autoscaling/v2` | `autoscaling` | Memory target is null by default — it ratchets up and stays. `targetKind` follows the chart's workload. |
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
| `podmonitor` | `monitoring.coreos.com/v1` | `podMonitor` | Scrapes pods with no Service in front, which is what a DaemonSet needs. |
| `certificate` | `cert-manager.io/v1` | `certificate` | Needs cert-manager. `dnsNames` defaults to the hosts the chart already serves. |
| `scaledobject` | `keda.sh/v1alpha1` | `scaledObject` | Needs KEDA. Do not enable `autoscaling` too — KEDA owns its own HPA. `targetKind` follows the chart's workload. |
| `vpa` | `autoscaling.k8s.io/v1` | `verticalPodAutoscaler` | `updateMode: "Off"` by default; quoted, or YAML reads it as `false`. `targetKind` follows the chart's workload. |
| `sealedsecret` | `bitnami.com/v1alpha1` | `sealedSecret` | Ciphertext is safe to commit. Sealed values are bound to a namespace and name. |
| `issuer` | `cert-manager.io/v1` | `issuer` | Namespaced. Prefer a ClusterIssuer when charts share an ACME account. |
| `referencegrant` | `gateway.networking.k8s.io/v1beta1` | `referenceGrant` | Lets an HTTPRoute in another namespace reach this Service. |
| `scaledjob` | `keda.sh/v1alpha1` | `scaledJob` | One Job per queue item, for bursty expensive work. |
| `virtualservice` | `networking.istio.io/v1` | `virtualService` | Needs Istio. `route` defaults to this chart's Service. The Gateway is cluster infrastructure and is not created here. |
| `destinationrule` | `networking.istio.io/v1` | `destinationRule` | Needs Istio. Pool limits and outlier ejection. `ISTIO_MUTUAL` by default — `DISABLE` here is what a STRICT PeerAuthentication then rejects. |
| `authorizationpolicy` | `security.istio.io/v1` | `authorizationPolicy` | Needs Istio. The L7 half of `networkpolicy`. `ALLOW` with an empty rule list denies everything, deliberately. |
| `grafanadashboard` | `v1` | `grafanaDashboard` | ConfigMap the Grafana sidecar reads. Wrong label fails silently. `templated` is off: Grafana's own `{{ }}` legends are not Helm templates. |
| `tests` | `v1` | `tests` | `helm test` hook that dials the Service. |

<br/>

## Requirements between resources

`ingress`, `httproute`, `servicemonitor`, `virtualservice`, `destinationrule` and `tests` need a `service`. Every workload needs a `serviceaccount`. `rbac` needs one too.

`hck add` reports an unmet requirement and the command that fixes it. It does not add it for you — a chart is a thing you are responsible for, and a tool that quietly adds a Service you did not ask for is harder to reason about than one that tells you.

<br/>

## Adding a resource to hck itself

1. Create `internal/render/templates/resources/<name>/` with `template.yaml.tmpl`, `values.yaml.tmpl` and `schema.json.tmpl`. The generation layer uses `[[ ]]` delimiters so Helm's `{{ }}` passes through untouched.
2. Add the entry to `resources` in `internal/catalog/catalog.go`, including its `ValuesKeys`.

The catalog, the values fragment and the schema fragment are cross-checked against each other by tests, so declaring a resource — or one of its values keys — in only some of them fails the build. See [DEVELOPMENT.md](DEVELOPMENT.md).
