# Gosu

Wails 곡 선택창과 Ebitengine 건반 게임. osu!mania·O2Jam·BMS·StepMania 채보를 읽고,
롱노트·키음·배경·스토리보드·Auto·리플레이를 재생한다. 브라우저도 같은 Go 게임 코드를 WebAssembly로 실행한다.

곡 선택·프리뷰·개인 설정·채보 편집·플레이 결과와 로컬 기록은 Wails에서 제공한다.
플레이는 별도 `game.exe`로 실행하며 곡과 오디오가 끝나면 결과 화면으로 돌아온다.
점수는 1,100,000점 만점이며, 기록·리플레이는 실행 파일 옆 `data/records/`에 저장한다.

## 실행

Go 1.25 이상. 모든 명령은 저장소 루트에서 실행한다. 원본 데모는 `songs/demo/`에 있다.

```powershell
# 네이티브 게임
go run ./cmd/game -chart songs/demo/demo.osu

# 브라우저 플레이: http://127.0.0.1:8080
pwsh -NoProfile -File scripts/buildweb.ps1
go run ./cmd/web -addr 127.0.0.1:8080 -songs songs

# 데스크톱 곡 선택창 (Wails 2.11.0 CLI)
pwsh -NoProfile -File scripts/desktop.ps1 dev
```

데스크톱 빌드는 `pwsh -NoProfile -File scripts/desktop.ps1 build`로 실행한다.
`build/bin/gosu.exe`, `game.exe`, `songs/`를 함께 배포한다.
Wails 설치·데모 복사·패키징·캐시 설정은 [빌드 안내](docs/dev/structure.md#빌드)를 참고한다.

## 폴더

| 위치 | 역할 |
|---|---|
| `cmd/desktop/main.go` | 데스크톱 진입점·기존 Wails 바인딩 이름 유지 |
| `internal/songselect` | 곡 목록 스캔·선택 확인·노트 분포 |
| `internal/menu` | Wails 메뉴 앱의 상태·기록·설정·편집·게임 프로세스·대기실 |
| `cmd/game`, `cmd/web` | 게임·웹 서버의 실행 진입점 |
| `cmd/multi`, `cmd/archive` | 멀티플레이·채보 수집 서버 |
| `cmd/chart` | 채보 변환·검사 도구 |
| `internal/play` | 게임 상태·실행·입력·오디오·멀티 연결 |
| `internal`의 나머지 패키지 | 채보·판정·렌더링 등 책임별 구현 |
| `scenes` | 데스크톱·웹 공용 메뉴 화면 (songselect·settings·editor·lobby) |
| `songlist`, `chartedit`, `audio`, `preferences` | 화면이 아닌 UI 모듈: 곡 목록 계산·편집 모델·오디오·설정 검증 |
| `songs/demo` | 테스트와 실행에 사용하는 원본 데모 |
| `scripts` | 제품 빌드·패키징·검증 스크립트 |
| `docs` | 제품 사용 안내·소스 리뷰·설계 자료·개발 계획 |
| `build` | 플랫폼 빌드 원본과 생성물 |
| `cmd/poc` | 제품과 분리한 입력·난이도 실험·데모 생성, 전용 구현·문서·스크립트 |

빌드 산출물은 `build/` 아래에만 둔다. 제품 실행 파일은 `build/bin/`, POC 실행 파일은 `build/poc/`, 웹 산출물은 `build/web/`다.
`build/bin`, `build/poc`, `build/web`, `.cache`, `data`, `artifacts`, 생성된 Wails 바인딩과 로컬 곡은 Git에서 제외한다.
`build/windows`, `build/appicon.png`, 원본 `songs/demo`는 포함한다.
소스 리뷰는 [구현된 기능과 코드 읽는 순서](docs/dev/review.md)에서 시작한다.
제품 구현은 `internal`과 UI 폴더(`scenes` 등)에 있고, 모든 실행 진입점은 `cmd` 아래에 있다.
실험 코드는 [POC 영역](cmd/poc/README.md)에 모았다. 제품 검증은 아래 명령으로 실행한다.

```powershell
pwsh -NoProfile -File scripts/check.ps1
```

상세 구조와 빌드는 [개발 안내](docs/dev/structure.md)를 참고한다.

## 문서

- [전체 문서 목차](docs/index.md)
- [곡 선택·프리뷰](docs/guide/songselect.md) · [플레이 설정](docs/guide/preferences.md) · [배속·Pitch·노트 배치](docs/guide/play-options.md)
- [단축키](docs/guide/shortcuts.md)
- [곡 검색·다운로드·설치](docs/guide/download.md)
- [채보·조작](docs/guide/charts.md) · [점수 규칙](docs/guide/score.md) · [채보 편집](docs/guide/edit.md)
- [설정·기록 검증과 복구](docs/guide/integrity.md)
- [브라우저 플레이](docs/guide/web.md) · [멀티플레이](docs/guide/multiplayer.md) · [채보 아카이브](docs/guide/archive.md)
- [점수 서버 제출·검증·재시도](docs/guide/submission.md)
- [개발 계획과 완료 목록](docs/roadmap.md)
