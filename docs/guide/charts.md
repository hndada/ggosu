# 채보와 플레이 경로

`chart.Chart`를 게임이 직접 읽는다. 원본 채보를 `.osu`로 재변환하지 않는다.

```text
원본 .osu / .ojn / .bms / .bme / .bml / .sm / .ggc
    → chart.Decode (파일에 들어 있는 모든 난이도)
    → chart.Chart (검증·정렬)
    → chart.Load (osu는 외부 .osb 병합)
    → rules.Session (입력 시각·판정·키음 이벤트)
    → internal/play (네이티브 입력·Ebitengine 화면·오디오)
```

`cmd/game`은 CLI 옵션과 네이티브·웹 실행 진입점만 제공한다.

## 곡 폴더

곡 선택은 실행 파일 옆 `songs` 폴더와 환경 설정 **곡 폴더** 탭에 추가한 로컬 폴더를 함께 읽는다. 경로를 붙여 넣거나(따옴표·끝 슬래시 허용) **찾아보기**로 고른다. 목록은 `settings.json`의 `songFolders`에 절대 경로로 저장하며, 추가·삭제하면 곡 목록을 다시 읽는다. 없어진 폴더는 곡 목록 오류로만 알린다.
추가 폴더는 목록을 읽을 때 음원 길이를 미리 재지 않는다. 키음 뱅크 전체를 읽으면 O2Mania `Music\o2jam`처럼 큰 폴더의 로딩이 오래 걸리기 때문이다. 곡을 선택하면 BGM·노트 시작 키음·롱노트 끝 키음의 재생 시각에 실제 샘플 길이를 더한 최댓값을 계산해 **전체 길이**로 표시한다. OJN 헤더의 길이는 사용하지 않는다. 길이는 원곡 1배속 기준이며, 음원 누락·손상으로 계산할 수 없으면 **미확인**으로 표시한다. 샘플 길이는 `audio-lengths.json`에 저장해 재선택·재실행 시 재사용하고, 음원 파일의 크기나 수정 시각이 바뀌면 다시 계산한다. 파일은 CPU 코어 수만큼 동시에 읽는다. O2Mania `Music\o2jam`의 OJN 1,150여 개(4,008채보)는 16코어에서 약 1.4초(한 코어로는 약 9초)에 읽으며, 결과는 한 코어로 읽을 때와 같다.
웹 서버는 `-songs` 폴더만 읽는다.

`hndada/gosu`의 메타데이터, Dynamics, 노트/롱노트, Sample, Replay 구분을 출발점으로 삼았다.
참고한 커밋은 `ec20db35ccf58f8d558304e4c69bf60ec89b9005`다. 이 저장소의 게임 및 포맷 구현은 새로 작성했으며,
기존 gosu 전체를 의존성으로 가져오지 않는다. 특히 기존 OJN→osu 변환에서 빠지는
마디 중간 BPM, 마디 길이, 키음, 배경음, 세 난이도를 직접 보존한다.

## Universal v1

직렬화 확장자는 `.ggc`이며 JSON이다. `internal/chart/chart.go`가 실제 스키마와 검증의 기준이다.

| 필드 | 의미 |
|---|---|
| `version` | 현재 1. 다른 버전은 오류 |
| `metadata` | 곡명, 아티스트, 제작자, 난이도, 원본 레벨 문자열, 미리듣기/리드인 |
| `source` | 원본 포맷, 파일 경로, 파일 내부 난이도 번호, 원본 바이트 SHA-256/MD5. 편집본은 `format: "ggc"`와 `originalFormat`으로 원본 형식을 보존 |
| `lanes` / `layout` | 왼쪽부터 0-based 레인. 1~18개. layout 생략은 일반 건반. BMS는 스크래치와 양쪽 그룹을 구분 |
| `notes` | tap, hold, roll, mine, lift, fake. `at_ns`, `end_ns`, lane, 시작/끝 키음 |
| `timing` | 해당 시점부터의 완전한 BPM/scroll/meter/highlight 상태와 정지 길이 |
| `assets` | 논리 ID → 채보 디렉터리에 상대적인 오디오/이미지/영상/키음 뱅크 경로 |
| `audio` | 배경음·자동 키음·스토리보드 Sample의 시간별 트리거 |
| `storyboard` | 배경 변경, 영상 메타데이터, 휴식, sprite와 명령. 미지원 원문은 `unsupported`에 보관 |
| `warnings` | 해석하거나 재현하지 않은 기능의 진단 |

