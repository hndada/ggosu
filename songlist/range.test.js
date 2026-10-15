import test from 'node:test';
import assert from 'node:assert/strict';
import { fields, bin, counts, toRange, toColumns, formats } from './range.js';
import { query } from './search.js';

test('level columns are fixed to Lv 1–40 with open ends', () => {
    const d = fields.level.domain([3, 7.5, 12]);
    assert.deepEqual(d, fields.level.domain([]));
    assert.equal(d.n, 40);
    const c = counts(d, [0, 3, 7.5, 7, 55]);
    assert.deepEqual([c[0], c[2], c[6], c[39]], [1, 1, 2, 1]);
    assert.equal(fields.level.value({ level: 'Lv. 9' }), null);
});

test('length columns are fixed ten seconds over 0:20–8:00 with open ends', () => {
    const d = fields.length.domain([5, 178, 900]);
    assert.deepEqual(d, fields.length.domain([]));
    assert.deepEqual(d, { start: 20, width: 10, n: 46, openEnd: true, openStart: true });
    assert.equal(bin(d, 5), 0);
    assert.equal(bin(d, 900), 45);
    assert.equal(formats.length(d, 7, 15), '1:30–2:59');
    assert.equal(formats.length(d, 7, 45), '1:30–');
    assert.equal(formats.length(d, 0, 3), '–0:59');
});

test('columns and value ranges convert both ways, full selection means no limit', () => {
    const level = fields.level.domain([]), length = fields.length.domain([0, 300]);
    assert.equal(toRange(level, 0, level.n - 1), null);
    assert.deepEqual(toRange(level, 7, 11), { min: 8, max: 13 });
    assert.deepEqual(toRange(level, 0, 4), { min: null, max: 6 });
    assert.deepEqual(toRange(level, 35, 39), { min: 36, max: null });
    assert.deepEqual(toColumns(level, { min: 8, max: 13 }), [7, 11]);
    assert.equal(formats.level(level, 35, 39), 'Lv 36–40+');
    assert.deepEqual(toRange(length, 7, length.n - 1), { min: 90, max: null });
    assert.deepEqual(toRange(length, 0, 3), { min: null, max: 60 });
    assert.deepEqual(toColumns(length, null), [0, length.n - 1]);
    assert.equal(formats.level(level, 7, 11), 'Lv 8–12');
    assert.equal(formats.level(level, 4, 4), 'Lv 5');
});

test('a level window keeps fractional levels with their whole number', () => {
    const entries = [12, 12.5, 13].map((level, index) => ({ title: 't', level: String(level), lanes: 7, length: 60, path: 'p', index }));
    assert.deepEqual(query(entries, { levelRange: { min: 12, max: 13 } }).map(r => r.entry.level), ['12', '12.5']);
    assert.deepEqual(query(entries, { levelRange: { min: 13, max: null } }).map(r => r.entry.level), ['13']);
});
