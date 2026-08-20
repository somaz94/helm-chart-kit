# 사용법

> English: [USAGE.md](USAGE.md) · 한국어 문서 모음: [README-ko.md](../README-ko.md)

<br/>

## hck init

질문 몇 개에 답해서 차트를 만듭니다.

```bash
hck init [chart-name] [flags]
```

| 플래그 | 기본값 | 의미 |
|---|---|---|
| `-d, --dir` | `.` | 차트를 만들 상위 디렉터리 |
| `--defaults` | `false` | 아무것도 묻지 않고 전부 기본값 |

모든 질문은 `hck new`의 플래그와 일대일로 대응하고, `init`은 끝날 때 **같은 결과를 내는 명령을 출력합니다.** 질문은 첫 차트를 위한 것이고, 플래그는 그다음부터를 위한 것입니다.

Enter를 누르면 대괄호 안의 기본값이 적용됩니다. 입력이 중간에 끊겨도 남은 질문은 기본값이 되므로, 아래처럼 돌려도 됩니다:

```bash
printf 'payments-api\nweb\n' | hck init
```

<br/>

## hck new

preset의 리소스로 채운 차트 디렉터리를 만듭니다.

```bash
hck new <chart-name> [flags]
```

| 플래그 | 기본값 | 의미 |
|---|---|---|
| `-p, --preset` | `web` | 시작 리소스 세트 |
| `-d, --dir` | `.` | 차트를 만들 상위 디렉터리 |
| `--with` | — | preset 위에 얹을 추가 리소스. 쉼표 구분 또는 반복 지정 |
| `--description` | `A Helm chart for <name>` | `Chart.yaml`의 description |
| `--version` | `0.1.0` | 차트 버전 |
| `--app-version` | `1.0.0` | 차트가 배포하는 애플리케이션의 버전 |
| `--schema` | `false` | `values.schema.json`도 함께 생성 |
| `--schema-strict` | `false` | `values.schema.json` 생성 + 선언되지 않은 최상위 키 거부 |
| `--platform` | — | 생성할 플랫폼 오버레이. 쉼표 구분: `aws`, `gcp`, `azure`, `onprem` |
| `--env` | — | 생성할 환경 오버레이. 쉼표 구분: `dev`, `staging`, `prod` |
| `--dry-run` | `false` | 무엇을 쓸지만 출력하고 종료 |

```bash
hck new payments-api
hck new billing-worker --preset worker
hck new sessions --preset stateful --with servicemonitor
hck new gateway --preset web --with httproute,prometheusrule --dry-run
hck new payments-api --schema
```

차트 이름은 소문자 DNS 라벨이어야 하고(Helm 자체 제약입니다), 대상 디렉터리는 비어 있거나 없어야 합니다.

<br/>

## hck add

이미 있는 차트에 리소스 템플릿을 추가합니다.

```bash
hck add <resource>... [flags]
```

| 플래그 | 기본값 | 의미 |
|---|---|---|
| `--chart` | `.` | 차트 디렉터리. 상위로 올라가며 `Chart.yaml`을 찾습니다 |
| `--dry-run` | `false` | 무엇을 쓸지만 출력하고 종료 |
| `--force` | `false` | 기존 템플릿을 덮어쓰고 두 번째 workload를 허용 |

```bash
hck add servicemonitor
hck add pdb networkpolicy
hck add externalsecret --chart ./charts/payments-api
```

`git`처럼 위로 거슬러 올라가므로, 차트 안 어디에서 실행해도 됩니다.

<br/>

### add가 보장하는 것

**아무것도 덮어쓰지 않습니다.** 이미 있는 템플릿 파일은 보고하고 건너뜁니다. 이미 있는 `values.yaml` 키도 보고만 하고 그대로 둡니다.

**`values.yaml`은 덧붙여질 뿐 다시 쓰이지 않습니다.** 병합이 텍스트 기반이고 추가만 하므로, 파일에 이미 있던 것은 바이트 단위로 살아남습니다 — 섹션 배너, 빈 줄, 줄 끝 주석, 키 순서까지. YAML 디코더와 인코더로 왕복시키면 키는 보존되지만 나머지는 사라집니다. 그래서 여기서는 그렇게 하지 않습니다.

**키의 설명은 키를 따라갑니다.** 키 위의 주석 블록은 그 키의 블록에 속하므로, `values.yaml`에 키와 함께 들어갑니다.

**같은 명령을 반복해도 아무 일도 일어나지 않습니다.** `hck add pdb`를 두 번 하면 한 번만 쓰고, 두 번째는 `nothing to do`를 보고합니다.

**의존성은 보고할 뿐 끌어오지 않습니다.** Service가 없는 차트에 `ingress`를 추가하면 무엇이 없는지와 그걸 고치는 명령을 출력합니다. 요청하지도 않은 Service를 조용히 만들어 주지 않습니다.

