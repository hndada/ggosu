import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const tick = () => new Promise(resolve => setImmediate(resolve));
let version = 0;
async function setup(t, app) {
    const old = { document: globalThis.document, window: globalThis.window };
    t.after(() => { globalThis.document = old.document; globalThis.window = old.window; });
    const fields = new Map();
    const create = () => ({
        value: '', checked: false, textContent: '', listeners: {}, children: [], style: {}, attributes: {},
        get valueAsNumber() { return this.value === '' ? NaN : Number(this.value); },
        get classList() { return { contains: name => this.className === name }; },
        addEventListener(k, fn) { const old = this.listeners[k]; this.listeners[k] = old ? () => { old(); fn(); } : fn; },
        dispatchEvent(e) { this.listeners[e.type]?.(); },
        setAttribute(k, v) { this.attributes[k] = v; },
        querySelector() { return null; },
        replaceWith() {},
        append(child) { this.children.push(child); child.parentElement = this; },
    });
    const element = id => {
        if (!fields.has(id)) fields.set(id, create());
        return fields.get(id);
    };
    const buttons = [1, 1.1, 1.25, 1.5].map(rate => Object.assign(create(), { dataset: { rate: String(rate) } }));
    element('rate-presets').querySelectorAll = () => buttons;
    Object.assign(element('rate-slider'), { min: '0', max: '1', step: 'any', value: '.3' });
    const groups = {};
    for (const [id, key, values] of [
        ['arrangement-buttons', 'mod', ['mirror', 'random']],
        ['visibility-buttons', 'visibility', ['hidden', 'fadein', 'cover', 'flashlight']],
        ['mode-buttons', 'value', ['live', 'auto']],
        ['health-buttons', 'value', ['0', '1', '2', '3']],
        ['health-fail-buttons', 'value', ['0', '1', '2', '3']],
        ['pitch-buttons', 'value', ['true', 'false']],
    ]) {
        groups[id] = values.map(value => Object.assign(create(), { dataset: { [key]: value } }));
        element(id).querySelectorAll = () => groups[id];
    }
    for (const id of ['mod-o2jam', 'mod-pills']) {
        const state = create();
        element(id).querySelector = () => state;
    }
    globalThis.document = { getElementById: element, createElement: create };
    globalThis.window = { go: { main: { App: app } } };
    const dependency = new URL('../../preferences/playback.js', import.meta.url).href;
    const source = readFileSync(new URL('./playback.js', import.meta.url), 'utf8')
    .replace("'../../preferences/language.js'", JSON.stringify(new URL('../../preferences/language.js', import.meta.url).href)).replace('/preferences/playback.js', dependency).replace('./slider.js', new URL('./slider.js', import.meta.url).href).replace('./mods.js', new URL('./mods.js', import.meta.url).href);
    const ui = await import('data:text/javascript;base64,' + Buffer.from(source + '\n//' + ++version).toString('base64'));
    await ui.mount(() => {});
    return { ui, element, buttons, groups };
}

test('arrangement toggles compose independently, persist in order and restore both active', async t => {
    const saved = [];
    const { element, groups, ui } = await setup(t, {
        PlaybackSettings: async () => ({ mode: 'live', mod: 'mirror+random', rate: 1, pitch: true }),
        SavePlayback: async p => saved.push(p),
    });
    const [mirror, random] = groups['arrangement-buttons'];
    assert.equal(mirror.attributes['aria-pressed'], 'true');
    assert.equal(random.attributes['aria-pressed'], 'true');
    assert.equal(element('arrangement-value').textContent, 'Mirror + Random');
    for (const b of [mirror, random, random, mirror, random]) b.listeners.click();
    await tick();
    assert.deepEqual(saved.map(p => p.mod), ['random', '', 'random', 'mirror+random', 'mirror']);
    assert.equal(element('lane-mod').value, 'mirror');
    assert.equal(mirror.attributes['aria-pressed'], 'true');
    assert.equal(random.attributes['aria-pressed'], 'false');
    groups['mode-buttons'][1].listeners.click();
    groups['health-buttons'][3].listeners.click();
    groups['pitch-buttons'][1].listeners.click();
    await tick();
    assert.deepEqual(saved.at(-1), { mode: 'auto', mod: 'mirror', rate: 1, pitch: false, health: 3, healthFail: 0 });
    ui.setBusy(true);
    assert.equal(element('play-controls').disabled, true);
});

test('visibility restores, switches exclusively, turns off and saves alongside arrangement', async t => {
    const saved = [];
    const { element, groups } = await setup(t, {
        PlaybackSettings: async () => ({ mode: 'live', mod: 'mirror', rate: 1, pitch: true, visibility: 'hidden' }),
        SavePlayback: async p => saved.push(p),
    });
    const [hidden, fadein, cover, flashlight] = groups['visibility-buttons'];
    assert.equal(hidden.attributes['aria-pressed'], 'true');
    for (const b of [fadein, cover, flashlight, flashlight]) b.listeners.click();
    await tick();
    assert.deepEqual(saved.map(p => p.visibility ?? ''), ['fadein', 'cover', 'flashlight', '']);
    assert.ok(saved.every(p => p.mod === 'mirror'));
    assert.equal(element('visibility-value').textContent, 'OFF');
    assert.ok(groups['visibility-buttons'].every(b => b.attributes['aria-pressed'] === 'false'));
    element('visibility-mod').disabled = true;
    hidden.listeners.click();
    assert.equal(element('visibility-mod').value, '');
});

