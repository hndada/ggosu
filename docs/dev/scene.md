# 플레이 Scene

`hndada/gosu`를 아이디어 레퍼런스로 삼아 키보드 플레이 Scene을 구성했다.
확인한 `level` 브랜치 커밋은 `ec20db35ccf58f8d558304e4c69bf60ec89b9005`다.
원본 구현을 의존성으로 가져오거나 원본 스킨·음원을 복사하지 않았다.

## 화면과 상태

1280×720 논리 화면에 중앙 레인, 좌측 콤보·판정·재생 정보, 우측 점수·정확도·판정 개수,
상단 곡 정보·채보 진행률, 하단 오차 미터를 배치한다. 창 비율이 다르면 Ebitengine이 화면을 맞춘다.
기본 HUD와 집중 HUD는 시작 전 설정하며 리플레이에서만 F6으로 전환한다. 집중은 측면 상세 HUD·곡 제목·오차 미터를 숨긴다. 곡/설정·실시간 비교·점수는 왼쪽 위/가운데/아래에, 채팅은 오른쪽 서랍에 둔다. 시간 표시는 설정의 막대·파이를 따른다.
콤보·판정은 기본적으로 레인 가운데에 표시하며 각각 레인 아래·왼쪽 HUD로 바꿀 수 있다. HUD 모드 전환은 위치를 바꾸지 않는다.
`view.SetFocus`는 표시 캐시만 재구성한다. 게임 상태·판정·오디오·리플레이는 변경하지 않는다.
레인 수 1~18, 홀드·롤·지뢰·리프트·Fake를 지원한다. 스크래치 역할과 더블 플레이 그룹은
`chart.Layout`에 있는 정보로 폭·색상·그룹 간격을 정한다.

시작 대기(Ready), 플레이(Playing), 일시정지(Paused), 오디오 종료 대기(Outro), 완료(Finished)를
명시적인 상태로 표시한다. 음수 시각의 노트·오디오가 있으면 그 시각부터 플레이 표시를 시작한다.
시작 대기·일시정지·종료 안내는 레인 영역에 표시하고 HUD는 유지한다.
재개 카운트다운은 `Paused` 상태의 `viewState.Countdown`으로 표시한다. 실제 3초의 종료·취소와 오디오 정지는 `internal/play/pause.go`가 맡으며 Scene은 숫자만 읽는다.
포커스 이탈과 Esc/P·웹 재개 버튼은 같은 정지·재개 경로를 사용한다. 정지 화면은 전체 dim과 재개·빠른 재시작·복귀 버튼을 그린다. 카운트다운 중의 입력은 판정으로 전달하지 않으며, 종료 후 큐에 남은 재개 이전 이벤트도 버린다. 재시작은 game이 세션·기록·타임라인·오디오·scene을 초기화하며, 디코딩된 에셋과 replay의 game-1x 좌표를 유지한다.
채보 진행률은 음원 길이와 별개이며, 노트가 끝난 뒤 남은 BGM·키음의 재생이 끝날 때까지 기다린다.
결과·기록·설정·곡 선택은 Wails가 계속 담당한다.

## 채택한 아이디어와 변경 근거

