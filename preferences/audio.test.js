import test from 'node:test';
import assert from 'node:assert/strict';
import { storedAudio, saveAudio } from './audio.js';

function storage() {
    const values = new Map();
    return { getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value) };
}

test('browser volumes survive reload, preserve mute, and recover a damaged primary', () => {
    const s = storage(), audio = { master: 50, music: 0, keys: 70, effects: 20, hit: 40, hitSound: 'wood', guide: 10 };
    saveAudio(audio, s);
    assert.deepEqual(storedAudio(s), audio);
    s.setItem('ggosu:audio', '{broken');
    assert.deepEqual(storedAudio(s), audio);
    s.setItem('ggosu:audio', JSON.stringify({ music: 0, keys: 30, effects: 10 }));
    assert.deepEqual(storedAudio(s), { master: 100, music: 0, keys: 30, effects: 10, hit: 0, hitSound: 'click', guide: 0 });
});

test('invalid volumes and denied storage are reported without claiming a save', () => {
    const s = storage();
    assert.throws(() => saveAudio({ master: 101 }, s));
    assert.throws(() => saveAudio({ keys: NaN }, s));
    assert.throws(() => saveAudio({ master: 30 }, { setItem() { throw new Error('denied'); } }), /denied/);
});
