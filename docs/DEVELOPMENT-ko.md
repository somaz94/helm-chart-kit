# 개발

이 프로젝트를 빌드하고 테스트하는 법, 그리고 여기에 기여하는 법을 담은 안내입니다.

> English: [DEVELOPMENT.md](DEVELOPMENT.md) · 한국어 문서 모음: [README-ko.md](../README-ko.md)

<br/>

## 사전 준비

- Go 1.26+
- Make
- `PATH`에 있는 `helm` — `hck check`와 그걸 사용하는 테스트에만 필요합니다. 없으면 해당 테스트는 건너뜁니다

<br/>

## 구조

```
cmd/cli/            Cobra 명령. 각각 패키지 수준 변수가 아니라 생성자입니다.
                    cobra 가 플래그를 주소에 바인딩하므로, 트리를 공유하면
                    한 실행의 플래그 값이 다음 실행으로 새어 나갑니다.
internal/catalog    무엇을 생성할 수 있는가: 리소스와 preset. 데이터만.
internal/render     임베드된 템플릿 세트와 렌더러.
internal/values     추가 전용 values.yaml 병합.
internal/schema     리소스 조각들로부터 values.schema.json 조립.
internal/docs       values.yaml → Markdown 표.
internal/chart      차트 디렉터리를 찾고 들여다보기.
internal/scaffold   요청을 Plan 으로 바꾸고, Apply가 기록.
internal/check      helm 으로 렌더한 뒤 자체 규칙 적용.
```

설계의 대부분을 떠받치는 결정이 둘 있습니다:

**Plan과 Apply는 분리돼 있습니다.** 그래야 `--dry-run`이 실제 실행의 근사치가 아니라 정확히 같은 것을 보여주고, 정작 까다로운 판단들을 디스크를 건드리지 않고도 테스트할 수 있습니다.

**values 병합은 YAML 왕복이 아니라 텍스트 기반입니다.** 디코드 후 다시 인코드하면 키와 주석은 보존되지만 빈 줄과 섹션 배너가 사라집니다. 그러면 "키 하나만 추가"하는 도구가 400줄짜리 파일을 조용히 리포맷하게 됩니다. 대신 원본 바이트는 절대 건드리지 않습니다 — 없는 키는 덧붙이고, 있는 키는 보고만 하고 그대로 둡니다.

<br/>

## 리소스 추가하기

1. `internal/render/templates/resources/<name>/`에 `template.yaml.tmpl`, `values.yaml.tmpl`, `schema.json.tmpl`을 만듭니다.
2. `internal/catalog/catalog.go`의 `resources`에 항목을 추가합니다. `ValuesKeys`도 함께요.

생성 계층은 `[[ ]]` 델리미터를 씁니다. 그래야 Helm 자신의 `{{ }}`가 그대로 통과하고, 템플릿을 읽으면 앞으로 만들어질 Helm 파일의 모습이 그대로 보입니다.

`schema.json.tmpl`은 리소스가 더하는 각 최상위 values 키를 그 키의 스키마로 매핑한 JSON 객체입니다. 바깥 틀(`$schema`, `title`, `type`, `properties`)은 `internal/schema`가 붙이므로, 조각은 키만 담습니다.

세 테스트가 카탈로그와 템플릿 트리를 교차검증합니다. 리소스나 키를 한쪽에만 선언하면 실패합니다:

- `internal/catalog`: 모든 카탈로그 항목이 템플릿 파일 셋을 다 가짐
- `internal/render`: 모든 템플릿 디렉터리가 렌더되고, 치환 안 된 델리미터가 남지 않으며, 모든 스키마 조각이 JSON 으로 파싱됨
- `internal/catalog`: `ValuesKeys`, `values.yaml.tmpl`이 선언하는 키, `schema.json.tmpl`이 선언하는 키가 **같은 순서의 같은 목록**

마지막 것이 스키마에서 특히 중요합니다. Helm은 렌더할 때마다 값을 `values.schema.json`에 비춰 검증하므로, `values.yaml`에는 있는데 스키마에 없는 키는 눈에 안 띄는 정도로 끝나지 않고 **차트 설치 자체를 막습니다.**

