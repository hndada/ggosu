# 폴더 구조와 빌드

모든 실행 진입점은 `cmd` 아래에 둔다. 실험·데모 생성 프로그램은 `cmd/poc`에 모은다.
`build`에는 플랫폼별 빌드 원본과 생성물을 둔다.
명령은 저장소 루트에서 실행한다.

```text
70.ggosu/
├─ go.mod, go.sum      하나의 Go 모듈
├─ cmd/                모든 실행 프로그램
│  ├─ desktop/         데스크톱 진입점·main.App 바인딩·wails.json
│  ├─ game/            main·네이티브 옵션·WebAssembly 진입점
│  ├─ web/             홈페이지·브라우저 플레이 서버
│  ├─ multi/           멀티플레이 서버
│  ├─ archive/         채보 색인·수집·검색 서버
│  ├─ chart/           채보 변환·검사 CLI
│  └─ poc/             제품과 분리한 실험·데모
│     ├─ input/        입력 검증 프로그램·사용법·검증 기록
│     ├─ difficulty/   난이도 실험 CLI·사용법·보고서 스크립트
│     ├─ demo/         songs/demo 에셋 생성 프로그램
│     └─ internal/difficulty/ 실험용 난이도 모델
├─ internal/           제품에서 사용하는 책임별 Go 패키지
│  ├─ chart/           채보 모델·포맷 등록·파일 읽기·곡 목록·자산 경로·편집 저장
│  │  └─ osu/ bms/ ojn/ sm/  원본 포맷별 Format 구현 (blank import로 등록)
│  ├─ piano/           piano 모드 판정·점수·리플레이·기록
│  ├─ play/           piano 모드 실행·입력 연결·레인·HUD·채팅 상태·점수 비교 이력
│  ├─ storyboard/     곡 시간에 따른 배경·스프라이트 연출
│  ├─ ui/             공용 글꼴·텍스트·일시정지 화면
│  ├─ keyboard/       OS별 입력 수신·이벤트 큐
│  ├─ clock/          공통 고해상도 시계·1배속 게임 타임라인
│  ├─ sound/          디코딩·오디오 플레이어·배속·Pitch
│  ├─ settings/       키 프로필·오프셋·화면·음량 설정
│  ├─ songselect/     곡 목록 스캔·선택 확인·노트 분포 (Wails 의존 없음)
│  ├─ menu/           Wails 메뉴 앱·바인딩·게임 프로세스·기록·설정·편집·대기실
│  ├─ launch/         메뉴와 상주 게임의 파이프 메시지·Windows 포커스 요청
│  ├─ multi/          멀티 프로토콜·클라이언트 연결·공유 플레이 조건·시계 동기화
│  └─ server/         서버 전용 패키지 (클라이언트는 import하지 않음)
│     ├─ multi/       방·매치 서버 (cmd/multi)
│     ├─ web/         홈페이지·브라우저용 HTTP API (cmd/web)
│     └─ archive/     채보 수집·캐시·배포 (cmd/archive)
├─ scenes/            데스크톱·웹 공용 메뉴 화면 (index.html·main.js·layout.css·theme.css·assets.go)
│  ├─ songselect/      곡 목록·단축키·결과와 기록·분포 그림·범위 창
│  ├─ settings/        설정 창·오프셋 입력·키 테스트 화면
│  ├─ editor/          채보 편집기·싱크 대화상자
│  └─ lobby/           멀티 대기실·채팅
├─ songlist/          곡 검색·목록 배치·범위 계산·기록 통계 (화면 없음)
├─ chartedit/         편집 Draft·온셋 분석·기기 보정·메트로놈 (화면 없음)
├─ audio/             곡 프리뷰·시간 늘이기·메뉴 효과음 (화면 없음)
├─ preferences/       표시 설정 검증·키 테스트 판정 (화면 없음)
├─ songs/demo/         원본 데모 채보·오디오·이미지
├─ scripts/            제품 빌드·패키징·검증 스크립트
├─ docs/
│  ├─ index.md         문서 목차
│  ├─ roadmap.md       개발 계획과 완료 목록
│  ├─ guide/           기능·사용법
│  ├─ design/          설계 결정·레퍼런스·후보·목업
│  ├─ research/        채보 소스 조사 자료
│  └─ dev/             제품 구현 구조·빌드·소스 리뷰
└─ build/
   ├─ appicon.svg      선택한 23번 아이콘의 벡터 원본
   ├─ appicon.png      Wails·네이티브 게임 공용 아이콘 PNG
   ├─ icon.go          네이티브 창에 사용할 PNG embed
   ├─ windows/         Windows manifest·아이콘·메타데이터 원본
   ├─ bin/             현재 제품 실행 파일·배포용 songs (Git 제외)
   ├─ poc/             입력·난이도 POC·데모 생성 실행 파일 (Git 제외)
   └─ web/             WebAssembly·다운로드 ZIP (Git 제외)
```

