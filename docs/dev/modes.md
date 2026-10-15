# 플레이 모드와 공용 코드

현재 구현은 레인별 키 입력을 판정하는 `piano` 모드다. `game`이라는 포괄적인 패키지 이름을 제거하고,
장면은 `internal/play`, 화면 없이 실행하는 판정과 저장 데이터는 `internal/piano`에 둔다.
플레이 코드는 `internal/play`에 바로 두며 별도의 장면 묶음 디렉터리를 두지 않는다.
`cmd/game`과 `game.exe`는 실행 진입점·배포 파일 이름으로 유지한다.

## 소유권

| 위치 | 책임 |
|---|---|
| `play/state.go`, `update.go` | piano 모드 한 판의 상태와 진행 순서 |
| `play/input.go`, `prepare.go` | 레인 키 바인딩, piano 모드 세션·Auto·리플레이 연결 |
| `play/view.go`, `field.go`, `notes.go`, `hud.go` | 레인 배치·노트·판정·piano 모드 점수의 화면 표현 |
| `play/network.go`, `history.go` | 멀티 통신을 플레이 진행에 연결하고 같은 채보 시점의 점수를 비교 |
| `play/chatstate.go`, `chat.go`, `drawer.go` | 채팅 창의 초안·스크롤·읽지 않은 메시지와 입력·표시·표시 데이터 |
| `multi/protocol.go`, `chat.go`, `selection.go` | 요청·이벤트·방·메시지 형식과 공유 플레이 조건 |
| `multi/client.go`, `sync.go` | WebSocket 연결과 서버 시계 동기화 |
| `server/multi` | 방·참가자·경기 운영과 채팅 배포 |
| `piano/session.go`, `score.go` | 홀드·롤·지뢰·리프트와 piano 모드 점수 규칙 |
| `piano/replay.go`, `osr.go`, `record.go`, `summary.go` | piano 모드 리플레이·OSR 가져오기·결과·달성 기록 |
| `storyboard/background.go`, `story.go` | 점수나 레인 배치와 무관한 배경·스토리보드 |
| `ui/text.go`, `pause.go` | 글꼴·텍스트 배치·일시정지 버튼과 카운트다운 표시 |
| `keyboard/queue.go` | 수신 스레드의 입력 이벤트를 게임 루프까지 순서대로 보관 |
| `clock` | 입력·플레이·동기화가 공유하는 시계와 고정 1배속 타임라인 |
| `sound` | 오디오 디코딩·배경음·키음·효과음·Time Rate·Pitch |
| `chart/osu`, `bms`, `ojn`, `sm` | 원본 포맷별 `chart.Format` 구현. `init`에서 `chart.Register`로 등록 (ojn은 .ojm 사운드 뱅크 포함) |
| `chart` (`format.go`, `load.go`, `scan.go`, `asset.go`) | 등록된 포맷으로 읽기·곡 목록·자산 접근·편집 저장·프리뷰 오디오 열기 |

공용 `storyboard`, `ui`, `keyboard`, `clock`, `sound`, `chart`는 `internal/piano`나 `play`를 import하지 않는다.
공용 배경과 오버레이는 게임 판정·입력·네트워크를 실행하지 않고 호출자가 전달한 데이터만 그린다.
`storyboard`와 `ui`는 서로 import하지 않는다. 배경·스토리보드는 그릴 대상의 크기를 사용하고,
텍스트·일시정지 화면은 `ui`에서 제공한다. 채팅 표시 데이터는 이를 사용하는 `play`가 소유한다.
일시정지 화면은 모드에서 먼저 화면을 그린 뒤 `ui.DrawPause`로 합성한다.
버튼 동작과 입력 해제·재개 시각은 해당 모드가 맡는다.

`keyboard.Queue`는 물리 키 이벤트를 저장하므로 레인 수나 판정 규칙을 알지 못한다.
오버플로는 Drain 이후에도 남으며, 모드는 이를 불완전한 입력으로 처리한다.
입력 수신과 게임 루프는 같은 `clock.Counter`를 사용한다.

## 태고 추가 시

태고 장면은 `internal/taiko`에 추가할 수 있다. 태고는 공용 시계·입력 큐·오디오·배경·글꼴·
일시정지 화면을 사용하고, 돈/캇 입력과 노트 표현·판정·연타·점수 규칙은 자체 구현한다.
piano 모드의 레인·홀드 판정이나 `piano.Score`를 태고의 공통 규칙으로 사용하지 않는다.

현재 채보의 노트 종류·레인 검증, 키 프로필, 리플레이의 lane-index·piano 모드 Rules·점수 저장 형식은
piano 모드 기준이다. 태고의 채보 지원과 기록 형식은 태고 구현 시 결정해야 한다.
기존 리플레이는 `ggosu-keys-v1`, `game-1x`, 버전과 JSON 필드를 유지한다.
재생 속도는 시간 진행량만 바꾸며 저장된 입력 시각을 다시 배율로 변환하지 않는다.

태고 패키지·가짜 판정 구현·미사용 모드 인터페이스는 이번 구조 변경에 추가하지 않았다.
두 번째 모드가 실제로 필요로 하는 계약이 확인되면 사용하는 쪽에서 작은 인터페이스를 정의한다.

## 패키지 통합

기존 21개 패키지를 13개로 줄였다.

- `density`, `mod`와 편집 데이터 복사 → `chart`의 파일.
- `format`, `preview`와 편집 파일 저장 → `library`의 파일. 이후 포맷 변환은 `chart/<포맷>`으로, 나머지는 `chart`로 옮겼다. `chart`는 `Format` 인터페이스만 알고 포맷 패키지는 사용하는 쪽이 blank import로 등록한다.
- `play`, `replay`, `record` → piano 모드 데이터 패키지 `piano`.
- 기존 `game`과 piano 모드 화면 → `play`.
- 배경·스프라이트 연출 → 모드와 무관한 `storyboard`.
- 글꼴·텍스트·일시정지 표현 → 모드와 무관한 `ui`.

장면 패키지는 `play`, 모드 패키지는 `piano`로 이름이 달라 import 별칭 없이 사용한다.
