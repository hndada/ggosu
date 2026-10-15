import test from 'node:test';
import assert from 'node:assert/strict';
import { query, random, localDate } from './search.js';

const entries = [
    { title: '곡 10', artist: 'Artist', creator: '', difficulty: 'Hard', level: '10', lanes: 4, bpmMin: 90, bpmMax: 180, length: 60, modified: '2026-10-08T12:00:00+09:00', path: 'a', index: 0 },
    { title: '곡 2', artist: 'ＢＡＮＤ', creator: 'Mapper', difficulty: 'Easy', level: '2', lanes: 7, bpmMin: 120, bpmMax: 120, length: 0, modified: '2026-10-07T12:00:00+09:00', path: 'b', index: 0 },
    { title: '다른 곡', artist: '', creator: 'Mapper', difficulty: 'Special', level: '', lanes: 4, bpmMin: 0, bpmMax: 0, length: 90, modified: '', path: 'c', index: 0 },
];
const indices = q => query(entries, q).map(r => r.index);

test('recent catalogue stays newest first and combines with existing filters', () => {
    const charts = entries.map((e, i) => ({ ...e, hash: String(i) }));
    const progress = new Map([['0', { lastPlayed: '2026-10-08T01:00:00Z' }], ['2', { lastPlayed: '2026-10-09T01:00:00Z' }]]);
    const q = { recent: true, sort: 'recent', progress };
    assert.deepEqual(query(charts, q).map(r => r.index), [2, 0]);
    assert.deepEqual(query(charts, { ...q, desc: true }).map(r => r.index), [2, 0]);
    assert.deepEqual(query(charts, { ...q, text: 'hard' }).map(r => r.index), [0]);
    assert.deepEqual(query(charts, { ...q, levelRange: { min: 10, max: 11 } }).map(r => r.index), [0]);
    assert.deepEqual(query(charts, { ...q, lanes: 7 }), []);
    assert.deepEqual(query(charts, { ...q, progress: new Map() }), []);
    assert.deepEqual(query(charts, { ...q, played: 'new', playedHashes: new Set(progress.keys()) }), []);
});

test('search normalizes Unicode and combines words and numeric conditions', () => {
    assert.deepEqual(indices({ text: 'band mapper' }), [1]);
    assert.deepEqual(indices({ text: 'mapper', field: 'artist' }), []);
    assert.deepEqual(indices({ text: 'HARD', field: 'difficulty', lanes: 4, levelMin: 10, levelMax: 10, lengthMin: 60, lengthMax: 60 }), [0]);
    assert.deepEqual(indices({ lanes: 4, levelMin: 0 }), [0]);
    assert.deepEqual(indices({ lengthMax: 0 }), [1]);
});
test('BPM filters use overlap, not full containment or scroll', () => {
    assert.deepEqual(indices({ bpmMin: 180, bpmMax: 200 }), [0]);
    assert.deepEqual(indices({ bpmMin: 181 }), []);
    assert.deepEqual(indices({ bpmMax: 0 }), []);
});
test('dates include both endpoints and exclude missing values', () => {
    const date = localDate(entries[0].modified);
    assert.deepEqual(indices({ from: date, to: date }), [0]);
    assert.equal(localDate(''), '');
    assert.equal(localDate('invalid'), '');
});
test('natural sort, descending order, and missing values at end', () => {
    assert.deepEqual(indices({ sort: 'level' }), [1, 0, 2]);
    assert.deepEqual(indices({ sort: 'level', desc: true }), [0, 1, 2]);
    assert.deepEqual(indices({ sort: 'bpm', desc: true }), [1, 0, 2]);
    assert.deepEqual(indices({ sort: 'creator', desc: true }), [1, 2, 0]);
    assert.deepEqual(indices({ sort: 'modified', desc: true }), [0, 1, 2]);
    assert.deepEqual(indices({ sort: 'length' }), [1, 0, 2]);
    assert.deepEqual(indices({ sort: 'length', desc: true }), [2, 0, 1]);
    assert.deepEqual(query(entries.slice(0, 2)).map(r => r.index), [1, 0]);
    assert.equal(entries[0].title, '곡 10');
});
test('ties remain deterministic and random only selects the filtered pool', () => {
    const tied = [{ ...entries[0], path: 'same', index: 2 }, { ...entries[0], path: 'same', index: 1 }];
    assert.deepEqual(query(tied).map(r => r.index), [1, 0]);
    const pool = query(entries, { lanes: 4 });
    assert.equal(random(pool, () => 0), pool[0]);
    assert.equal(random(pool, () => 0.999999), pool.at(-1));
    assert.equal(random([], () => 0), null);
});


test('rate-adjusted sorting and filtering follow displayed levels when order reverses', () => {
    const adjusted = [
        { ...entries[0], hash: 'a', rating: 10, rates: { '1.5': 20 } },
        { ...entries[1], hash: 'b', rating: 12, rates: { '1.5': 16 } },
        { ...entries[2], hash: 'unknown', rating: 9 },
    ];
    assert.deepEqual(query(adjusted, { sort: 'level', rate: 1.5 }).map(r => r.index), [1, 0, 2]);
    assert.deepEqual(query(adjusted, { sort: 'level', rate: 1.5, desc: true }).map(r => r.index), [0, 1, 2]);
    assert.deepEqual(query(adjusted, { rate: 1.5, levelRange: { min: 16, max: 17 } }).map(r => r.index), [1]);
    assert.deepEqual(query(adjusted, { sort: 'level', rate: 1 }).map(r => r.index), [2, 0, 1]);
});
