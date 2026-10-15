import { tr } from '../preferences/language.js';
// Offline timing analysis. All values are seconds on the decoded audio timeline.
// Scores rank candidates; they are not probabilities of a correct musical beat.
const fps = 200;
const median = a => quantile(a, .5);
function quantile(a, q) { const x = a.toSorted((a, b) => a - b), k = (x.length - 1) * q; return x.length ? x[Math.floor(k)] * (1 - k % 1) + x[Math.ceil(k)] * (k % 1) : 0; }
const mod = (x, p) => (x % p + p) % p;

export function onsets(channels, sampleRate) {
    if (!channels.length || !Number.isFinite(sampleRate) || sampleRate < 8000 || !channels[0].length) throw new Error(tr('분석할 PCM 음원이 없습니다.'));
    if (channels.some(c => c.length !== channels[0].length) || channels[0].length / sampleRate > 1800) throw new Error(tr('같은 길이의 채널과 30분 이하 음원이 필요합니다.'));
    const length = channels[0].length, hop = Math.round(sampleRate / fps), rate = sampleRate / hop;
    const count = Math.ceil(length / hop), energy = new Float64Array(count), bands = [new Float64Array(count), new Float64Array(count)];
    const low = channels.map(() => 0), mid = channels.map(() => 0);
    const a = 1 - Math.exp(-2 * Math.PI * 180 / sampleRate), b = 1 - Math.exp(-2 * Math.PI * 1800 / sampleRate);
    for (let i = 0; i < length; i++) {
        const k = Math.floor(i / hop);
        for (let c = 0; c < channels.length; c++) {
            const x = channels[c][i]; low[c] += a * (x - low[c]); mid[c] += b * (x - mid[c]);
            energy[k] += x * x; bands[0][k] += low[c] * low[c]; bands[1][k] += (mid[c] - low[c]) ** 2;
        }
    }
    const flux = new Float64Array(count);
    for (const band of [energy, ...bands]) {
        let history = 0;
        for (let i = 0; i < count; i++) {
            const v = Math.sqrt(band[i] / hop / channels.length);
            flux[i] += Math.max(0, v - history); history = .65 * history + .35 * v;
        }
    }
    // Normalize within two seconds so a loud chorus doesn't erase a quiet verse.
    const sum = new Float64Array(count + 1);
    for (let i = 0; i < count; i++) sum[i + 1] = sum[i] + flux[i];
    let peakFlux = 0; for (const x of flux) peakFlux = Math.max(peakFlux, x);
    const floor = peakFlux * .002;
    for (let i = 0; i < count; i++) {
        const lo = Math.max(0, i - rate), hi = Math.min(count, Math.ceil(i + rate));
        flux[i] /= Math.max(floor, (sum[hi] - sum[Math.floor(lo)]) / (hi - Math.floor(lo)), 1e-8);
    }
    const threshold = Math.max(1.5, quantile(Array.from(flux), .75)), peaks = [];
    for (let i = 1; i + 1 < count; i++) {
        if (flux[i] < threshold || flux[i] <= flux[i - 1] || flux[i] < flux[i + 1]) continue;
        // Return to the attack's beginning, rather than aligning to its loudest sample.
        let start = i;
        while (start > Math.max(0, i - 4) && energy[start - 1] < energy[start] * .8) start--;
        let sample = start * hop, end = Math.min(length, (i + 1) * hop), max = 0;
        for (let j = sample; j < end; j++) for (const ch of channels) max = Math.max(max, Math.abs(ch[j]));
        while (sample < end && channels.every(ch => Math.abs(ch[sample]) < max * .15)) sample++;
        const peak = { at: sample / sampleRate, weight: Math.min(12, flux[i]) };
        if (peaks.length && peak.at - peaks.at(-1).at < .065) { if (peak.weight > peaks.at(-1).weight) peaks[peaks.length - 1] = peak; }
        else peaks.push(peak);
    }
    return { peaks, duration: length / sampleRate };
}

function periods(peaks, minBPM, maxBPM) {
    // Pair histogram votes on periodicity, retaining half/double-tempo alternatives.
    const step = .0025, lo = 60 / maxBPM, hi = 60 / minBPM;
    const hist = new Float64Array(Math.ceil(hi / step) + 2);
    for (let i = 0; i < peaks.length; i++) for (let j = i + 1; j < peaks.length; j++) {
        const delta = peaks[j].at - peaks[i].at; if (delta > hi * 2) break;
        const w = Math.sqrt(peaks[i].weight * peaks[j].weight);
        for (let n = 1; n <= 4; n++) {
            const p = delta / n; if (p < lo || p > hi) continue;
            const index = Math.round(p / step); hist[index] += w / n;
        }
    }
    const ranked = [];
    for (let i = Math.ceil(lo / step); i < hist.length - 1; i++) {
        const value = hist[i - 1] + 2 * hist[i] + hist[i + 1];
        if (value > 0) ranked.push({ period: i * step, value });
    }
    const out = [];
    for (const p of ranked.sort((a, b) => b.value - a.value)) {
        if (!out.some(x => Math.abs(x - p.period) < .012)) out.push(p.period);
        if (out.length === 18) break;
    }
    return out;
}

