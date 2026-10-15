import test from 'node:test';
import assert from 'node:assert/strict';
import { levelAt, remember } from './level.js';

const e = { hash: 'h', rating: 10, level: '7', rates: { '1.1': 11, '1.25': 12, '1.5': 14, '1.75': 16, '2': 20 } };

test('levelAt reads the preset levels the scan stored', () => {
    assert.deepEqual(levelAt(e, 1), { value: 10, estimated: false });
    assert.deepEqual(levelAt(e, 1.5), { value: 14, estimated: false });
    assert.deepEqual(levelAt(e, 2), { value: 20, estimated: false });
});

test('levelAt estimates between presets until the exact level is asked for', () => {
    assert.deepEqual(levelAt(e, 1.375), { value: 13, estimated: true });
    remember('h', 1.375, 13.4);
    assert.deepEqual(levelAt(e, 1.375), { value: 13.4, estimated: false });
    assert.deepEqual(levelAt(e, 0.8), { value: null, estimated: false });
    remember('h', 0.8, 7.5);
    assert.deepEqual(levelAt(e, 0.8), { value: 7.5, estimated: false });
});

test('a chart without a calculated rating keeps its stated level', () => {
    assert.deepEqual(levelAt({ level: '7' }, 1.5), { value: 7, estimated: false });
});
