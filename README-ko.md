# helm-chart-kit

[![CI](https://github.com/somaz94/helm-chart-kit/actions/workflows/ci.yml/badge.svg)](https://github.com/somaz94/helm-chart-kit/actions/workflows/ci.yml)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Latest Tag](https://img.shields.io/github/v/tag/somaz94/helm-chart-kit)](https://github.com/somaz94/helm-chart-kit/tags)
[![Top Language](https://img.shields.io/github/languages/top/somaz94/helm-chart-kit)](https://github.com/somaz94/helm-chart-kit)

`hck` — 차트를 만든 뒤에도 계속 손볼 수 있는 단일 바이너리 차트 스캐폴더입니다.

> 자세한 문서는 [docs/](docs/) 폴더에 있습니다:
>
> [사용법](docs/USAGE-ko.md) |
> [리소스](docs/RESOURCES-ko.md) |
> [개발](docs/DEVELOPMENT-ko.md)
>
> English: [README.md](README.md) |
> [Usage](docs/USAGE.md) |
> [Resources](docs/RESOURCES.md) |
> [Development](docs/DEVELOPMENT.md)

<br/>

## 이 도구가 푸는 문제

`helm create`는 파일 여섯 개를 쓰고 끝납니다. 실제로 운영에 들어가는 차트는 그 뒤로 PodDisruptionBudget, NetworkPolicy, ServiceMonitor, HTTPRoute를 하나씩 달게 되는데, 그때마다 "그거 있던 차트가 어디였더라" 하며 복사해 옵니다.

비싼 건 붙여넣기가 아닙니다. `values.yaml`이 비쌉니다. 새 키를 넣어야 하고, 설명도 달아야 하고, 이미 있는 400줄이 그 편집을 무사히 넘겨야 합니다. 그래서 손으로 옮기게 되고, 주석은 따라오지 않고, 차트마다 섹션이 조금씩 어긋납니다.

`hck add`가 그 단계를 대신합니다:

```bash
$ hck add servicemonitor
updated payments-api

  +  templates/servicemonitor.yaml
  ~  values.yaml

  values.yaml wrote: metrics

  note: servicemonitor needs a CRD the cluster may not have (monitoring.coreos.com/v1)
```

`values.yaml`은 **덧붙여질 뿐 다시 쓰이지 않습니다.** 편집 전에 있던 바이트가 편집 후에도 바이트 단위로 그대로 남습니다 — 구분 배너도, 빈 줄도, 줄 끝 주석도. 병합이 텍스트 기반이고 추가만 하기 때문입니다. 이미 있는 키는 보고만 하고 건드리지 않습니다.

<br/>

## `helm create`로는 왜 안 되나

| | `helm create` | `hck` |
|---|---|---|
| **만들 수 있는 리소스** | 6개 고정 | 32개, 조합 가능 |
| **기존 차트에 추가** | — | `hck add <resource>` |
| **추가 시 `values.yaml`** | — | 덧붙임, 기존 바이트 그대로 |
| **값 설명** | 거의 없음 | 모든 키에 그 키가 있는 이유가 적혀 있음 |
| **Gateway API, Istio, ServiceMonitor, ExternalSecret** | — | 있음 |
| **preset** | 한 가지 형태 | 11개: `web`, `gateway`, `mesh`, `worker`, `queue`, `stateful`, `daemon`, `cronjob`, `monitored`, `secure`, `minimal` |
| **플랫폼별 값** | 없음 | `aws`, `gcp`, `azure`, `onprem` 오버레이 |
| **환경별 값** | 없음 | `dev`, `staging`, `prod` 오버레이 |
| **커스텀 시작점** | `--starter`, 차트 통째로만 | 리소스 단위, 조합 가능 |
| **검증** | `helm lint` | `helm template` + `helm lint` + 자체 규칙 |
| **이미지 태그** | `appVersion`으로 폴백 | 필수 — 없으면 렌더가 실패 |
| **한 차트에 workload 둘** | 허용 | 이유와 함께 거부 |
| **멱등성** | 덮어씀 | 절대 덮어쓰지 않고, 보고한 뒤 멈춤 |

<br/>

## 설치

```bash
# Homebrew (macOS, Linux)
brew install --cask somaz94/tap/helm-chart-kit

# Scoop (Windows)
scoop bucket add somaz94 https://github.com/somaz94/scoop-bucket
scoop install helm-chart-kit

# curl
curl -fsSL https://raw.githubusercontent.com/somaz94/helm-chart-kit/main/scripts/install.sh | bash

# go install
go install github.com/somaz94/helm-chart-kit/cmd@latest
```

`hck new`와 `hck add`는 런타임 의존성이 없습니다. `hck check`는 일부러 바깥의 `helm`을 그대로 실행합니다 — 그래야 내장 사본이 아니라 **당신의 helm**이 실제로 하는 일을 보고할 수 있습니다.

<br/>

## 빠른 시작

대부분은 명령 셋으로 끝납니다:

```bash
hck new payments-api      # 차트 만들기
hck add servicemonitor    # 있는 차트에 추가
hck check                 # 렌더하고 자체 규칙 적용
```

어떤 플래그가 필요한지 모르겠다면 `hck init`이 대신 물어보고, 끝나면 같은
결과를 내는 명령을 알려줍니다:

```bash
hck init payments-api
```

아래는 전부 opt-in입니다. JSON Schema도, 값 표도, 플랫폼 오버레이도 요청하지
않은 차트에는 생기지 않습니다 — 위의 세 명령으로 충분하다면 그걸로 끝입니다.

<details>
<summary>나머지 기능 한눈에 보기</summary>

```bash
# preset 이 새 차트의 출발점을 정합니다
hck new billing-worker --preset worker     # Service 없음, ingress 경로 없음
hck new sessions --preset stateful         # 레플리카별 볼륨을 가진 StatefulSet
hck new log-shipper --preset daemon        # 모든 노드에 DaemonSet
hck new edge --preset gateway              # Ingress 대신 HTTPRoute
hck new checkout --preset mesh             # Istio 라우팅, 접근 정책, mTLS
hck new mailer --preset queue              # 큐 길이로 KEDA 가 스케일
hck new payments --preset monitored        # web 에 Prometheus 한 벌 추가
hck new tokens --preset secure             # web 에 RBAC, TLS, ESO 추가
hck new tiny --preset minimal              # Deployment 와 Service 뿐

# 여러 개를 한 번에, 또는 먼저 무슨 일이 일어날지 보기
hck add pdb networkpolicy
hck add httproute --dry-run

# 다시 빼기. 템플릿만 지우고 values.yaml 은 절대 다시 쓰지 않습니다
hck remove ingress
hck remove hpa pdb --dry-run

# 지금의 hck 가 생성하는 것과 더 이상 같지 않은 템플릿 확인
hck sync
hck sync --check                           # CI 게이트
hck sync --write deployment                # hck 쪽 버전으로 가져오기

# 실제 values 로 검사하거나, 경고도 실패로 취급
hck check -f values/prod.yaml --strict
hck check --off HCK025                     # 이 차트는 CPU limit 을 쓰겠다는 뜻

# hck 가 거부하는 것에 대한 탈출구: 두 번째 워크로드, 비어 있지 않은 디렉터리.
# 이미 있는 파일은 건드리지 않습니다
hck new sidecar-pair --preset web --with daemonset --force

# 플랫폼 오버레이: EKS / GKE / AKS / 자체 운영에서 달라지는 것만
hck new payments-api --platform aws
hck platform add gcp azure
hck check --platform aws                   # 거기에 설치된 상태로 검사

# 환경 오버레이: 얼마나 세게 돌릴 것인가
hck new payments-api --env dev,prod
hck env add staging
hck check --platform aws --env prod        # 두 축 동시에

# 값을 JSON Schema 로 기술하거나 표로 문서화
hck schema --write
hck docs --write
hck schema --check && hck docs --check     # CI 게이트

# 무엇이 있는지 둘러보기
hck list
hck list rules                             # check 규칙과 그 ID
```

</details>

<br/>

## `hck init`

아래 플래그들은 익혀둘 가치가 있지만, 첫 차트에서부터 그럴 필요는 없습니다:

```console
$ hck init payments-api
Chart name? [payments-api]
  presets:
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

마지막에 **같은 결과를 내는 명령을 출력합니다.** 질문은 첫 차트를 위한 것이고 플래그는 그다음부터를 위한 것이기 때문입니다.이 동등성은 주장이 아니라 검증된 사실입니다 — `TestInitPrintsAWorkingEquivalent`가 출력된 명령을 실제로 실행해서 두 디렉터리를 파일 단위로 비교합니다.

`--defaults`는 아무것도 묻지 않고, 입력이 중간에 끊기면 남은 질문은 기본값으로 채워집니다. 그래서 앞의 두 질문만 답하는 heredoc으로도 돌릴 수 있습니다.

<br/>

## 차트가 처음 갖는 것

```
payments-api/
├── Chart.yaml
├── .helmignore
├── values.yaml                       # 리소스마다 설명이 달린 섹션 하나
├── ci/
│   └── install-values.yaml           # 렌더를 가능하게 하는 최소 오버라이드. .helmignore가 패키지에서 제외
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

첫 실행부터 `hck check`를 지적 사항 없이 통과합니다.

`--schema`를 주면 `values.yaml` 옆에 `values.schema.json`이 함께 생깁니다.

<br/>

## `values.schema.json`

Helm은 렌더할 때마다 차트의 값을 `values.schema.json`에 비춰 검증합니다. 이건 양날입니다 — 좋은 스키마는 오타를 에러 메시지로 바꿔주지만, **어설픈 스키마는 잘 돌던 values 파일을 실패한 릴리스로 바꿉니다.** `hck`가 이 파일을 직접 쓰라고 하지 않고 생성해 주는 이유, 그리고 opt-in인 이유가 그것입니다:

```bash
hck new payments-api --schema      # 스키마와 함께 생성
hck schema --write                 # 스키마가 없는 기존 차트에 추가
hck schema                         # 쓰지 않고 출력만
hck schema --check                 # 디스크의 파일이 낡았으면 실패
```

차트가 스키마를 갖게 되면 `hck add`가 함께 맞춰줍니다 — values 키를 더하는 리소스는 같은 실행에서 그 키의 스키마도 함께 더합니다.

생성되는 스키마는 **일부러 관대합니다.** 객체는 열어두고, 기본값이 빈 스칼라에는 실제로 받아들이는 값들의 union 타입을 매깁니다. 그래서 `service.nodePort`는 처음 들어 있는 빈 문자열만이 아니라 문자열과 정수를 모두 받습니다. 대신 **쿠버네티스가 실제로 제약하는 것**은 제약합니다 — `image.pullPolicy`는 셋 중 하나, `service.type`은 넷 중 하나, `cronjob.concurrencyPolicy`는 셋 중 하나입니다.

`--strict`는 최상위를 닫습니다. 선언되지 않은 최상위 키가 조용히 무시되는 대신 에러가 됩니다:

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

이때도 중첩된 객체는 열린 채입니다 — 목적은 오타를 잡는 것이지 쿠버네티스 API를 모델링하는 게 아니니까요. `global`은 항상 허용되므로 서브차트 값도 그대로 동작합니다.

`hck schema --check`가 CI 게이트입니다. 스키마를 다시 만들어 커밋된 파일과 다르면 실패하므로, **`values.yaml`만 손으로 고치고 스키마가 따라가지 않은 경우**를 잡아냅니다.

<br/>

## 플랫폼 오버레이

같은 차트라도 EKS, GKE, AKS, 자체 운영 클러스터에서 원하는 값이 다릅니다 — 여기선 IAM 롤 어노테이션, 저기선 ingress 클래스, 또 다른 데선 스토리지 클래스. `hck`는 그 차이를 오버레이로 만들어 줍니다:

```bash
hck new payments-api --platform aws          # values-aws.yaml과 함께 생성
hck platform add gcp azure                   # 이미 가진 차트에 추가
hck platform list                            # 무엇이 있고, 이 차트는 무엇을 가졌는지
```

오버레이는 **대체가 아닙니다.** Helm은 언제나 `values.yaml`을 먼저 읽으므로, 이 파일은 **달라지는 것만** 담습니다:

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

차트가 **실제로 가진 리소스만** 오버레이에 값을 내놓습니다. `worker` 차트는 ServiceAccount 어노테이션은 받되 ingress 클래스는 받지 않습니다 — Ingress가 없으니까요. 플랫폼별로 달라지는 게 하나도 없는 차트에는 헤더만 있는 파일 대신 **아무 파일도 만들어 주지 않습니다.**

**`hck check --platform aws`는 오버레이를 적용한 상태로 차트를 렌더합니다.** 렌더되지 않는 오버레이는 없느니만 못합니다 — 누군가 그걸로 설치하기 직전까지는 멀쩡한 설정처럼 보이기 때문입니다.

| 플랫폼 | 다루는 것 |
|---|---|
| `aws` | IRSA, ALB ingress, NLB service, gp3 볼륨, 존 분산, Secrets Manager |
| `gcp` | Workload Identity, GCE ingress, NEG service, pd-balanced 볼륨, Secret Manager |
| `azure` | Workload Identity(다들 잊는 파드 라벨 포함), Application Gateway, managed-csi, Key Vault |
| `onprem` | ingress-nginx, MetalLB, 직접 제공하는 스토리지 클래스, Vault, 사설 레지스트리 pull secret |

<br/>

## 환경 오버레이

플랫폼이 *어디에 올리는가*를 말한다면, 환경은 *얼마나 세게 굴리는가*를 말합니다. 둘은 직교하고, 겹쳐 쌓입니다:

```bash
helm install app . -f values-aws.yaml -f values-prod.yaml
#                     ↑ EKS에            ↑ 레플리카 3, 중단 예산, 엄격한 프로브
```

환경이 **마지막**에 옵니다. `helm`은 `-f`를 왼쪽에서 오른쪽으로 적용하므로, prod가 요구하는 크기가 플랫폼 오버레이의 값을 이깁니다.

| 환경 | 형태 |
|---|---|
| `dev` | 레플리카 1, 작은 요청량, 중단 예산 없음, 느긋한 liveness probe — 디버거에 멈춰 있는 파드는 죽은 파드가 아닙니다 |
| `staging` | 레플리카 2, prod의 비율을 유지한 축소판, HPA와 NetworkPolicy를 켜둠 — 프로덕션이 의존하기 전에 둘 다 실제로 굴려봅니다 |
| `prod` | 레플리카 3, `maxUnavailable: 0` 롤아웃, 종료 유예 60초, HPA 3–20(빠르게 늘리고 천천히 줄임), PDB, `helm test` 끔 |

```bash
hck new payments-api --env dev,prod
hck env add staging
hck env list
hck check --env prod --strict
```

<br/>

## `hck docs`

`values.yaml`은 이미 스스로를 설명합니다 — 모든 키가 helm-docs 관례인 `-- ` 주석을 달고 있습니다. `hck docs`는 그걸 다시 읽어 표로 만듭니다:

```console
$ hck docs
| Key | Type | Default | Description |
|---|---|---|---|
| `replicaCount` | int | `1` | Number of pod replicas. Ignored when autoscaling.enabled is true. |
| `image.pullPolicy` | string | `IfNotPresent` | Image pull policy. One of: `Always`, `IfNotPresent`, `Never`. |
| `service.nodePort` | string/int/null | `""` | Fixed node port. Only read when type is NodePort. |
```

타입과 허용값은 **스키마에서** 옵니다. `values.yaml`은 그걸 표현할 수 없기 때문입니다 — 기본값이 `""`인 키는 문자열을 받는지 숫자를 받는지 아무 말도 하지 않고, API가 받아들이는 네 가지 값에 대해서는 더더욱 그렇습니다. 이를 위해 `values.schema.json`이 커밋돼 있을 필요는 없습니다. 파일이 없으면 즉석에서 조립해 씁니다.

`--write`는 차트 `README.md` 안의 두 마커 사이만 교체하고 나머지는 그대로 둡니다:

```markdown
<!-- hck:values:start -->
... 생성된 표 ...
<!-- hck:values:end -->
```

README가 없는 차트에는 하나 만들어 줍니다. `hck docs --check`가 CI 게이트입니다.

<br/>

## 차트에 박아 넣은 의견들

생성된 차트는 중립적이지 않습니다.이 도구가 내린 판단과 그 이유는 이렇습니다.

**`image.tag`는 필수입니다.** `appVersion` 폴백이 없습니다. `appVersion`에 뭐가 적혀 있든 조용히 배포하는 차트가 바로 낡은 이미지가 프로덕션까지 올라가는 경로입니다. 대신 렌더가 실패합니다.

**CPU limit은 없고, memory limit은 항상 있습니다.** CPU limit에 걸리는 CFS 스로틀링은 노드가 바빠지기 한참 전부터 레이턴시 스파이크를 만듭니다. 메모리는 압축 불가능하므로 상한을 둡니다.

**기본값은 restricted Pod Security Standard입니다.** `runAsNonRoot`, `readOnlyRootFilesystem`, 모든 capability 제거. 관대하게 시작해서 조이는 대신, 의도적으로 풀어 쓰는 쪽입니다.

**NetworkPolicy는 기본 거부이고 DNS만 열려 있습니다.** 두 `policyTypes`를 빈 규칙 목록으로 렌더하므로 전부 거부됩니다. `kube-system` DNS egress 규칙은 기본으로 켜두는데, 그게 없으면 호스트명 기반 규칙이 전부 조용히 실패합니다.

**차트당 workload는 하나입니다.** Deployment가 있는 차트에 `hck add statefulset`은 거부되고, `hck new --preset web --with daemonset`도 마찬가지입니다. 둘은 같은 values 키를 호환되지 않는 형태로 놓고 다투므로, 결과물은 렌더는 되지만 apply되지 않습니다. 이미 그런 차트라면 `hck check`가 `HCK030`으로 보고합니다.

**`serviceAccount.automount`는 꺼져 있습니다.** 토큰은 자격증명입니다. 워크로드가 실제로 API 서버를 호출할 때 켜세요.

<br/>

## `hck check`가 강제하는 자체 규칙

| 규칙 | 심각도 | 잡아내는 것 |
|---|---|---|
| `HCK001` | error | 차트가 렌더되지 않음 |
| `HCK002` | warn | `helm lint` 지적 사항 |
| `HCK010`–`HCK013` | warn | `values.yaml`·`.helmignore` 없음, 잘못된 `apiVersion`, 빈 description |
| `HCK020` | warn | 파드가 `runAsNonRoot`를 설정하지 않음 |
| `HCK021` | error | 컨테이너에 이미지가 없거나 태그가 없음 |
| `HCK022` | error | 이미지 태그가 `:latest` |
| `HCK023` | warn | 리소스 요청량 없음 |
| `HCK024` | warn | memory limit 없음 |
| `HCK025` | warn | CPU limit이 설정됨 |
| `HCK026` | warn | `allowPrivilegeEscalation`이 꺼져 있지 않음 |
| `HCK027` | error | 컨테이너가 privileged로 실행됨 |
| `HCK028`–`HCK029` | warn | 장기 실행 워크로드에 readiness/liveness probe 없음 |
| `HCK030` | warn | 차트가 primary workload를 둘 이상 렌더함 |
| `HCK031` | warn | HPA와 KEDA ScaledObject가 동시에 워크로드를 스케일링 |
| `HCK032` | warn | 파드를 축출하는 update mode의 VPA가 HPA와 공존 |
| `HCK033` | warn | 스케일러가 차트에 없는 workload를 가리킴 |
| `HCK034` | warn | 차트가 만든 Issuer를 자기 Certificate가 쓰지 않음 |
| `HCK035` | warn | Service가 아무도 선언하지 않은 컨테이너 포트 이름으로 넘김 |
| `HCK036` | warn | PodDisruptionBudget이 자발적 중단을 영영 허용하지 않음 |

경고는 기본적으로 통과하고, `--strict`를 주면 실패합니다. `hck list rules`를 실행하면 `hck check`가 쓰는 문구 그대로 같은 표를 볼 수 있습니다.

규칙에 동의하지 않는 차트는 `Chart.yaml` 옆에 자기 `.hck.yaml`을 두고 그렇게 밝힙니다.

```yaml
rules:
  HCK025: off      # this chart wants its CPU limits
  HCK023: error    # and will not ship without requests
```

각 규칙은 `off`, `warn`, `error` 중 하나를 받습니다. 없는 ID를 적으면 조용히 무시하지 않고 오류를 냅니다. `HCK025`를 적어 두는 목적 자체가 그 지적을 그만 보는 것인데, 철자가 틀려서 계속 보고되는 상태는 규칙이 옳아서 계속 보고되는 상태와 구별되지 않기 때문입니다. `HCK001`만 예외로, 렌더되지 않는 차트에는 더 보고할 것이 없으므로 설정 대상이 아닙니다. 차트가 꺼 둔 규칙은 지적 사항과 함께 출력됩니다. 규칙 절반을 꺼 놓은 차트에서 나온 깨끗한 결과는 보이는 것만큼을 말해 주지 않기 때문입니다.

CI에서는 `--format json`이 같은 실행 결과를 문서로 내놓습니다. `ok` 필드는 종료 코드와 일치합니다.

```bash
hck check --format json --strict
```

<br/>

## 라이선스

[Apache 2.0](LICENSE)
