"""Render the exact exported POC charts and summarize their measured costs."""

import argparse
import csv
import json
from collections import defaultdict
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont


FONT = Path("C:/Windows/Fonts/malgun.ttf")
BOLD = Path("C:/Windows/Fonts/malgunbd.ttf")
COLORS = ("#2563eb", "#d97706")


def font(size, bold=False):
    return ImageFont.truetype(str(BOLD if bold else FONT), size)


def load(path):
    return json.loads(path.read_text(encoding="utf-8-sig"))


def panel(draw, box, case, chart, limit=800, note_height=7):
    left, top, right, bottom = box
    draw.text((left + 15, top + 10), case["name"], fill="#172033", font=font(26, True))
    metric = case["region"]
    subtitle = f'{metric["notes_per_second"]:.1f} notes/s  |  D {metric["difficulty"]:.4f}'
    draw.text((left + 15, top + 49), subtitle, fill="#475569", font=font(21))
    plot_left, plot_right = left + 83, right - 24
    plot_top, plot_bottom = top + 137, bottom - 25
    width = (plot_right - plot_left) / chart["lanes"]
    layouts = chart["layout"]
    for lane in range(chart["lanes"]):
        x = plot_left + width * (lane + 0.5)
        group = layouts[lane].get("group", 0)
        label = layouts[lane]["name"]
        draw.text((x, plot_top - 38), label, fill=COLORS[group % 2], font=font(23, True), anchor="mm")
        draw.line((x, plot_top - 12, x, plot_bottom), fill="#dce1e8", width=2)
    for millis in range(0, limit + 1, 100):
        y = plot_bottom - (plot_bottom - plot_top) * millis / limit
        draw.line((plot_left - 8, y, plot_right + 8, y), fill="#e9edf2", width=1)
        if millis % 200 == 0:
            draw.text((plot_left - 17, y), str(millis), fill="#475569", font=font(20), anchor="rm")
    for note in chart["notes"]:
        millis = note["at_ns"] / 1e6
        if not 0 <= millis <= limit:
            continue
        lane = note["lane"]
        x = plot_left + width * (lane + 0.5)
        y = plot_bottom - (plot_bottom - plot_top) * millis / limit
        group = layouts[lane].get("group", 0)
        half = min(49, width * 0.32)
        color = COLORS[group % 2]
        if note["kind"] == "hold":
            end = note["end_ns"] / 1e6
            end_y = plot_bottom - (plot_bottom - plot_top) * min(end, limit) / limit
            draw.line((x, y, x, end_y), fill=color, width=11)
            if end <= limit:
                draw.rectangle((x - half, end_y - 2, x + half, end_y + 2), fill="white", outline=color, width=2)
            else:
                draw.polygon(((x - 7, end_y + 7), (x + 7, end_y + 7), (x, end_y - 4)), fill=color)
        draw.rounded_rectangle((x - half, y - note_height, x + half, y + note_height), radius=2, fill=color)


def plot(root, data, folder, output, original=False):
    cases = data["cases"][:2] if original else data["cases"]
    rows = (len(cases) + 1) // 2
    image = Image.new("RGB", (1400, 115 + rows * 340), "white")
    draw = ImageDraw.Draw(image)
    title = "기존 chord / trill 실험에 사용한 채보" if original else "Chord끼리 비교한 10가지 채보"
    draw.text((35, 19), title, fill="#172033", font=font(32, True))
    draw.text((35, 67), "첫 0–800ms · 시간 ↑ (ms) · 파랑: 손 0 / 주황: 손 1 · 가로 막대: 탭", fill="#475569", font=font(23))
    for i, case in enumerate(cases):
        chart = load(root / folder / f"{i + 1:02}.ggc")
        if chart["notes"] != case["chart"]["notes"] or chart["layout"] != case["chart"]["layout"]:
            raise ValueError("Plot source differs from the analyzed chart")
        col, row = i % 2, i // 2
        panel(draw, (20 + col * 690, 115 + row * 340, 690 + col * 690, 435 + row * 340), case, chart)
    image.save(root / output)


def link(path):
    return path.resolve().as_posix()