Wails 앱은 `cmd/desktop`에서 빌드한다. 루트에서 `scripts/desktop.ps1`을 실행하면 이 위치로 이동한다.
별도 게임 프로세스는 `cmd/game`에서 빌드하여
`gosu.exe` 옆의 `game.exe`로 실행한다. 웹도 같은 `cmd/game`을 WebAssembly로 빌드한다.
데스크톱은 `game.exe -resident` 하나를 유지한다. 곡 선택 중 숨긴 창을 준비하고, 표준 입력으로 플레이 옵션을 전달한다. 첫 Draw와 곡 완료는 별도 메시지로 돌아온다. 완료 후 결과를 저장하고 곡별 입력·오디오 플레이어·이미지를 정리한 뒤 창을 숨기며, 메뉴 종료 시 입력 파이프를 닫고 프로세스 종료를 기다린다. 직접 CLI 실행은 기존 단일 곡 동작을 유지한다.
메뉴 UI는 화면 코드와 화면이 아닌 코드를 나눈다. DOM을 만들거나 바꾸는 코드는 `scenes/`, 계산·검증·오디오 처리는 하는 일별 루트 폴더(`songlist`, `chartedit`, `audio`, `preferences`)에 둔다.
한 모듈이 둘 다 가졌으면 같은 파일 이름으로 나눈다. 예: `songlist/range.js`(계산)와 `scenes/songselect/range.js`(범위 창).
각 폴더는 `assets.go`로 자기 파일을 embed하고, `scenes.Assets`가 화면을 `/`에, 모듈 폴더를 `/<폴더>/`에 합쳐 Wails·웹·멀티 서버에 제공한다. 웹 어댑터는 `internal/server/web/static`에서 embed한다.

루트에는 Go 실행 소스를 두지 않는다. `cmd/desktop/main.go`의 `main.App`이 `menu.App`을 포함해 기존 `window.go.main.App` 바인딩 이름을 유지한다.
곡 목록 스캔과 선택 확인은 `internal/songselect`, 앱 상태·기록·설정·편집·게임 실행·대기실과 해당 테스트는 `internal/menu`에 둔다.
`menu.App`은 `songselect.Songs`를 비공개 필드로 가지며 `Catalog`·`Distribution` 바인딩만 넘겨준다. 락은 `menu.App`의 것을 먼저 잡고 `Songs`의 것을 나중에 잡는다.
`menu.Run`이 Wails 설정·시작 이벤트·에셋 HTTP 처리·종료를 연결한다. `main.go`에 바인딩 메서드를 전달용 함수로 중복 작성하지 않는다.

`cmd/game`에는 `main.go`, `native.go`의 CLI 옵션 처리, `browser_js.go`의 웹 진입점만 둔다.
Windows 실행 파일 아이콘은 `icon_windows_{amd64,arm64,386}.syso` 리소스로 포함한다.
이 리소스는 `build/windows/icon.ico`를 winres v0.3.1의 `ResourceSet.SetIcon(ID(1), icon)`과 `WriteObject`로 컴파일한 것이다.
네이티브는 옵션을 `play.Config`로 전달해 `play.Run`을 호출하고, 웹은 `play.RunBrowser`를 호출한다.
piano 모드 실행과 화면 구현·회귀 테스트는 `internal/play`에 있다. 화면용 `view`는 비공개 타입이며, 판정은 `internal/piano`의 `Session`을 사용한다. 공용 배경·연출은 `internal/storyboard`, 텍스트·일시정지 화면은 `internal/ui`, 입력 큐는 `keyboard.Queue`에 둔다.
현재 internal의 Go 패키지는 14개다. 태고 등 다른 모드와의 재사용 범위는 [플레이 모드 경계](modes.md)에 정리한다.

루트 `bin/`은 사용하지 않는다. 모든 빌드 산출물은 `build/bin`, `build/poc`, `build/web` 중 해당 위치에 둔다.
`build/bin`에는 gosu·game·web·multi·archive·chart의 현재 빌드만 둔다.
`build/windows`와 `build/appicon.png`는 빌드 원본이므로 생성물과 구분한다.
`build.Icon`은 같은 PNG를 embed하며, 네이티브 플레이 창은 이를 `ebiten.SetWindowIcon`에 전달한다.

`chart`와 `piano` 등 제품 구현은 `internal`에 둔다.
미보정 난이도 모델은 `internal/difficulty`에 둔다. 곡 선택창은 `difficulty.Rate` 값을 잠정 레벨로 쓰고 `ratings.json`에 캐시한다.
실험 CLI·합성 부하·측정 화면·검증 기록은 `cmd/poc`에서만 관리한다.
제품에서도 사용하는 `internal/keyboard`, `internal/clock`, `internal/piano`는 제품 영역에 유지한다.
자세한 기능별 책임과 호출 순서는 [소스 리뷰 안내](review.md), 실험 실행은 [POC 안내](../../cmd/poc/README.md)를 참고한다.

## 빌드

Wails 2.11.0 CLI를 설치한다. 이미 `.cache/wails.exe`가 있으면 재설치할 필요가 없다.