test('rapid edits save in order and invalid input stays unsaved after an earlier save completes', async t => {
    const calls = [];
    let finish;
    const first = new Promise(resolve => { finish = resolve; });
    const { ui, element } = await setup(t, {
        PlaybackSettings: async () => ({ mode: 'auto', mod: 'mirror', rate: 1.5, pitch: false }),
        SavePlayback: async p => { calls.push(p); if (calls.length === 1) await first; },
    });
    assert.equal(element('pitch').checked, false);
    const rate = element('time-rate');
    rate.value = 1.75; rate.listeners.input();
    await tick();
    rate.value = 1.9; rate.listeners.input();
    rate.value = ''; rate.listeners.input();
    finish(); await tick(); await tick();
    assert.deepEqual(calls.map(p => p.rate), [1.75, 1.9]);
    assert.equal(ui.needsSave(), true);
    rate.value = 2; rate.listeners.input();
    await tick();
    assert.equal(ui.needsSave(), false);
    assert.equal(calls.at(-1).rate, 2);
});

test('save failure keeps the value editable and can be retried', async t => {
    let fail = true;
    const { ui, element } = await setup(t, {
        PlaybackSettings: async () => ({ mode: 'live', mod: '', rate: 1, pitch: true }),
        SavePlayback: async () => { if (fail) throw new Error('disk full'); },
    });
    const rate = element('time-rate'); rate.value = 1.2; rate.listeners.input();
    await tick();
    assert.equal(ui.needsSave(), true);
    assert.match(element('playback-status').textContent, /disk full/);
    fail = false; rate.listeners.change(); await tick();
    assert.equal(ui.needsSave(), false);
});

test('custom entry keeps the full scale and preset positions while changing allowed values', async t => {
    const saved = [];
    const { ui, element, buttons } = await setup(t, {
        PlaybackSettings: async () => ({ mode: 'live', mod: '', rate: 1, pitch: true }),
        SavePlayback: async p => saved.push(p.rate),
    });
    const rate = element('time-rate'), slider = element('rate-slider'), custom = element('rate-custom');
    assert.equal(element('rate-input').hidden, true);
    assert.equal(slider.min, '0');
    assert.equal(slider.max, '1');
    assert.equal(slider.valueAsNumber, .3);
    const positions = buttons.map(b => b.style.left);
    assert.equal(positions[0], 'calc((100% - 50px) * 0.3 + 25px)');
    custom.listeners.click();
    assert.equal(custom.attributes['aria-pressed'], 'true');
    assert.equal(element('rate-custom-state').textContent, 'ON');
    assert.equal(element('rate-value').hidden, true);
    assert.equal(rate.valueAsNumber, 1);
    assert.equal(slider.valueAsNumber, .3);
    assert.deepEqual(buttons.map(b => b.style.left), positions);
    const nub = slider.parentElement.children[2];
    assert.equal(nub.style.left, 'calc((100% - 50px) * 0.3)');
    custom.listeners.click();
    assert.equal(custom.attributes['aria-pressed'], 'false');
    assert.equal(element('rate-value').hidden, false);
    assert.equal(slider.valueAsNumber, .3);
    assert.deepEqual(buttons.map(b => b.style.left), positions);
    buttons[2].listeners.click(); await tick();
    assert.equal(rate.valueAsNumber, 1.25);
    assert.equal(buttons[2].attributes['aria-pressed'], 'true');
    slider.value = .75; slider.listeners.input(); await tick();
    assert.equal(rate.valueAsNumber, 1.5);
    custom.listeners.click();
    assert.equal(slider.min, '0');
    assert.equal(slider.max, '1');
    assert.equal(element('rate-input').hidden, false);
    for (const value of [0, 1]) { slider.value = value; slider.listeners.input(); await tick(); }
    rate.value = 1.234; rate.listeners.input(); await tick();
    assert.equal(rate.valueAsNumber, 1.234);
    custom.listeners.click(); await tick();
    assert.equal(rate.valueAsNumber, 1.25);
    assert.deepEqual(saved, [1.25, 1.5, .5, 2, 1.234, 1.25]);
    assert.deepEqual(buttons.map(b => b.style.left), positions);
    assert.equal(slider.valueAsNumber, .56);
    ui.setBusy(true); assert.equal(element('rate-controls').disabled, true);
    custom.listeners.click(); assert.equal(custom.attributes['aria-pressed'], 'false');
    ui.setBusy(false); assert.equal(element('rate-controls').disabled, false);
});

