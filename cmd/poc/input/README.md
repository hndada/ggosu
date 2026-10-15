# 네이티브 키보드 → Ebitengine 입력 POC

Windows / Linux / macOS 64비트, Go 1.25 이상.
프레임 독립 수신 경로와 OS별 실행 조건은 [플랫폼 검증](platforms.md)에 정리했다.
명령은 저장소 루트에서 실행한다. [리플레이 실험](replay.md)과 [검증 기록](validation.md)을 함께 보존한다.
아래 WM_INPUT 설명은 Windows 경로이며 다른 OS도 같은 게임 콜백과 큐를 사용한다.

실제 키보드는 별도 OS 스레드의 `WM_INPUT`으로 수신한다. 수신 즉시
게임 객체의 `receive()` 콜백을 호출하고, 고정 크기 큐에 각 누름·해제를
보관한다. Ebitengine은 다음 `Update()`에서 큐를 비운다. 엔진 자체의
`IsKeyPressed()` 상태에 이벤트를 주입하거나 엔진 소스를 수정하지 않는다.

```text
키보드 → Windows WM_INPUT → 별도 수신 스레드 → game.receive → inbox
                                                          ↓
                                               Ebitengine Update → Draw
```

## 실행

빌드된 `build/poc/inputpoc.exe`를 실행하거나 다음 명령을 사용한다.

```powershell
go run ./cmd/poc/input
```

프로젝트 안에 의존성과 빌드 캐시를 둘 경우:

```powershell
$env:GOMODCACHE = Join-Path (Get-Location) '.cache\mod'
$env:GOCACHE = Join-Path (Get-Location) '.cache\build'
go build -o build\poc\inputpoc.exe ./cmd/poc/input
.\build\poc\inputpoc.exe
```

Go 프로그램을 제한하는 샌드박스에서는 GLFW가 창을 생성할 때
`ChangeWindowMessageFilterEx: Access is denied`가 발생할 수 있다.
이번 실행 환경에서는 샌드박스 밖에서 GUI 검증을 진행했다.
관리자 권한을 요구하도록 만든 프로그램은 아니다.

## 조작

| 키 | 동작 |
|---|---|
| D / F / J / K | 실제 키보드 입력, 레인 표시와 기본 Ebitengine 입력 개수 비교 |
| F1 | VSync 켜기/끄기 |
| F2 | TPS 60 → 240 → 1000 → 프레임 동기화 순환 |
| F3 | 2초마다 `Draw()`를 100ms 막는 부하 켜기/끄기 |
| F5 | 8,000 이벤트/초 합성 테스트, 기본 5초 |
| F6 | 현재 화면 PNG와 JSON 보고서 저장 |
| P | 게임 시간 일시정지/재개 |
| F7 | 게임 배속 0.5 → 1 → 1.5 → 2 순환 |
| Esc / 창 닫기 | 종료, 수신 스레드 정리 후 최종 보고서 저장 |

실제 키보드 입력은 이 프로세스의 창이 전면일 때만 읽는다.
메시지 전용 창의 `INPUTSINK` 등록은 수신 스레드를 분리하기 위한 것이며,
다른 앱이 전면이면 키보드 데이터 내용을 읽기 전에 버린다.
포커스를 잃으면 레인 유지 상태를 초기화한다.

`-record artifacts\input.json`으로 D/F/J/K 누름·해제를 1.0배속 기준 게임 시각으로
정상 종료 시 저장한다. `-replay artifacts\input.json -playback 0.5`로 재생한다.
정지·배속 변경에 따른 시간 변환과 입력 정책은 [리플레이 안내](replay.md)를 참고한다.
이 기능은 네이티브 입력 POC에 연결되어 있다.
저장 파일의 `time_base`는 `game-1x`이며 리플레이 중 P/F7로 정지·배속을
변경해도 저장된 입력 시각은 유지된다.

## 측정 의미

Windows QPC를 하나의 기준 시계로 공유한다. 각 이벤트의 `At`은
수신 스레드가 메시지를 관측한 시각이며 **물리적으로 키를 누른 시각이 아니다**.
이 PC에서는 QPC 주파수가 10,000,000Hz였다. 주파수는 보고서에도 저장한다.

| 화면 / JSON 항목 | 범위 |
|---|---|
| Native receive → game callback | 수신 메시지 관측부터 콜백의 큐 삽입 직전까지. Raw에서는 전면 확인·패킷 해석도 포함 |
| Native receive → Update applied | 수신부터 실제 `Update()` 소비까지. 프레임·틱 대기 포함 |
| Synthetic PostMessage → receive | 합성 메시지를 게시한 시점부터 수신 스레드 관측까지 |
| Queue dropped | 이 앱의 큐가 넘쳐 버린 이벤트 수. 장치/드라이버 누락을 감지하는 값은 아님 |
| Sequence gaps / order errors | 앱이 부여한 수신 번호 기준의 큐 누락·역전 |

