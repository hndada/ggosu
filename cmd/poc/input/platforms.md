# OS별 입력 수신

수신 경로는 `OS 이벤트 → 디코드 → func(Event) → 고정 크기 큐`다.
이 경로에는 Sleep/Ticker, FPS/TPS 검사, 1ms polling 제한이 없다.
키보드의 보고율은 장치와 OS가 결정하며, 앱은 준비된 이벤트를 즉시 처리한다.

| OS | 구현 | 이 환경에서 확인 |
|---|---|---|
| Windows 64비트 | 전용 스레드의 WM_INPUT / Raw Input | 실제 실행·8,000 이벤트/초 합성 부하 |
| Linux amd64/arm64 | evdev / epoll | 입력 테스트 바이너리·전체 POC cross build |
| macOS amd64/arm64 | IOHIDManager / 전용 CFRunLoop | 입력 테스트 바이너리·전체 POC cross build |

게임의 `receive()`와 `inbox.push()`는 세 OS에서 같다. 루프 하나에서 이벤트 순서를
부여하므로 한 프레임 사이의 누름·해제를 하나의 최종 상태로 합치지 않는다.
입력 즉시 전달과 Update/Draw에서 반영하는 시점은 구분한다.

## 최소 수신 코드 위치

- Windows: `internal/keyboard/native_windows.go`의 `windowProc` / `read`
- Linux: `internal/keyboard/native_linux.go`의 EpollWait / Read 루프
- macOS: `internal/keyboard/native_darwin.go`의 입력 값 콜백
- macOS 초기화·외부 API 바인딩: `internal/keyboard/hid_darwin.go`
- Linux/macOS 공통 수명·전달: `internal/keyboard/source.go`
- 공통 키 식별자: `internal/keyboard/keys.go`

키는 ingress에서 한 번 정규화한다. Windows scancode, Linux evdev code,
macOS HID usage를 물리 키 위치로 읽는다. 문자 입력용 레이아웃 변환은 거치지 않는다.
D/F/J/K는 공통 ASCII 식별자이며, 수정·제어·기능 키는 VK 호환 식별자다.
`Scan`은 OS 고유 값을 유지한다. 알 수 없는 Windows scancode는 기존 VK로 보존한다.

## 실행

각 OS에서 같은 명령을 사용한다. backend는 Go의 OS build tag로 선택된다.

```sh
mkdir -p build/bin
go build -o build/poc/inputpoc ./cmd/poc/input
./build/poc/inputpoc -selftest -rate 8000 -duration 5s -stall 100ms
```

Windows 실행 파일은 `build/poc/inputpoc.exe`다. 이전 Linux/macOS 검증 빌드는 [옛 빌드 ZIP](../../../artifacts/19-build-cleanup-20261009/old-builds.zip)에 보존했다.
입력 POC의 이식 범위이며, 별도 Wails 곡 선택창의 OS별 실행 경로와 패키징은 별개다.

## Linux 사용 조건

시작할 때 읽을 수 있는 `/dev/input/event*` 키보드를 연다. 사용할 장치를 지정할 수 있다.

```sh
GGOSU_INPUT_DEVICES=/dev/input/event3 ./inputpoc
```

선택 장치의 읽기 권한이 필요하다. 권한·그룹·udev 규칙은 코드에서 변경하지 않는다.
읽을 수 있는 키보드가 없으면 화면에 알려주고 물리 입력을 비활성화한다.
합성 테스트는 독립된 pipe로 계속 동작한다. Hotplug는 아직 없으므로 재연결 후 재시작한다.
커널의 SYN_DROPPED는 조용히 무시하지 않고 오류로 보고한다.

활성 상태는 게임에서 SetActive로 갱신한다. evdev는 창 입력 보안 경계가 아니며,
Update가 막히면 포커스 변경 반영도 늦을 수 있다. 배포 전 창/세션 권한 연동은 별도 점검 대상이다.

## macOS 사용 조건

macOS 10.15 이상, 시스템 IOKit/CoreFoundation/AppKit을 동적으로 연결한다.
IOHID 입력 권한을 확인하고 승인되지 않았다면 화면에 알려준다. Input Monitoring을
승인한 뒤 재시작한다. 키보드를 독점하지 않는다. 앱 활성 상태와 NSRunningApplication.isActive를
검사한다. 이 속성 갱신도 AppKit 메인 run-loop 정책에 영향을 받는다.

HID 입력과 합성 wake 메시지는 같은 전용 CFRunLoop에서 전달된다. 초기화·matching dictionary와
외부 API 연결은 입력 콜백 밖에 있다. Close는 wake source를 통해 루프를 종료·회수한다.

## 검증 결과

- Windows go test ./..., go vet ./... 통과. 샌드박스 임시 폴더 rename 제한은
  같은 테스트를 일반 실행 환경에서 다시 실행해 확인했다.
- 변경 후 Windows 합성 부하: 약 7,999.1 이벤트/초, 40,000개 수신·반영,
  큐 드롭·순서 오류 0. Draw를 100ms 정지해도 수신 유지. 원본: artifacts/01-poc-input/portable-windows.json.
- Linux/macOS amd64·arm64 입력 패키지와 전체 Ebitengine POC 빌드 통과.
- 물리 키 코드, 수정 키 좌우, 누름·해제·반복 입력, 부분 패킷과 kernel queue loss 테스트 추가.
- Linux/macOS 루프 전달·종료 테스트 바이너리도 빌드했다. 실행 호스트가 없어 실제 실행은 미확인이다.

합성 부하는 Windows WM_APP, Linux pipe, macOS CFRunLoop wake source를 사용한다.
모든 OS에서 물리 키보드 8,000Hz를 측정한 결과가 아니며, 앱의 수신 경로에 rate limit을
두지 않은 구현과 빌드 검증을 구분한다.
