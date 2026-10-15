# 네이티브 입력과 게임 전달

제품은 internal/keyboard.Source로 OS의 물리 키 이벤트를 수신한다.
internal/play/native.go가 Source를 시작하고 종료 시 Close로 전용 루프를 회수한다.
입력 측정 화면·합성 부하·OS 경로 검사는 [입력 POC](../../cmd/poc/input/README.md)에 분리했다.

## 실행 경로

OS 키 이벤트 → keyboard.Event → keyboard.Queue.Push → scene.readInput → rules.Session.Input

OS 콜백은 시각과 키의 누름·해제를 큐에 저장한다.
internal/keyboard/queue.go의 Queue는 mutex로 보호하는 고정 크기 큐이고 게임 루프가 Drain으로 순서대로 소비한다.
한 프레임 사이의 누름·해제를 마지막 상태로 합치지 않는다. 큐가 넘치면 점수·기록을 신뢰할 수 없으므로 오류로 종료한다.

readInput은 포커스·일시정지·채팅 입력과 inputSince를 확인하고,
이벤트 발생 시각을 Timeline으로 곡 시간에 변환해 Session에 전달한다.
리플레이·Auto도 Player의 이벤트를 같은 Session에 전달한다.
브라우저 이벤트 수신은 internal/play/browser_js.go에 있고 네이티브 OS 수신과 별도 경로다.

## 코드 책임

| 위치 | 책임 |
|---|---|
| internal/keyboard/event.go, keys.go | 이벤트와 물리 키 식별자 |
| internal/keyboard/native_windows.go | 전용 스레드의 Raw Input·WM_INPUT |
| internal/keyboard/native_linux.go | evdev·epoll |
| internal/keyboard/native_darwin.go, hid_darwin.go | IOHIDManager·CFRunLoop |
| internal/keyboard/source.go | 공통 이벤트 전달과 루프 종료 |
| internal/clock | 입력 시각과 게임의 고정 1배속 시간축 |
| internal/keyboard/queue.go | 모드 공용 입력 큐·순서 보존·오버플로 감지 |
| internal/play/input.go | 레인 바인딩·물리 상태·판정 입력·기록 |
| internal/play/pause.go, chat.go | 정지·포커스·채팅 입력 제어 |

물리 키 위치를 ingress에서 정규화하며 문자 입력용 레이아웃 변환을 거치지 않는다.
Scan은 OS 고유 값을 유지한다. Source의 활성 상태는 게임에서 SetActive로 갱신한다.
Linux는 입력 장치 읽기 권한, macOS는 Input Monitoring 승인이 필요하다.

회귀 테스트는 keyboard 디코드·순서·루프 종료, clock 시간 변환,
게임의 짧은 누름/해제·큐 손실·포커스·재개 처리를 확인한다.
과거 OS별 빌드·합성 부하 수치는 [POC 플랫폼 검증](../../cmd/poc/input/platforms.md)과
[측정 기록](../../cmd/poc/input/validation.md)에 보관한다.
