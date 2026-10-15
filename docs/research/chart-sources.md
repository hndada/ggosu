# 채보 소스 조사

채보 검색·다운로드 기능을 만들기 전에 지금 접근 가능한 채보 소스를 정리한다.
2026-10-09에 각 엔드포인트를 직접 호출해 응답을 확인했다. 확인하지 못한 항목은 따로 표시한다.

## 요약

| 포맷 | 소스 | 검색 API | 다운로드 | 단위 | 상태 |
|---|---|---|---|---|---|
| osu!mania | osu.direct | `GET /api/v2/search` (JSON, 키 불필요) | `GET /api/d/{setId}` → .osz | 비트맵셋 | 동작 |
| osu!mania | Nerinyan | `GET api.nerinyan.moe/search` | `GET /d/{setId}` → `dl.nerinyan.moe` | 비트맵셋 | 다운로드 동작, 검색 530 |
| osu!mania | catboy.best | `/api/v2/search` | `/d/{setId}` | 비트맵셋 | 403 (UA 등록 필요로 보임) |
| osu!mania | osu! 공식 API v2 | `GET /api/v2/beatmapsets/search` | 공식 다운로드는 로그인 필요 | 비트맵셋 | OAuth 필요, 미확인 |
| StepMania | EtternaOnline | `GET api.etternaonline.com/api/packs` (JSON, 1,731쪽) | 응답의 `download` (.zip) | 팩 | 동작 |
| StepMania | Stepmania Online | API 없음, HTML | `GET /download/pack/{id}/` → .zip | 팩 | 동작 (HTML 파싱 필요) |
| StepMania | Zenius-I-vanisher | API 없음 | 팩·곡 단위 | 팩·곡 | 미확인 |
| BMS | BMS Search | `GET api.bmssearch.net/v1/...` (JSON) | 응답의 `downloads[]` (외부 링크) | 곡 | 동작 |
| BMS | 난이도표 457개 (DARKSABUN 목록) | 표 목록 JSON → `header.json` → 데이터 JSON | 표 항목의 `url` (외부 링크) | 채보 (md5) | 주요 82개 중 79개 동작 |
| O2Jam | — | 없음 | — | — | 공개 아카이브 없음 |

## osu!mania

osu!mania는 API가 가장 잘 갖춰져 있다. mania 모드 필터와 키 수(`cs`)가 응답에 포함되어 바로 검색 조건으로 쓸 수 있다.

### osu.direct

- 검색: `https://osu.direct/api/v2/search?mode=3&amount=N&query=...`
  - `mode=3`이 mania다.
  - 응답은 osu! API v2 `beatmapset` 배열과 같은 구조다. `id`, `title`, `artist`, `creator`, `status`, `bpm`, `preview_url`, `covers`, `beatmaps[]`를 갖는다.
  - `beatmaps[]`의 `mode`, `cs`(=키 수), `version`, `difficulty_rating`, `total_length`, `checksum`(md5)를 쓸 수 있다.
- 다운로드: `https://osu.direct/api/d/{beatmapsetId}`
  - 15초짜리 서명 URL로 302 리다이렉트한 뒤 `.osz`를 받는다.
- 공식 서버(Bancho) 비트맵을 미러링하므로 ranked·loved·graveyard 상태가 그대로 온다.

### Nerinyan

- 다운로드 `https://api.nerinyan.moe/d/{setId}`는 `dl.nerinyan.moe/v2/d/{setId}`로 302 리다이렉트한다.
- 검색 `https://api.nerinyan.moe/search`는 조사 시점에 Cloudflare 530을 반환했다. 대체 미러로만 쓴다.

### catboy.best

- User-Agent를 확인하며, 임의 UA로는 검색·다운로드 모두 403이었다. 사용 조건을 확인하기 전까지는 쓰지 않는다.

### osu! 공식 API v2

- `beatmapsets/search`는 OAuth 클라이언트 자격 증명이 필요하다. 다운로드는 사용자 로그인 세션이 필요해서 클라이언트 기능에는 맞지 않는다.

## StepMania (.sm / .ssc)

StepMania는 곡 단위보다 **팩(zip)** 단위로 배포된다. 팩 하나가 수백 MB이므로 목록에 크기를 보여 주어야 한다.

### EtternaOnline

- 목록: `https://api.etternaonline.com/api/packs?page=N&limit=M`
  - Laravel 페이지네이션 (`data[]`, `meta.last_page`). 조사 시점 기준 1,731쪽이다 (기본 page size).
- 상세: `https://api.etternaonline.com/api/packs/{id}`
  - `name`, `song_count`, `size`("609.6 MB" 문자열), `tags[]`, `download`를 갖는다.
  - `tags`에 `keyCount`(`4k` 등)와 `skillset`(`jacks`, `chordjacks` 등)이 있어 키 수 필터로 쓸 수 있다.
  - `download`는 `https://downloads.etternaonline.com/ranked/{팩 이름}.zip` 형식이다.
