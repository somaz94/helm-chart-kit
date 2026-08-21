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

질문은 세 개입니다. 이름, preset, 그리고 그 위에 더할 것. 나머지는 preset이 답합니다 — 어느 플랫폼을 위해 쓰인 preset인지, `values.schema.json`을 만들지, values 표를 쓸지. `init`은 preset이 정한 값을 보여 주고 그대로 갈지 한 번만 묻습니다.

```
Extra resources? (comma-separated) [none]

  base also decided:
    platform overlays: onprem
    environment overlays: none
    values.schema.json: yes
    values table in README.md: yes

Keep those? [Y/n]
```

아니라고 답하면 그 네 개를 다시 묻는데, 각 질문의 기본값은 빈 값이 아니라 preset이 고른 값입니다. 이 값을 하나도 들고 있지 않은 preset — `base`와 `base-aws`를 뺀 전부 — 은 `hck new`가 원래 쓰던 기본값 그대로가 되므로, 그대로 가겠다고 답해도 달라지는 것이 없습니다.

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
| `--force` | `false` | 비어 있지 않은 디렉터리에도 씀 |

```bash
hck new payments-api
hck new billing-worker --preset worker
hck new sessions --preset stateful --with servicemonitor
hck new gateway --preset web --with httproute,prometheusrule --dry-run
hck new payments-api --schema
hck new blue-green --preset web --with daemonset
```

차트 이름은 소문자 DNS 라벨이어야 하고(Helm 자체 제약입니다), 대상 디렉터리는 비어 있거나 없어야 합니다.

두 번째 workload는 거부되지 않고 만들어진 뒤 알림이 붙습니다. 가드를 걸어 한 번에 하나만 렌더되는 workload 템플릿 두 개는 흔한 형태이고, 둘이 동시에 렌더되는 것은 그렇지 않아서 `hck check`가 `HCK030`으로 보고합니다 — 가드가 보이는 렌더 쪽에서요. `--force`는 여기에 관여하지 않습니다. 비어 있지 않은 디렉터리에 `--force`로 쓰면 **빠진 것만 채우고 아무것도 덮어쓰지 않습니다.** 이미 있는 파일은 전부 건너뛰고 — `values.yaml`이 그중 첫 번째입니다 — 계획에 하나씩 이름이 찍힙니다. 그래서 `hck new --force`는 템플릿을 잃어버린 차트를 되살리는 방법이지, 차트를 확장하는 방법이 아닙니다. 확장은 `hck add`입니다. 그쪽은 `values.yaml`을 건너뛰는 대신 덧붙입니다.

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
| `--force` | `false` | 기존 템플릿을 덮어씀 |