<br/>

## hck check

설치돼 있는 `helm`으로 차트를 렌더한 뒤, 나온 매니페스트에 자체 규칙을 적용합니다.

```bash
hck check [flags]
```

| 플래그 | 기본값 | 의미 |
|---|---|---|
| `--chart` | `.` | 차트 디렉터리 |
| `-f, --values` | — | helm에 넘길 values 파일. 반복 지정 가능 |
| `--platform` | — | 함께 적용할 플랫폼 오버레이. 쉼표 구분 |
| `--env` | — | 함께 적용할 환경 오버레이. 쉼표 구분. `--platform` 뒤에 적용되므로 이쪽이 이깁니다 |
| `--strict` | `false` | 경고도 실패로 취급 |
| `--print` | `false` | 렌더된 매니페스트를 출력 |
| `--no-render` | `false` | helm을 건너뛰고 차트 디렉터리만 읽는 규칙만 실행 |

```bash
hck check
hck check --chart ./charts/payments-api
hck check -f values/prod.yaml --strict
hck check --no-render          # helm 없이도 동작
```

`-f`를 주지 않았고 차트에 `ci/install-values.yaml`이 있으면 그 파일을 씁니다. `image.tag`가 필수인 차트는 기본값만으로 렌더되지 않습니다 — 필수로 둔 이유가 바로 그것입니다. 그래서 이 CI values 파일이 있어야 애초에 검사가 가능합니다.

`hck check`는 error 지적이 하나라도 있으면 0이 아닌 코드로 종료합니다. `--strict`를 주면 경고만 있어도 마찬가지입니다.

<br/>

## hck schema

차트가 가진 리소스로부터 `values.schema.json`을 조립합니다.

```bash
hck schema [flags]
```

| 플래그 | 기본값 | 의미 |
|---|---|---|
| `--chart` | `.` | 차트 디렉터리 |
| `--write` | `false` | 차트에 `values.schema.json` 기록 |
| `--check` | `false` | 디스크의 파일이 생성될 내용과 다르면 실패 |
| `--strict` | 기존 파일의 설정 | 선언되지 않은 최상위 키를 거부 |

```bash
hck schema                     # 출력
hck schema --write             # 차트에 기록
hck schema --write --strict    # 최상위를 닫은 채로 기록
hck schema --check             # CI 게이트
```

플래그가 없으면 스키마는 stdout으로 나가고, 차트는 그대로 남습니다. `--write`와 `--check`는 함께 쓸 수 없습니다.

Helm은 **매 렌더마다** 병합된 값을 이 파일에 비춰 검증합니다. 그래서 생성되는 스키마는 일부러 관대합니다 — 객체는 열어두고, 기본값이 빈 스칼라에는 실제로 받아들이는 값들의 union 타입을 매깁니다. 어설픈 스키마는 차트를 문서화하는 게 아니라 **망가뜨립니다.**

`--strict`는 최상위만 닫습니다. 중첩된 객체는 열린 채인데, 목적이 오타를 잡는 것이지 쿠버네티스 API를 모델링하는 게 아니기 때문입니다. `global`은 항상 허용되므로 서브차트 값도 계속 동작합니다.

`--strict`는 한 번 정하면 계속 따라갑니다. 한 번 strict로 스키마를 쓰면 `hck schema`와 `hck add` 모두 그 상태를 유지합니다. 의도적으로 풀려면 `--strict=false`를 주세요.

<br/>

### 최신 상태 유지하기

스키마를 가진 차트는 `values.yaml`이 선언하는 **모든** 키를 기술해야 합니다. 아니면 helm이 차트를 거부합니다. 이를 유지하는 장치가 둘 있습니다:

- `hck add`는 `values.yaml`에 덧붙일 때마다 스키마를 다시 만듭니다. 스키마가 없는 차트에는 새로 만들지 않습니다 — 이 파일은 opt-in입니다.
- `hck schema --check`는 다시 만들어 비교하므로, **`values.yaml`만 손으로 고치고 스키마가 따라가지 않은 경우**를 CI가 잡아냅니다.

<br/>

## hck platform

플랫폼 오버레이는 대상마다 달라지는 값만 담고, 그 외에는 아무것도 담지 않습니다.

```bash
hck platform list                    # 아는 플랫폼과, 이 차트가 가진 것
hck platform add aws                 # values-aws.yaml 생성
hck platform add gcp azure           # 여러 개 한 번에
hck platform add onprem --dry-run
hck platform add aws --force         # 이미 있는 것을 다시 쓰기
```

