import test from 'node:test';
import assert from 'node:assert/strict';
import { query } from './search.js';
import { defaultView, keyCounts, layout, visibleCharts, step, sibling, songStep } from './list.js';

const chart = (folder, title, difficulty, level, lanes, length, index = 0) => ({ path: `${folder}/${difficulty}`, folder, title, difficulty, level, lanes, length, index, artist: '', creator: '', hash: `${folder}-${difficulty}` });
const entries = [
    chart('a', 'Song A', 'Hard', '12', 7, 150),
    chart('b', 'Song B', 'NM', '8', 7, 178),
    chart('b', 'Song B', 'HD', '12', 7, 178),
    chart('b', 'Song B', 'EX', '15', 7, 178),
    chart('c', 'Song C', 'Easy', '3', 4, 200),
];

test('played filter uses chart digests and text search ignores levels', () => {
    const playedHashes = new Set(['b-HD']);
    assert.deepEqual(query(entries, { played: 'played', playedHashes }).map(r => r.entry.difficulty), ['HD']);
    assert.equal(query(entries, { played: 'new', playedHashes }).length, 4);
    assert.equal(query(entries, { text: '12' }).length, 0);
});

test('clear lamp sort ranks lamps upward and filter keeps any chosen lamp', () => {
    const progress = new Map([['b-HD', { perfect: true, fullCombo: true, completed: true }], ['b-EX', { completed: true }], ['b-NM', { completed: false }], ['a-Hard', { fullCombo: true, completed: true }]]);
    const rows = query(entries, { lanes: 7, sort: 'lamp', progress });
    assert.deepEqual(rows.map(r => r.entry.difficulty), ['NM', 'EX', 'Hard', 'HD']);
    const { items } = layout(rows, { view: 'chart', sort: 'lamp', progress });
    assert.deepEqual(items.filter(i => i.type === 'separator').map(i => i.label), ['플레이함', '완주', '풀콤보', '퍼펙트']);
    assert.deepEqual(query(entries, { lamps: new Set(['none', 'perfect']), progress }).map(r => r.entry.difficulty), ['HD', 'Easy']);
    assert.deepEqual(query(entries, { lamps: new Set(['none']), progress: null }), []);
    assert.equal(defaultView('lamp'), 'chart');
    const down = query(entries, { lanes: 7, sort: 'lamp', desc: true, progress });
    assert.deepEqual(down.map(r => r.entry.difficulty), ['HD', 'Hard', 'EX', 'NM']);
    assert.deepEqual(layout(down, { view: 'chart', sort: 'lamp', progress }).items.filter(i => i.type === 'separator').map(i => i.label), ['퍼펙트', '풀콤보', '완주', '플레이함']);
});

test('key counts list every key count and count matches', () => {
    assert.deepEqual([...keyCounts(entries, query(entries, { text: 'song b' }))], [[4, 0], [7, 3]]);
});

test('song view groups by song and orders charts by level', () => {
    const rows = query(entries, { lanes: 7, sort: 'level' });
    const { groups, order } = layout(rows, { view: 'song', sort: 'level' });
    assert.deepEqual(groups.map(g => g.title), ['Song B', 'Song A']);
    assert.deepEqual(groups[0].charts.map(c => c.entry.difficulty), ['NM', 'HD', 'EX']);
    assert.deepEqual([groups[0].minLevel, groups[0].maxLevel], [8, 15]);
    assert.deepEqual(order, [1, 2, 3, 0]);
});

test('chart view keeps sort order and inserts separators', () => {
    const rows = query(entries, { lanes: 7, sort: 'level' });
    const { items, order } = layout(rows, { view: 'chart', sort: 'level' });
    assert.deepEqual(items.map(i => i.type === 'separator' ? i.label : i.entry.difficulty), ['Lv 08', 'NM', 'Lv 12', 'Hard', 'HD', 'Lv 15', 'EX']);
    assert.deepEqual(order, [1, 0, 2, 3]);
    assert.equal(defaultView('title'), 'song');
    assert.equal(defaultView('level'), 'chart');
});

test('long songs show a window around the selection', () => {
    const group = { charts: Array.from({ length: 10 }, (_, i) => ({ index: i })) };
    assert.deepEqual(visibleCharts(group, 0), { charts: group.charts.slice(0, 6), before: 0, after: 4 });
    assert.deepEqual(visibleCharts(group, 9), { charts: group.charts.slice(4), before: 4, after: 0 });
    assert.equal(visibleCharts({ charts: group.charts.slice(0, 3) }, 1).after, 0);
});

test('navigation steps through order, siblings and songs', () => {
    const rows = query(entries, { lanes: 7 });
    const { groups, order } = layout(rows, { view: 'song', sort: 'title' });
    assert.equal(step(order, 0, 1), 1);
    assert.equal(step(order, 3, 1), 3);
    assert.equal(step(order, -1, 1), order[0]);
    assert.equal(sibling(rows, 1, 1), 2);
    assert.equal(sibling(rows, 3, 1), 3);
    assert.equal(songStep(groups, 0, 1), 1);
    assert.equal(songStep(groups, 2, -1), 0);
});

test('charts of one song follow level when the sort key ties', () => {
    const shuffled = [entries[3], entries[1], entries[2]].map((e, i) => ({ ...e, index: i }));
    assert.deepEqual(query(shuffled, { sort: 'title' }).map(r => r.entry.difficulty), ['NM', 'HD', 'EX']);
});


test('rate levels determine separators and same-song chart order', () => {
    const entries = [
        { ...chart('same', 'Song', 'A', '10', 4, 60), rating: 10, rates: { '1.5': 20 } },
        { ...chart('same', 'Song', 'B', '12', 4, 60), rating: 12, rates: { '1.5': 16 } },
    ];
    const rows = query(entries, { sort: 'level', rate: 1.5 });
    const flat = layout(rows, { view: 'chart', sort: 'level', rate: 1.5 });
    assert.deepEqual(flat.items.filter(i => i.type === 'separator').map(i => i.label), ['Lv 16', 'Lv 20']);
    const grouped = layout(rows, { view: 'song', sort: 'level', rate: 1.5 });
    assert.deepEqual(grouped.order, [1, 0]);
    assert.deepEqual([grouped.groups[0].minLevel, grouped.groups[0].maxLevel], [16, 20]);
    assert.equal(sibling(rows, 1, 1, 1.5), 0);
});