모든 시각은 **배속이 적용되지 않은 곡 오디오 시간축의 signed int64 나노초**다.
음수는 리드인과 타이밍 포인트를 표현하며 허용 범위는 ±24시간이다.
tap은 `end_ns == at_ns`, hold/roll은 `end_ns > at_ns`다.
볼륨 0은 무음이고 1은 원래 크기다. pan은 -1~1이다.

시간이 같은 이벤트는 원본 순서를 유지한다. 같은 레인에서 겹치는 노트·중복 시작,
짝 없는 롱노트, 역전된 시간, 잘못된 숫자/레인/키음 참조는 오류다.
`chart.Normalize` 이후 `Validate`를 호출하며, JSON 직접 읽기는 이미 정렬된 자료를 요구한다.

`chart.Clock`은 beat→시간 변환을 담당한다. BPM 변화는 마디 안에서도 적용한다.
정지 시점의 노트는 정지 **직전**에 발생하고, 이후 beat의 시간에는 정지를 더한다.
SM OFFSET의 부호는 StepMania와 같이 적용한다. 스크롤 위치는 BPM×스크롤 배율을 적분하되 정지 중 고정한다.
판정은 이 스크롤 위치에 영향을 받지 않는다.

## 지원 범위

| 원본 | 구현된 부분 | 현재 제한 |
|---|---|---|
| osu!mania `.osu` | Mode 3, 1~18K, tap/hold, 기본·상속 타이밍/SV, 샘플 선택·볼륨, Unicode 메타데이터, 배경/휴식/스토리보드 Sample | standard/taiko/catch 및 변환 채보는 오류 |
| O2Jam `.ojn` | CP949 메타데이터, Easy/Normal/Hard, 7K, 마디 길이, 마디 내 BPM, LN, WAV/OGG 샘플 참조·pan·volume, 자동 키음 | M30, OJM, 암호화 OMC 키음 재생 지원. 실제로 잘린 뱅크도 앞쪽의 온전한 샘플은 재생하며 손상된 부분은 진단 |
| BMS `.bms/.bme/.bml` | UTF-8/Shift-JIS, 5/7키+스크래치 및 양손, BGM, WAV/BMP, 직접·확장 BPM, STOP, 마디 길이, LNTYPE 1/LNOBJ | RANDOM/IF와 LNTYPE 2는 오류. 투명·poor BGA 및 미지원 채널은 완전 재현하지 않음 |
| StepMania `.sm` | 파일 내 여러 NOTES, dance/pump/kb7/beat 레이아웃, BPM/STOP/OFFSET, tap/hold/roll/mine/lift/fake | warp/negative BPM/delay/scroll/speed/time signature 확장은 오류. BGCHANGES 미구현. `.ssc` 미지원 |
| Universal `.ggc` | 스키마 검증, 읽기/쓰기, 공통 플레이 | 편집 화면에서 osu!mania `.osu`로만 내보냄 (BMS/SM 등 그 밖의 원본 포맷으로는 변환하지 않음) |

O2Mania가 재생하는 OJN은 그대로 읽는다. 채널 9 이상은 모두 배경음이다(O2Mania 채보는 23~29번도 쓴다). 한 난이도의 구간이 깨졌으면 그 난이도만 빼고 나머지를 읽는다. 머리 없는 LN 끝, LN 안의 탭, 겹친 LN 머리, 같은 레인에 겹친 노트는 버리고, 끝나지 않은 LN과 길이 0인 LN은 탭으로 바꾼다. 고친 내용은 채보 경고로 남긴다. 원래 정상인 채보는 결과와 해시가 바뀌지 않는다.

