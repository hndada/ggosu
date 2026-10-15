"""Render and summarize the exact long-note POC fixtures and computed costs."""

import argparse
from pathlib import Path

from PIL import Image, ImageDraw

from plotdifficulty import font, link, load, panel


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("root", type=Path)
    root = parser.parse_args().root.resolve()
    cases = load(root / "holds.json")["cases"]
    image = Image.new("RGB", (1400, 120 + 6 * 430), "white")
    draw = ImageDraw.Draw(image)
    draw.text((35, 18), "롱노트 테스트에 사용한 12가지 채보", fill="#172033", font=font(32, True))
    draw.text((35, 66), "첫 0–1600ms · 시간 ↑ · 세로 기둥: 롱노트 · 위 화살표: 계속 유지 · 파랑/주황: 손 0/1", fill="#475569", font=font(21))
    for i, case in enumerate(cases):
        chart = load(root / "charts" / f"{i + 1:02}.ggc")
        if chart["notes"] != case["chart"]["notes"]:
            raise ValueError("Plot source differs from analyzed chart")
        x, y = 20 + (i % 2) * 690, 120 + (i // 2) * 430
        panel(draw, (x, y, x + 670, y + 410), case, chart, limit=1600, note_height=3)
    image.save(root / "patterns.png")
    model = cases[0]["report"]["model"]
    weight = cases[0]["report"]["options"].get("interaction_load", 1)
    lines = ["# 롱노트 검증", "", f"같은 고정 계수와 {model}를 사용했다. 간섭의 누적 비율은 {weight:g}이며 즉시 간섭 비용은 그대로다. 실제 인체 난이도 보정은 하지 않았다.", "",
             "![원본 롱노트 채보](" + link(root / "patterns.png") + ")", "",
             "A·B·C는 손 0, D·E·F는 손 1이다. 그림은 실제 내보낸 .ggc의 첫 0~1600ms이며 시간이 아래에서 위로 증가한다. 위 화살표는 표시 범위 밖으로 이어지는 LN이며, 끝 시각은 사례에 따라 4000ms 또는 6000ms다.", "",
             "## 유지 중 입력 비교", "", "500~5500ms의 같은 5초를 관측한다. 유지 LN의 시작·끝 움직임을 제외했다. 모든 경우 탭 24개, 4.8 notes/s다.", "",
             "| 채보 | 동작 비용 | 간섭 비용 | 반복 비용 | 난이도 |", "|---|---:|---:|---:|---:|"]
    for i in [0, 1, 2, 3, 4, 5, 11]:
        case, m = cases[i], cases[i]["region"]
        lines.append(f'| [{case["name"]}]({link(root / "charts" / f"{i + 1:02}.ggc")}) | {m["movement"]:.4f} | {m["interaction"]:.4f} | {m["repeat"]:.4f} | {m["difficulty"]:.4f} |')
    lines += ["", "같은 손 LN의 존재로 간섭과 반복 비용이 증가한다. 다른 손 LN은 이 예시 프로필에서 탭 기준군과 같다.",
              "B/C 교대는 LN 손가락에 전달되는 기본 간섭량은 같지만, 움직이는 손가락에 집중되는 반복 부담이 줄어든다.",
              "두 개 같은 손 LN은 각각의 유지 손가락에 비용이 생긴다. 다른 손에 추가한 LN은 같은 손 탭에 별도 비용을 만들지 않는다.", "",
              "## 새 노트 없는 유지 손가락의 비용", "",
              "아래 값은 A를 유지하는 동안 B를 24회 누를 때, 새 노트가 없는 A에 기록된 비용이다.", "",
              "| 손가락 | 새 누름 수 | 동작 비용 | 간섭 비용 | 반복 비용 |", "|---|---:|---:|---:|---:|"]
    m = cases[1]["region"]
    for i in [0, 1]:
        cost = m["fingers"][i]
        lines.append(f'| {chr(65 + i)} | {m["notes_per_finger"][i]} | {cost["movement"]:.4f} | {cost["interaction"]:.4f} | {cost["repeat"]:.4f} |')
    lines += ["", "## 해제 직후 입력과 동작 겹침", "",
              "아래는 준비·유지·해제·후속 입력 전체를 원래 창 경계에서 측정한다. 위의 유지 구간 표와 관측 범위가 다르다.", "",
              "| 채보 | 동작 비용 | 간섭 비용 | 반복 비용 | 난이도 |", "|---|---:|---:|---:|---:|"]
    for i in [8, 9, 10]:
        case, m = cases[i], cases[i]["region"]
        lines.append(f'| [{case["name"]}]({link(root / "charts" / f"{i + 1:02}.ggc")}) | {m["movement"]:.4f} | {m["interaction"]:.4f} | {m["repeat"]:.4f} | {m["difficulty"]:.4f} |')
    lines += ["", "40ms 간격은 해제 20ms + 다음 누름 20ms로 압축하며 배수는 각각 2.5다. 위치 1→0→1의 연속성은 유지된다.",
              "B가 LN 끝 25ms 뒤에 입력하는 경우, A 해제와 B 누름이 1000~1025ms에 겹친다. 이 구간의 기본 간섭은 A/B 각각 0.175다.",
              "끝 시각과 같은 손가락의 다음 입력 시각이 같으면 유효한 복귀 시간을 만들 수 없으므로 오류로 처리하는 것도 테스트했다. 판정창 안의 입력 이동은 아직 모델에 없다.", "",
              "## 발견한 모델 제한: 유지 시간 자체", "",
              "| 단독 LN | 동작 비용 | 반복 비용 | 난이도 |", "|---|---:|---:|---:|"]
    for i in [6, 7]:
        case, m = cases[i], cases[i]["region"]
        lines.append(f'| [{case["name"]}]({link(root / "charts" / f"{i + 1:02}.ggc")}) | {m["movement"]:.4f} | {m["repeat"]:.4f} | {m["difficulty"]:.4f} |')
    lines += ["", "현재 순수한 유지 자체에는 비용이 없다. 단독 LN을 오래 누르면 새 작업 없이 느린 회복만 일어나므로 해제 시 반복 비용이 줄어든다.",
              "따라서 '더 긴 LN 자체가 더 어렵다'는 부담을 아직 설명하지 못한다. 테스트가 통과했다는 것은 현재 계산식이 일관되게 작동한다는 뜻이며 이 제한이 해결됐다는 뜻은 아니다.", ""]
    if (root / "holds-1.json").exists():
        lines += ["## 간섭 누적 비율 비교", "", "같은 채보·행렬·반복 보정 강도를 유지하고 누적 비율만 변경했다. 모든 조건에서 LN 없는 기준 난이도는 같다.", "",
                  "| 누적 비율 α | 동작 비용 | 즉시 간섭 비용 | 반복 비용 | LN 난이도 | 기준 대비 증가 |", "|---:|---:|---:|---:|---:|---:|"]
        for alpha in ("1", "0.5", "0.25", "0"):
            sweep = load(root / f"holds-{alpha}.json")["cases"]
            base, m = sweep[0]["region"], sweep[1]["region"]
            increase = 100 * (m["difficulty"] / base["difficulty"] - 1)
            lines.append(f'| {alpha} | {m["movement"]:.4f} | {m["interaction"]:.4f} | {m["repeat"]:.4f} | {m["difficulty"]:.4f} | {increase:.2f}% |')
        lines += ["", "α=1은 이전 v2 결과를 재현하고, α=0.25가 새 기본값이다. α=0도 즉시 간섭을 제거하지 않는다. 이미 남은 부담이 있다면 즉시 간섭에 반복 보정이 적용될 수 있다.",
                  "AB↔AC > AB↔DE 순서는 네 비율 모두에서 유지되며, 부분 구간 적분과 창 분할 시 비용 총량도 보존된다.", ""]
    lines += [
              "## 검증과 재현", "",
              "동일 입력량·다른 손·행렬 제거·회복 속도 변경·2배속·부분 구간 적분·7ms 창 분할·해제 복귀 조건을 테스트했다.",
              "기본 동작과 간섭 계수는 변경하지 않았다. JSON에는 손가락별 비용과 load_start/load_end를 보존한다.", "",
              "```powershell", "go run ./cmd/poc/difficulty -suite holds", "go run ./cmd/poc/difficulty -suite holds -rate 2",
              "go run ./cmd/poc/difficulty -suite holds -hold-recovery 1", "go test ./cmd/poc/difficulty ./cmd/poc/internal/difficulty", "```"]
    (root / "report.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
    with Image.open(root / "patterns.png") as check:
        check.verify()
    print(root / "patterns.png")
    print(root / "report.md")


if __name__ == "__main__":
    main()
