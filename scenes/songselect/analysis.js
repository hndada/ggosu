import { tr } from '../../preferences/language.js';
// Axes and labels follow the page text colour so the chart works on the dark theme.
const NS = 'http://www.w3.org/2000/svg';
function svgNode(name, attrs = {}, text = '') {
    const e = document.createElementNS(NS, name);
    for (const [key, value] of Object.entries(attrs)) e.setAttribute(key, String(value));
    if (text) e.textContent = text;
    return e;
}
function cell(row, text, header = false) {
    const e = document.createElement(header ? 'th' : 'td'); e.textContent = text; row.append(e);
}
export function renderDistribution(container, report, { compact = false } = {}) {
    container.replaceChildren();
    if (!report) return;
    const { bins, start, step, lanes, total, mines } = report;
    const peakNPS = Math.max(0, ...bins.map(b => (b.tap + b.hold + b.lift) / step));
    const peakHolds = Math.max(0, ...bins.map(b => b.peakHolds));
    const maxNPS = peakNPS || 1, maxHolds = peakHolds || 1;
    const summary = document.createElement('p');
    summary.textContent = tr`${total}개 노트 시작 · 지뢰 ${mines}개 · 구간 평균 최대 ${peakNPS.toFixed(2)} NPS · 동시 LN 최대 ${peakHolds}개 · 구간 ${step}초`;
    const legend = document.createElement('p');
    legend.textContent = tr('원곡 1배속 시간축: 파랑 Tap · 보라 LN 시작(roll 포함) · 초록 Lift · 주황 구간 내 동시 LN 최댓값(오른쪽 축). Fake 제외. 구간 위에 마우스를 올리면 수치를 볼 수 있습니다.');
    const svg = svgNode('svg', { viewBox: '0 0 660 260', width: 660, height: 260, role: 'img', fill: 'currentColor', 'aria-label': tr`시간별 노트 시작 밀도와 동시 롱노트 수. ${summary.textContent}` });
    const left = 50, top = 25, width = 555, height = 185, bottom = top + height;
    svg.append(svgNode('rect', { x: left, y: top, width, height, fill: 'none', stroke: 'currentColor', 'stroke-opacity': 0.5 }));
    for (let n = 0; n <= 4; n++) {
        const y = bottom - n * height / 4;
        svg.append(svgNode('line', { x1: left, x2: left + width, y1: y, y2: y, stroke: 'currentColor', 'stroke-opacity': 0.15 }));
        svg.append(svgNode('text', { x: left - 5, y: y + 4, 'text-anchor': 'end', 'font-size': 12 }, (maxNPS * n / 4).toFixed(2)));
        svg.append(svgNode('text', { x: left + width + 5, y: y + 4, 'font-size': 12 }, (maxHolds * n / 4).toFixed(1)));
    }
    svg.append(svgNode('text', { x: left, y: 15, 'font-size': 12 }, 'NPS'));
    svg.append(svgNode('text', { x: left + width, y: 15, 'text-anchor': 'end', 'font-size': 12 }, tr('동시 LN')));
    const bw = width / Math.max(1, bins.length);
    const paths = ['', '', ''], colors = ['#4d8fe8', '#a879dd', '#3bb57c'];
    let line = '';
    bins.forEach((b, i) => {
        let y = bottom;
        for (const [j, key] of ['tap', 'hold', 'lift'].entries()) {
            const h = b[key] / step / maxNPS * height;
            if (h) paths[j] += `M${left + i * bw},${y}h${bw}v${-h}h${-bw}Z`;
            y -= h;
        }
        const yLine = bottom - b.peakHolds / maxHolds * height;
        line += i ? `V${yLine}H${left + (i + 1) * bw}` : `M${left},${yLine}H${left + bw}`;
    });
    paths.forEach((d, i) => svg.append(svgNode('path', { d, fill: colors[i] })));
    svg.append(svgNode('path', { d: line, stroke: '#f0952e', 'stroke-width': 2, fill: 'none' }));
    bins.forEach((b, i) => {
        const hit = svgNode('rect', { x: left + i * bw, y: top, width: bw, height, fill: 'transparent' });
        hit.append(svgNode('title', {}, tr`${(start + i * step).toFixed(2)}–${(start + (i + 1) * step).toFixed(2)}초: Tap ${b.tap}, LN 시작 ${b.hold}, Lift ${b.lift}, 지뢰 ${b.mine}, 동시 LN ${b.peakHolds}`)); svg.append(hit);
    });
    for (let n = 0; n <= 4; n++) svg.append(svgNode('text', { x: left + width * n / 4, y: bottom + 20, 'text-anchor': 'middle', 'font-size': 12 }, `${(start + bins.length * step * n / 4).toFixed(1)}s`));
    const laneText = document.createElement('p'); laneText.textContent = tr('원본 레인별 노트 시작 수: ') + lanes.map((n, i) => `${i + 1}: ${n}`).join(' / ');
    const details = document.createElement('details'), heading = document.createElement('summary'); heading.textContent = tr('구간별 수치 보기'); details.append(heading);
    const table = document.createElement('table'), head = document.createElement('thead'), header = document.createElement('tr');
    for (const label of [tr('시작 (초)'), 'Tap', tr('LN 시작'), 'Lift', tr('지뢰'), 'NPS', tr('동시 LN')]) cell(header, label, true);
    head.append(header); table.append(head);
    const body = document.createElement('tbody');
    bins.forEach((b, i) => { const row = document.createElement('tr'); for (const v of [(start + i * step).toFixed(2), b.tap, b.hold, b.lift, b.mine, ((b.tap + b.hold + b.lift) / step).toFixed(2), b.peakHolds]) cell(row, v); body.append(row); });
    table.append(body); details.append(table);
    // The song select shows only the chart; the result screen keeps the numbers and legend.
    if (compact) container.append(svg); else container.append(summary, legend, svg, laneText, details);
}