| 플랫폼 | 다루는 것 |
|---|---|
| `aws` | IRSA, ALB ingress, NLB service, gp3 볼륨, 존 분산, Secrets Manager |
| `gcp` | Workload Identity, GCE ingress, NEG service, pd-balanced 볼륨, Secret Manager |
| `azure` | Workload Identity(필수 파드 라벨 포함), Application Gateway, managed-csi, Key Vault |
| `onprem` | ingress-nginx, MetalLB, 직접 제공하는 스토리지 클래스, Vault, 사설 레지스트리 pull secret |

오버레이는 대체가 아닙니다. Helm은 언제나 `values.yaml`을 먼저 읽으므로, 생성되는 파일은 **차이만** 담습니다:

```bash
helm install payments-api . -f values-aws.yaml
```

차트가 실제로 가진 리소스만 오버레이에 값을 내놓습니다. `worker` 차트는 ServiceAccount 어노테이션은 받되 ingress 클래스는 받지 않습니다 — Ingress가 없으니까요. 플랫폼별 차이가 하나도 없는 차트는 헤더만 있는 파일 대신 아무 파일도 받지 않습니다.

`hck platform add`는 이미 있는 오버레이를 절대 덮어쓰지 않습니다. 자유롭게 편집하세요. 덮어쓰려면 `--force`를 주세요.

<br/>

### 오버레이 검사하기

```bash
hck check --platform aws
hck check --platform aws,gcp --strict
```

**오버레이를 적용한 상태로** 차트를 렌더합니다. 그것이 오버레이가 애초에 렌더되기는 하는지 알 수 있는 유일한 방법입니다. 오버레이는 더해지는 것이라 `ci/install-values.yaml`을 대체하지 않고 그 위에 얹힙니다. 그래야 차트가 요구하는 이미지 태그를 여전히 받습니다.

<br/>

## hck env

환경 오버레이는 차트를 얼마나 세게 돌릴 것인지를 담습니다. 플랫폼과 직교하고, 플랫폼 뒤에 쌓입니다.

```bash
hck env list
hck env add prod
hck env add dev staging
hck env add prod --dry-run
hck env add prod --force
```

| 환경 | 형태 |
|---|---|
| `dev` | 레플리카 1, 작은 요청량, 중단 예산 없음, 느긋한 liveness probe, CronJob 정지 |
| `staging` | 레플리카 2, prod의 비율을 유지한 축소판, HPA와 NetworkPolicy 켬 |
| `prod` | 레플리카 3, `maxUnavailable: 0`, 종료 유예 60초, HPA 3–20, PDB, `helm test` 끔 |

```bash
helm install app . -f values-aws.yaml -f values-prod.yaml
```

환경이 마지막에 오는 이유는 `helm`이 `-f`를 왼쪽에서 오른쪽으로 적용하기 때문입니다. prod가 요구하는 레플리카 수가 플랫폼 오버레이가 설정한 값을 이깁니다. `hck check --platform aws --env prod`도 helm에 같은 순서로 넘깁니다.

<br/>

## hck docs

`values.yaml`을 Markdown 표로 바꿉니다.

```bash
hck docs [flags]
```

| 플래그 | 기본값 | 의미 |
|---|---|---|
| `--chart` | `.` | 차트 디렉터리 |
| `--write` | `false` | 차트의 `README.md`에 표를 기록 |
| `--check` | `false` | README의 표가 생성될 내용과 다르면 실패 |

```bash
hck docs                       # 출력
hck docs --write               # README.md에 기록
hck docs --check               # CI 게이트
```

설명은 파일 자체에서 옵니다. `-- `로 시작하는 주석 줄이 그 아래 키를 설명합니다. **섹션 배너는 설명이 아닙니다** — `-- ` 접두사만이 설명을 시작하므로, `values.yaml`을 구획하는 배너들이 표로 새어 들어가지 않습니다.

타입과 허용값은 스키마에서 옵니다. `values.schema.json`이 커밋돼 있을 필요는 없습니다. 파일이 없으면 차트가 가진 리소스로부터 즉석에서 조립합니다.

`--write`는 `<!-- hck:values:start -->`와 `<!-- hck:values:end -->` 사이만 교체하고 나머지는 건드리지 않습니다. 마커가 없는 README에는 `## Values` 제목 아래에 마커째 덧붙이고, README가 없는 차트에는 하나 만들어 줍니다. `--write`와 `--check`는 함께 쓸 수 없습니다.

<br/>

## hck list

```bash
hck list              # preset과 리소스
hck list presets
hck list resources
```

`[crd]`가 붙은 리소스는 대상 클러스터에 없을 수도 있는 CRD나 기능 게이트가 있어야 동작합니다.

<br/>

## 셸 자동완성

```bash
hck completion zsh  > "${fpath[1]}/_hck"
hck completion bash > /etc/bash_completion.d/hck
hck completion fish > ~/.config/fish/completions/hck.fish
```

자동완성은 preset과 리소스 이름을 알고 있어서, `hck add <TAB>`을 누르면 추가 가능한 것들이 나옵니다.