키음 뱅크는 one-based 참조를 사용한다. OJN type의 OGG 플래그에는 1000을 더한다.
M30은 무암호화, nami(16), 0412(32)를 지원한다. 음원은 WAV/Ogg/MP3를 48kHz stereo PCM으로 사전 디코딩한다.
osu 기본 hitsound가 없으면 자체 합성한 짧은 음을 사용하며, osu 스킨 음색을 복제하지 않는다.
누락된 사용자 음원/이미지는 실행 중 경고에 표시된다.

## 스토리보드

채보 내부 Events와 같은 폴더의 `.osb`를 병합한다. Sprite의 F/M/MX/MY/S/V/R/C를
시간 기준으로 평가하고 배경 이미지와 함께 그린다. easing 0/1/2는 각각 linear/out/in이며,
다른 easing은 경고를 내고 linear로 재현한다. 위치는 osu의 640×480 좌표계에서 확대한다.
영상·Animation·Loop·Trigger·Parameter는 원문/메타데이터를 보관하고 경고한다.
미지원 그룹의 하위 명령을 일반 명령으로 잘못 재생하지 않는다.
현재 sprite는 플레이필드 아래에 그리며, osu의 Pass/Fail 조건 및 Overlay 합성까지 호환되는 렌더러는 아니다.

## 게임·리플레이

브라우저도 같은 Go 게임·판정 코드를 WebAssembly로 실행한다. 웹 입력과 실행 방법은 [웹 안내](web.md)에 정리했다.

Wails의 곡 목록은 실제 `songs` 디렉터리를 읽는다. 원본 하나에 난이도가 여러 개면 각각 표시한다.
플레이는 목록에 있는 파일/난이도를 재검증한 뒤 `game.exe`에 전달한다.
이 실행 파일은 기존 사각형 샘플 대신 실제 건반 플레이를 실행한다.

기본 입력은 기존 `internal/keyboard`의 네이티브 수신 스레드와 공유 QPC 시계다.
수신 콜백은 고정 크기 큐에만 쓰며 Update에서 시간 순으로 판정한다.
이미 소비한 게임 시각보다 늦게 전달된 입력은 마지막 소비 시각으로 제한한다.
큐 넘침과 수신 오류는 플레이를 실패시켜 잘못된 기록을 저장하지 않는다.
반복 down을 제거하고, 일시정지·포커스 상실에서는 레인을 해제한다.
`-input ebiten`은 프레임 단위 입력 대안이다.

통일 판정 규칙 ID는 `ggosu-keys-v1`이다. osu/O2Jam/BMS/SM 각각의 원래 점수 규칙을 복제하지 않는다.

| 판정 | 오차 | 점수 가중치 |
|---|---:|---:|
| KOOL | ≤22ms | 1 |
| COOL | ≤45ms | 1 |
| GOOD | ≤90ms | 0.5 |
| MISS | 나머지, 또는 135ms 창 이후 | 0 |

홀드 시작·끝은 각각 판정한다. 끝까지 누르면 끝 판정은 자동 KOOL이고 일찍 떼면 오차로 판정한다.
hold·roll 시작이 MISS면 끝도 즉시 MISS로 처리하며 유지·반복 입력으로 끝 점수를 얻을 수 없다.
Roll은 200ms 이내에 다시 눌러야 유지된다. Mine은 ±135ms 내 누르거나 유지하면 MISS,
피하면 KOOL다. Lift는 해제, Fake는 표시용이다. MISS는 콤보를 끊는다.
점수는 Flow 70만·Precision 30만·KOOL 보너스 10만을 누적하는 110만점 만점이다.
Accuracy는 처리한 판정의 가중치 평균이며 회복 요소·보너스와 별개다. 완주 전 값은 최종 정확도가 아니다.
계산식과 기존 기록 호환 범위는 [점수 규칙](score.md)을 참고한다.

