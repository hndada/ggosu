# 입력 POC 검증 기록

2026-10-06, Windows amd64, Go 1.25.1, Ebitengine v2.10.4.
고해상도 측정은 QPC 10,000,000Hz로 진행했다.

실제 `WM_INPUT` 경로는 별도 `-oscheck` 실행으로 확인했다. POC 전면 창에
`SendInput`으로 보낸 D/F/J/K 누름·해제 8개가 RAW 수신 8개, Update 반영 8개,
기본 Ebitengine 입력 4회 누름·4회 해제로 확인됐다. 큐 드롭·순서 오류·번호 공백은 0이었다.
해당 첫 성공 실행의 콘솔 읽기 출력은 [보존 기록](../../../artifacts/01-poc-input/oscheck-first-success.json)에
옮겨 두었다. 이후 재실행은 포커스를 받지 못해 키를 보내지 않고 실패했다.
최근 실행 결과는 [OS 경로 보고서](../../../artifacts/01-poc-input/oscheck.json)에 남아 있으며 첫 성공과 구분한다.
최신 검사기는 누름·해제를 한 번의 SendInput 호출로 묶어 키를 눌린 상태로 남기지 않는다.

## 확인한 결과

모든 경우 5초 동안 40,000개 합성 이벤트를 게시하고, 수신 콜백과
Ebitengine `Update()`에서 40,000개 전부를 확인했다.
큐 드롭·수신 번호 공백·순서 오류는 모두 0이었다.

| 조건 | 실제 게시/초 | 콜백 p99 | Post→수신 p99 | 수신→Update p99 | 수신→Update 최대 |
|---|---:|---:|---:|---:|---:|
| VSync 켬 / 60TPS | 7,999.1 | 0.4µs | 124.8µs | 16.505ms | 20.612ms |
| 위 조건 + 2초마다 Draw 100ms 정지 | 7,998.7 | 0.4µs | 92.6µs | 19.100ms | 117.174ms |
| VSync 끔 / 프레임 동기 TPS | 7,998.8 | 0.4µs | 133.3µs | 1.157ms | 16.548ms |

프레임 동기 실행은 이 실행에서 약 1,186 FPS/TPS였다. VSync 실행은
약 240 FPS / 60 TPS였다. 목표 수신률 8,000과 렌더링·업데이트 빈도는 다르다.

원본 데이터:

- [60TPS JSON](../../../artifacts/01-poc-input/vsync60.json), [화면](../../../artifacts/01-poc-input/vsync60.png)
- [Draw 정지 JSON](../../../artifacts/01-poc-input/stall100.json), [화면](../../../artifacts/01-poc-input/stall100.png)
- [VSync 해제 JSON](../../../artifacts/01-poc-input/uncapped.json), [화면](../../../artifacts/01-poc-input/uncapped.png)

이 결과는 네이티브 메시지 수신→게임 콜백 전달이 다음 프레임을 기다리지 않고,
렌더링을 정지해도 수신 루프가 계속 동작하는 것을 보여준다.
반면 게임 상태를 `Update()`에서 반영하는 현재 구현은 프레임·틱 대기를 포함한다.
그래서 즉시 전달과 즉시 판정·키음·화면 출력은 별개다.

## 검증 범위와 한계

- 합성 입력은 전용 `WM_APP` 메시지이며 실제 `WM_INPUT` 키보드 데이터가 아니다.
- 1ms 단위 배치 게시이므로 125µs 간격의 장치 보고를 재현하지 않는다.
- 콜백 p99는 합성 경로의 값이다. 실제 Raw Input은 전면 앱 확인과 패킷 읽기·해석 비용이 추가된다.
- OS 키보드 입력의 WM_INPUT 경로는 SendInput으로 검증했으나, 실제 물리적 키보드/8,000Hz 장치,
  하드웨어 발생→수신 지연, 오디오·화면 출력까지의 지연은 미검증이다.
- p50·p95·p99는 최근 4,096개 표본이다. 평균·최대는 전체 40,000개 기준이다.
- 콜백 구간은 계측용 QPC 호출을 포함하며, 0.4µs를 실사용 전체 지연으로 해석하지 않는다.
- `go test ./...`, `go vet ./...`, 네이티브 GUI 빌드와 세 자동 실행은 통과했다.
- Raw Input 디코더, 64비트 Windows ABI, 메시지 루프 종료/재시작, 프레임 사이 이벤트 보존,
  큐 넘침·동시 소비, QPC 단조 증가·무할당 읽기, 큐 전달 무할당을 테스트했다.
- 첫 실제 WM_INPUT 실행에서 버퍼 정렬 문제로 GetRawInputData가 주소를 거부했다.
  버퍼를 8바이트 정렬 저장소로 바꾸고 정렬 회귀 테스트와 OS 입력 경로 검사로 확인했다.
- 레이스 검사는 CGo용 `gcc`가 설치되지 않아 실행하지 못했다. 이 POC를 위해 컴파일러를 설치하지 않았다.
- GUI는 샌드박스의 `ChangeWindowMessageFilterEx` 거부로 샌드박스 밖에서 실행했다.

## 실제 키보드 확인 방법

1. `build/poc/inputpoc.exe`를 실행하고 앱 창에 포커스를 둔다.
2. D/F/J/K를 눌렀다 놓는다. RAW 개수·레인 색·최근 이벤트를 확인한다.
3. F6을 누른다. 기본적으로 `artifacts/01-poc-input/manual.png`와 `artifacts/report.json`이 저장된다.
4. 다른 앱으로 이동해 키를 입력한다. 이 POC의 RAW 수신 개수가 증가하지 않는지 확인한다.
5. 창을 닫는다. 수신 스레드와 Raw Input 등록을 정리하고 최종 보고서를 저장한다.
