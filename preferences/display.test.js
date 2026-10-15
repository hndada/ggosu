import test from 'node:test';
import assert from 'node:assert/strict';
import { defaultDisplay, storedDisplay, validateDisplay, saveDisplay } from './display.js';

test('window resolution survives reload and old settings keep 1600x900', () => {
    assert.equal(validateDisplay({ dim: 75, background: true, storyboard: true }).resolution, '1600x900');
    const values = new Map();
    const storage = { getItem: k => values.get(k) ?? null, setItem: (k, v) => values.set(k, v) };
    for (const resolution of ['1024x640', '1280x720', '1366x768', '1600x900', '1920x1080', '2560x1440', '3840x2160']) {
        saveDisplay({ ...defaultDisplay(), resolution }, storage);
        assert.equal(storedDisplay(storage).resolution, resolution);
    }
    for (const resolution of ['800x600', '1280X720', '1280x720 ', 1280, false]) {
        assert.throws(() => validateDisplay({ ...defaultDisplay(), resolution }));
    }
});

test('Early/Late thresholds survive reload and old settings default to COOL', () => {
    assert.equal(validateDisplay({ dim: 75, background: true, storyboard: true }).timingThreshold, 'cool');
    const values = new Map();
    const storage = { getItem: k => values.get(k) ?? null, setItem: (k, v) => values.set(k, v) };
    for (const timingThreshold of ['kool', 'cool', 'good', 'off']) {
        saveDisplay({ ...defaultDisplay(), timingThreshold }, storage);
        assert.equal(storedDisplay(storage).timingThreshold, timingThreshold);
    }
    for (const timingThreshold of ['miss', 'COOL', 1, false]) {
        assert.throws(() => validateDisplay({ ...defaultDisplay(), timingThreshold }));
    }
});

test('note thickness defaults for old settings and survives browser reload', () => {
    const data = new Map();
    const storage = { getItem: k => data.get(k) ?? null, setItem: (k, v) => data.set(k, v) };
    assert.equal(validateDisplay({ dim: 75, background: true, storyboard: true }).noteThickness, 100);
    assert.equal(validateDisplay({ ...defaultDisplay(), noteThickness: 0 }).noteThickness, 100);
    for (const noteThickness of [50, 100, 150, 200]) {
        saveDisplay({ ...defaultDisplay(), noteThickness }, storage);
        assert.equal(storedDisplay(storage).noteThickness, noteThickness);
    }
    for (const noteThickness of [49, 201, 100.5, '100', false, NaN]) {
        assert.throws(() => validateDisplay({ ...defaultDisplay(), noteThickness }));
    }
});

test('every skin choice survives browser reload', () => {
    const data = new Map();
    const storage = { getItem: k => data.get(k) ?? null, setItem: (k, v) => data.set(k, v) };
    for (const skin of ['argon', 'triangles', 'classic', 'circles', 'o2jam']) {
        saveDisplay({ ...defaultDisplay(), skin }, storage);
        assert.equal(storedDisplay(storage).skin, skin);
    }
});

test('lane lines default off and their explicit choice survives reload', () => {
    assert.equal(validateDisplay({ dim: 75, background: true, storyboard: true }).laneLines, false);
    const data = new Map();
    const storage = { getItem: k => data.get(k) ?? null, setItem: (k, v) => data.set(k, v) };
    saveDisplay({ ...defaultDisplay(), laneLines: true }, storage);
    assert.equal(storedDisplay(storage).laneLines, true);
    assert.throws(() => validateDisplay({ ...defaultDisplay(), laneLines: 'yes' }));
});

test('HUD settings retain background preferences and accept older saved settings', () => {
    const old = { dim: 0, background: false, storyboard: false };
    assert.deepEqual(storedDisplay({ getItem: () => JSON.stringify(old) }), { ...defaultDisplay(), ...old });
    assert.deepEqual(validateDisplay({ ...old, hud: 'focus' }), { ...defaultDisplay(), ...old, hud: 'focus' });
    assert.deepEqual(storedDisplay({ getItem: () => JSON.stringify({ ...old, comboPosition: 'below', judgementPosition: 'left' }) }), { ...defaultDisplay(), ...old, comboPosition: 'below', judgementPosition: 'left' });
    assert.deepEqual(storedDisplay({ getItem: () => JSON.stringify({ ...old, timeDisplay: 'pie' }) }), { ...defaultDisplay(), ...old, timeDisplay: 'pie' });
    assert.deepEqual(validateDisplay({ ...old, windowMode: 'fullscreen', letterbox: true, letterboxWidth: 50, letterboxY: -50, cursorSize: 0 }), { ...defaultDisplay(), ...old, windowMode: 'fullscreen', letterbox: true, letterboxWidth: 50, letterboxY: -50 });
    assert.deepEqual(storedDisplay({ getItem: () => '{broken' }), defaultDisplay());
    assert.deepEqual(storedDisplay({ getItem() { throw new Error('storage unavailable'); } }), defaultDisplay());
    for (const change of [{ hud: 'other' }, { dim: -1 }, { dim: 101 }, { background: 'false' }, { comboPosition: 'other' }, { judgementPosition: 'other' }, { timeDisplay: 'clock' }, { skin: 'legacy' }, { windowMode: 'exclusive' }, { letterboxWidth: 10 }, { letterboxX: 51 }, { cursorSize: 300 }, { letterbox: 'yes' }]) assert.throws(() => validateDisplay({ ...defaultDisplay(), ...change }));
});