```powershell
New-Item -ItemType Directory -Force .cache, build/bin | Out-Null
$env:GOBIN = Join-Path (Get-Location) '.cache'
go install github.com/wailsapp/wails/v2/cmd/wails@v2.11.0
pwsh -NoProfile -File scripts/desktop.ps1 build
```

Wails의 post-build hook이 `build/bin/game.exe`도 만든다. Wails의 post-build hook은 `build/bin`을
작업 디렉터리로 사용하므로 `cmd/desktop/wails.json`의 hook 소스 경로는 `../../cmd/game`이다.
설정 파일의 프런트엔드·에셋·빌드 폴더는 `cmd/desktop` 기준이며, `../../internal`·`../../scenes`와 모듈 폴더를 개발 모드에서 감시한다. 개발 모드는 `scenes/`만 디스크에서 읽으므로 모듈 폴더는 `menu`의 에셋 핸들러가 embed본으로 제공한다.
원본 데모를 배포할 때는 두 실행 파일 옆에 복사한다.
곡 목록은 `songs/demo/` 밖에서 읽을 수 있는 채보가 없을 때만 데모를 표시한다. 일반 곡이 하나라도 있으면 데스크톱·웹 목록 모두에서 데모를 제외한다.

```powershell
New-Item -ItemType Directory -Force build/bin/songs/demo | Out-Null
Copy-Item -Path songs/demo/* -Destination build/bin/songs/demo -Recurse -Force
```

서버·도구는 필요한 프로그램만 빌드한다.

```powershell
go build -o build/bin/web.exe ./cmd/web
go build -o build/bin/multi.exe ./cmd/multi
go build -o build/bin/archive.exe ./cmd/archive
go build -o build/bin/chart.exe ./cmd/chart
pwsh -NoProfile -File scripts/buildweb.ps1
```

데스크톱 ZIP 생성은 `pwsh -NoProfile -File scripts/packdesktop.ps1`로 실행한다.
이 스크립트는 `build/bin/gosu.exe`, `game.exe`, `songs/`를 묶어
`build/web/downloads/gosu-windows.zip`에 저장한다.

## 로컬 파일과 검증

- `.cache/`: 의존성·컴파일 캐시·임시 파일·로컬 Wails CLI.
- `data/`: 사용자 설정·기록·수집 데이터.
- `artifacts/`: 측정 보고서·검증 화면·분석 결과. 문서의 산출물 링크는 로컬에서만 존재할 수 있다.
- `build/poc/`: 제품과 분리한 입력·난이도 실험·데모 생성 실행 파일. `scripts/buildpoc.ps1`로 다시 만든다.
- 옛 실행 파일은 [보관 ZIP](../../artifacts/19-build-cleanup-20261009/old-builds.zip)에 모았다. 제품 빌드 폴더에 과거 버전을 쌓아두지 않는다.
- `scenes/wailsjs/`: Wails가 생성하는 JavaScript 바인딩.
- `songs/`: 로컬 곡 라이브러리. 원본 `songs/demo/`만 버전 관리한다.

위의 생성물과 로컬 데이터는 `.gitignore`로 제외한다. `build/windows`와 `build/appicon.png`는 빌드 원본이므로 포함한다.
Wails의 로컬 실행 기록·설정은 실행 파일 옆 `build/bin/data/`에 생긴다. 저장소 루트의 `data/`는 서버·수집 도구의 데이터 위치다.

UI의 CSS 허용 범위는 `scenes/style.test.js`에서 검사한다.
배치는 layout.css 등에, 색·글자 크기·허용된 전환은 theme.css에 둔다. 색은 `:root` 변수로 정한다.
테마 후보와 결정은 [설계 자료](../design/theme-candidates/index.html)에 보존한다.
`scenes/style.test.js`가 `scenes/`와 모듈 폴더의 CSS·style 속성·JavaScript 스타일 변경 허용 범위를 검사한다.
파일을 추가하면 해당 폴더 `assets.go`의 embed 목록에도 넣는다. 웹 서버는 `scenes.Paths()`로 경로를 등록하므로 따로 갱신하지 않는다.

```powershell
# 제품만 검사: Go 테스트·vet·POC 의존성·JS 테스트·문법
pwsh -NoProfile -File scripts/check.ps1

# 실험까지 함께 검사할 때만
pwsh -NoProfile -File scripts/check.ps1 -POC
```

Windows에서 기본 Go 캐시나 임시 파일에 `Access is denied`가 발생하면 저장소 안의 캐시를 사용한다.

```powershell
$cacheRoot = Join-Path (Get-Location) '.cache'
New-Item -ItemType Directory -Force -Path (Join-Path $cacheRoot 'mod'), (Join-Path $cacheRoot 'build'), (Join-Path $cacheRoot 'tmp') | Out-Null
$env:GOMODCACHE = Join-Path $cacheRoot 'mod'
$env:GOCACHE = Join-Path $cacheRoot 'build'
$env:TEMP = Join-Path $cacheRoot 'tmp'
$env:TMP = $env:TEMP
```