def summarize(root, basic, chords):
    lines = ["# 채보 배치와 chord 간 비교", "", "실제 CLI가 내보낸 `.ggc`를 그림의 원본으로 사용했다. 모든 계수는 미보정 예시값이다.", "",
             "## 기존 비교의 원본", "", "2레인 합성 채보를 24주기 생성했다. 노트 48개이며, 각 손가락은 200ms마다 누른다.", "",
             "![원본 chord와 trill](" + link(root / "original-patterns.png") + ")", "",
             "| 시각(ms) | Chord | Trill |", "|---:|---|---|"]
    groups = []
    for case in basic["cases"][:2]:
        by_time = defaultdict(list)
        for note in case["chart"]["notes"]:
            by_time[note["at_ns"] // 1_000_000].append(chr(65 + note["lane"]))
        groups.append(by_time)
    for millis in range(0, 801, 100):
        a, b = ("+".join(g.get(millis, [])) or "—" for g in groups)
        lines.append(f"| {millis} | {a} | {b} |")
    lines += ["", "Chord의 A·B는 같은 시각, trill의 B는 A보다 100ms 늦다. 다른 손 통제군은 노트 배치가 같고 손 배정과 간섭 표만 다르다.", ""]
    for i, case in enumerate(basic["cases"][:4]):
        lines.append(f'- [{case["name"]}]({link(root / "basic" / f"{i + 1:02}.ggc")})')
    lines += ["", "## Chord끼리의 계산", "", "A·B·C는 손 0, D·E·F는 손 1이다. 모든 비교는 600~6600ms의 같은 6초를 측정한다.",
              "기존 chord/trill 표는 4초 측정이므로 아래 난이도와 그대로 비교하지 않는다.", "",
              "![Chord 배치](" + link(root / "chord-patterns.png") + ")", "",
              "| 채보 | notes/s | 동작 비용 | 간섭 비용 | 반복 비용 | 총 strain | 난이도 |", "|---|---:|---:|---:|---:|---:|---:|"]
    for i, case in enumerate(chords["cases"]):
        m = case["region"]
        repeat = m.get("repeat", 0)
        lines.append(f'| [{case["name"]}]({link(root / "chords" / f"{i + 1:02}.ggc")}) | {m["notes_per_second"]:.2f} | {m["movement"]:.4f} | {m["interaction"]:.4f} | {repeat:.4f} | {m["movement"] + m["interaction"] + repeat:.4f} | {m["difficulty"]:.4f} |')
    history = chords["cases"][0]["report"]["options"].get("repeat_gain", 0) > 0
    if history:
        opts = chords["cases"][0]["report"]["options"]
        hold_rate, limit, gain = opts["hold_recovery"], opts["load_limit"], opts["repeat_gain"]
        interaction_load = opts.get("interaction_load", 1)
        lines += ["", "## 잔류 부담과 회복", "",
                  f"동작 비용 + {interaction_load:g}×간섭 비용으로 손가락별 잔류 부담을 쌓는다. 즉시 동작·간섭 비용은 원래 값을 유지한다. 이동 중에는 회복하지 않고, 유지 중에는 자유 휴식의 {hold_rate:g}배로 회복한다. 자유 휴식의 회복 시간상수는 {opts['recovery_ns'] / 1e6:g}ms다.",
                  f"잔류 부담은 0~{limit:g}로 제한한다. 기존 비용에 1 + {gain:g}×(부담/{limit:g})²를 적용하므로 배수는 최대 {1 + gain:g}다. 반복으로 늘어난 비용은 잔류 부담에 다시 넣지 않는다.",
                  "기본 동작·간섭 비용은 기존과 같고, 반복 비용을 별도 열에 기록한다. 창 경계나 출력 조각 수에 의존하지 않도록 상태 변화와 비용을 정확하게 적분한다.", ""]
        if (root / "recovery.json").exists():
            recovery = load(root / "recovery.json")["cases"]
            lines += ["각 속도에서 가운데 20회 동시타(노트 40개)를 측정했다. 같은 속도 안에서 밀도·동작·행렬 비용이 같다. 속도가 느려지면 관측 시간은 길어진다.", "",
                      "| 동시타 간격 | AB↔AC 난이도 | AB↔DE 난이도 | 비율 |", "|---|---:|---:|---:|"]
            for a, b in zip(recovery[::2], recovery[1::2]):
                da, db = a["region"]["difficulty"], b["region"]["difficulty"]
                period = a["name"].split(" / ")[1]
                lines.append(f"| {period} | {da:.4f} | {db:.4f} | {da / db:.4f} |")
            lines += ["", "반복 보정을 끈 recovery-disabled.json에서는 각 속도의 AB↔AC와 AB↔DE가 다시 같다.",
                      "같은 손이라는 패턴 이름에 보너스를 붙인 것이 아니라 실제로 공유되는 손가락의 부담 이력으로 구분한다. 행렬 제거 시에도 방향이 유지되는 것을 테스트했다.", ""]
    lines += ["", "## 이번 계산에서 확인한 것", "",
              "- 같은 AB chord도 400→200→100ms로 빨라지면 난이도가 증가한다. 100ms에서는 복귀·다음 누름이 압축되어 동작 비용 자체도 증가한다.",
              "- 같은 10 notes/s에서 AB/200ms와 ABC/300ms를 비교하면 기본 동작 비용은 120으로 같다. 쌍별 간섭·구간 집중도와 반복 회복이 함께 작용한다.",
              "- 같은 15 notes/s에서 ABC와 ABD는 동작 비용이 같고, 같은 손의 쌍 개수 차이로 간섭 비용이 달라진다.",
              "- AB와 AC는 기본 프로필의 두 쌍 계수가 같으므로 같다. 손가락 쌍별 계수를 바꾸면 구분되는지도 테스트했다.",
              ("- 잔류 부담을 켜면 AB 반복 > AB↔AC > AB↔DE로 구분된다. 빠른 입력이 어느 손가락에 집중되는지 반영된다." if history else
               "- AB 반복, AB↔AC 교대, AB↔DE 교대는 반복 부담을 끈 설정에서 같다. 부담 분산의 차이가 이 속도에서 계산되지 않는다."),
              "", "계수에 의해 구분되는 경우와 현재 모델이 구분하지 못하는 경우를 함께 확인한 결과다. 실제 사람의 상대 난이도를 검증한 결과는 아니다.", "",
              "## 재현", "", "```powershell", "go run ./cmd/poc/difficulty", "go run ./cmd/poc/difficulty -suite chords",
              "go run ./cmd/poc/difficulty -suite chords -profile artifacts/difficulty/chord-review/profile-6fingers.json",
              "go run ./cmd/poc/difficulty -suite recovery",
              "go run ./cmd/poc/difficulty -suite recovery -repeat-gain 0",
              "go test ./cmd/poc/difficulty ./cmd/poc/internal/difficulty", "```", "",
              "각 `.ggc`는 원본 시간축으로 저장했다. JSON에는 평가 구간, 손 배정, 원본 노트, 손가락별 비용, 순간 strain과 순위를 함께 기록한다."]
    (root / "report.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
    with (root / "chords.csv").open("w", newline="", encoding="utf-8-sig") as stream:
        writer = csv.writer(stream)
        writer.writerow(["case", "notes_per_second", "movement", "interaction", "repeat", "strain", "difficulty"])
        for case in chords["cases"]:
            m = case["region"]
            repeat = m.get("repeat", 0)
            writer.writerow([case["name"], m["notes_per_second"], m["movement"], m["interaction"], repeat, m["movement"] + m["interaction"] + repeat, m["difficulty"]])
    (root / "profile-6fingers.json").write_text(json.dumps(chords["cases"][0]["report"]["profile"], indent=2) + "\n", encoding="utf-8")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("root", type=Path)
    args = parser.parse_args()
    root = args.root.resolve()
    basic, chords = load(root / "basic.json"), load(root / "chords.json")
    plot(root, basic, "basic", "original-patterns.png", original=True)
    plot(root, chords, "chords", "chord-patterns.png")
    summarize(root, basic, chords)
    for name in ("original-patterns.png", "chord-patterns.png"):
        with Image.open(root / name) as image:
            image.verify()
        print(root / name)
    print(root / "report.md")


if __name__ == "__main__":
    main()
