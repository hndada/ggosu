import test from 'node:test';
import assert from 'node:assert/strict';
import { defaultPlayback, validatePlayback, storedPlayback, savePlayback } from './playback.js';

test('play options survive reload including false Pitch and Auto', () => {
    const data = new Map();
    const storage = { getItem: k => data.get(k) ?? null, setItem: (k, v) => data.set(k, v) };
    assert.deepEqual(storedPlayback(storage), defaultPlayback());
    const p = { health: 2, healthFail: 1, mode: 'auto', mod: 'mirror+random', visibility: 'hidden', rate: 1.75, pitch: false };
    savePlayback(p, storage);
    assert.deepEqual(storedPlayback(storage), p);
    for (const change of [{ visibility: 'hidden+fadein' }, { visibility: 1 }, { rate: 0.49 }, { rate: 2.01 }, { rate: NaN }, { pitch: 'false' }, { mod: 'shuffle' }, { mode: 'replay' }]) assert.throws(() => validatePlayback({ ...p, ...change }));
    assert.throws(() => savePlayback(p, { setItem() { throw new Error('disk full'); } }), /disk full/);
});


test('health defaults to stage 3 with no game over and keeps explicit off', () => {
    assert.equal(validatePlayback({ mode: 'live', mod: '', rate: 1, pitch: true }).health, 3);
    for (const health of [0, 1, 2, 3]) assert.equal(validatePlayback({ ...defaultPlayback(), health }).health, health);
    for (const health of [-1, 4, 1.5]) assert.throws(() => validatePlayback({ ...defaultPlayback(), health }));
    for (const healthFail of [-1, 4, 1.5]) assert.throws(() => validatePlayback({ ...defaultPlayback(), healthFail }));
    assert.throws(() => validatePlayback({ ...defaultPlayback(), health: 1, healthFail: 2 }));
    assert.equal(validatePlayback({ ...defaultPlayback(), health: 0, healthFail: 3 }).health, 0);
});
