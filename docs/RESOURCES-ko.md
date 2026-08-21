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
| `minimal` | serviceaccount, deployment, service |
| `gateway` | serviceaccount, deployment, service, httproute, hpa, pdb, networkpolicy, configmap, tests |
| `mesh` | serviceaccount, deployment, service, virtualservice, destinationrule, authorizationpolicy, hpa, pdb, configmap, tests |
| `queue` | serviceaccount, deployment, scaledobject, pdb, networkpolicy, configmap |
| `monitored` | serviceaccount, deployment, service, ingress, hpa, pdb, networkpolicy, configmap, servicemonitor, prometheusrule, grafanadashboard, tests |
| `secure` | serviceaccount, rbac, deployment, service, ingress, certificate, externalsecret, hpa, pdb, networkpolicy, configmap, tests |
| `base` | serviceaccount, deployment, service, ingress, httproute, hpa, pvc, certificate, configmap, tests |
| `base-aws` | serviceaccount, deployment, service, ingress, hpa, pdb, externalsecret, pvc, configmap, tests |

모든 preset은 workload를 정확히 하나만 가지지만, 차트는 둘 이상 가질 수 있습니다. 이 둘은 다른 이야기입니다. preset이 하나를 고르는 건 preset이라면 무엇이든 골라야 하기 때문이고, `HCK030`이 보고하는 것은 **둘이 동시에 렌더되는** 차트입니다. 가드를 걸어 하나만 렌더되게 해 둔 차트가 workload 템플릿을 둘 **가지고 있는** 것은 흔한 형태입니다 — Deployment 옆에 Argo Rollout을 두고 각각 자기 `.enabled` 아래에서 하나의 pod 템플릿을 공유하는 식입니다. `hck add`와 `hck new`는 그런 차트를 만들어 주고, 무엇을 만들었는지 말해 줍니다. 둘 다 실제로 렌더되는지는 렌더에 대한 질문이고, 그래서 그 질문은 렌더에서 던집니다.

두 preset은 리소스 목록보다 많은 것을 답합니다. `base`와 `base-aws`는 어느 ApplicationSet 저장소가 나란히 두고 쓰는 하우스 차트 한 쌍을 본뜬 것입니다 — 같은 차트가 서로 다른 플랫폼에 답하는 형태이지요. 그래서 각자 자기가 쓰이도록 만들어진 플랫폼을 들고 있고, 원본 차트가 둘 다 갖고 있으므로 `values.schema.json`과 values 표도 함께 요청합니다.

| Preset | Platform | `values.schema.json` | Values table |
|---|---|---|---|
| `base` | `onprem` | yes | yes |
| `base-aws` | `aws` | yes | yes |

`hck init`이 이 값을 읽기 때문에 질문이 일곱 개가 아니라 세 개입니다. 나머지 preset은 이 값을 하나도 들고 있지 않으므로 기본 동작이 그대로입니다. 이건 결정이 아니라 기본값입니다. init은 preset이 정한 것을 보여 주고 아니라는 답도 받아 주며, `hck new`의 플래그는 여전히 전부 우선합니다.

`base`는 Ingress와 HTTPRoute를 둘 다 가집니다. `web`과 `gateway`가 각각 하나씩만 고르는 것과 다릅니다. 원본 차트가 각각을 자기 `.enabled`로 가드해 두고, 빌드 시점이 아니라 설치할 때마다 고르기 때문입니다.

담을 리소스가 없어 옮기지 못한 것들: 온프렘 쪽은 인증서 갱신 CronJob, `imagePullSecret`, 그리고 claim에 짝이 되는 `PersistentVolume`. AWS 쪽은 Argo Rollout과 그 AnalysisTemplate, EFS `PersistentVolume`, preview 환경용 Ingress 3종, preview Service입니다.


이 중 셋은 무엇을 뺐는지가 더 중요합니다. `mesh`에는 NetworkPolicy가 없습니다. 메시 안에서 "누가 이 워크로드를 호출할 수 있는가"는 AuthorizationPolicy가 L7에서 신원과 함께 답하는 질문이고, L3에서 같은 이름으로 한 번 더 답하면 서로 다른 두 답이 남습니다. `queue`에는 HPA가 없습니다. 레플리카 수는 KEDA `ScaledObject`가 가져가며, 둘이 함께 그 수를 움직이는 상황이 바로 `HCK031`이 보고하는 것입니다. `secure`는 Certificate만 두고 Issuer는 두지 않습니다. Certificate의 기본값은 공용 Issuer가 사는 ClusterIssuer이고, 네임스페이스 Issuer를 옆에 두고 둘을 연결하지 않는 것이 `HCK034`가 보고하는 상황입니다 — 차트 전용 Issuer가 필요하면 `hck add issuer`가 있습니다.

<br/>

## 그룹

카탈로그는 리소스 32개이고 이름은 전부 Kubernetes 종류(kind)입니다. 그래서 알파벳 목록은 *무엇이 있는가*만 답하고 그 이상은 답하지 못합니다. 모니터링 한 벌을 찾으려면 그게 `servicemonitor`, `prometheusrule`, `grafanadashboard`라고 불린다는 걸 이미 알고 있어야 했습니다.

모든 리소스는 그룹 하나에 속하고, `@`로 시작하는 이름은 그 그룹의 구성원 전체를 뜻합니다.

```bash
hck add @observability                  # the whole monitoring setup
hck new payments-api --with @secrets    # every way of getting a secret in
```

