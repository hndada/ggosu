// Query the loaded catalogue. Keep original indices for selection and playback.
import { levelAt } from './level.js';
import { CLEAR_LAMPS, clearLamp } from './analysis.js';
const collator = new Intl.Collator('ko', { numeric: true, sensitivity: 'base' });
const text = value => String(value ?? '').normalize('NFKC').toLocaleLowerCase('ko');
const missing = value => value == null || value === '';

function numeric(value) {
    if (missing(value) || String(value).trim() === '') return null;
    const n = Number(value);
    return Number.isFinite(n) ? n : null;
}
function within(value, min, max) {
    if (min == null && max == null) return true;
    return value != null && (min == null || value >= min) && (max == null || value <= max);
}
// Local dates match the date shown in the UI, including the entire final day.
export function localDate(value) {
    const d = new Date(value);
    if (missing(value) || !Number.isFinite(d.getTime())) return '';
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

// Level is filtered by number, not searched as text, so "12" does not mix titles and levels.
function matches(e, q) {
    const fields = q.field === 'all' || !q.field ? ['title', 'artist', 'creator', 'difficulty'] : [q.field];
    const haystack = text(fields.map(k => e[k] ?? '').join(' '));
    if (!text(q.text).trim().split(/\s+/).every(word => haystack.includes(word))) return false;
    if (q.lanes && e.lanes !== q.lanes) return false;
    if (!within(levelAt(e, q.rate ?? 1).value, q.levelMin, q.levelMax)) return false;
    // A dragged level window: the upper end is exclusive so 12.5 belongs with 12.
    const level = levelAt(e, q.rate ?? 1).value, lr = q.levelRange;
    if (lr && !(level != null && (lr.min == null || level >= lr.min) && (lr.max == null || level < lr.max))) return false;
    if (q.bpmMin != null || q.bpmMax != null) {
        if (!(e.bpmMin > 0 && e.bpmMax > 0)) return false;
        if (q.bpmMin != null && e.bpmMax < q.bpmMin) return false;
        if (q.bpmMax != null && e.bpmMin > q.bpmMax) return false;
    }
    if (!within(numeric(e.length), q.lengthMin, q.lengthMax)) return false;
    // An entry matches any of the length ranges. The upper end is exclusive.
    if (q.lengthRanges?.length && !q.lengthRanges.some(r => (r.min == null || e.length >= r.min) && (r.max == null || e.length < r.max))) return false;
    if (q.played === 'new' && q.playedHashes?.has(e.hash)) return false;
    if (q.played === 'played' && !q.playedHashes?.has(e.hash)) return false;
    if (q.recent && !q.progress?.has(e.hash)) return false;
    // Any of the chosen clear lamps; without readable records nothing matches.
    if (q.lamps?.size && !(q.progress && q.lamps.has(clearLamp(q.progress.get(e.hash))))) return false;
    const date = localDate(e.modified);
    if ((q.from || q.to) && !date) return false;
    return (!q.from || date >= q.from) && (!q.to || date <= q.to);
}

function value(e, key, q) {
    if (key === 'bpm') return e.bpmMin > 0 ? e.bpmMin : null;
    if (key === 'modified') return missing(e.modified) || !Number.isFinite(Date.parse(e.modified)) ? null : Date.parse(e.modified);
    if (key === 'level') return levelAt(e, q.rate ?? 1).value;
    if (key === 'lamp') return q.progress ? CLEAR_LAMPS.findIndex(([k]) => k === clearLamp(q.progress.get(e.hash))) : null;
    return e[key];
}

export function query(entries, q = {}) {
    const key = q.sort || 'title', direction = q.desc ? -1 : 1;
    return entries.map((entry, index) => ({ entry, index })).filter(row => matches(row.entry, q)).sort((a, b) => {
        const x = key === 'recent' ? Date.parse(q.progress?.get(a.entry.hash)?.lastPlayed) : value(a.entry, key, q);
        const y = key === 'recent' ? Date.parse(q.progress?.get(b.entry.hash)?.lastPlayed) : value(b.entry, key, q);
        // Missing values stay at the end in both directions.
        if (missing(x) !== missing(y)) return missing(x) ? 1 : -1;
        let cmp = 0;
        if (!missing(x)) cmp = typeof x === 'number' && typeof y === 'number' ? x - y : collator.compare(String(x), String(y));
        if (cmp) return cmp * (key === 'recent' ? -1 : direction);
        if (key === 'bpm' && a.entry.bpmMax !== b.entry.bpmMax) return (a.entry.bpmMax - b.entry.bpmMax) * direction;
        if (key === 'level') {
            cmp = collator.compare(a.entry.difficulty ?? '', b.entry.difficulty ?? '');
            if (cmp) return cmp * direction;
        }
        // Within one song, charts follow their level so easier charts come first.
        const la = levelAt(a.entry, q.rate ?? 1).value, lb = levelAt(b.entry, q.rate ?? 1).value;
        return collator.compare(a.entry.title, b.entry.title) || (la != null && lb != null ? la - lb : (la == null) - (lb == null)) ||
            collator.compare(a.entry.path, b.entry.path) || a.entry.index - b.entry.index;
    });
}

export function random(rows, draw = Math.random) {
    return rows.length ? rows[Math.floor(draw() * rows.length)] : null;
}
