import test from 'node:test';
import assert from 'node:assert/strict';
import { analyze, calibration, inputPosition } from './beatfit.js';
import { Draft } from './edit.js';
import { deviceSettings } from './sync.js';

function clicks(bpm, offset = .237, duration = 48, rate = 16000, jitter = 0) {
    const x = new Float32Array(duration * rate);
    for (let beat = 0, t = offset; t < duration - .03; t = offset + ++beat * 60 / bpm) {
        const start = Math.round((t + Math.sin(beat * 7) * jitter) * rate);
        for (let i = 0; i < rate * .025 && start + i < x.length; i++) x[start + i] += Math.exp(-i / (rate * .006)) * Math.cos(i * .4);
    }
    return x;
}
test('joint BPM/phase fit preserves a fractional tempo over three minutes', () => {
    const result = analyze([clicks(137.3, .237, 180)], 16000);
    const p = result.candidates.find(p => Math.abs(p.bpm - 137.3) < .02);
    assert.ok(p, JSON.stringify(result.candidates));
    assert.ok(Math.abs(p.offset - .237) < .008, p.offset);
    assert.ok(Math.abs(180 * (p.bpm / 137.3 - 1)) < .008, p.bpm);
    assert.ok(p.p95 < .008, p.p95);
});
test('silence, lead-in, anti-phase stereo and half/double tempo alternatives', () => {
    const x = clicks(120, 3.237), result = analyze([x, Float32Array.from(x, v => -v)], 16000);
    assert.ok(Math.abs(result.candidates[0].bpm - 120) < .05, JSON.stringify(result.candidates));
    assert.ok(Math.abs(result.candidates[0].offset - 3.237) < .008);
    assert.ok(result.candidates.some(p => Math.abs(p.bpm - 60) < .05));
    assert.ok(result.candidates.some(p => Math.abs(p.bpm - 240) < .05));
    assert.throws(() => analyze([new Float32Array(16000 * 10)], 16000), /어택/);
});
test('tempo change remains visible in section analysis instead of a single trusted average', () => {
    const a = clicks(120, .237, 64), b = clicks(150, .237, 64);
    a.set(b.subarray(32 * 16000), 32 * 16000);
    const r = analyze([a], 16000);
    assert.ok(Math.abs(r.sections[0].bpm - 120) < .1);
    assert.ok(Math.abs(r.sections.at(-1).bpm - 150) < .1, JSON.stringify(r.sections));
    assert.ok(r.sections.some(s => s.changed), JSON.stringify(r.sections));
});
test('slow drum attacks at 44.1kHz align to their start, not an empty preceding frame', () => {
    const rate = 44100, x = new Float32Array(48 * rate), bpm = 137.3, offset = .237;
    for (let beat = 0, at = offset; at < 47.9; at = offset + ++beat * 60 / bpm) {
        const start = Math.round(at * rate);
        for (let i = 0; i < rate * .1 && start + i < x.length; i++) x[start + i] += .6 * Math.exp(-i / (rate * .015)) * Math.cos(i * .03);
    }
    const r = analyze([x], rate), p = r.candidates[0];
    assert.ok(Math.abs(p.bpm - bpm) < .005, JSON.stringify(r.candidates));
    assert.ok(Math.abs(p.offset - offset) < .002, p.offset);
    assert.ok(r.candidates.some(p => Math.abs(p.bpm - bpm * 2) < .02));
});
test('calibration resists outliers and does not serialize device correction in a chart', () => {
    const c = calibration([...Array.from({ length: 20 }, (_, i) => 57 + i % 5), 180, -170, NaN]);
    assert.equal(c.inputMS, 59); assert.equal(c.count, 20);
    assert.throws(() => calibration([1, 2]), /12/);
    assert.throws(() => calibration(Array.from({ length: 20 }, (_, i) => i % 2 ? 100 : -100)), /편차/);
    assert.ok(Math.abs(inputPosition(10.059, 8, 1000, 2, 59) - 5000) < 1e-8);
    const d = new Draft({ notes: [], timing: [{ at_ns: 0, bpm: 120 }], metadata: {} });
    const before = d.document(); calibration(Array(20).fill(59)); assert.deepEqual(d.document(), before);
    assert.deepEqual(deviceSettings({ audioMS: Infinity, inputMS: 1001 }), { audioMS: 0, inputMS: 0 });
    assert.deepEqual(deviceSettings({ audioMS: 80, inputMS: -57 }), { audioMS: 80, inputMS: -57 });
});
test('a sustained tone and weaker offbeat hi-hats do not move the drum beat grid', () => {
    const rate = 16000, x = new Float32Array(rate * 48), bpm = 137.3, offset = .237;
    for (let i = 0; i < x.length; i++) x[i] = .012 * Math.sin(i * .087);
    for (let beat = 0, at = offset; at < 47.9; at = offset + ++beat * 60 / bpm) {
        let start = Math.round(at * rate);
        for (let i = 0; i < rate * .08 && start + i < x.length; i++) x[start + i] += .6 * Math.exp(-i / (rate * .012)) * Math.cos(i * .08);
        start = Math.round((at + 30 / bpm) * rate);
        for (let i = 0; i < rate * .02 && start + i < x.length; i++) x[start + i] += .05 * Math.exp(-i / (rate * .003)) * Math.cos(i * 2.1);
    }
    const p = analyze([x], rate).candidates[0];
    assert.ok(Math.abs(p.bpm - bpm) < .01); assert.ok(Math.abs(p.offset - offset) < .002);
});
