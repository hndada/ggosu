import { tr } from '../../preferences/language.js';
import { levelAt } from '/songlist/level.js';

const $ = id => document.getElementById(id);
const number = n => n == null ? '—' : n.toLocaleString('ko-KR', { maximumFractionDigits: 2 });
const time = n => `${Math.floor(Math.max(0, n) / 60)}:${String(Math.floor(Math.max(0, n) % 60)).padStart(2, '0')}`;

export function summary(entry, report) {
    if (!report || !(report.step > 0)) return null;
    const holds = report.bins.reduce((sum, b) => sum + b.hold, 0);
    const taps = report.bins.reduce((sum, b) => sum + b.tap + b.lift, 0);
    const peak = Math.max(0, ...report.bins.map(b => (b.tap + b.hold + b.lift) / report.step));
    return { total: report.total, taps, holds, ratio: report.total ? holds / report.total * 100 : 0,
        average: entry.length > 0 ? report.total / entry.length : 0, peak };
}

// A narrow overview: each bar is the note rate of its time slice; it does not turn note density into a difficulty rating.
export function overview(report, limit = 40) {
    if (!report || !(report.step > 0)) return [];
    const size = Math.max(1, Math.ceil(report.bins.length / limit)), bars = [];
    for (let i = 0; i < report.bins.length; i += size) {
        const end = Math.min(i + size, report.bins.length);
        const notes = report.bins.slice(i, end).reduce((sum, b) => sum + b.tap + b.hold + b.lift, 0);
        bars.push({ from: report.start + i * report.step, to: report.start + end * report.step, nps: notes / ((end - i) * report.step) });
    }
    return bars;
}

// Like osu!lazer's SquareGraph: linear, the chart's densest slice is full height.
export function densityHeights(bars) {
    const max = Math.max(0, ...bars.map(b => b.nps));
    return bars.map(b => max > 0 ? b.nps / max * 100 : 0);
}

function cover(url) {
    const image = $('song-cover');
    if (image.dataset.url === url) return;
    image.dataset.url = url;
    if (!url) { image.hidden = true; image.removeAttribute('src'); return; }
    // The previous cover stays until the new one is decoded, so switching charts never blanks the hero.
    const next = new Image();
    next.src = url;
    next.decode().then(() => { if (image.dataset.url === url) { image.src = url; image.hidden = false; } },
        () => { if (image.dataset.url === url) image.hidden = true; });
}

export function render(entry, entries, report, progress, backgroundURL, rate = 1, loading = false) {
    const e = entry;
    $('song-name').textContent = e?.title || (loading ? tr('곡 로딩 중…') : tr('곡을 선택해 주세요'));
    $('chart-line').hidden = !e;
    $('difficulty').textContent = e ? e.difficulty : '—';
    const duration = $('song-duration');
    duration.hidden = !e;
    duration.textContent = !e ? '' : e.durationPending ? tr('전체 길이 계산 중…') : e.audioDuration > 0 ? tr`전체 길이 ${time(e.audioDuration / 1e9)}` : tr('전체 길이 미확인');
    duration.title = e?.durationError || tr('원곡 1배속 기준');
    const level = String(e?.level ?? '').trim(), { value: n, estimated } = levelAt(e, rate);
    $('level').textContent = !e ? '' : n != null ? n.toFixed(1) : e.rating > 0 ? '…' : level || '?';
    $('hero-level').title = e?.rating > 0 ? tr`계산 난이도${rate !== 1 ? ` · ${rate}x${estimated ? tr(' 추정') : ''}` : ''} (잠정)` : tr('채보에 기재된 레벨');
    const source = $('source-level');
    source.hidden = !(e?.rating > 0 && level);
    source.textContent = source.hidden ? '' : `(${level})`;
    source.title = source.hidden ? '' : tr`원본 레벨 ${level}`;
    source.setAttribute('aria-label', source.title);
    cover(backgroundURL || '');
    const bpm = e?.bpmMin > 0 ? `${e.bpmMin === e.bpmMax ? number(e.bpmMin) : `${number(e.bpmMin)}–${number(e.bpmMax)}`}BPM` : '';
    const s = e ? summary(e, report) : null;
    const graph = $('info-density'), bars = overview(e ? report : null), heights = densityHeights(bars);
    graph.replaceChildren(...bars.map((b, i) => {
        const bar = document.createElement('span');
        bar.style.height = `${heights[i]}%`;
        bar.title = tr`${b.from.toFixed(1)}–${b.to.toFixed(1)}초 · ${b.nps.toFixed(2)} NPS`;
        return bar;
    }));
    graph.setAttribute('aria-label', s ? tr`원곡 1배속 노트 밀도. 평균 ${s.average.toFixed(2)}, 최고 구간 ${s.peak.toFixed(2)} NPS` : tr('노트 밀도 미확인'));
    graph.title = s ? [`${number(s.total)} Notes (LN ${s.ratio.toFixed(1)}%)`, bpm, time(e.length), tr`평균 ${s.average.toFixed(2)} · 최고 ${s.peak.toFixed(2)} NPS`].filter(Boolean).join(' · ') : '';
}