평균·최댓값은 전체 실행, p50·p95·p99는 최근 4,096개 표본이다.
스크린샷 저장은 명시적 F6 또는 자동 테스트 마지막 프레임에만 한다.
측정을 위해 매 이벤트에 디스크 로그를 쓰지 않는다.

`receive()`는 게임 객체의 메서드이지만 네이티브 스레드에서 실행된다.
공유 큐 외의 게임 상태나 Ebitengine 그래픽 API를 여기서 변경하지 않는다.
이 POC는 즉시 **전달**을 검증하며, 즉시 판정·키음·화면 출력까지 구현하지 않는다.
수신 스레드는 `Close()`에서 종료·회수되고 Raw Input 등록도 해제한다.

## 자동 검증

```powershell
go test ./...
go vet ./...

.\build\poc\inputpoc.exe -selftest -rate 8000 -duration 5s `
  -vsync=true -tps=60 -report artifacts\01-poc-input\vsync60.json -capture artifacts\01-poc-input\vsync60.png

.\build\poc\inputpoc.exe -selftest -rate 8000 -duration 5s `
  -vsync=true -tps=60 -stall 100ms -report artifacts\01-poc-input\stall100.json -capture artifacts\01-poc-input\stall100.png

.\build\poc\inputpoc.exe -selftest -rate 8000 -duration 5s `
  -vsync=false -tps=-1 -report artifacts\01-poc-input\uncapped.json -capture artifacts\01-poc-input\uncapped.png

# OS 경로 검사: POC가 전면일 때만 D/F/J/K 누름·해제 8개를 보낸다.
.\build\poc\inputpoc.exe -oscheck -report artifacts\01-poc-input\oscheck.json -capture artifacts\01-poc-input\oscheck.png
```

합성 테스트는 같은 수신 창에 전용 `WM_APP` 메시지를 게시한다.
각 키의 누름·해제를 따로 전달하며, 수신 스레드·콜백·큐·Ebitengine 소비 경로를
통과한다. 1ms 단위로 배치 게시하므로 **125µs 간격의 실제 장치 보고나
`WM_INPUT` 경로를 검증한 결과가 아니다**. OS 키보드 입력을 주입하지 않는다.

별도 `-oscheck`는 POC가 전면인 경우에만 Windows `SendInput`으로 D/F/J/K의
누름·해제 8개를 보낸다. 이를 실제 `WM_INPUT` → 디코더 → 콜백 → `Update()`에서
확인하고 종료한다. 다른 앱이 전면이면 키를 보내지 않으며 12초 뒤 실패 처리한다.
이 검사는 OS 입력 경로를 확인하지만 물리적 키보드의 보고율을 재현하지 않는다.

단위 테스트는 Raw Input 패킷 해석·Windows ABI·수신 창 종료/재시작·
프레임 사이 누름/해제 보존·큐 넘침·동시 소비·QPC 단조 증가를 확인한다.
고해상도 시계와 게임 객체의 큐 전달 구간에 이벤트당 힙 할당이 없는지도 검사한다.

실제 키보드 검증은 앱에서 D/F/J/K를 눌러 RAW 이벤트가 올라오는지 확인하고
F6으로 결과를 저장한다. 실제 8,000Hz 장치와 키음/화면까지의 지연은 별도 측정 대상이다.

## 코드 위치

- `internal/keyboard`: 메시지 전용 창·Raw Input 수신·패킷 해석
- `internal/clock`: Windows QPC 기준 시계
- `internal/clock/timeline.go`: 정지·배속 변경을 반영하는 게임 시계
- `internal/keys`: 게임 시간 입력 파일과 순서대로 소비하는 Player
- `cmd/poc/input/playback.go`: 입력 기록·정지·배속 제어·리플레이 적용
- `cmd/poc/input/inbox.go`: 콜백과 게임 사이의 고정 크기 큐
- `cmd/poc/input/main.go`: 옵션·실행·수신 루프 종료와 최종 저장
- `cmd/poc/input/game.go`: 콜백·Update 소비·키 상태·검증 상태
- `cmd/poc/input/report.go`: 측정 보고서 스키마와 JSON 저장
- `cmd/poc/input/view.go`: 레인·측정 화면
- `cmd/poc/input/load.go`: 합성 메시지 부하

`build/poc/`과 `artifacts/`는 로컬 빌드·검증 산출물이며 버전 관리에서 제외한다.
프런트엔드·브라우저 통신과 OS 입력 수집 경로를 섞지 않는 것이 이 단계의 범위다.