- 키보드(4K) 위주 팩이다.

### Stepmania Online

- 2018년에 닫힌 원래 서비스의 팩 4,000여 개를 다시 모은 검색 사이트다. 최신 팩도 계속 추가되며 id는 10,700번대까지 있다.
- JSON API는 없다. 서버 렌더링 HTML이다.
  - 팩 상세: `https://stepmaniaonline.net/pack/{id}` (곡 수, 크기, 채보 수, 미터 범위, 곡별 BPM·길이·채보 종류)
  - 다운로드: `https://stepmaniaonline.net/download/pack/{id}/` → `application/zip`
- 대부분 DDR 스타일(4키 패드)이다. `robots`가 `noindex, nofollow`이므로 대량 수집은 하지 않고 사용자가 고른 팩만 받는다.

### Zenius-I-vanisher

- DDR 공식곡 시뮬레이션 팩 중심이다. 공개 API는 없고 커뮤니티 도구(zenius-downloader)가 HTML을 파싱한다. 이번 조사에서는 직접 호출하지 않았다.

## BMS

BMS는 곡 파일을 한곳에서 호스팅하지 않는다. 검색 서비스와 난이도표는 **메타데이터와 외부 링크**만 준다.
링크는 Google Drive, OneDrive, MEGA, 이벤트 페이지 등이라 자동 다운로드가 어렵다.

### BMS Search

- API 기본 주소: `https://api.bmssearch.net/v1`
  - `GET /bmses/search?q=...&limit=N` 곡 검색. `title`, `artist`, `genre`, `exhibition`(이벤트), `downloads[]`(`url`, `description`), `previews[]`를 갖는다.
  - `GET /patterns/{md5}`, `GET /patterns/sha256/{sha256}` 채보 단위 조회. `laneType`(`B_7K` 등), `difficulty`, `playlevel`, `totalNotes`, `bpm`, `file.hashMd5`를 갖는다.
  - 그 밖에 `/bmses/{id}/patterns`, `/exhibitions/search`, `/patterns/search` 경로가 프런트엔드 번들에 있다 (응답 미확인).
- 난이도표의 md5로 곡을 찾은 뒤 `downloads[]` 링크를 보여 주는 식으로 연결할 수 있다.

### 난이도표

BMS 채보를 찾는 사실상의 표준 경로다. 표 하나가 채보 수백~수천 개를 md5와 레벨로 묶고, 곡 링크를 함께 준다.

#### 형식

1. 표 페이지 HTML의 `<meta name="bmstable" content="header.json">`을 읽는다. 일부 표는 header JSON 주소를 바로 준다.
2. header JSON: `name`, `symbol`, `data_url`, (선택) `level_order`, `course`.
3. `data_url`(header 기준 상대 경로)의 JSON 배열: 채보마다 `md5`, `sha256`(일부만), `level`, `title`, `artist`, `url`(본체), `url_diff`(차분).
- 오래된 표는 Shift_JIS로 응답하기도 하고, UTF-8 BOM이 붙기도 한다.

#### 표 목록

- DARKSABUN Table List `https://darksabun.club/table/tablelist.html`가 표 457개를 관리한다.
- 원본 데이터는 JSON으로 받을 수 있다:
  `https://script.google.com/macros/s/AKfycbzaQbcI9UZDcDlSHHl2NHilhmePrNrwxRdOFkmIXsfnbfksKKmAB3V65WZ8jPWU-7E/exec?table=tablelist`
  - 항목: `name`, `symbol`, `tag1`(SP 319 · EVENT 57 · DP 49 · PMS 23 · etc 9), `tag2`(Personal 177 · Self-made Chart Only 135 · General 82 · Chart Event 48 · BMS Event 9 · Uploader 6), `url`, `comment`, `date`.
- `tag2 = General` 82개를 모두 열어 봤다. 79개가 동작하고, 3개(Luminous rejected·voting, たんぱつ難易度表 +)는 404였다.
- General 표 전체의 고유 md5는 약 45,300개다 (SP 19,000 · DP 5,500 · PMS 20,800).

#### 주요 표 (2026-10-09 기준 채보 수)