function align(peaks, period) {
    const bins = Math.ceil(period / .003), votes = new Float64Array(bins);
    for (const p of peaks) {
        const k = Math.floor(mod(p.at, period) / period * bins);
        for (let d = -2; d <= 2; d++) votes[mod(k + d, bins)] += p.weight * (3 - Math.abs(d));
    }
    let phase = 0; for (let i = 1; i < bins; i++) if (votes[i] > votes[phase]) phase = i;
    let offset = phase / bins * period, matches = [];
    for (let iteration = 0; iteration < 6; iteration++) {
        const byBeat = new Map(), limit = Math.min(.065, period * .18);
        for (const p of peaks) {
            const beat = Math.round((p.at - offset) / period), error = p.at - offset - beat * period;
            if (Math.abs(error) > limit) continue;
            const prev = byBeat.get(beat);
            if (!prev || p.weight / (1 + Math.abs(error) * 50) > prev.weight / (1 + Math.abs(prev.error) * 50)) byBeat.set(beat, { ...p, beat, error });
        }
        matches = [...byBeat.values()]; if (matches.length < 6) return null;
        const spread = Math.max(.004, median(matches.map(p => Math.abs(p.error))) * 3);
        const fit = matches.filter(p => Math.abs(p.error) <= spread);
        let weight = 0, x = 0, y = 0;
        for (const p of fit) { weight += p.weight; x += p.weight * p.beat; y += p.weight * p.at; }
        x /= weight; y /= weight; let xx = 0, xy = 0;
        for (const p of fit) { xx += p.weight * (p.beat - x) ** 2; xy += p.weight * (p.beat - x) * (p.at - y); }
        if (!xx) return null;
        period = xy / xx; offset = y - x * period;
    }
    const errors = matches.map(p => Math.abs(p.at - offset - p.beat * period));
    const first = Math.min(...matches.map(p => p.beat)), last = Math.max(...matches.map(p => p.beat));
    const coverage = matches.length / (last - first + 1);
    const strength = matches.reduce((s, p) => s + p.weight, 0) / peaks.reduce((s, p) => s + p.weight, 0);
    const p95 = quantile(errors, .95), score = Math.sqrt(coverage) * strength / (1 + p95 * 20);
    return { bpm: 60 / period, offset: offset + first * period, period, score, coverage, median: median(errors), p95, matched: matches.length };
}

function candidates(peaks, minBPM, maxBPM) {
    const out = [];
    for (const period of periods(peaks, minBPM, maxBPM)) {
        const p = align(peaks, period);
        if (p && p.bpm >= minBPM && p.bpm <= maxBPM && !out.some(x => Math.abs(x.bpm - p.bpm) < .15)) out.push(p);
    }
    out.sort((a, b) => b.score - a.score);
    if (out.length) for (const factor of [.5, 2]) {
        const p = align(peaks, out[0].period * factor);
        if (p && p.bpm >= minBPM && p.bpm <= maxBPM && !out.some(x => Math.abs(x.bpm - p.bpm) < .15)) out.push(p);
    }
    return out.sort((a, b) => b.score - a.score).slice(0, 5);
}

export function analyze(channels, sampleRate, { minBPM = 40, maxBPM = 300 } = {}) {
    if (!(minBPM >= 30 && maxBPM <= 400 && minBPM < maxBPM)) throw new Error(tr('BPM 범위를 확인해 주세요 (30–400).'));
    const { peaks, duration } = onsets(channels, sampleRate);
    if (duration < 8 || peaks.length < 12) throw new Error(tr('최소 8초와 충분한 박자 어택이 필요합니다. 조용한 곡은 수동 타이밍을 사용해 주세요.'));
    const results = candidates(peaks, minBPM, maxBPM);
    if (!results.length) throw new Error(tr('일관된 박자 후보를 찾지 못했습니다.'));
    const sections = [];
    for (let from = 0; from + 8 <= duration; from += 16) {
        const to = Math.min(duration, from + 16), local = peaks.filter(p => p.at >= from && p.at < to);
        const choices = local.length >= 12 ? candidates(local, minBPM, maxBPM) : [];
        const top = choices[0];
        const changed = !!top && Math.min(...[.5, 1, 2].map(f => Math.abs(top.bpm * f / results[0].bpm - 1))) > .01;
        sections.push({ from, to, candidates: choices, bpm: top?.bpm ?? null, changed, uncertain: !top || top.coverage < .5 || top.p95 > .025 });
    }
    return { candidates: results, sections, duration, onsets: peaks.length };
}

export function calibration(errors) {
    const valid = errors.filter(x => Number.isFinite(x) && Math.abs(x) <= 200);
    if (valid.length < 12) throw new Error(tr('유효한 입력이 12회 이상 필요합니다.'));
    const center = median(valid), deviations = valid.map(x => Math.abs(x - center));
    const spread = median(deviations);
    if (spread > 35) throw new Error(tr('입력 편차가 큽니다. 일정하게 눌러 다시 보정해 주세요.'));
    const kept = valid.filter(x => Math.abs(x - center) <= Math.max(15, spread * 3));
    if (kept.length < 12) throw new Error(tr('일정한 입력이 12회 이상 필요합니다.'));
    return { inputMS: Math.round(median(kept)), spread: Math.round(spread), count: kept.length };
}

export function eventTime(context, timestamp) {
    // Map a keyboard event's performance-clock timestamp to the audio clock.
    const now = performance.now(), at = Number.isFinite(timestamp) && Math.abs(timestamp - now) < 60000 ? timestamp : now;
    return context.currentTime + (at - now) / 1000;
}

export function inputPosition(audioTime, start, cursor, rate, inputMS) {
    return cursor + (audioTime - start - inputMS / 1000) * rate * 1000;
}
