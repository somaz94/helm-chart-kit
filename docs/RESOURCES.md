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
| `minimal` | serviceaccount, deployment, service |
| `gateway` | serviceaccount, deployment, service, httproute, hpa, pdb, networkpolicy, configmap, tests |
| `mesh` | serviceaccount, deployment, service, virtualservice, destinationrule, authorizationpolicy, hpa, pdb, configmap, tests |
| `queue` | serviceaccount, deployment, scaledobject, pdb, networkpolicy, configmap |
| `monitored` | serviceaccount, deployment, service, ingress, hpa, pdb, networkpolicy, configmap, servicemonitor, prometheusrule, grafanadashboard, tests |
| `secure` | serviceaccount, rbac, deployment, service, ingress, certificate, externalsecret, hpa, pdb, networkpolicy, configmap, tests |
| `base` | serviceaccount, deployment, service, ingress, httproute, hpa, pvc, certificate, configmap, tests |
| `base-aws` | serviceaccount, deployment, service, ingress, hpa, pdb, externalsecret, pvc, configmap, tests |

Every preset carries exactly one workload, and a chart may carry more. Those are different statements. A preset picks one because a preset has to pick something; a chart that renders two is what `HCK030` reports, and a chart that *carries* two guarded so only one renders is an ordinary shape — a Deployment beside an Argo Rollout, each under its own `.enabled`, sharing one pod template. `hck add` and `hck new` build it and say what they built. Whether both actually render is a question about the render, and that is where it is asked.

Two presets answer more than the resource list. `base` and `base-aws` are modelled on a pair of house charts an ApplicationSet repo keeps side by side — the same chart answering a different platform — so each one carries the platform it was written for, and asks for a `values.schema.json` and a values table because the charts they come from ship both:

| Preset | Platform | `values.schema.json` | Values table |
|---|---|---|---|
| `base` | `onprem` | yes | yes |
| `base-aws` | `aws` | yes | yes |

`hck init` reads those, which is why it puts three questions rather than seven. Every other preset carries none of them and the defaults are unchanged. They are defaults and not decisions: init shows what the preset resolved to and takes no for an answer, and every flag on `hck new` still overrides.

`base` carries both an Ingress and an HTTPRoute, where `web` and `gateway` each pick one. The chart it comes from guards each on its own `.enabled` and chooses per install rather than at build time.

Not carried over, for want of a resource to carry them: a certificate-renewal CronJob, an `imagePullSecret`, and a `PersistentVolume` to match the claim on the on-prem side; an Argo Rollout and its AnalysisTemplate, an EFS `PersistentVolume`, three preview-environment Ingresses and a preview Service on the AWS side.


Three of them are shaped by what they leave out, which is the part worth reading twice. `mesh` has no NetworkPolicy: in a mesh, who may call this workload is the AuthorizationPolicy's answer, at L7 and with an identity, and a second answer at L3 is a different question wearing the same name. `queue` has no HPA: a KEDA `ScaledObject` owns the replica count, and the two driving it together is exactly what `HCK031` reports. `secure` ships a Certificate and no Issuer: the Certificate defaults to a ClusterIssuer, which is where a shared one lives, and shipping a namespaced Issuer beside it without wiring the two together is what `HCK034` reports — `hck add issuer` is there for the chart that wants its own.

<br/>

## Groups

The catalog is 37 resources and their names are Kubernetes kinds, so an alphabetical list answers *what exists* and nothing else. Finding the three pieces of a monitoring setup meant already knowing they are called `servicemonitor`, `prometheusrule` and `grafanadashboard`.

Every resource belongs to one group, and a name opening with `@` stands for the group's members:

```bash
hck add @observability                  # the whole monitoring setup
hck new payments-api --with @secrets    # every way of getting a secret in
```

| Group | For | Resources |
|---|---|---|
| `@workload` | what runs | cronjob, daemonset, deployment, job, statefulset |
| `@exposure` | what reaches it | backendconfig, frontendconfig, httproute, ingress, referencegrant, service |
| `@scaling` | how many, and what may evict them | hpa, pdb, scaledjob, scaledobject, vpa |
| `@access` | identity, permissions and who may connect | networkpolicy, rbac, serviceaccount |
| `@secrets` | secrets and the certificates that need them | certificate, externalsecret, issuer, managedcertificate, sealedsecret, secret, secretstore |
| `@observability` | scraping, alerting and dashboards | grafanadashboard, podmonitor, podmonitoring, prometheusrule, servicemonitor |
| `@mesh` | Istio routing and policy | authorizationpolicy, destinationrule, virtualservice |
| `@chart` | configuration, storage and the chart's own install test | configmap, pvc, tests |

`hck list resources` prints them in this order — something runs, something reaches it, it scales, it is locked down, it is watched — which is roughly the order the decisions come in. Groups and resources are separate namespaces, kept apart by the `@`: a group and a resource may end up sharing a name, and without the prefix which one won would be decided by lookup order.

A group beside one of its own members resolves once, so `--with @exposure,service` is `@exposure`.