| 구분 | 표 | 기호 | 채보 | 주소 |
|---|---|---|---|---|
| SP 통상 | 通常難易度表 | ☆ | 634 | `https://darksabun.club/table/archive/normal1/` |
| SP 통상 | NEW GENERATION 通常 | ▽ | 4,776 | `http://rattoto10.jounin.jp/table.html` |
| SP 발광 | 発狂BMS難易度表 | ★ | 1,035 | `https://darksabun.club/table/archive/insane1/` |
| SP 발광 | NEW GENERATION 発狂 | ▼ | 2,192 | `http://rattoto10.jounin.jp/table_insane.html` |
| SP 고난도 | Overjoy | ★★ | 255 | `http://lr2.sakura.ne.jp/overjoy.php` |
| SP 투표 | Satellite | sl | 2,467 | `https://stellabms.xyz/sl/table.html` |
| SP 투표 | Stella | st | 2,342 | `https://stellabms.xyz/st/table.html` |
| SP 투표 | Starlight | sr | 862 | `https://stellabms.xyz/sr/table.html` |
| SP 투표 | Solar | so | 314 | `https://stellabms.xyz/so/table.html` |
| SP 투표 | Supernova | sn | 125 | `https://stellabms.xyz/sn/table.html` |
| SP 투표 | EXOPLANET | fr | 319 | `https://stellabms.xyz/fr/table.html` |
| SP LN | LN難易度 | ◆ | 485 | `http://flowermaster.web.fc2.com/lrnanido/gla/LN.html` |
| SP LN | Luminous | ln | 604 | `https://ladymade-star.github.io/luminous/table.html` |
| SP 스크래치 | 皿難易度表 (1st~3rd) | ◎ | 145·201·368 | `http://minddnim.web.fc2.com/sara/` |
| SP 연타 | 連打難易度表 | ≡ | 141 | `http://renda.konjiki.jp/` |
| SP 차분 | 差分アップローダー難易度表 | ろだ | 1,157 | `http://upl.konjiki.jp/` |
| DP | 発狂DP難易度表 | ★ | 752 | `https://deltabms.yaruki0.net/table/data/insane_head.json` |
| DP | δ難易度表 | δ | 748 | `https://deltabms.yaruki0.net/table/data/dpdelta_head.json` |
| DP | Δx難易度表 | ★ | 2,276 | `https://deltabms.yaruki0.net/table/data/dpdeluxe_head.json` |
| DP | DP Satellite / DP Stella | DPsl·DPst | 750·158 | `https://stellabms.xyz/dp/`, `/dpst/` |
| PMS | PMSデータベース (Lv1~45) | PLv | 7,264 | `https://pmsdifficulty.xxxxxxxx.jp/PMSdifficulty.html` |
| PMS | 発狂PMSデータベース (lv46~) | P● | 8,532 | `https://pmsdifficulty.xxxxxxxx.jp/insane_PMSdifficulty.html` |

- stellabms.xyz 계열(Satellite·Stella 등)은 각각 `table_vote`(투표 중), `table_rec`(추천), `table_sub`(보조) 변형이 있다.
- stellabms.xyz·deltabms·PMS 데이터베이스는 `sha256`도 준다. 오래된 표는 `md5`만 준다.
- Luminous는 `url`이 비어 있어 곡 링크를 BMS Search 등에서 md5로 찾아야 한다.

#### 곡 링크

- 표 항목의 `url`은 대부분 파일이 아니라 이벤트 소개 페이지다. 많이 나오는 도메인은
  `manbow.nothing.sh`(BOF 등 이벤트, 22,367), `pmsdifficulty.xxxxxxxx.jp`(9,413), `drive.google.com`(5,757),
  `venue.bmssearch.net`(3,923), `web.archive.org`(3,727), `mega.nz`, `onedrive.live.com`, `dropbox.com` 순이다.
- 따라서 자동 다운로드는 Google Drive·Dropbox 같은 직접 파일 링크 일부만 가능하다. 나머지는 링크를 열어 주고, 받은 파일을 사용자가 `songs/`에 넣게 한다.
- 차분(`url_diff`)은 본체 곡 폴더에 추가로 넣어야 하는 `.bms` 파일이다.

#### 팩 모음

- `https://bms.kyouko.moe/`(주요 난이도표 패키지), `https://package.bms.ms/`(이벤트 팩 100개 이상). 응답은 확인하지 않았다.

## O2Jam (.ojn / .ojm)

- 공식 서비스가 종료되어 공개된 채보 저장소나 API가 없다.
- 남아 있는 배포본은 대부분 출처가 불분명한 클라이언트 덤프이며, 저작권 상태도 불명확하다.
- osu!mania로 변환된 O2Jam 채보는 osu! 미러에서 검색할 수 있다 (`query=o2jam`).
- 따라서 O2Jam은 원격 검색 대상에서 제외하고 로컬 파일만 지원한다.

## 그 밖의 후보

- Quaver: `https://api.quavergame.com/v2/mapset/search?search=...`가 JSON으로 동작한다. 현재 `.qua` 파서가 없으므로 후보로만 남긴다.

## 구현 방향

1. **검색 대상**: osu.direct(osu!mania), EtternaOnline(StepMania 팩), BMS Search + 난이도표(BMS 메타데이터).
2. **공통 결과 모델**: 소스, 원격 id, 제목, 아티스트, 제작자, 키 수 목록, 난이도 범위, BPM, 크기, 다운로드 방식(직접/외부 링크).
3. **다운로드**: osu.direct·EtternaOnline·Stepmania Online은 직접 받아 `songs/`에 압축을 푼다. BMS는 외부 링크를 열어 준다.
4. **예의**: User-Agent에 앱 이름을 넣고, 요청 간격을 두고, 사용자가 고른 항목만 받는다. 사이트 전체를 미리 긁어 오지 않는다.
