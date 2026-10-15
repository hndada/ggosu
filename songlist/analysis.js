import { tr } from '../preferences/language.js';
export function eligible(r) { return !['unavailable', 'legacy'].includes(r.verification) && r.scoreVersion === 2 && r.mode === 'live' && r.completed && !r.practice && r.rate === 1; }

// Matches piano.TotalPP and the desktop's daily key count. Call with verified
// records, never unchecked localStorage values.
export function playerStats(records, now = new Date()) {
    const best = new Map();
    let keysToday = 0;
    for (const r of records) {
        const date = new Date(r.date);
        if (r.mode === 'live' && date.toDateString() === now.toDateString()) keysToday += (r.replay.events ?? []).filter(e => e.down).length;
        if (!eligible({ ...r, scoreVersion: r.version }) || r.replay.rules !== 'ggosu-keys-v2' || r.replay.mods?.kind || r.replay.mods?.visibility || !(r.score.Possible > 0) || !(r.replay.duration_ns > 0)) continue;
        const pp = 10 * r.score.Possible / (r.replay.duration_ns / 1e9) * Math.max(0, Math.min(1, (r.accuracy - .8) / .2));
        best.set(r.replay.chart_hash, Math.max(best.get(r.replay.chart_hash) ?? 0, pp));
    }
    const pp = [...best.values()].sort((a, b) => b - a).reduce((total, value, i) => total + value * .95 ** i, 0);
    return { pp, keysToday };
}

// Browser counterpart of record.Summarize; input rows carry the chart hash and rules.
export function summarize(rows) {
    const summaries = new Map();
    for (const r of rows) {
        if (r.mode !== 'live' || !['ggosu-keys-v1', 'ggosu-keys-v2'].includes(r.rules) || !r.hash || !Number.isFinite(Date.parse(r.date))) continue;
        if (!summaries.has(r.hash)) summaries.set(r.hash, { hash: r.hash, lastPlayed: r.date, completed: false, fullCombo: false, perfect: false, gauge: 0, bestPoints: null });
        const s = summaries.get(r.hash);
        if (Date.parse(r.date) > Date.parse(s.lastPlayed)) s.lastPlayed = r.date;
        const score = normalizeScore(r.score);
        if (!eligible(r) || !(score.Possible > 0)) continue;
        s.bestPoints = s.bestPoints == null ? score.Points : Math.max(s.bestPoints, score.Points);
        if (r.mods?.kind || r.mods?.visibility) continue;
        s.completed = true;
        s.fullCombo ||= score.Miss === 0 && score.MaxCombo === score.Possible;
        s.perfect ||= score.Kool + score.Cool === score.Possible;
        // Stages fall automatically, so one play clears the stage it ends on. Older gauge tables do not count.
        const h = r.health;
        if (h?.gauge === 1 && !h.failed && h.value > 0) s.gauge = Math.max(s.gauge, h.level);
    }
    return [...summaries.values()].sort((a, b) => a.hash.localeCompare(b.hash));
}

export function achievement(s) {
    if (!s) return tr('미플레이');
    return s.perfect ? tr('퍼펙트') : s.fullCombo ? tr('풀콤보') : s.completed ? tr('완주') : tr('플레이함');
}
// Clear lamps from lowest to highest, like IIDX's clear lamp sort; the label is achievement()'s.
// 완주 means the gauge was off or emptied; 1–3단 is the highest health stage still standing at the end.
export const CLEAR_LAMPS = [['none', tr('미플레이')], ['played', tr('플레이함')], ['clear', tr('완주')], ['gauge1', tr('1단 완주')], ['gauge2', tr('2단 완주')], ['gauge3', tr('3단 완주')], ['fullcombo', tr('풀콤보')], ['perfect', tr('퍼펙트')]];
export function clearLamp(s) {
    if (!s) return 'none';
    return s.perfect ? 'perfect' : s.fullCombo ? 'fullcombo' : s.gauge > 0 ? `gauge${Math.min(3, s.gauge)}` : s.completed ? 'clear' : 'played';
}
// The second lamp is the clear lamp: the best achievement across plays, independent of points.
// Clear, full combo and perfect share it; a play without a clear leaves it off.
export function lamps(s) {
    const lamp = clearLamp(s), combo = lamp === 'played' ? 'none' : lamp;
    return {
        combo,
        comboLabel: combo === 'perfect' ? tr('퍼펙트 · 전부 COOL 이상') : combo === 'none' ? tr('완주 기록 없음') : CLEAR_LAMPS.find(([k]) => k === combo)[1],
    };
}
// Browser storage can contain the former field names; display canonical names.
export function normalizeScore(s) {
    const { Perfect, Great, ...score } = s;
    return { ...score, Kool: s.Kool ?? Perfect ?? 0, Cool: s.Cool ?? Great ?? 0 };
}
export function sameMods(a = {}, b = {}) {
    if ((a.visibility || '') !== (b.visibility || '')) return false;
    if ((a.kind || '') !== (b.kind || '')) return false;
    return !a.kind || JSON.stringify(a.mapping ?? []) === JSON.stringify(b.mapping ?? []);
}
export function best(rows) {
    return rows.reduce((a, b) => !a || b.score.Points > a.score.Points || b.score.Points === a.score.Points && b.score.MaxCombo > a.score.MaxCombo ? b : a, null);
}
export function comparison(rows, r) {
    if (!eligible(r)) return { status: 'excluded' };
    const previous = best(rows.filter(p => p.id !== r.id && eligible(p) && sameMods(p.mods, r.mods) && Date.parse(p.date) <= Date.parse(r.date)));
    if (!previous) return { status: 'first' };
    const delta = { points: r.score.Points - previous.score.Points, accuracy: (r.accuracy - previous.accuracy) * 100, combo: r.score.MaxCombo - previous.score.MaxCombo };
    const score = normalizeScore(r.score), before = normalizeScore(previous.score);
    for (const grade of ['Kool', 'Cool', 'Good', 'Miss']) delta[grade] = score[grade] - before[grade];
    return { status: 'compared', previous, delta, improved: delta.points > 0 };
}
export function modsText(m = {}, detail = false) {
    const visibility = { hidden: 'Hidden', fadein: 'Fade In', cover: 'Cover', flashlight: 'Flashlight' }[m.visibility];
    const arrangement = !m.kind ? '' : m.kind === 'mirror' ? 'Mirror' : `${m.kind === 'mirror+random' ? 'Mirror + Random' : 'Random'} · seed ${m.seed ?? 0}` + (detail ? tr` · 매핑(원본→현재): ${(m.mapping ?? []).map((n, i) => `${i + 1}→${n + 1}`).join(', ')}` : '');
    const o2jam = m.o2jam ? `O2Jam ${m.o2jam.toUpperCase()}` + (m.pills ? tr(' + 알약') : '') : '';
    return [arrangement, visibility, o2jam].filter(Boolean).join(' / ') || tr('배치 변경 없음');
}
