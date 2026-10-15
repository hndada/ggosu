# 채보 아카이브

`cmd/archive`는 원격 채보 소스를 색인하고, 파일을 받아 두고, 검색·다운로드 API로 다시 내보낸다.
소스별 조사 내용은 [채보 소스 조사](../research/chart-sources.md)를 참고한다.

```powershell
go build -o build/bin/archive.exe ./cmd/archive
build/bin/archive.exe index -root D:rchive -source all
build/bin/archive.exe fetch -root D:rchive -source osu -status ranked -keys 4,7
build/bin/archive.exe fetch -root D:rchive -source osu -status ranked -keys 4,7 -budget 200GB -download
build/bin/archive.exe fetch -root D:rchive -source bms -table sl -download
build/bin/archive.exe fetch -root D:rchive -source osu,bms -tier candidate -download
build/bin/archive.exe fetch -root D:rchive -source etterna -top 500 -download
build/bin/archive.exe scan -root D:rchive
build/bin/archive.exe serve -root D:rchive -addr :8090
build/bin/archive.exe stats -root D:rchive
```

## 단계

1. **index**: 소스의 팩 목록을 `index/<source>.jsonl`에, 채보 목록을 `charts/<source>.jsonl`에 덮어쓴다. 메타데이터만 받는다.
   - Etterna 곡 목록과 BMS Search는 각각 요청 2천 번 안팎이라 30분 이상 걸린다. `-charts=false`면 팩 목록만 갱신하고 채보 목록은 그대로 둔다.