| 레퍼런스 | 현재 선택 | 판단 근거 |
|---|---|---|
| [Play의 구성 요소 분리](https://github.com/hndada/gosu/blob/ec20db35ccf58f8d558304e4c69bf60ec89b9005/game/piano/play.go) | 배경·필드·노트·HUD를 역할별 파일로 분리 | 가독성: 판정·입력·오디오와 화면 코드를 따로 읽고 수정할 수 있다 |
| [키 수별 레인 폭·색상](https://github.com/hndada/gosu/blob/ec20db35ccf58f8d558304e4c69bf60ec89b9005/game/piano/options.go) | 대칭 색상·중앙 키 강조, 스크래치·그룹은 원본 레이아웃 사용 | 확장성: 게임 종류를 키 수로 추측하지 않고 실제 레인 정보를 반영한다 |
| [미리 계산한 위치와 표시 범위](https://github.com/hndada/gosu/blob/ec20db35ccf58f8d558304e4c69bf60ec89b9005/game/piano/notedrawer.go) | 누적 스크롤과 노트 시작·끝 위치를 준비 시 계산하고 레인별 범위를 이진 검색 | 성능: 매 프레임 전체 노트와 타이밍을 반복 순회하지 않는다 |
| 같은 NoteDrawer의 프레임별 인덱스 수집·정렬 | 정렬된 레인 목록을 역순으로 그린다 | 성능·가독성: 임시 목록·정렬을 없애고 가까운 노트가 먼 노트를 덮는 순서를 명시한다 |
| 같은 NoteDrawer의 키음 기본값·롱노트 위치 변경 | 렌더링은 채보와 점수를 수정하지 않는다 | 확장성: 판정·리플레이의 원본 해시와 화면 설정이 서로 영향을 주지 않는다 |
| [배속 변경 시 위치 재계산](https://github.com/hndada/gosu/blob/ec20db35ccf58f8d558304e4c69bf60ec89b9005/game/piano/play.go) | 불변 위치에 현재 스크롤 배율만 투영 | 성능: 스크롤 속도를 바꿀 때 전체 노트 위치를 다시 수정하지 않는다 |
| [Update() any와 공유 Context](https://github.com/hndada/gosu/blob/ec20db35ccf58f8d558304e4c69bf60ec89b9005/scene/context.go) | 구체적인 `view`, 읽을 세션, 명시적인 프레임 `State` | 가독성·확장성: 현재 필요한 책임과 전달 데이터를 타입으로 드러낸다 |
| [필드 안의 콤보](https://github.com/hndada/gosu/blob/ec20db35ccf58f8d558304e4c69bf60ec89b9005/game/piano/options.go) | 콤보·판정은 레인 가운데가 기본이며 개별 위치 변경을 지원. 점수는 측면 HUD | 사용자 선택: 노트를 보면서 판정·콤보를 확인하고 필요하면 레인 밖으로 이동 |
| [이미지 기반 숫자·스킨](https://github.com/hndada/gosu/blob/ec20db35ccf58f8d558304e4c69bf60ec89b9005/game/scoredrawer.go) | 벡터 도형·한 번 준비한 폰트·변경 시 갱신하는 HUD 문자열 | 확장성: 외부 스킨 없이 실행하고 이후 필드·HUD 렌더러에 스킨을 연결할 수 있다 |

현재 UI는 판정한 점수를 그대로 표시한다. 점수 보간 애니메이션은 넣지 않았다.
기본 색상·레이아웃은 Scene의 기본값이다. 배경 dim·표시 여부·스크롤·변속은 [개인 플레이 설정](../guide/preferences.md)에서 변경한다.
기존 지원 범위대로 영상 재생과 일부 스토리보드 명령은 미지원이다.

## 책임과 연결

```text
Wails 선택·설정
    → cmd/game: 실행 옵션
        → internal/play: 입력 / 1배속 게임 시간 / 리플레이 / 오디오 / 프로세스 종료
            → rules.Session: 판정·점수·키음 이벤트
            → view.Update(viewState): 이미 판정된 결과와 화면 상태를 읽음
            → view.Draw: 배경 → 필드·노트 → HUD → 상태 안내
    → Wails 결과·기록
```

- `internal/play/draw.go`: Scene 생성과 Ebitengine Draw 연결.
- `internal/play/view.go`: 상태·판정 표시 갱신과 합성 순서.
- `internal/play/field.go`: 레이아웃·불변 스크롤 위치·표시 범위.
- `internal/play/notes.go`: 클리핑·노트·롱노트·키·입력 효과.
- `internal/play/hud.go`: piano 모드 HUD 캐시·진행률·오차 미터·상태 안내.
- `internal/storyboard/background.go`, `story.go`: 미리 분류한 배경과 곡 시점별 스프라이트 연출.
- `internal/ui/text.go`, `pause.go`: 모드 공용 글꼴·텍스트 배치·일시정지 화면.
- `internal/play/meter.go`: 성공한 입력의 오차 표본.

배경·스토리보드는 레인과 HUD 뒤에서만 합성한다. 장식이 게임 입력 영역을 덮지 않도록 한 정책이다.
스토리보드의 640×480 좌표는 16:9 화면에 균일하게 맞춘다.
정적 필드·HUD는 한 번 그려 재사용하고, 노트는 레인 크기의 오프스크린 이미지에서 잘라 합성한다.
게임 시간과 입력 이벤트 큐는 유지하며 Draw에서 판정·오디오를 진행하지 않는다.

윈도우에서는 설치된 맑은 고딕을 읽어 한글 제목을 표시한다. 폰트 파일은 배포물에 복사하지 않는다.
없는 경우와 다른 플랫폼·웹은 번들 Go 폰트를 사용하며 해당 폰트의 문자 지원 범위가 적용된다.

## 성능 확인

Windows/amd64, AMD Ryzen 7 7800X3D에서 2만 노트·256개 타이밍·7개 레인으로 측정했다.

| CPU 조회 경로 | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| 레인별 표시 범위 조회 | 112.6 | 0 | 0 |
| 이전 전체 노트·선형 타이밍 순회 | 11,039,358 | 0 | 0 |

고정된 한 프레임 위치를 반복 조회한 CPU 벤치마크다. Scene 전체 Update/Draw 비용·GPU·FPS·입력 지연 측정값은 아니다.
새 경로는 준비 시 위치 캐시를 만들고 메모리를 추가로 사용한다. 그리기 비용은 표시 후보 수에 따라 달라진다.
스크롤 배율 0에서 노트가 같은 위치에 겹치거나 긴 홀드 안에 많은 Fake가 들어가면 후보 수가 늘 수 있다.
스토리보드 보간의 임시 슬라이스도 고정 크기 배열로 바꾸고 기존 보간 테스트로 결과를 확인했다.

```powershell
go test ./internal/play -run '^$' -bench BenchmarkField -benchmem -benchtime=300ms
```

## 검증

아래 14,000점은 Scene 도입 당시 이전 점수 규칙의 결과다. 현재 점수는 [110만점 규칙](../guide/score.md)이다.

- `go test ./...`, `go vet ./...`, Wails 빌드, WebAssembly 빌드 통과.
- 변속·스크롤 배율 0·정지·동일 시각 타이밍·음수 시각에서 기존 `Chart.Position`과 위치 일치.
- 시작이 화면 밖에 있는 롱노트도 표시 범위에서 누락하지 않음.
- 1~18키 레이아웃이 HUD와 겹치지 않음.
- 표시 갱신·동일 시각 판정·반복 갱신이 점수·채보·오차 표본을 변경하거나 중복 생성하지 않음.
- 실제 GPU의 일시정지 캡처 전후에도 채보 해시·점수·판정 개수 유지.
- 실제 4키·7키 플레이, 시작 대기, 일시정지, 완료 캡처를 열어 확인.
- osu!mania 데모 완주: 14 KOOL, 14,000점, 입력 24개로 기존 결과 유지.

로컬 확인 파일: `artifacts/07-scene/scene-4k.png`, `scene-7k-playing.png`, `scene-ready.png`,
`scene-paused.png`, `scene-complete.png`. 물리 장치의 입력 지연이나 모든 원본 채보의 호환성을 검증한 결과는 아니다.

일시정지 렌더링 테스트는 네이티브 창을 열기 때문에 명시적으로 실행할 때만 동작한다.

```powershell
$env:GGOSU_SCENE_CAPTURE = Join-Path (Get-Location) 'artifacts\07-scene'
$env:GGOSU_HUD_CAPTURE = 'countdown' # standard / focus / countdown
go test ./internal/play -run TestPausedRender -count=1
Remove-Item Env:GGOSU_HUD_CAPTURE
Remove-Item Env:GGOSU_SCENE_CAPTURE
```
