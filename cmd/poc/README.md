# POC・실험

제품에 연결하지 않은 모델과 실험용 실행 프로그램을 이 영역에 모았다.
소스·실행 안내·검증 기록·보고서 스크립트를 함께 관리한다. 명령은 저장소 루트에서 실행한다.

| 위치 | 내용 | 제품과의 관계 |
|---|---|---|
| [input](input/README.md) | 네이티브 입력 측정 화면, 합성 부하, OS 경로 검사, 배속 기록 실험 | 제품의 keyboard·clock·replay를 소비 |
| [difficulty](difficulty/README.md) | 손가락 상태 기반 난이도 모델의 통제 비교 CLI | 게임 판정·점수·곡 목록에 미연결 |
| [chat](chat/README.md) | 실제 방에 가상 참가자가 대화를 보내는 디버그 발생기 | 제품의 멀티 프로토콜·채팅 서버를 소비 |
| `demo` | 자체 제작 채보·오디오·이미지 생성 | 출력은 저장소 루트의 `songs/demo` |
| `internal/difficulty` | 실험 모델과 계산 검증 테스트 | cmd/poc 아래에서만 import 가능 |
| `difficulty/scripts` | 원본 실험 채보와 계산 결과의 보고서 생성 | 제품 빌드에 미사용 |

별도 go.mod는 만들지 않는다. 같은 제품의 채보·입력 코드를 실험에서 재사용하고,
Go의 `cmd/poc/internal` 경계와 제품 검증의 의존성 검사로 제품이 실험 구현을 참조하지 않게 한다.
새 실험도 구현·테스트·문서·전용 스크립트를 이 영역 안에 둔다.

```powershell
# 실험만 검사
go test ./cmd/poc/...
go vet ./cmd/poc/...

# POC 실행 파일은 build/poc에만 생성
pwsh -NoProfile -File scripts/buildpoc.ps1

# 입력 측정
go run ./cmd/poc/input

# 난이도 통제 비교
go run ./cmd/poc/difficulty -suite chords
go run ./cmd/poc/difficulty -suite recovery
go run ./cmd/poc/difficulty -suite holds

# 원본 데모 에셋 재생성
go run ./cmd/poc/demo

# 제품과 실험을 모두 검사
pwsh -NoProfile -File scripts/check.ps1 -POC
```

입력 실험의 [리플레이](input/replay.md), [OS별 구현 검증](input/platforms.md),
[측정 기록](input/validation.md)은 당시 검증 범위를 설명한다.
과거 측정 결과가 현재 제품의 물리 장치 지연이나 전체 프레임 성능을 보증하지는 않는다.
산출물은 기존 `artifacts/`에 보존하며 소스에 포함하지 않는다.
실험 실행 파일은 `build/poc/`, 현재 제품 실행 파일은 `build/bin/`에 둔다.
