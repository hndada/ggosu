# 구현된 기능과 코드 읽는 순서

제품 코드의 진입점·상태 소유자·실제 호출 경로를 정리한다.
POC는 [별도 영역](../../cmd/poc/README.md), 제안·목업은 [설계 자료](../design/README.md),
미구현 기능은 [개발 계획](../roadmap.md)에 둔다.

## 먼저 읽을 흐름

1. `internal/chart/chart.go`: 제품 전체가 공유하는 채보·노트·타이밍·미디어 모델.
2. `internal/chart/load.go`, `scan.go` → `format.go`(등록된 `Format`) → `internal/chart/<포맷>`: 곡 폴더를 스캔하고 원본 채보를 공통 모델로 읽는다.
3. `internal/piano/session.go` → `score.go`: 입력·시간 진행에 따른 판정과 점수. 네이티브와 웹이 공유한다.
4. `internal/play/prepare.go` → `update.go`: 설정·Mods·리플레이를 적용하고 게임 루프를 실행한다.
5. `internal/play/view.go`: 판정 상태를 읽어 레인·HUD·배경·채팅을 그린다.
6. `cmd/desktop/main.go` → `internal/menu/run.go`, `play.go` → `scenes/main.js`: 데스크톱에서 게임을 실행하고 결과를 곡 선택 화면에 연결한다.

```text
Wails 진입점: cmd/desktop/main.go의 main.App (menu.App 포함)
             → internal/menu/run.go: Wails 시작·이벤트·에셋·종료
             → internal/menu/App: 메뉴 앱 상태
             → internal/songselect/Songs: 곡 목록·선택 확인
                ├─ catalog.go / settings.go / edit.go / records.go
                ├─ lobby.go: 대기실 연결·곡 제안·게임 참가
                └─ launch.go → play.go → game.exe

웹: cmd/web → internal/server/web → scenes + web.js → WebAssembly 게임

게임 진입점: cmd/game/main.go
             ├─ cmd/game/native.go: CLI → play.Config → play.Run
             └─ cmd/game/browser_js.go: play.RunBrowser

게임 실행: internal/play/native.go 또는 browser_js.go → prepare.go → update.go
                        ├─ input.go: 수신 이벤트·키 상태 → Session.Input
                        ├─ controls.go + pause.go: 배속·스크롤·일시정지
                        ├─ internal/piano: 리플레이 입력·판정·점수
                        ├─ audio.go + bank.go + stretch.go: 음악·키음·효과음
                        ├─ internal/play: 표시
                        └─ record.go: 결과·리플레이 저장
```

## 기능별 실제 소스

| 구현된 기능 | UI·실행 연결 | 핵심 구현 |
|---|---|---|
| 곡 목록·검색·키 수·필터·정렬·랜덤·최근 플레이 | `scenes/main.js`, `scenes/songselect/catalog.js`, `shortcuts.js`, `range.js`; `internal/songselect/songs.go` | `internal/chart`; 화면 없는 처리는 `songlist/search.js`, `list.js`, `range.js` |
| 선택곡 프리뷰·Time Rate·Pitch | `audio/preview.js`, `stretch.js`; `internal/menu/edit.go` | `chart.OpenAudio` |
| 개인 키·표시·오디오·오프셋 설정 | `scenes/settings/preferences.js`, `offsets.js`, `keytest.js`; `preferences/display.js`, `keytest.js`; `internal/menu/settings.go` | `internal/settings` |
| 게임 실행·중복 방지·종료·결과 복귀 | `internal/menu/app.go`, `launch.go`, `play.go` | `cmd/game`은 진입점, `internal/play`은 게임 실행; menu.App이 자식 프로세스의 수명을 소유 |
| 채보 판정·점수·Auto·배치 변형 | `internal/play/prepare.go`, `input.go`, `update.go` | `internal/piano`, `internal/chart`, `internal/piano` |
| 네이티브 이벤트·게임 시간 | `internal/play/input.go`, `native.go` | `internal/keyboard`, `internal/clock` |
| 브라우저 게임·기록 저장 | `internal/server/web/static/web.js`; `internal/play/browser_js.go` | 동일한 Go 판정·Scene, 브라우저 저장은 localStorage |
| 음악·키음·배속·Pitch·효과음 | `internal/play/audio.go` | `internal/play/bank.go`, `stretch.go`; 오디오가 끝나는 시각은 `chart.AudioDuration`, 데스크톱은 카탈로그 스캔 때 `data/audio-lengths.json`에 캐시 |
| 레인·노트·판정 미터·HUD·배경·스토리보드 | `internal/play/draw.go`, `assets.go` | `internal/play`, `internal/storyboard`, `internal/ui`; 이미지 디코더는 공용 assets.go에서 등록 |
| 기록·리플레이·이전 최고 비교·분포 | `internal/menu/records.go`, `internal/songselect/songs.go`(분포); `scenes/songselect/records.js`, `analysis.js`; `songlist/analysis.js` | `internal/piano`, `internal/chart` |
| 채보 편집·파형·구간 재생·BPM 추정·Tap 입력·사본 저장 | `internal/menu/edit.go`; `scenes/editor/editor.js`, `sync.js`; `chartedit/edit.js`, `beatfit.js`, `beatfitworker.js`, `sync.js` | `internal/chart` |
| 멀티 대기실·곡 제안·시작 동기화·점수 공유·채팅 | `internal/menu/lobby.go`; `scenes/lobby/lobby.js`, `chat.js`; `internal/play/network.go`, `chat.go` | `internal/multi`(프로토콜·클라이언트·공유 플레이 조건·시계 동기화), `internal/server/multi`(서버); 서버 진입점 `cmd/multi` |
| 점수 서버 자동 제출·접수 상태·재시도 | `internal/menu/submission.go`; `scenes/settings/submission.js`, `scenes/songselect/records.js`; 웹 `preferences/submission.js` + `internal/server/web/static/web.js` | `internal/score` HTTP 클라이언트, `internal/server/scores` 리플레이 재계산·영구 저장·중복 방지; 서버 진입점 `cmd/web` |
| 채보 아카이브·수집·검색 | `cmd/archive` | `internal/server/archive` |
| 곡 받기·설치·곡 목록 반영 | `scenes/songselect/download.js`, `internal/menu/download.go`; 웹 `internal/server/web/download.go` | `internal/download`: 검색·다운로드·압축 해제·채보 검증·취소·중복 확인 |
| 채보 검사·변환 CLI | `cmd/chart` | `internal/chart` |
| 홈페이지·웹 다운로드 | `cmd/web`, `internal/server/web/home.go` | `internal/server/web/static/home.*`; 다운로드 묶음은 `scripts/packdesktop.ps1` |