| Group | For | Resources |
|---|---|---|
| `@workload` | what runs | cronjob, daemonset, deployment, job, statefulset |
| `@exposure` | what reaches it | httproute, ingress, referencegrant, service |
| `@scaling` | how many, and what may evict them | hpa, pdb, scaledjob, scaledobject, vpa |
| `@access` | identity, permissions and who may connect | networkpolicy, rbac, serviceaccount |
| `@secrets` | secrets and the certificates that need them | certificate, externalsecret, issuer, sealedsecret, secret |
| `@observability` | scraping, alerting and dashboards | grafanadashboard, podmonitor, prometheusrule, servicemonitor |
| `@mesh` | Istio routing and policy | authorizationpolicy, destinationrule, virtualservice |
| `@chart` | configuration, storage and the chart's own install test | configmap, pvc, tests |

`hck list resources`는 이 순서로 출력합니다 — 무언가 돌고, 무언가 그것에 닿고, 스케일되고, 잠기고, 관측된다 — 대체로 결정이 내려지는 순서입니다. 그룹과 리소스는 서로 다른 이름 공간이고 `@`가 그 둘을 갈라 놓습니다. 나중에 그룹과 리소스가 같은 이름을 갖게 될 수 있는데, 접두사가 없으면 어느 쪽이 이기는지를 조회 순서가 정하게 됩니다.

그룹을 자기 구성원과 나란히 써도 한 번만 풀립니다. `--with @exposure,service`는 `@exposure`와 같습니다.

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
| `hpa` | `autoscaling/v2` | `autoscaling` | 메모리 타깃은 기본 null — 한 번 올라가면 내려오지 않기 때문입니다. `targetKind`는 차트의 workload를 따라갑니다. |
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
| `scaledobject` | `keda.sh/v1alpha1` | `scaledObject` | KEDA 필요. `autoscaling`은 같이 켜지 마세요 — KEDA가 자체 HPA를 만듭니다. `targetKind`는 차트의 workload를 따라갑니다. |
| `vpa` | `autoscaling.k8s.io/v1` | `verticalPodAutoscaler` | 기본 `updateMode: "Off"`. 따옴표 필수 — 없으면 YAML이 이 값을 `false`로 읽습니다. `targetKind`는 차트의 workload를 따라갑니다. |
| `sealedsecret` | `bitnami.com/v1alpha1` | `sealedSecret` | 암호문은 커밋해도 안전. 봉인된 값은 네임스페이스와 이름에 묶입니다. |
| `issuer` | `cert-manager.io/v1` | `issuer` | 네임스페이스 단위. 여러 차트가 ACME 계정을 공유한다면 ClusterIssuer가 낫습니다. |
| `referencegrant` | `gateway.networking.k8s.io/v1beta1` | `referenceGrant` | 다른 네임스페이스의 HTTPRoute가 이 Service에 닿게 허용합니다. |
| `scaledjob` | `keda.sh/v1alpha1` | `scaledJob` | 큐 항목당 Job 하나. 한꺼번에 몰리는 무거운 작업에 적합합니다. |
| `virtualservice` | `networking.istio.io/v1` | `virtualService` | Istio 필요. `route`는 이 차트의 Service가 기본값. Gateway는 클러스터 인프라라 여기서 만들지 않습니다. |
| `destinationrule` | `networking.istio.io/v1` | `destinationRule` | Istio 필요. 커넥션 풀 상한과 이상 엔드포인트 축출. 기본값은 `ISTIO_MUTUAL` — 여기서 `DISABLE`로 두면 STRICT PeerAuthentication이 거부합니다. |
| `authorizationpolicy` | `security.istio.io/v1` | `authorizationPolicy` | Istio 필요. `networkpolicy`의 L7 쪽 절반. 규칙이 빈 `ALLOW`는 전부 거부이며, 그것이 의도된 동작입니다. |
| `grafanadashboard` | `v1` | `grafanaDashboard` | Grafana 사이드카가 읽는 ConfigMap. 라벨이 틀리면 조용히 실패합니다. `templated`는 꺼짐 — Grafana 자체의 `{{ }}` 범례는 Helm 템플릿이 아닙니다. |
| `tests` | `v1` | `tests` | Service를 호출하는 `helm test` hook. |

<br/>

## 리소스 간 의존 관계

`ingress`, `httproute`, `servicemonitor`, `virtualservice`, `destinationrule`, `tests`는 `service`가 필요합니다. 모든 workload는 `serviceaccount`가 필요하고, `rbac`도 그렇습니다.

`hck add`는 충족되지 않은 의존성과 그걸 고치는 명령을 **보고만** 합니다. 대신 추가해 주지는 않습니다 — 차트는 어디까지나 사용자가 책임지는 것이고, 요청하지도 않은 Service를 조용히 만들어 주는 도구는 알려주는 도구보다 이해하기 어렵기 때문입니다.

<br/>

## hck 자체에 리소스 추가하기

1. `internal/render/templates/resources/<name>/`에 `template.yaml.tmpl`, `values.yaml.tmpl`, `schema.json.tmpl`을 만듭니다. 생성 계층은 `[[ ]]` 델리미터를 쓰므로 Helm의 `{{ }}`는 그대로 통과합니다.
2. `internal/catalog/catalog.go`의 `resources`에 항목을 추가합니다. `ValuesKeys`도 함께요.

카탈로그, values 조각, 스키마 조각은 서로 교차검증됩니다. 리소스나 values 키를 일부에만 선언하면 빌드가 실패합니다. [DEVELOPMENT-ko.md](DEVELOPMENT-ko.md)를 보세요.