`@`로 시작하는 이름은 그룹이고 그 구성원 전체를 뜻합니다 — `hck add @observability`는 모니터링 한 벌 전부입니다. 그룹 목록은 [RESOURCES-ko.md](RESOURCES-ko.md#그룹)와 `hck list resources`에 있습니다. 그룹을 자기 구성원과 나란히 써도 한 번만 풀립니다. 그리고 그룹은 한 플랫폼에만 있는 리소스로는 확장되지 않습니다 — 그런 리소스는 이름으로 지목해야 하고, 그룹이 무엇을 뺐는지는 알려 줍니다.

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

## hck remove

차트에 더 이상 필요 없는 리소스의 템플릿 파일을 지웁니다.

```bash
hck remove <resource>... [flags]
hck rm <resource>...
```

| 플래그 | 기본값 | 의미 |
|---|---|---|
| `--chart` | `.` | 차트 디렉터리. 상위 디렉터리에서 `Chart.yaml`을 찾아 올라갑니다 |
| `--dry-run` | `false` | 무엇을 지울지만 출력하고 종료 |
| `--force` | `false` | 편집된 템플릿, 그리고 다른 리소스가 아직 필요로 하는 리소스도 삭제 |

```bash
hck remove ingress
hck remove hpa pdb --dry-run
hck rm servicemonitor --chart ./charts/payments-api
```

지우는 것은 템플릿뿐입니다. `values.yaml`은 절대 다시 쓰지 않습니다 — 이 도구 전체가 그 불변식 위에 서 있습니다. 그래서 지워진 리소스가 넣었던 키는 있던 자리에 그대로 남고, 대신 계획이 그 키를 알려 줍니다.

```
updated demo

  -  templates/hpa.yaml

  values.yaml still declares (now unused): autoscaling
```

그 키를 지우는 일은 부수 효과가 아니라 결정이 됩니다. 다음 두 가지 삭제는 `--force` 없이는 거부됩니다.

- **다른 리소스가 아직 필요로 하는 리소스.** Ingress 밑에서 Service를 빼면 렌더는 되지만 동작하지 않는 차트가 남고, helm이 돌기 전까지 아무도 그 사실을 말해 주지 않습니다. 둘을 함께 지우면 플래그가 필요 없습니다.
- **편집된 템플릿.** hck가 생성하는 것과 다른 파일은 누군가의 작업물이고, 리소스 이름을 잘못 친 것만으로 그것이 지워져서는 안 됩니다.

두 리소스가 공유하는 키는 다른 쪽이 아직 차트에 있는 한 고아가 되지 않습니다. `persistence`는 PVC의 것인 만큼 StatefulSet의 것이기도 합니다.

차트에 `values.schema.json`이 있으면 계획이 한 가지를 더 말합니다. 스키마는 여전히 그 고아 키들을 기술하고 있고, 다음 `hck add`가 그 키 없이 스키마를 다시 만듭니다. 그 시점에 strict 스키마는 values 파일을 거부합니다. 그전에 키를 지우십시오.

<br/>

## hck sync

차트의 생성된 파일 중 지금의 `hck`가 쓰는 것과 달라진 것을 보고합니다.

```bash
hck sync [resource...] [flags]
```

| 플래그 | 기본값 | 의미 |
|---|---|---|
| `--chart` | `.` | 차트 디렉터리 |
| `--check` | `false` | 하나라도 다르면 0이 아닌 코드로 종료 |
| `--write` | `false` | 지정한 템플릿을 hck가 생성하는 것으로 덮어쓰기 |
| `--all` | `false` | `--write`와 함께, 달라진 템플릿 전부를 가져오기 |
| `--format` | `text` | Output format: `text` or `json` (report only) |

```bash
hck sync                      # 무엇이 다른지
hck sync --check              # 같은 내용, 다르면 0이 아닌 종료 코드
hck sync --write ingress      # 이 템플릿들을 hck 쪽 버전으로
hck sync --write --all        # 전부 hck 쪽 버전으로
```

`--format json`은 같은 리포트를 문서로 냅니다. CI에서는 이쪽을 쓰세요. 위의 표는 사람이 읽으라고 만든 것이고, 그 열·기호·들여쓰기는 인터페이스가 아닙니다.

```console
$ hck sync --format json
{
  "chart": "payments-api",
  "files": [
    { "resource": "templates/_helpers.tpl", "path": "templates/_helpers.tpl", "state": "edited", "skeleton": true },
    { "resource": "ingress", "path": "templates/ingress.yaml", "state": "current", "skeleton": false }
  ],
  "ok": false
}
```

`state`는 `current`, `edited`, `missing`, `unreadable` 중 하나입니다. `edited`는 누군가 고쳤다는 뜻이 아닙니다 — hck는 그것과 자기 템플릿이 바뀐 것을 구분하지 못합니다. `Chart.yaml`과 `values.yaml`은 `current`로 보고되는 대신 **아예 없습니다.** 비교 대상이 아니고, 아무도 확인하지 않은 것을 "current"라고 말하는 셈이 되니까요.

```
sync demo

  = templates/deployment.yaml  current
  ~ templates/hpa.yaml         differs from what hck generates
  = templates/service.yaml     current
```

말해 줄 수 없는 것은 "왜"입니다. 직접 편집한 템플릿과 나중 버전의 hck가 개선한 템플릿은 여기서 보면 같은 바이트입니다. 둘 다 그저 지금의 hck가 생성하는 것이 아닐 뿐입니다. 기본 동작이 보고인 이유, `--write`가 이름을 받는 이유, 그리고 이름도 `--all`도 없는 `--write`가 거부되는 이유가 이것입니다. "이 파일이 바뀌어야 하는가"의 답은 그 파일을 편집한 사람의 것입니다.

`--write`는 덮어씁니다. 먼저 차이를 읽으십시오. 버전 관리 아래의 차트라면 쉬운 일이고, 아니라면 되돌릴 수 없는 일입니다.


### 무엇을 비교하는가

리소스 템플릿, 그리고 hck가 온전히 소유하는 스켈레톤 파일 네 개입니다.

| 파일 | 비교하는 이유 |
|---|---|
| `templates/_helpers.tpl` | 모든 리소스 템플릿이 이 파일을 `include` 합니다. hck가 헬퍼를 추가하면, 그 헬퍼가 없는 오래된 차트는 그대로 렌더됩니다. |
| `templates/NOTES.txt` | 설치와 업그레이드마다 출력됩니다. |
| `.helmignore` | 패키지에 무엇이 딸려 들어가는지를 결정합니다. |
| `ci/install-values.yaml` | `image.tag`에 폴백이 없으므로, 애초에 차트를 검사 가능하게 만드는 파일입니다. |

`Chart.yaml`과 `values.yaml`은 의도적으로 비교하지 않습니다. 앞의 것은 사용자가 관리하는 파일이고 — dependencies, maintainers, 그리고 변경마다 올라가는 version이 자랍니다 — 뒤의 것은 덧붙여지기만 합니다. 둘 중 하나라도 다시 생성하면 드리프트가 아닌 것을 드리프트로 보고하게 되고, `--write`는 hck가 쓴 적 없는 부분을 지웁니다.

스켈레톤 파일이 아예 없어진 경우는 그렇게 보고되고 `--write`가 되돌려 놓습니다. values를 갖지 않는 파일이기 때문입니다. 리소스 템플릿이 없어진 경우는 다릅니다. 그때 파일을 쓰면 `values.yaml`에 키가 없는 리소스가 차트에 생기고, 그것은 `hck add`가 할 일입니다.

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
| `--all` | `false` | 차트가 가진 오버레이의 모든 조합과, 아무것도 없는 경우까지 검사 |
| `--strict` | `false` | 경고도 실패로 취급. info 지적은 실패시키지 않음 |
| `--off` | — | 이번 실행에서만 끌 규칙, 쉼표로 구분. `"*"`는 끌 수 있는 규칙을 전부 끔 |
| `--print` | `false` | 렌더된 매니페스트를 출력 |
| `--no-render` | `false` | helm을 건너뛰고 차트 디렉터리만 읽는 규칙만 실행 |
| `--format` | `text` | 사람이 읽으면 `text`, CI가 읽으면 `json` |

```bash
hck check
hck check --chart ./charts/payments-api
hck check -f values/prod.yaml --strict
hck check --all                # 설치될 수 있는 모든 조합
hck check --no-render          # helm 없이도 동작
hck check --format json        # 같은 실행, 기계가 읽는 형태
```

`-f`를 주지 않았고 차트에 `ci/install-values.yaml`이 있으면 그 파일을 씁니다. `image.tag`가 필수인 차트는 기본값만으로 렌더되지 않습니다 — 필수로 둔 이유가 바로 그것입니다. 그래서 이 CI values 파일이 있어야 애초에 검사가 가능합니다.

`hck check`는 error 지적이 하나라도 있으면 0이 아닌 코드로 종료합니다. `--strict`를 주면 경고만 있어도 마찬가지입니다. info 지적은 어떤 엄격도에서도 실패로 이어지지 않습니다.

<br/>

### 세 가지 심각도

| Severity | 뜻 | `--strict`에서 실패? |
|---|---|---|
| `error` | 차트가 렌더되지 않거나 적용되지 않음 | 예. `--strict` 없이도 실패 |
| `warn` | 동작은 하지만 하우스 룰이 지적하는 관행 | 예 |
| `info` | 차트가 놓일 클러스터에 대해 차트가 갖는 전제조건 | 아니오 |

마지막 것이 있는 이유는, 차트에 필요한 것 중에는 누구의 잘못도 아닌 게 있기 때문입니다. `hck new --platform aws`는 `persistence.storageClass: gp3`를 씁니다. EKS에서 요청하기에 옳은 클래스이면서, 동시에 EKS가 만들어주지 않는 클래스입니다. 차트도 맞고 조언도 맞는데, 그걸 향한 `PersistentVolumeClaim`은 누군가 그 클래스를 만들어줄 때까지 Pending에 머뭅니다. `HCK040`이 이걸 보고합니다.

이걸 경고로 보고하면 hck가 직접 만든 차트가 hck 자신의 `--strict`에서 실패하게 됩니다. 반대로 아무 말도 하지 않으면, 읽는 사람은 영영 스케줄되지 않는 파드를 보고서야 알게 됩니다. 그래서 보고는 하되 실패시키지는 않습니다. 클래스를 만드는 게 자기 일이라고 보는 차트는 `HCK040: warn`으로 올리면 되고, 그때부터는 `--strict`가 실제로 막아섭니다.

<br/>

### 차트가 설치될 수 있는 모든 조합

오버레이를 든 차트는 그중 한 조합으로 설치됩니다. `hck check`는 정확히 하나만 렌더하는데, 어느 것을 렌더할지는 플래그가 정합니다. 그래서 플래그를 주지 않으면 아무도 설치하지 않는 조합을 렌더하게 됩니다.

```console
$ ls values-*.yaml
values-aws.yaml  values-dev.yaml  values-gcp.yaml  values-prod.yaml

$ hck check --strict          # passes, and applied none of the four
```

`--all`은 대신 모든 조합을 렌더합니다.

```console
$ hck check --all --strict
check payments-api  9 combination(s)

  ok    base
  ok    dev
  FAIL  prod            1 warning(s)
          warn  HCK036  PodDisruptionBudget/payments-api
                maxUnavailable is 0, so no voluntary disruption is ever allowed: ...
  ok    aws
  ok    aws + dev
  FAIL  aws + prod      1 warning(s)
          ...
  ok    gcp
  ok    gcp + dev
  FAIL  gcp + prod      1 warning(s)
          ...

  9 combination(s), 6 ok, 3 failed
```

조합의 집합은 두 축의 곱이고, 각 축은 차트가 가진 것과 아무것도 없음을 함께 내놓습니다. 두 축은 직교하므로 — 차트는 어딘가에 어떤 크기로 설치됩니다 — 배제되는 짝이 없습니다. 그래서 두 축에 걸친 오버레이 4개는 조합 9개가 되고, hck 자신의 오버레이 7개는 20개가 됩니다. 한 축에서 둘을 고르는 일은 없습니다. 차트는 한 플랫폼에 한 크기로 설치되므로 `aws + gcp`는 검사할 대상이 아닙니다.

오버레이가 없는 차트는 조합이 하나이고, 그건 `hck check`가 지금까지 늘 돌리던 바로 그 실행입니다. `--all`은 기존 동작의 상위집합이지, 오버레이를 쓰지도 않는 차트에 다른 질문을 던지는 게 아닙니다.

조합 하나에 대한 판정은 단일 실행이 내리는 판정과 같습니다. 그래서 `--all`과 `--platform`/`--env`를 손으로 돌린 루프는 서로 어긋날 수 없습니다. 하나가 실패해도 멈추지 않습니다 — 알고 싶은 것은 어느 조합이 깨졌는가이기 때문입니다.

함께 쓸 수 없는 플래그가 넷 있습니다. 각각 결과가 검사하지 않은 것을 주장하게 만들기 때문입니다.

| `--all`과 함께 | 이유 |
|---|---|
| `--platform`, `--env` | 같은 질문을 조합 하나에 대해 답합니다. 둘 다 적는 건 답이 둘인 요청입니다 |
| `--print` | 렌더된 차트 하나를 출력합니다. 읽고 싶은 조합을 지목하세요 |
| `--no-render` | 오버레이는 `-f` 인자라 렌더 없이는 아무것도 바꾸지 않습니다. 모든 조합이 같은 차트를 보고하게 됩니다 |

<br/>

### 규칙 끄기

규칙에 동의하지 않는 차트는 `Chart.yaml` 옆에 자기 `.hck.yaml`을 두고 그렇게 밝힙니다.

```yaml
rules:
  HCK025: off      # this chart wants its CPU limits
  HCK023: error    # and will not ship without requests
```

각 규칙은 `off`, `info`, `warn`, `error` 중 하나를 받고, ID 목록은 `hck list rules`로 볼 수 있습니다. 양방향 모두 의도한 것입니다. `HCK040: warn`으로 올리면 클러스터 전제조건이 `--strict`에서 실패하게 되고, 반대로 경고를 `info`로 내리면 그 지적을 계속 보면서도 그것 때문에 멈추지는 않습니다. 뒤쪽은 `off`로는 표현할 수 없는 절반입니다. 다음 세 가지는 의도한 동작입니다.

- 없는 ID는 조용히 무시하지 않고 오류를 냅니다. 철자가 틀려서 계속 보고되는 규칙은 옳아서 계속 보고되는 규칙과 구별되지 않습니다.
- `HCK001`은 설정할 수 없습니다. 렌더되지 않는 차트에는 더 보고할 것이 없습니다.
- 차트가 꺼 둔 규칙은 지적 사항과 함께 출력됩니다. 그래야 깨끗한 결과를 아무도 묻지 않은 질문과 혼동하지 않습니다.

이 파일은 `.helmignore`에 생성되므로 패키지에는 들어가지 않습니다.

`"*"`는 설정할 수 있는 모든 규칙을 가리키고, 이름을 적은 ID가 양방향으로 그보다 우선합니다. 자체 규칙을 대체로 비켜 두고 싶은 차트는 한 번만 그렇게 적으면 되고, 남겨 둔 둘은 파일에 그대로 드러납니다.

```yaml
rules:
  "*": off
  HCK021: error    # except: an untagged image is still a defect
```

와일드카드는 `HCK001`에 닿지 않습니다. 다른 무엇도 닿지 않는 것과 같은 이유입니다.

차트를 고치지 않고 이번 실행에서만 같은 말을 하려면 `--off`를 씁니다.

```bash
hck check --off HCK025,HCK011
hck check --off '*'
```

`--off`는 차트가 자기 파일에 적어 둔 것을 대체하지 않고 그 위에 얹힙니다. 그 파일이 거부하는 두 가지 — 없는 ID와 `HCK001` — 를 똑같이 거부하고, 결과의 `not checked:` 줄에도 똑같이 실립니다. 규칙을 절반쯤 끄고 돌린 실행도 그 사실을 그대로 말한다는 뜻입니다.

<br/>

### JSON 출력

`--format json`은 실행 한 번을 문서 하나로 씁니다.

```json
{
  "chart": "payments-api",
  "overlays": ["aws", "prod"],
  "findings": [
    {"rule": "HCK023", "severity": "warn", "where": "Deployment/payments-api container=app", "message": "no resource requests — ..."}
  ],
  "errors": 0,
  "warnings": 1,
  "infos": 0,
  "disabled": ["HCK025"],
  "ok": true
}
```

`ok`는 `--strict`를 포함한 종료 코드와 일치하므로, CI는 결과를 grep하는 대신 그대로 읽으면 됩니다. `infos`는 `warnings`와 따로 세며 절대 합쳐지지 않습니다. 둘을 더해서 쓰는 쪽은 `--strict`가 일부러 통과시키는 바로 그것 때문에 차트를 실패시키게 됩니다.

`--all`은 조합마다 항목 하나씩, 두 번째 형태를 씁니다.

```json
{
  "chart": "payments-api",
  "combinations": [
    {"chart": "payments-api", "findings": [], "errors": 0, "warnings": 0, "infos": 0, "ok": true},
    {"chart": "payments-api", "overlays": ["aws"], "findings": [], "errors": 0, "warnings": 0, "infos": 1, "ok": true}
  ],
  "errors": 0,
  "warnings": 0,
  "infos": 1,
  "failed": 0,
  "ok": true
}
```

각 항목은 그 자체로 완전한 단일 실행 문서입니다. 하나만 떼어내도 이미 읽을 줄 아는 형태가 나옵니다. `overlays`가 없으면 base 조합인데, 이는 오버레이를 지목하지 않은 단일 실행에서 그 키가 빠지는 것과 같습니다. 최상위 집계는 단일 실행과 같은 이름을 그대로 쓰므로 `.ok`와 `.errors`는 두 형태 모두에서 동작하고, `combinations`와 `failed`만 새로 생긴 것입니다. `failed`는 지적이 아니라 조합을 셉니다. 한 조합이 지적을 여럿 담을 수 있고, `--strict`에서는 error가 하나도 없이 경고만으로도 조합이 실패하기 때문입니다. 여기서 `--print`는 효과가 없습니다. JSON 문자열 안에 든 매니페스트는 읽기에도 파싱하기에도 맞지 않습니다.

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
hck list              # preset, 리소스, check 규칙
hck list presets
hck list resources
hck list rules
```

`[crd]`가 붙은 리소스는 대상 클러스터에 없을 수도 있는 CRD나 기능 게이트가 있어야 동작하고, `[platform]`은 그 플랫폼에만 존재하는 리소스라는 표시입니다. `hck list rules`는 모든 `HCK0xx`를 기본 심각도와 함께 출력합니다 — 차트의 `.hck.yaml`이 가리키는 바로 그 ID입니다.

`--format json`은 같은 내용을 문서로 냅니다 — `presets`, `resources`, `groups`, `rules`. 섹션을 지목하면 나머지는 아예 빠지므로, 소비하는 쪽에서 "없음"과 "묻지 않음"을 구분할 수 있습니다.

```bash
hck list resources --format json | jq -r '.resources[] | select(.platform == "gcp") | .name'
hck list presets   --format json | jq -r '.presets[].name'
```

스크립트에서는 표가 아니라 JSON을 읽으세요. 카탈로그를 그룹으로 묶을 때 표의 들여쓰기가 한 번 바뀌었고, `awk` 파서 세 개가 같이 깨졌습니다.

<br/>

## 셸 자동완성

```bash
hck completion zsh  > "${fpath[1]}/_hck"
hck completion bash > /etc/bash_completion.d/hck
hck completion fish > ~/.config/fish/completions/hck.fish
```

자동완성은 preset과 리소스 이름을 알고 있어서, `hck add <TAB>`을 누르면 추가 가능한 것들이 나옵니다.