Edit의 Test는 편집 내용·오디오·레인 키의 미리보기다. 게임 판정이 연결된 테스트는 아직 없다.
미보정 난이도 모델 `internal/difficulty`의 값을 곡 선택창 레벨·레벨 정렬·필터에 잠정치로 쓴다. 점수·PP에는 쓰지 않는다.

## 상태와 경계

- `menu.App`은 메뉴 앱의 결과·프로세스·멀티 연결을 소유하며 mutex로 보호한다. 곡 목록은 `songselect.Songs`가 자체 락으로 보호한다. 같은 패키지의 `launch.go`는 실행 설정을 준비하고 `play.go`는 프로세스 준비·종료를 처리한다. `cmd/desktop`의 `main.App`은 이 타입을 포함해 JS 바인딩 이름만 유지한다.
- `internal/play`의 비공개 `scene`은 piano 모드 한 판의 타임라인·입력 큐·Session·오디오·view를 소유한다. OS 수신 콜백은 `keyboard.Queue`에 넣고 게임 루프가 순서대로 소비한다. `cmd/game`은 옵션과 진입점만 가진다. 채팅 창 상태는 `play/chatstate.go`, 점수 비교 이력은 `play/history.go`가 소유한다. `internal/multi`에는 화면 상태를 두지 않는다.
- `rules.Session`은 판정·점수의 소유자다. 화면과 웹 JavaScript에서 게임 판정 규칙을 다시 구현하지 않는다.
- `scenes/main.js`는 곡 선택과 화면 전환을 연결한다. 목록 표시·단축키·결과/기록은 `scenes/songselect/`, 오프셋은 `scenes/settings/offsets.js`에 있다. 화면을 만들지 않는 계산은 `songlist`·`chartedit`·`audio`·`preferences`에 있다.
- Wails는 `window.go.main.App`을 바인딩하고 웹은 `internal/server/web/static/web.js`가 같은 UI API를 제공한다.
- 각 UI 폴더의 `assets.go`가 자기 파일을 embed하고 `scenes.Assets`가 합친다. 파일을 추가하면 그 폴더의 embed 목록만 갱신한다.

## 리뷰·검증 범위

`*_test.go`와 `*.test.js`는 해당 제품 기능의 회귀 테스트이므로 기능 옆에 둔다.
`cmd/poc/`, `.cache/`, `build/bin/`, `build/poc/`, `build/web/`, `artifacts/`, `data/`, 로컬 곡과 생성된 Wails 바인딩은 제품 구현 리뷰에서 제외할 수 있다.
`docs/design/`의 HTML·CSS는 화면 후보 자료이며 실행되는 UI는 `scenes`·`songlist`·`chartedit`·`audio`·`preferences`와 `internal/server/web/static`이다.

```powershell
# 제품 코드만 테스트·vet·의존성·JS 검사
pwsh -NoProfile -File scripts/check.ps1

# 제품 빌드
pwsh -NoProfile -File scripts/desktop.ps1 build
pwsh -NoProfile -File scripts/buildweb.ps1
```