레인 리플레이 JSON은 음수 리드인의 입력도 보존하고, 기존 version 2 포맷에 `lane-index` 인코딩, 채보 digest, 레인 수,
규칙 ID를 추가한다. 물리 키와 OS에 독립적이고 다른 채보/규칙에서는 재생을 거부한다.
기존 POC `windows-vk` 기록의 호환성은 유지한다. `.osr`은 osu!mania만 읽고 원본 MD5를 확인한다.
원본의 점수 숫자는 사용하지 않고 같은 통일 판정 경로로 다시 계산한다. mods는 0 또는 Autoplay만 허용한다.
배속은 게임이 시간축을 따라가는 속도만 바꾸며, 리플레이의 저장 시각은 바꾸지 않는다.

곡 선택창에서는 일반/Auto 실행, 결과 화면, 다시 하기, 저장한 기록의 리플레이 재생을 제공한다.
결과와 입력은 `data/records/`의 한 파일에 저장한다. Auto·리플레이·배속·미완주 기록은
일반 플레이 최고 기록과 구분한다. CLI는 `-result`와 `-replay-record`로 같은 파일을 사용할 수 있다.

오디오는 Ebitengine 출력 버퍼와 프레임에서 시작한다. 배속에서 Pitch On은 음높이도 바꾸며, `-pitch=false`는 원래 음높이를 유지한다.
Pitch Off는 `internal/play/stretch.go`의 WSOLA로 원래 음높이를 유지한다.
샘플 단위 오디오 마스터 동기화와 출력 장치 지연 보정은 아직 제공하지 않는다.
시킹은 새 Session을 만들고 입력을 해당 위치까지 재소비해야 하며 현재 화면에는 시킹 조작을 넣지 않았다.

## 실행

```powershell
$env:GOMODCACHE = Join-Path (Get-Location) '.cache/mod'
$env:GOCACHE = Join-Path (Get-Location) '.cache/build'

# 포함된 데모: 키음·BGM·배경·움직이는 sprite·변속·롱노트
go run ./cmd/game -chart songs/demo/demo.osu

# OJN 난이도: 0 Easy, 1 Normal, 2 Hard
go run ./cmd/game -chart songs/my-song/song.ojn -index 2

# 기록과 2배속 재생
go run ./cmd/game -chart songs/demo/demo.osu -record artifacts/play.json
go run ./cmd/game -chart songs/demo/demo.osu -replay artifacts/play.json -speed 2

# 검증·전체 난이도 검사·공통 포맷 내보내기
go run ./cmd/chart -input songs/demo/demo.osu -verify
go run ./cmd/chart -input songs/my-song/song.ojn -index 2 -out songs/my-song/hard.ggc

# 실제 게임 자동 플레이와 캡처
go run ./cmd/game -chart songs/demo/demo.osu -auto -duration 11s `
  -capture artifacts/chart.png -report artifacts/score.json