test('drag and keyboard select rates on the fixed scale in both modes', async t => {
    const { element } = await setup(t, {
        PlaybackSettings: async () => ({ mode: 'live', mod: '', rate: 1, pitch: true }),
        SavePlayback: async () => {},
    });
    const slider = element('rate-slider'), rate = element('time-rate'), custom = element('rate-custom');
    const key = key => slider.listeners.keydown({ key, preventDefault() {} });
    for (const [position, value] of [[0, 1], [.42, 1.1], [.56, 1.25], [1, 1.5]]) {
        slider.value = position; slider.listeners.input();
        assert.equal(rate.valueAsNumber, value);
    }
    key('Home'); assert.equal(rate.valueAsNumber, 1);
    key('ArrowRight'); assert.equal(rate.valueAsNumber, 1.1);
    key('ArrowRight'); assert.equal(rate.valueAsNumber, 1.25);
    key('ArrowLeft'); assert.equal(rate.valueAsNumber, 1.1);
    custom.listeners.click();
    key('ArrowRight'); assert.equal(rate.valueAsNumber, 1.11);
    key('Home'); assert.equal(rate.valueAsNumber, .5);
    key('ArrowLeft'); assert.equal(rate.valueAsNumber, .5);
    key('End'); assert.equal(rate.valueAsNumber, 2);
    key('ArrowRight'); assert.equal(rate.valueAsNumber, 2);
    for (const [position, value] of [[.15, .75], [.42, 1.1], [.56, 1.25], [.875, 1.75]]) {
        slider.value = position; slider.listeners.input();
        assert.equal(rate.valueAsNumber, value);
        assert.equal(slider.attributes['aria-valuenow'], String(value));
    }
    await tick();
});

test('a stored custom rate is restored without rounding or a new save', async t => {
    const { element, buttons } = await setup(t, {
        PlaybackSettings: async () => ({ mode: 'auto', mod: 'mirror', rate: .876, pitch: false }),
        SavePlayback: async () => assert.fail('restoring must not save'),
    });
    assert.equal(element('time-rate').valueAsNumber, .876);
    assert.equal(element('rate-custom').attributes['aria-pressed'], 'true');
    assert.equal(element('rate-input').hidden, false);
    assert.equal(element('rate-value').textContent, '0.876×');
    assert.equal(buttons[0].style.left, 'calc((100% - 50px) * 0.3 + 25px)');
});

test('O2Jam cycles easy, normal, hard, off on one button and pills follow it', async t => {
    const saved = [];
    const { element, groups } = await setup(t, {
        PlaybackSettings: async () => ({ mode: 'live', mod: '', rate: 1, pitch: true, health: 3, healthFail: 0, o2jam: 'hard', pills: true }),
        SavePlayback: async p => saved.push(p),
    });
    const o2 = element('mod-o2jam'), pills = element('mod-pills');
    assert.equal(o2.querySelector().textContent, 'HARD');
    assert.equal(pills.attributes['aria-pressed'], 'true');
    assert.equal(groups['health-buttons'].every(b => b.disabled), true);
    o2.listeners.click(); await tick();
    assert.equal(saved.at(-1).o2jam, undefined);
    assert.equal(pills.disabled, true);
    assert.equal(groups['health-buttons'].some(b => b.disabled), false);
    for (const level of ['easy', 'normal', 'hard']) {
        o2.listeners.click(); await tick();
        assert.equal(saved.at(-1).o2jam, level);
    }
    pills.listeners.click(); await tick();
    assert.equal(saved.at(-1).pills, false);
    assert.equal(element('o2jam-value').textContent, 'HARD');
});

test('health policies restore, clamp to the starting stage and keep off selectable', async t => {
    const saved = [];
    const { element, groups } = await setup(t, {
        PlaybackSettings: async () => ({ mode: 'live', mod: '', rate: 1, pitch: true, health: 3, healthFail: 2 }),
        SavePlayback: async p => saved.push(p),
    });
    const starts = groups['health-buttons'], failures = groups['health-fail-buttons'];
    assert.equal(starts[3].attributes['aria-pressed'], 'true');
    assert.equal(failures[2].attributes['aria-pressed'], 'true');
    starts[1].listeners.click(); await tick();
    assert.equal(saved.at(-1).health, 1);
    assert.equal(saved.at(-1).healthFail, 1);
    assert.equal(failures[2].disabled, true);
    assert.equal(failures[1].attributes['aria-pressed'], 'true');
    failures[2].listeners.click(); await tick();
    assert.equal(saved.at(-1).healthFail, 1);
    failures[0].listeners.click(); await tick();
    assert.equal(saved.at(-1).healthFail, 0);
    starts[0].listeners.click(); await tick();
    assert.equal(saved.at(-1).health, 0);
    assert.equal(failures.every(b => b.disabled), true);
    starts[3].listeners.click(); await tick();
    failures[3].listeners.click(); await tick();
    assert.equal(saved.at(-1).health, 3);
    assert.equal(saved.at(-1).healthFail, 3);
    assert.equal(element('health-fail').value, '3');
});