2. **fetch**: 색인에서 아직 받지 않은 항목을 받는다.
   - 기본은 **계획만 출력하는 dry run**이다. 네트워크와 디스크를 건드리지 않고 항목 수와 예상 용량만 보여 준다.
   - `-download`를 붙여야 실제로 받는다.
   - `-budget`(기본 10GB)만큼 받으면 멈추고, `-max-size`(기본 2GB)보다 큰 파일은 건너뛴다. `-budget`은 실행 한 번의 양이라 다시 실행하면 또 받는다.
   - `-reserve`(기본 20GB): 파일이 놓이는 디스크의 여유 공간이 이보다 작아지기 전에 멈춘다. 다시 실행해도 지켜지므로 다른 데이터와 같이 쓰는 디스크는 이것으로 막는다. 크기를 모르는 항목은 `-max-size`만큼 필요하다고 본다.
   - 끊긴 파일은 `.part`로 남고 다음 실행에서 Range 요청으로 이어 받는다.
   - 실패한 항목은 기록해 두고 건너뛴다. `-retry`로 다시 시도한다.
   - 필터: `-status`, `-keys`, `-q`(제목·아티스트·제작자), `-table`(BMS 표 기호+레벨 접두사, 예: `sl`, `st3`), `-limit`.
   - `-tier`: 이 등급 이상만 받는다 (`official`, `candidate`, `unvetted`). 아래 [등급](#등급) 참고.
   - `-top N`: 다른 필터를 통과한 팩 중 플레이 수 상위 N개만, 많이 플레이된 순서로 받는다. 이미 받은 팩도 N에 포함되므로 다시 실행해도 같은 범위다.
3. **scan**: 받은 팩을 열어 채보마다 지문(fingerprint)을 만든다. 바뀌지 않은 팩은 다시 읽지 않는다.
   - zip·osz는 직접 읽는다. rar·7z·lzh는 7-Zip이 있어야 한다 (`-7z`, 기본은 PATH와 Program Files에서 찾음). 채보 파일만 임시 폴더에 풀었다가 지운다.
   - 게임이 쓰는 파서(`chart.Decode`)를 그대로 쓰므로 osu·bms·bme·bml·sm·ojn을 읽는다. `.ssc`, `.bmson`은 아직 못 읽는다.
4. **serve**: 색인과 받은 파일을 읽어 HTTP로 제공한다. `-reload` 간격(기본 5분)마다 다시 읽으므로 fetch를 돌리는 중에도 서버를 켜 둘 수 있다.

## 저장 구조

```
<root>/
  index/<source>.jsonl   팩(다운로드 단위) 스냅숏
  charts/<source>.jsonl  원격 메타데이터에서 얻은 채보 기록
  state/<source>.jsonl   다운로드 기록, 같은 ID는 마지막 줄이 유효
  files/<source>/<id>/   받은 파일 (서버가 준 파일 이름 그대로)
  scan/<source>.jsonl    받은 팩에서 읽은 채보 기록, 같은 ID는 마지막 줄이 유효
```

`<source>`는 `osu`, `etterna`, `smo`, `bms`다.

디스크 여러 개에 나눌 때는 `files/<source>`를 다른 디스크의 폴더로 연결한다.
Windows는 junction(`mklink /J`), Linux는 symlink나 bind mount를 쓴다. `-reserve`는 연결된 디스크의 여유 공간을 본다.

```powershell
New-Item -ItemType Directory -Force D:rchive\files, E:rchive-bms
cmd /c mklink /J D:rchive\files\bms E:rchive-bms
```

## 채보 통합

같은 채보가 여러 팩에 실려 있어도 채보 하나로 보이고, 그 채보는 **수록 팩 목록**을 갖는다.
팩 이름으로 검색해도 그 팩의 채보가 나온다.

채보 기록은 신원(ID)을 여러 개 갖고, ID가 하나라도 같으면 같은 채보로 합친다.

| ID | 출처 | 의미 |
|---|---|---|
| `fp:` | scan | 레인 수·타이밍·노트(ms 반올림)의 해시. 제목·키음·이미지는 빼므로 이름을 바꿔 다른 팩에 넣어도 같다 |
| `md5:` | osu checksum, BMS 난이도표, BMS Search, scan | 채보 파일 md5. 파일 하나에 채보 하나인 osu·BMS에만 쓴다 |
| `eo:` | EtternaOnline | EtternaOnline 채보 ID. EtternaOnline이 이미 여러 팩의 같은 곡을 합쳐 준다 |
| `pk:` | EtternaOnline, scan | 소스/팩/제목/난이도. 다운로드 전 메타데이터와 같은 팩의 scan 결과를 잇는다 |

- 다운로드 전에도 osu(세트별 난이도 md5), Etterna(팩별 곡·난이도), BMS(난이도표 md5, BMS Search md5·이벤트)는 채보 단위로 검색된다.
- SMO 팩은 채보 목록 API가 없어 scan 뒤에 채보가 생긴다. scan한 SMO 채보는 지문으로 Etterna 채보와 합쳐진다.
- BMS 이벤트(BOF 등)는 `source: "event"`인 팩으로 들어간다. 검색은 되지만 그 자체로 받는 파일은 아니다.
- 한 팩에 제목·난이도 이름이 같은 다른 채보(리믹스, 다른 버전)가 있으면 그 `pk:`는 모호하므로 쓰지 않는다.
- 메타데이터가 서로 다르면 scan 결과를 먼저 쓰고, 나머지는 먼저 읽은 값을 쓴다.
- `lanes`는 스크래치를 포함한 실제 레인 수다 (BMS 7키 = 8, 14키 = 16).

## 등급

커뮤니티 심사를 얼마나 거쳤는지를 세 단계로 나눈다. 팩과 채보 모두 등급을 갖는다.

| 등급 | 값 | osu! | BMS | StepMania |
|---|---|---|---|---|
| 공인 | `official` | ranked, approved, loved | DARKSABUN `General` 목록의 본표 (Satellite, Stella, 発狂 등) | EtternaOnline 팩 (EtternaOnline은 순위표용으로 받아들인 팩만 올린다) |
| 후보 | `candidate` | qualified, pending | 본표의 투표(`Voting`)·보조(`Sub`)·추천(`Recommend`)·隔離 표, 개인 표, BMS Search에만 있는 이벤트 원본 채보 | — |
| 미검증 | `unvetted` | wip, graveyard | — | Stepmania Online 팩 |

- BMS 곡·차분 항목은 그 곡을 싣는 표 중 가장 높은 등급을 갖는다.
- 채보는 합쳐진 기록 중 가장 높은 등급을 갖는다. 예: SMO 팩에서 scan한 채보가 Etterna 채보와 지문이 같으면 공인이 된다.
- 플레이 수(`plays`)는 osu! 세트·난이도 플레이 수와 Etterna 팩·곡 플레이 수다. 채보는 기록 중 가장 큰 값을 갖는다. BMS와 SMO에는 없다.
- Etterna는 팩이 모두 공인이라, 용량을 줄이려면 등급 대신 `-top`으로 플레이 수 상위 팩만 받는다. 상위 500팩이 전체 플레이의 83%(192 GB)다.

## 소스별 동작

| 소스 | 색인 | 항목 단위 | 다운로드 |
|---|---|---|---|
| osu | osu.direct 검색 (mania, 상태별) 또는 osu! 공식 API | 비트맵셋 | osu.direct → Nerinyan 순서 |
| etterna | EtternaOnline 팩 API | 팩 | `downloads.etternaonline.com` |
| smo | Stepmania Online `/api/packs` CSV | 팩 | 본 서버 → 미러 순서 |
| bms | DARKSABUN 표 목록 → 각 난이도표 | 곡 본체·차분 | 링크 해석 후 직접 받기 |

### osu

- osu.direct는 offset 10,000까지만 넘겨 준다. 상태마다 오래된 순·최신 순으로 한 번씩 읽어 상태당 최대 2만 세트를 모은다.
- graveyard처럼 2만 세트가 넘는 상태는 중간이 빠진다. 환경 변수 `OSU_CLIENT_ID`, `OSU_CLIENT_SECRET`(osu! 계정 설정의 OAuth 앱)을 지정하면 공식 API cursor로 전체를 색인한다.

### bms

- 기본은 DARKSABUN 표 목록에서 `General` 태그 표만 읽는다. `-bms-tags ""`로 모든 표를 읽는다.
- 같은 본체 링크를 쓰는 채보는 한 곡으로 묶는다. 차분 링크는 `kind: diff`, `parent: <곡 ID>`인 별도 항목이다.
  여러 곡이 같은 업로더 페이지를 차분 링크로 쓰므로 차분 항목은 곡 + 차분 링크 단위로 나눈다.
- 링크 해석:
  - Google Drive 파일, Dropbox, 파일 확장자 링크, web.archive.org 파일 → 바로 받는다.
  - 이벤트 페이지(manbow, venue.bmssearch 등)와 Google Drive 공개 폴더 → 페이지 한 단계 안에서 위 링크를 찾는다.
    폴더는 `embeddedfolderview`로 키 없이 목록을 읽는다.
  - 페이지에 파일이 4개 이상이면 업로더·작가 홈·이벤트 아카이브로 보고 고르지 않는다. "pick one manually"로 기록한다.
  - MEGA, OneDrive, MediaFire, axfc 등 → 지원하지 않음으로 기록한다.
- 여러 표에 실린 곡을 먼저 받도록 정렬한다.
- 압축 파일(zip·rar·7z·lzh)이 아닌 응답은 버린다. 차분은 `.bms` 텍스트도 받는다.

## API

| 경로 | 내용 |
|---|---|
| `GET /api/charts?q=&lanes=4,7&format=&tier=&sort=&stored=1&offset=&limit=` | 채보 검색. 제목·아티스트·제작자·난이도·레벨·난이도표·**수록 팩 이름**에서 찾는다. `tier`는 최소 등급, `sort`는 `plays`(플레이 수) 또는 `packs`(수록 팩·표 수) |
| `GET /api/charts/{id}` | 채보 하나. `fp:…`, `md5:…`, `eo:…` 등 아무 ID로 찾는다 |
| `GET /api/md5/{md5}` | `md5:` ID로 찾기 (BMS·osu 호환) |
| `GET /api/packs?q=&source=&keys=&status=&kind=&tier=&sort=plays&stored=1&offset=&limit=` | 팩 검색 |
| `GET /api/packs/{source}/{id}` | 팩 하나와 수록 채보. `event/<id>`도 된다 |
| `GET /api/stats` | 소스별 팩 수·보관 수·보관 용량, 등급별 채보 수 |
| `GET /d/{source}/{id}` | 받은 파일. Range 요청 지원 |

- 검색은 NFKC·대소문자를 무시하고 공백으로 나눈 단어를 모두 포함하는 것을 찾는다. `limit` 최대 100.
- 채보 응답의 `packs[]`마다 `download`가 있으면 아카이브가 그 팩을 갖고 있다는 뜻이고, `path`는 팩 안의 채보 파일 위치다.

## 용량 (2026-10-09 색인 기준)

| 소스 | 항목 | 용량 |
|---|---|---|
| etterna | 팩 1,731 | 433 GB |
| smo | 팩 10,016 | 2,656 GB (6 GB 넘는 팩도 있음) |
| osu | mania 세트 38,888 색인 (ranked 7,731 · loved 1,085 · pending 5,469 · wip 4,586 · graveyard 19,999, 중간 누락) | 크기 미제공, 세트당 수~수십 MB |
| bms | 곡·차분 42,959 (난이도표 29,442 + BMS Search에만 있는 곡) | 크기 미제공 |

채보 기록은 osu 111,733 · Etterna 128,282 · BMS 186,174(난이도표 + BMS Search)개다.
합치면 **채보 374,989개**이고, 그중 85,014개는 두 팩 이상에 실려 있다. 합치기는 3초 정도 걸린다.
색인과 채보 기록을 합친 크기는 약 145 MB다.

StepMania 두 소스는 같은 팩이 겹치므로 실제 필요한 용량은 합계보다 작다.
처음에는 `-status ranked`, `-keys` 같은 필터와 `-budget`으로 범위를 좁혀 받는다.

## 예의

- 호스트마다 요청 간격을 둔다 (`-interval`, 기본 1초). 429·5xx는 `Retry-After`를 따르며 다시 시도한다.
- `-ua`에 연락처를 넣는다. 예: `-ua "ggosu-archive/0.1 (+mailto:you@example.com)"`.
- Stepmania Online은 검색엔진 수집을 거부하므로(`noindex`) 받는 범위를 좁히고 간격을 넓게 둔다.
- 재배포 허락은 소스와 제작자마다 다르다. 공개 아카이브로 열기 전에 각 소스 운영자와 제작자 정책을 확인한다.

## 테스트 다운로드 결과 (2026-10-09)

소스마다 소량만 받아 보고 확인 뒤 지웠다.

| 소스 | 시도 | 성공 | 비고 |
|---|---|---|---|
| osu (ranked 7K) | 2 | 2 | `.osz`, 서버가 준 파일 이름 유지 |
| etterna | 1 | 1 | 36 MB 팩 |
| smo | 1 | 1 | 25 MB 팩 |
| bms, Satellite(`-table sl`) | 15 | 10 | 실패는 죽은 호스트·삭제된 Dropbox·작가 홈 |
| bms, 표 수 많은 순 | 15 | 0~1 | 대부분 BOF 등 이벤트 아카이브 폴더(44~137 파일)거나 1GB 이상 |

서버에서는 검색·md5 조회·다운로드(원본과 바이트 일치)·Range·없는 항목 404를 확인했다.

### scan과 채보 합치기

Etterna와 SMO 양쪽에 있는 팩 2개, osu 세트 2개, BMS 곡 3개(rar 2개)를 받아 scan했다.

- BMS rar(7-Zip): マジカル縦連打의 채보 3개가 난이도표(▽4·▽7·▽10)와 BOF III 이벤트 기록에 md5로 합쳐졌다.
- osu: Brainiac Maniac은 메타데이터 2개와 scan 2개가 채보 2개로 합쳐졌다.
- 팩 이름("McPeePants Originals"), 이벤트 이름("fighters")으로 채보·곡이 검색된다.
- 게임 파서가 거부해 scan하지 못한 실제 파일이 있다. 파서를 고친 뒤 `scan -retry`로 다시 읽는다.
  - osu v11 맵의 3필드 hit sample(`2:0:2`)
  - SM의 범위를 벗어난 `#SAMPLESTART`
  - BMS BGA가 정의되지 않은 이미지를 가리킴
  - `.dwi` 팩 (지원 안 함)

## 아직 안 되는 것

- 여러 표에 실린 오래된 BMS 곡은 곡 단위가 아니라 **이벤트 팩**(BOF 등) 안에 있다. 이벤트 팩을 통째로 받아 md5로 곡을 찾는 팩 단위 수집이 필요하다.

- 난이도표에 곡 링크가 없는 BMS 채보 7,689개 중 BMS Search에 같은 md5가 있는 것은 그쪽 팩으로 이어진다. 둘 다 없는 채보는 받을 곳이 없다.
- SMO 팩은 받아서 scan하기 전까지 채보 목록이 없다.
- `.ssc`, `.dwi`, `.bmson` 채보는 scan에서 읽지 못한다.
- 팩도 BMS Search 곡도 없는 채보 약 25,000개(대부분 곡 링크 없는 난이도표 채보)는 팩 이름으로 찾을 수 없고 난이도표로만 찾는다.
- osu! graveyard 전체 색인은 공식 API 자격 증명이 있어야 한다.
