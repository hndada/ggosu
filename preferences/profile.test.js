import test from 'node:test';
import assert from 'node:assert/strict';
import { defaultProfile, storedProfile, saveProfile, resetProfile, effectiveProfile, keepProfile, storedOffsets, saveOffsets, storedName, saveName } from './profile.js';

function storage() {
    const values = new Map();
    return { getItem: k => values.get(k) ?? null, setItem: (k, v) => values.set(k, v) };
}
test('browser scroll and cover are shared while keys and offsets survive per key count', () => {
    const s = storage();
    saveProfile(4, { ...defaultProfile(4), keys: ['1', '2', '3', '4'], scrollSpeed: 18, scrollSpeedOffset: 1.5, exposureOffset: 120 }, s);
    const p7 = storedProfile(7, s);
    assert.equal(p7.scrollSpeed, 18);
    assert.equal(p7.scrollSpeedOffset, 0);
    saveProfile(7, { ...p7, scrollMode: 'exposure', exposureMS: 900, cover: { ...p7.cover, suddenOn: true } }, s);
    const p4 = storedProfile(4, s);
    assert.deepEqual(p4.keys, ['1', '2', '3', '4']);
    assert.equal(effectiveProfile(p4).scrollSpeed, 19.5);
    assert.equal(effectiveProfile(p4).exposureMS, 1020);
    assert.equal(p4.cover.suddenOn, true);
    const reset = resetProfile(4, s);
    assert.deepEqual(reset.keys, ['D', 'F', 'J', 'K']);
    assert.equal(reset.exposureMS, 900);
    assert.equal(reset.scrollSpeedOffset, 0);
    assert.equal(reset.exposureOffset, 0);
    assert.equal(reset.cover.suddenOn, true);
});
test('in-play adjustments keep offsets and do not replace custom keys', () => {
    const s = storage(), p = { ...defaultProfile(4), keys: ['1', '2', '3', '4'], scrollSpeedOffset: 1.5, exposureOffset: 120 };
    saveProfile(4, p, s);
    const played = { ...effectiveProfile(p), scrollSpeed: 20, exposureMS: 1000 };
    keepProfile(4, played, s);
    assert.deepEqual(storedProfile(4, s).keys, p.keys);
    assert.equal(storedProfile(4, s).scrollSpeed, 18.5);
    assert.equal(storedProfile(7, s).exposureMS, 880);
});
test('bad keys and settings are rejected without overwriting the saved profile', () => {
    const s = storage(), p = defaultProfile(4);
    saveProfile(4, p, s);
    for (const change of [{ keys: ['D', 'D', 'J', 'K'] }, { keys: ['P', 'F', 'J', 'K'] }, { scrollSpeed: NaN }, { exposureMS: 1 }, { speedChange: 3 }, { cover: { sudden: 2, hidden: 0, lift: 0 } }]) assert.throws(() => saveProfile(4, { ...p, ...change }, s));
    assert.deepEqual(storedProfile(4, s), p);
    s.setItem('ggosu:settings', '{broken');
    assert.throws(() => storedProfile(4, s));
    assert.equal(s.getItem('ggosu:settings'), '{broken');
});
test('manual timing is stored by chart hash and names survive reload', () => {
    const s = storage();
    saveOffsets('a', { globalMS: 20, chartMS: -5 }, s);
    saveOffsets('b', { globalMS: 25, chartMS: 8 }, s);
    assert.deepEqual(storedOffsets('a', s), { globalMS: 25, chartMS: -5 });
    assert.deepEqual(storedOffsets('', s), { globalMS: 25, chartMS: 0 });
    assert.throws(() => saveOffsets('a', { globalMS: 1001, chartMS: 0 }, s));
    assert.equal(saveName(' 고수 ', s), '고수');
    assert.equal(storedName(s), '고수');
    assert.throws(() => saveName('x'.repeat(33), s));
});
