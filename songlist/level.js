// A chart's level: the calculated rating when the scan has one, else the number the chart itself states.
export function levelOf(e) {
    if (e?.rating > 0) return e.rating;
    const text = String(e?.level ?? '').trim(), n = Number(text);
    return text !== '' && Number.isFinite(n) ? n : null;
}

// Time Rates the scan rates ahead of time (difficulty.Presets); entry.rates holds all but 1x by String(rate).
export const presets = [1, 1.1, 1.25, 1.5, 1.75, 2];
// Levels at other rates, asked for the selected chart: `${hash}@${rate}` → level.
const asked = new Map();
export function remember(hash, rate, value) { asked.set(`${hash}@${rate}`, value); }

function at(e, rate) {
    const v = rate === 1 ? e.rating : e.rates?.[String(rate)] ?? asked.get(`${e.hash}@${rate}`);
    return v > 0 ? v : null;
}

// levelAt is the level shown at a Time Rate: { value, estimated }. A chart without a calculated
// rating keeps its stated level at every rate. Between presets that were not asked for, the value is
// read off the line between the two neighbouring presets; below 1x there is nothing to read it from.
export function levelAt(e, rate = 1) {
    if (!(e?.rating > 0) || !Number.isFinite(rate)) return { value: levelOf(e), estimated: false };
    const exact = at(e, rate);
    if (exact != null) return { value: exact, estimated: false };
    const hi = presets.findIndex(p => p > rate);
    if (hi <= 0) return { value: null, estimated: false };
    const a = at(e, presets[hi - 1]), b = at(e, presets[hi]);
    if (a == null || b == null) return { value: null, estimated: false };
    return { value: a + (b - a) * (rate - presets[hi - 1]) / (presets[hi] - presets[hi - 1]), estimated: true };
}
