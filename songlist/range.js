// Range windows over a distribution: drag to choose a level or length range instead of typing numbers.
import { levelOf } from './level.js';

export const fields = {
    // Fixed Lv 1–40 so the window means the same thing for every key count and for downloadable charts.
    // The first and last columns also hold levels below 1 and above 40.
    level: { value: levelOf, domain: () => ({ start: 1, width: 1, n: 40, openEnd: true, openStart: true }) },
    // Fixed ten-second columns over 0:20–8:00; the first and last columns also hold shorter and longer charts.
    length: { value: e => Number.isFinite(e.length) ? e.length : null, domain: () => ({ start: 20, width: 10, n: 46, openEnd: true, openStart: true }) },
};

export function bin(d, value) { return Math.min(d.n - 1, Math.max(0, Math.floor((value - d.start) / d.width))); }
export function counts(d, values) {
    const out = new Array(d.n).fill(0);
    for (const v of values) out[bin(d, v)]++;
    return out;
}
// Columns lo..hi as a value range with an exclusive end; null when every column is chosen.
export function toRange(d, lo, hi) {
    if (lo <= 0 && hi >= d.n - 1) return null;
    return { min: lo <= 0 && (d.openStart || d.start === 0) ? null : d.start + lo * d.width, max: hi >= d.n - 1 && d.openEnd ? null : d.start + (hi + 1) * d.width };
}
export function toColumns(d, range) {
    if (!range) return [0, d.n - 1];
    const lo = range.min == null ? 0 : bin(d, range.min);
    const hi = range.max == null ? d.n - 1 : Math.max(lo, bin(d, range.max - 1e-9));
    return [lo, hi];
}

export const clock = s => `${Math.floor(s / 60)}:${String(Math.floor(s % 60)).padStart(2, '0')}`;
export const formats = {
    level: (d, lo, hi) => {
        const a = d.start + lo * d.width, b = d.start + (hi + 1) * d.width - 1;
        const end = hi >= d.n - 1 && d.openEnd ? `${b}+` : String(b);
        return a === b ? `Lv ${end}` : `Lv ${a}–${end}`;
    },
    length: (d, lo, hi) => `${lo <= 0 && d.openStart ? '' : clock(d.start + lo * d.width)}–${hi >= d.n - 1 && d.openEnd ? '' : clock(d.start + (hi + 1) * d.width - 1)}`,
};
// Axis labels: sparse enough not to collide.
export const axes = {
    level: (d, i) => i === 0 || (i + 1) % 5 === 0 ? String(d.start + i * d.width) : '',
    length: (d, i) => (d.start + i * d.width) % 60 === 0 ? clock(d.start + i * d.width) : '',
};
