import { tr } from './language.js';
// Key setup for the settings dialog: keys pressed in order fill the lanes, lanes
// light while held,
// the chord counter exposes keyboard rollover limits and a short visual test
// measures hit timing. This file holds the pure helpers; the screen half is
// scenes/settings/keytest.js.
const reserved = new Set(['P', 'ESCAPE', 'F6', 'F7', 'PAGEUP', 'PAGEDOWN']);
export const WINDOW = 150;

export const keyName = code => code.startsWith('Key') ? code.slice(3) : code.startsWith('Digit') ? code.slice(5) : code.toUpperCase();
export function classify(name) {
    if (reserved.has(name)) return 'reserved';
    return name === 'SPACE' || name === 'SEMICOLON' || /^[A-Z0-9]$/.test(name) ? 'ok' : 'unsupported';
}
export const lanesOf = (keys, name) => keys.flatMap((k, i) => k === name ? [i] : []);
export function duplicates(keys) {
    const out = new Set();
    keys.forEach((k, i) => { const j = keys.indexOf(k); if (j !== i) out.add(i).add(j); });
    return out;
}
// Lane a key may not take during sequential capture: one already assigned before
// the cursor. Lanes from the cursor on are about to be replaced, so they do not count.
export const conflict = (keys, at, name) => keys.slice(0, at).indexOf(name);
// The first reason the backend would reject these keys, or '' when they are valid.
export function problem(keys) {
    for (const k of keys) {
        if (classify(k) === 'reserved') return tr`${k}는 재생 제어용 예약 키라 레인에 쓸 수 없습니다.`;
        if (classify(k) === 'unsupported') return tr`${k}는 지원하지 않는 키입니다. A–Z, 0–9, Space, 세미콜론을 사용하세요.`;
    }
    for (const k of keys) {
        const lanes = lanesOf(keys, k);
        if (lanes.length > 1) return tr`${k}가 레인 ${lanes.map(l => l + 1).join(', ')}에 중복됐습니다.`;
    }
    return '';
}
// Stairs up and down, then two full chords while the hands can still cover every lane.
export function pattern(lanes, gap = 250) {
    const order = [...Array(lanes).keys()];
    const notes = [...order, ...order.slice(0, -1).reverse()].map((lane, i) => ({ lane, at: i * gap }));
    let at = (notes.length + 1) * gap;
    if (lanes > 1 && lanes <= 8) for (let c = 0; c < 2; c++, at += 2 * gap) for (const lane of order) notes.push({ lane, at });
    return notes;
}
// Index of the closest unjudged note in the lane within the hit window, or -1.
export function judge(notes, lane, t, window = WINDOW) {
    let best = -1;
    notes.forEach((n, i) => {
        if (n.lane !== lane || n.result !== undefined || Math.abs(t - n.at) > window) return;
        if (best < 0 || Math.abs(t - n.at) < Math.abs(t - notes[best].at)) best = i;
    });
    return best;
}
export function summary(notes) {
    const hits = notes.filter(n => typeof n.result === 'number').map(n => n.result);
    const mean = hits.length ? hits.reduce((a, b) => a + b, 0) / hits.length : 0;
    const spread = hits.length ? Math.sqrt(hits.reduce((a, b) => a + (b - mean) ** 2, 0) / hits.length) : 0;
    return { hits: hits.length, total: notes.length, mean: Math.round(mean), spread: Math.round(spread) };
}