test('dragged HUD layouts survive browser save/reload and reject invalid coordinates', () => {
	const hudLayout = { hitPosition: 500, score: { x: 260, y: -70 }, combo: { x: -180, y: 45 }, ghost: { x: 20, y: -30, scale: 125 } };
	const d = validateDisplay({ ...defaultDisplay(), hudLayout });
	assert.deepEqual(storedDisplay({ getItem: () => JSON.stringify(d) }), d);
	assert.deepEqual(d.hudLayout, hudLayout);
	for (const l of [null, [], { hitPosition: 239 }, { hitPosition: 641 }, { hitPosition: 500.5 }, { score: { x: 1281, y: 0 } }, { combo: { x: 0, y: NaN } }, { unknown: { x: 0, y: 0 } }]) assert.throws(() => validateDisplay({ ...defaultDisplay(), hudLayout: l }));
});

test('HUD scale persists with positions and rejects invalid scale',()=>{
    const d=validateDisplay({...defaultDisplay(),hudLayout:{score:{x:-300,y:100,scale:150}}});
    assert.equal(storedDisplay({getItem:()=>JSON.stringify(d)}).hudLayout.score.scale,150);
    for(const scale of [24,201,1.5,'150']) assert.throws(()=>validateDisplay({...defaultDisplay(),hudLayout:{score:{x:0,y:0,scale}}}));
});

test('score and judgement counts retain independent positions and scales after reload', () => {
    const data = new Map();
    const storage = { getItem: k => data.get(k) ?? null, setItem: (k, v) => data.set(k, v) };
    const score = { x: -230, y: 110, scale: 125 };
    const counts = { x: -180, y: 220, scale: 150 };
    saveDisplay({ ...defaultDisplay(), hudLayout: { score, counts } }, storage);
    assert.deepEqual(storedDisplay(storage).hudLayout, { score, counts });
    saveDisplay({ ...storedDisplay(storage), hudLayout: { counts } }, storage);
    assert.deepEqual(storedDisplay(storage).hudLayout, { counts });
    for (const p of [{ x: 1281, y: 0 }, { x: 0, y: 721 }, { x: 0, y: 0, scale: 201 }]) {
        assert.throws(() => validateDisplay({ ...defaultDisplay(), hudLayout: { counts: p } }));
    }
});

test('damaged display settings recover from backup and preserve original bytes', () => {
    const values = new Map();
    const storage = { getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value) };
    const saved = saveDisplay({ ...defaultDisplay(), dim: 40 }, storage);
    const broken = '{broken'; storage.setItem('ggosu:display', broken);
    const notices = [];
    assert.deepEqual(storedDisplay(storage, text => notices.push(text)), saved);
    assert.equal(notices.length, 1);
    assert.equal([...values].find(([key]) => key.startsWith('ggosu:display:invalid:'))[1], broken);
    assert.deepEqual(storedDisplay(storage), saved);
    for (const text of ['null', '[]', '"abc"', '{"dim":999}']) {
        storage.setItem('ggosu:display', text); storage.setItem('ggosu:display:backup', 'broken');
        assert.deepEqual(storedDisplay(storage), defaultDisplay());
    }
    storage.setItem('ggosu:display', JSON.stringify({ ...saved, dim: 60 }));
    assert.equal(storedDisplay(storage).dim, 60);
});


test('high tick rates and frame limits survive browser storage with a legacy default', () => {
    assert.equal(validateDisplay({ ...defaultDisplay(), tps: 0 }).tps, 240);
    for (const tps of [-1, 60, 144, 240, 360, 480, 1000]) {
        assert.equal(validateDisplay({ ...defaultDisplay(), tps, frameLimit: 'unlimited' }).tps, tps);
    }
    for (const tps of [-2, 29, 1001, 240.5]) assert.throws(() => validateDisplay({ ...defaultDisplay(), tps }));
    assert.throws(() => validateDisplay({ ...defaultDisplay(), frameLimit: 'bad' }));
});


test('ghost target defaults to nine hundred thousand and can be disabled or changed', () => {
    assert.equal(defaultDisplay().ghostScore, 900000);
    assert.equal(validateDisplay({ ...defaultDisplay(), ghost: 'off', ghostScore: 900000 }).ghostScore, 900000);
    assert.throws(() => validateDisplay({ ...defaultDisplay(), ghostScore: -1 }));
});