# 곡 선택창과 플레이 실행 파일 빌드
pwsh -NoProfile -File scripts/desktop.ps1 build
```

`-keys D,F,J,K`처럼 레인 키를 지정할 수 있다. P는 예약 키다.
P 일시정지/3초 카운트다운 후 재개, F6 기본/집중 HUD, F7 배속, PageUp/PageDown 표시 속도, Esc 종료.
캡처 종료 기준은 `-frames` 또는 `-duration`이다. Draw 빈도는 60Hz로 고정되어 있지 않으므로
끝까지의 검증에는 `-duration`을 사용한다.

내보낸 `.ggc`는 상대 에셋 경로를 유지하므로 에셋이 있는 채보는 원본 옆으로 내보낸다.
앱 배포 시 `gosu.exe`, `game.exe`, `songs/`를 함께 둔다.
자체 제작 데모 에셋은 `go run ./cmd/poc/demo`로 재생성할 수 있다.

## 검증과 참고 자료

현재 제품 검증은 `pwsh -NoProfile -File scripts/check.ps1`로 실행한다.
실험은 [POC 문서](../../cmd/poc/README.md)에서 별도로 검사한다. 아래 수치·실행 기록은 이전 기능 구현 시 확인한 결과다.

- 파서·CP949/채널 해석·타이밍·롱노트·레인 리플레이 바인딩·OSR·경로 보호·스토리보드 평가 테스트.
- 1ms/17ms/250ms/7s 간격 소비에 대한 같은 리플레이의 판정 및 결과 비교.
- gosu의 실제 `triangles` 4K와 `circles!` 7K 채보를 읽고 자동 판정: 각각 193/192 KOOL, MISS 0.
- 창에서 데모 완주: 14 KOOL, MISS 0. 저장한 리플레이의 2배속 재생도 같은 결과.
- OJN/OJM 자체 데모 실제 창: 5 KOOL, MISS 0. BMS 자체 데모 실제 창: 8 KOOL, MISS 0.
- SM 4K 자체 데모 실제 창: 9 KOOL, MISS 0. 결과·리플레이 묶음의 완주 상태와 저장된 입력을 확인.
- 텍스트/바이너리 채보 퍼즈 10초: 약 318만 입력, 실패 없음. 이 결과가 모든 상용 채보의 호환성을 보증하지는 않는다.
- 실제 상용 OJN/BMS/SM의 전체 음원·시각 비교 및 물리 건반의 플레이 판정은 별도 검증 대상이다.

자료: [gosu](https://github.com/hndada/gosu/tree/ec20db35ccf58f8d558304e4c69bf60ec89b9005),
[osu 공식 포맷](https://osu.ppy.sh/wiki/en/Client/File_formats/osu_(file_format)),
[Open2Jam OJN](https://github.com/open2jamorg/open2jam/blob/master/parsers/src/org/open2jam/parsers/OJNParser.java),
[Open2Jam OJM/M30](https://github.com/open2jamorg/open2jam/blob/master/parsers/src/org/open2jam/parsers/OJMParser.java),
[StepMania SM 로더](https://github.com/stepmania/stepmania/blob/5_1-new/src/NotesLoaderSM.cpp).

## 롱노트 해제와 기록 규칙 (2026-10-12)

신규 플레이는 `ggosu-keys-v3`를 사용한다. hold는 끝에서 키를 떼어야 하며, 떼는 판정 창은 osu!lazer처럼 머리 판정의 1.5배다(KOOL 33ms, COOL 67.5ms, GOOD 135ms). 끝 시각 +135ms를 지나도록 누르면 끝 판정이 MISS다. roll의 지속 입력 규칙은 유지한다. 기존 v2 기록은 머리와 같은 떼기 판정 창으로, v1 기록은 당시 자동 끝 판정 규칙으로 읽고 재생하며 새 규칙의 최고점 집계와 구분한다.

점수 기록은 완주한 양수 점수 또는 체력에 의해 탈락한 결과만 새로 저장한다. 일반 중도 종료·시작 취소는 점수 파일/브라우저 기록을 만들지 않는다. 체력 탈락은 0점도 보존하고 `completed:false`와 단계·남은 체력·탈락 시각을 저장한다. 명시적인 CLI `-record` 디버그 리플레이 출력은 별도다.

`replay.pauses`에 정지, 재개 요청, 카운트다운 취소, 실제 재개, 정지 상태 종료를 순서대로 저장한다. 각 항목의 `at_ns`는 곡의 고정 1배속 좌표, `elapsed_ns`는 해당 실행 시작 이후 실제 시간이며 `reason`과 `rate`도 포함한다. 판정 입력 시각에는 정지 시간을 더하지 않는다. 결과 화면은 정지 횟수·총 시간·재개 취소 횟수를 표시한다. 빠른 재시작은 새로운 실행과 기록을 시작한다.

일시정지는 유지 중인 롱노트를 강제로 해제하거나 체력을 깎지 않는다. 재개할 때 실제로 누르지 않는 키의 해제를 적용하므로 카운트다운 동안 다시 누르고 있어야 이어서 유지할 수 있다.
