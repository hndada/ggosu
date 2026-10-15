import { tr } from '../preferences/language.js';
// Song list layout: one key count at a time, grouped by song or flat by chart.
import { levelAt } from './level.js';
import { CLEAR_LAMPS, clearLamp } from './analysis.js';

const collator = new Intl.Collator('ko', { numeric: true, sensitivity: 'base' });

// Song attributes keep a song's charts together; chart attributes scatter them.
export const SONG_SORTS = new Set(['title', 'artist', 'creator', 'modified', 'recent']);
export function defaultView(sort) { return SONG_SORTS.has(sort) ? 'song' : 'chart'; }

function chartOrder(a, b, rate = 1) {
    const x = levelAt(a.entry, rate).value, y = levelAt(b.entry, rate).value;
    if ((x == null) !== (y == null)) return x == null ? 1 : -1;
    return (x ?? 0) - (y ?? 0) || collator.compare(a.entry.difficulty ?? '', b.entry.difficulty ?? '') || a.entry.index - b.entry.index;
}
// A song is its folder, except O2Jam: one .ojn holds a whole song and o2jam folders keep thousands side by side.
function songKey(e) { return /\.ojn$/i.test(e.path ?? '') ? e.path : e.folder; }
function separator(e, sort, rate, progress) {
    if (sort === 'level') { const n = levelAt(e, rate).value; return n == null ? 'Lv ?' : `Lv ${String(Math.floor(n)).padStart(2, '0')}`; }
    if (sort === 'length') return tr`${Math.floor(e.length / 60)}분대`;
    if (sort === 'lamp') { const k = progress && clearLamp(progress.get(e.hash)); return k ? tr(CLEAR_LAMPS.find(([key]) => key === k)[1]) : tr('기록 없음'); }
    return '';
}

// Charts matching everything except the key count, counted per key count.
export function keyCounts(entries, rows) {
    const counts = new Map([...new Set(entries.map(e => e.lanes))].sort((a, b) => a - b).map(n => [n, 0]));
    for (const { entry } of rows) counts.set(entry.lanes, (counts.get(entry.lanes) ?? 0) + 1);
    return counts;
}

// rows: query() output already filtered to one key count and sorted.
// Returns groups (song view) or items with separators (chart view) and the navigation order.
export function layout(rows, { view, sort, rate = 1, progress = null }) {
    if (view === 'chart') {
        const items = [];
        let last = null;
        for (const row of rows) {
            const label = separator(row.entry, sort, rate, progress);
            if (label && label !== last) items.push({ type: 'separator', label });
            last = label;
            items.push({ type: 'chart', index: row.index, entry: row.entry });
        }
        return { view, items, order: rows.map(r => r.index) };
    }
    const groups = new Map();
    for (const row of rows) {
        const key = songKey(row.entry);
        if (!groups.has(key)) groups.set(key, []);
        groups.get(key).push(row);
    }
    const list = [...groups].map(([folder, charts]) => {
        charts.sort((a, b) => chartOrder(a, b, rate));
        const levels = charts.map(c => levelAt(c.entry, rate).value).filter(n => n != null);
        return { folder, title: charts[0].entry.title, charts: charts.map(c => ({ index: c.index, entry: c.entry })), minLevel: levels.length ? Math.min(...levels) : null, maxLevel: levels.length ? Math.max(...levels) : null };
    });
    return { view, groups: list, order: list.flatMap(g => g.charts.map(c => c.index)) };
}

// Charts shown inside an expanded song: all when few, otherwise a window around the selection.
export function visibleCharts(group, selected, limit = 6) {
    if (group.charts.length <= limit) return { charts: group.charts, before: 0, after: 0 };
    const at = Math.max(0, group.charts.findIndex(c => c.index === selected));
    const start = Math.min(Math.max(0, at - Math.floor(limit / 2)), group.charts.length - limit);
    return { charts: group.charts.slice(start, start + limit), before: start, after: group.charts.length - start - limit };
}

export function step(order, current, delta) {
    if (!order.length) return -1;
    const at = order.indexOf(current);
    if (at < 0) return order[delta < 0 ? order.length - 1 : 0];
    return order[Math.min(order.length - 1, Math.max(0, at + delta))];
}
// Left/right: the previous or next chart of the same song, in song order.
export function sibling(rows, current, delta, rate = 1) {
    const e = rows.find(r => r.index === current)?.entry;
    if (!e) return current;
    const same = rows.filter(r => songKey(r.entry) === songKey(e)).sort((a, b) => chartOrder(a, b, rate)).map(r => r.index);
    return same[Math.min(same.length - 1, Math.max(0, same.indexOf(current) + delta))];
}
// Shift+up/down in song view: first chart of the previous or next song.
export function songStep(groups, current, delta) {
    const at = groups.findIndex(g => g.charts.some(c => c.index === current));
    if (at < 0) return groups.length ? groups[0].charts[0].index : -1;
    return groups[Math.min(groups.length - 1, Math.max(0, at + delta))].charts[0].index;
}
