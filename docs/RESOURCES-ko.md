# preset과 리소스

> English: [RESOURCES.md](RESOURCES.md) · 한국어 문서 모음: [README-ko.md](../README-ko.md)

<br/>

## Preset

| Preset | 리소스 |
|---|---|
| `web` | serviceaccount, deployment, service, ingress, hpa, pdb, networkpolicy, configmap, tests |
| `worker` | serviceaccount, deployment, pdb, networkpolicy, configmap |
| `cronjob` | serviceaccount, cronjob, configmap |
| `stateful` | serviceaccount, statefulset, service, pdb, networkpolicy, configmap |
| `daemon` | serviceaccount, daemonset, configmap, networkpolicy |

모든 preset은 workload를 정확히 하나만 가집니다. 이건 관례가 아니라 테스트로 강제됩니다. 한 차트에 workload가 둘이면 같은 values 키(`image`, `resources`, `updateStrategy`)를 호환되지 않는 형태로 놓고 다투기 때문입니다.

<br/>

## 리소스

| 이름 | apiVersion | values 키 | 비고 |
|---|---|---|---|
| `deployment` | `apps/v1` | 여럿 | 무상태 workload. HPA가 켜져 있으면 `replicas`를 렌더하지 않습니다. |
| `statefulset` | `apps/v1` | 여럿 | `volumeClaimTemplates`로 레플리카별 볼륨. headless Service가 필요합니다. |
| `daemonset` | `apps/v1` | 여럿 | 기본으로 모든 taint를 허용하므로 컨트롤 플레인 노드에도 뜹니다. |
| `cronjob` | `batch/v1` | `cronjob` | `timeZone`을 명시하고 `concurrencyPolicy: Forbid`. |
| `job` | `batch/v1` | `job` | 마이그레이션용으로 Helm hook으로도 쓸 수 있습니다. |
| `service` | `v1` | `service` | StatefulSet 피어 DNS를 위한 `headless: true`. |
| `ingress` | `networking.k8s.io/v1` | `ingress` | Gateway API를 쓸 수 있다면 `httproute` 쪽이 낫습니다. |
| `httproute` | `gateway.networking.k8s.io/v1` | `httpRoute` | Gateway API CRD 필요. backend는 이 차트의 Service가 기본값. |
| `hpa` | `autoscaling/v2` | `autoscaling` | 메모리 타깃은 기본 null — 한 번 올라가면 내려오지 않기 때문입니다. |
| `pdb` | `policy/v1` | `podDisruptionBudget` | `minAvailable`이나 `maxUnavailable` 중 하나만, 절대 둘 다는 안 됩니다. |
| `networkpolicy` | `networking.k8s.io/v1` | `networkPolicy` | DNS egress 규칙이 있는 기본 거부. |
| `serviceaccount` | `v1` | `serviceAccount` | `automount` 기본 꺼짐. |
| `rbac` | `rbac.authorization.k8s.io/v1` | `rbac` | `clusterScoped`가 아니면 네임스페이스 Role. |
| `configmap` | `v1` | `configMap` | 파드 어노테이션에 해시로 들어가므로 설정 변경이 실제로 롤아웃됩니다. |
| `secret` | `v1` | `secret` | 설치 시점 값 전용. 릴리스 자체가 읽을 수 있는 Secret입니다. |
| `externalsecret` | `external-secrets.io/v1` | `externalSecret` | External Secrets Operator 필요. |
| `pvc` | `v1` | `persistence` | 기본으로 `helm.sh/resource-policy: keep`. |
| `servicemonitor` | `monitoring.coreos.com/v1` | `metrics` | Prometheus Operator 필요. `additionalLabels`를 설정하지 않으면 영영 스크레이프되지 않습니다. |
| `prometheusrule` | `monitoring.coreos.com/v1` | `prometheusRule` | 규칙이 `tpl`을 거치므로 릴리스 단위로 범위를 좁힐 수 있습니다. |
| `podmonitor` | `monitoring.coreos.com/v1` | `podMonitor` | Service 없는 파드를 직접 스크레이프. DaemonSet에 필요한 형태입니다. |
| `certificate` | `cert-manager.io/v1` | `certificate` | cert-manager 필요. `dnsNames`는 차트가 이미 서빙하는 호스트가 기본값. |
| `scaledobject` | `keda.sh/v1alpha1` | `scaledObject` | KEDA 필요. `autoscaling`은 같이 켜지 마세요 — KEDA가 자체 HPA를 만듭니다. |
| `vpa` | `autoscaling.k8s.io/v1` | `verticalPodAutoscaler` | 기본 `updateMode: "Off"`. 따옴표 필수 — 없으면 YAML이 이 값을 `false`로 읽습니다. |
| `sealedsecret` | `bitnami.com/v1alpha1` | `sealedSecret` | 암호문은 커밋해도 안전. 봉인된 값은 네임스페이스와 이름에 묶입니다. |
| `issuer` | `cert-manager.io/v1` | `issuer` | 네임스페이스 단위. 여러 차트가 ACME 계정을 공유한다면 ClusterIssuer가 낫습니다. |
| `referencegrant` | `gateway.networking.k8s.io/v1beta1` | `referenceGrant` | 다른 네임스페이스의 HTTPRoute가 이 Service에 닿게 허용합니다. |
| `scaledjob` | `keda.sh/v1alpha1` | `scaledJob` | 큐 항목당 Job 하나. 한꺼번에 몰리는 무거운 작업에 적합합니다. |
| `grafanadashboard` | `v1` | `grafanaDashboard` | Grafana 사이드카가 읽는 ConfigMap. 라벨이 틀리면 조용히 실패합니다. `templated`는 꺼짐 — Grafana 자체의 `{{ }}` 범례는 Helm 템플릿이 아닙니다. |
| `tests` | `v1` | `tests` | Service를 호출하는 `helm test` hook. |

<br/>

## 리소스 간 의존 관계

`ingress`, `httproute`, `servicemonitor`, `tests`는 `service`가 필요합니다. 모든 workload는 `serviceaccount`가 필요하고, `rbac`도 그렇습니다.

`hck add`는 충족되지 않은 의존성과 그걸 고치는 명령을 **보고만** 합니다. 대신 추가해 주지는 않습니다 — 차트는 어디까지나 사용자가 책임지는 것이고, 요청하지도 않은 Service를 조용히 만들어 주는 도구는 알려주는 도구보다 이해하기 어렵기 때문입니다.

<br/>

## hck 자체에 리소스 추가하기

1. `internal/render/templates/resources/<name>/`에 `template.yaml.tmpl`, `values.yaml.tmpl`, `schema.json.tmpl`을 만듭니다. 생성 계층은 `[[ ]]` 델리미터를 쓰므로 Helm의 `{{ }}`는 그대로 통과합니다.
2. `internal/catalog/catalog.go`의 `resources`에 항목을 추가합니다. `ValuesKeys`도 함께요.

카탈로그, values 조각, 스키마 조각은 서로 교차검증됩니다. 리소스나 values 키를 일부에만 선언하면 빌드가 실패합니다. [DEVELOPMENT-ko.md](DEVELOPMENT-ko.md)를 보세요.