다른 리소스의 값을 읽는 템플릿은 반드시 괄호 형태를 써야 합니다 — `.Values.autoscaling.enabled`가 아니라 `(.Values.autoscaling).enabled`. 그 리소스가 아예 차트에 없을 수 있기 때문입니다. Sprig의 `dig`는 여기서 동작하지 않습니다. `.Values`는 `map[string]interface{}`가 아니라 `chartutil.Values`입니다.

<br/>

## 오버레이 축

플랫폼(`aws`/`gcp`/`azure`/`onprem`)과 환경(`dev`/`staging`/`prod`)은 같은 기계를 씁니다. 조각은 `templates/resources/<name>/values-<suffix>.yaml.tmpl`에 두고, `scaffold.buildOverlay`가 조립합니다.

**두 축이 같은 키를 동시에 건드려서는 안 됩니다.** 플랫폼은 *무엇이 어떻게 연결되는가*(어노테이션, 클래스, 스토어 참조)를 말하고, 환경은 *무엇이 켜져 있고 얼마나 큰가*를 말합니다. 둘 다 `-f` 인자가 되므로, 양쪽이 건드리는 키는 의도가 아니라 **인자 순서**로 결정됩니다. 테스트 둘이 이를 강제합니다:

- `TestPlatformOverlaysDoNotToggle` — 플랫폼 오버레이에 `*.enabled` 금지
- `TestOverlayOrderDoesNotChangeTheRender` — 12쌍을 양방향으로 렌더해 비교

플랫폼이 정말로 무언가를 지원하지 못한다면, 그건 `Platform.Needs`에 적을 일입니다.

<br/>

## 빌드

```bash
make build           # 바이너리 빌드 → ./bin/hck
make clean           # 빌드 산출물 제거
make install         # /usr/local/bin에 설치
```

<br/>

## 테스트

```bash
make test            # 유닛 테스트 실행 (별칭)
make test-unit       # go test ./... -v -race -cover
make cover           # 커버리지 리포트 생성
make cover-html      # 브라우저로 커버리지 리포트 열기
```

<br/>

## 코드 품질

```bash
make fmt             # 코드 포맷 (go fmt)
make vet             # go vet 실행
```

<br/>

## 작업 흐름

```bash
make check-gh        # gh CLI 설치·인증 확인
make branch name=feature-name        # main 에서 기능 브랜치 생성
make pr title="feat: add feature"    # 테스트 → 푸시 → PR 생성 (본문 자동 생성)
```

`make pr` 이 자동으로 하는 일:
1. `go test ./... -race -cover`와 `go vet` 실행
2. 브랜치를 origin에 푸시
3. 커밋 이력에서 PR 본문 생성 (`feat:`, `fix:`, `test:`, `docs:`로 분류)
4. 변경된 테스트 패키지를 감지해 테스트 계획 체크리스트 구성
5. `gh pr create`로 PR 생성

<br/>

## CI/CD 워크플로

| 워크플로 | 트리거 | 설명 |
|----------|---------|-------------|
| `ci.yml` | push (main), PR, dispatch | 유닛 테스트 → 빌드 → 버전 확인 |
| `release.yml` | `v*` 태그 푸시 | GoReleaser (바이너리 + Homebrew + Scoop) |
| `changelog-generator.yml` | 릴리스 후, PR 머지 | CHANGELOG.md 자동 생성 |
| `contributors.yml` | changelog 이후 | CONTRIBUTORS.md 자동 생성 |
| `stale-issues.yml` | 매일 cron | 오래된 이슈 자동 종료 |
| `dependabot-auto-merge.yml` | PR (dependabot) | minor/patch 업데이트 자동 머지 |
| `issue-greeting.yml` | 이슈 생성 | 환영 메시지 |

<br/>

### 워크플로 연쇄

```
v* 태그 푸시 → Create release (GoReleaser)
                └→ Generate changelog
                      └→ Generate Contributors
```

<br/>

## 관례

- **커밋**: Conventional Commits (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `ci:`, `chore:`)
- **시크릿**: `PAT_TOKEN`(저장소 간 작업), `GITHUB_TOKEN`(릴리스)
- **paths-ignore**: `.github/workflows/**`, `**/*.md`
- **문서**: 영문이 원본이고 `-ko.md`가 번역본입니다. 한쪽을 고치면 **반드시 짝을 함께** 고쳐야 합니다
