# 게임 시간과 리플레이

이 문서는 제품의 `cmd/game`과 `internal/piano` 기준이다.
입력 측정 프로그램의 조작·실험 기록은 [POC 리플레이 문서](../../cmd/poc/input/replay.md)에 따로 보존한다.

## 시간축

OS 입력은 `internal/clock.Counter`의 시각으로 수신한다.
`clock.Timeline.Position`은 입력이 발생한 시각에 적용되던 배속으로 게임 위치를 계산한다.
정지와 배속 변경은 타임라인 구간을 추가하며 과거 입력의 시각을 바꾸지 않는다.

리플레이의 `time_base`는 `game-1x`, `at_ns`는 고정된 곡 시간의 나노초다.
2배속으로 실제 5초 후 입력했다면 곡 시간은 10초다. 재생 속도를 바꿔도 저장한 10초를 다시 곱하거나 나누지 않는다.
같은 시각의 이벤트는 배열 순서를 유지하므로 한 프레임 사이의 누름·해제를 보존한다.

## 채보와 판정 일치

제품은 `rules.NewChartReplay`로 `lane-index` 기록을 만든다.
`Replay.Match`는 채보 SHA-256·레인 수·판정 규칙·배치 설정을 확인하고 불일치하면 재생을 거부한다.
`rules.Auto`도 일반 입력 이벤트를 생성해 같은 `rules.Session.Input` 판정을 거친다.
`ReadOSR`은 osu!mania 입력을 제품 레인 이벤트로 변환한다.

## 저장·복원 연결

| 위치 | 책임 |
|---|---|
| `internal/play/input.go` | Session에 적용한 레인 입력을 기록 |
| `internal/play/update.go` | 현재 게임 위치까지 도착한 리플레이 이벤트를 소비 |
| `internal/play/native.go`, `browser_js.go` | 채보 일치 확인·저장된 배치·오프셋·프로필 복원 |
| `internal/play/prepare.go` | 설정 적용·Session·Auto 이벤트 준비 |
| `internal/play/record.go` | 종료 시 held 입력 해제·기록 길이·결과 저장 |
| `internal/piano/replay.go` | 파일 검증·Auto 이벤트·Player |
| `internal/piano` | 점수 규칙 버전과 메타데이터를 포함한 결과 파일 |
| `internal/menu/records.go`, `launch.go` | 기록 조회·재생 실행 설정 |
| `internal/server/web/static/web.js` | 브라우저 기록 저장·가져오기·다운로드 |

미완주 기록은 종료 시각까지만 재생한다. 초기 Time Rate와 Pitch, Mods와 실제 매핑,
오프셋·개인 프로필의 복원 범위는 [플레이 옵션](../guide/play-options.md)을 참고한다.