`secretstore` is a pair with `externalsecret`, and the same shape as `issuer` with `certificate`: the namespaced kind for a chart that owns its own credentials path, where a `ClusterSecretStore` belongs to whoever runs the cluster. Nothing wires the two together — `externalSecret.secretStoreRef` has a meaningful default and silently repointing it would change what an existing chart reads from — so `HCK039` reports the pair left unwired. The provider block is filled in by the platform overlay: `values-aws.yaml` for Secrets Manager, `values-gcp.yaml` for Secret Manager, `values-azure.yaml` for Key Vault.

<br/>

## Platform-only resources

A group says what a resource is for. A platform says where it exists at all, and the two are separate questions: a GKE `ManagedCertificate` is a secrets resource that happens to be GKE-only.

This is the axis an overlay cannot cover. `values-gcp.yaml` changes what an annotation says; it cannot conjure the kind the annotation names. Before these existed the gcp Ingress overlay carried the line commented out with *"provision it separately"* — a reference by name to something the chart had no way to provide, which is exactly what `HCK034` and `HCK035` are about.

| Resource | Platform | Replaces | Referenced by |
|---|---|---|---|
| `managedcertificate` | `gcp` | `certificate` (cert-manager) | Ingress annotation |
| `podmonitoring` | `gcp` | `servicemonitor` | nothing — selects pods directly |
| `backendconfig` | `gcp` | — | Service annotation |
| `frontendconfig` | `gcp` | — | Ingress annotation |

Three of the four are reached by name from an annotation, so **the chart sets that annotation itself** when the resource is on, and drops it when the resource is off. Naming one that does not exist is the quiet failure here — GKE applies the annotation and falls back to a health check on `/`, or serves plaintext on port 80. `HCK038` reports it.

Two rules follow from the axis:

- **`HCK037`** — a chart carrying both halves of a pair. A cert-manager `Certificate` beside a `ManagedCertificate` is two certificates and one Ingress; a `ServiceMonitor` beside a `PodMonitoring` scrapes the workload twice. Same shape as `HCK031` and `HCK032`.
- **`HCK038`** — an annotation naming a `BackendConfig` or `FrontendConfig` the chart does not render.

**A group never expands to a platform-only resource.** `hck add @secrets` on an EKS chart must not put a GKE CRD in it, so the expansion leaves them out and says which:

```console
$ hck add @secrets
  +  templates/certificate.yaml
  ...
  note: left out of the group because they exist on one platform only: managedcertificate (gcp) — add by name to include one
```

Named explicitly, it goes in. hck never refuses one — it cannot know what cluster the chart will be installed on.

<br/>

## What a platform overlay can ask for and cannot provide

A platform overlay names things. Most of them the chart can then render — that is what the platform-only resources above are for. One kind it cannot: an object that is **cluster-scoped**.

`values-aws.yaml` sets `persistence.storageClass: gp3`, and it is right to. gp3 has the durability of gp2, costs less, and lets IOPS and throughput be set independently of size. GKE and AKS ship an equivalent class; EKS ships `gp2` and nothing else, so on a stock EKS cluster that claim binds to nothing.

hck cannot close that gap by adding a `storageclass` resource. A `StorageClass` is cluster-scoped, so two releases of the same chart in one cluster would create the same object and the second `helm install` fails — the same collision that kept `ClusterSecretStore` and `ClusterPodMonitoring` out of the catalog. Naming it per release avoids the collision and produces one identical class per release, which is not what a cluster-wide class is.

So it is reported instead, as `HCK040`:

```console
$ hck check --platform aws --strict
check demo (aws)

  info  HCK040  StatefulSet/demo volumeClaimTemplate=data
        requests storage class "gp3", which this chart does not create and the
        platform does not ship. EKS ships gp2 and no gp3 — create one against
        the EBS CSI driver first. Until then the claim stays Pending and the
        pod never schedules, and nothing reports a failure

  ok 0 warnings, 0 errors, 1 info(s)
```

It is an `info` rather than a warning, and that is the whole design. Nothing here is a mistake: the chart is right, the overlay is right, and the only thing missing is outside both. A warning would mean a chart hck itself generated fails hck's own `--strict`. Saying nothing would leave it to be discovered from a pod that never schedules.

The rule reports a class name only if it recognises it. Two lists in `internal/check/rules.go` hold every name hck's own overlays write — the ones the platform creates, and the ones somebody has to — and a name in neither is the user's own, about which hck knows nothing. `TestEveryOverlayStorageClassIsClassified` fails when a new overlay names a class neither list covers, because the alternative is silence, and silence from a rule reads exactly like the chart being fine.

| Overlay | Class | Ships with the platform |
|---|---|---|
| `values-aws.yaml` | `gp3` | No — create one against the EBS CSI driver |
| `values-gcp.yaml` | `premium-rwo` | Yes |
| `values-azure.yaml` | `managed-csi-premium` | Yes |
| `values-onprem.yaml` | `local-path` | No — k3s ships it, a stock cluster does not |

A chart that treats the class as its own job to create says so, and then `--strict` does stop it:

```yaml
rules:
  HCK040: warn
```

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
