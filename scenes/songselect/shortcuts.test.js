import test from 'node:test';
import assert from 'node:assert/strict';
import { mount, settings, mods } from './shortcuts.js';

function setup(t) {
    const saved = Object.fromEntries(['document', 'HTMLInputElement', 'HTMLSelectElement', 'HTMLTextAreaElement', 'HTMLButtonElement'].map(k => [k, globalThis[k]]));
    t.after(() => Object.assign(globalThis, saved));
    class Field { value = ''; blur() {} focus() {} closest() { return null; } }
    globalThis.HTMLInputElement = globalThis.HTMLSelectElement = globalThis.HTMLTextAreaElement = Field;
    globalThis.HTMLButtonElement = class {};
    const handlers = [], fields = new Map();
    const field = id => { if (!fields.has(id)) fields.set(id, { hidden: true, open: false, value: '', clicks: 0, click() { this.clicks++; } }); return fields.get(id); };
    field('#songselect').hidden = false;
    const state = { dialog: null, background: false };
    globalThis.document = {
        body: { classList: { contains: () => state.background } },
        querySelector: id => id === 'dialog[open]' ? state.dialog : field(id),
        addEventListener: (name, fn, capture) => handlers.push({ fn, capture: !!capture }),
    };
    const search = new Field();
    const key = (key, extra = {}) => {
        const e = { key, target: search, preventDefault() { this.defaultPrevented = true; }, stopImmediatePropagation() { this.stopped = true; }, ...extra };
        for (const h of [...handlers].sort((a, b) => Number(b.capture) - Number(a.capture))) { h.fn(e); if (e.stopped) break; }
        return e;
    };
    return { field, state, search, key };
}

test('Auto, scroll and settings chords route without taking text editing or repeating toggles', t => {
    const { search, key } = setup(t);
    const calls = [];
    mount(['Ctrl+A', 'Ctrl++', 'Ctrl+=', 'Ctrl+-', 'Ctrl+F3', 'Ctrl+F4', 'F2', 'ArrowDown'].map(key => ({ key, run: () => calls.push(key) })), search);
    key('a', { ctrlKey: true });
    search.value = 'artist';
    assert.ok(!key('a', { ctrlKey: true }).defaultPrevented);
    assert.ok(!key('Backspace', { ctrlKey: true }).defaultPrevented);
    key('+', { ctrlKey: true, shiftKey: true }); key('=', { ctrlKey: true });
    key('-', { ctrlKey: true }); key('F3', { ctrlKey: true }); key('F4', { ctrlKey: true });
    key('F2'); key('F2', { repeat: true }); key('ArrowDown', { repeat: true });
    key('F2', { isComposing: true }); key('F2', { defaultPrevented: true });
    assert.deepEqual(calls, ['Ctrl+A', 'Ctrl++', 'Ctrl+=', 'Ctrl+-', 'Ctrl+F3', 'Ctrl+F4', 'F2', 'ArrowDown']);
});

test('Ctrl+O toggles settings and leaves other dialogs, editor and active play alone', t => {
    const { field, state, key } = setup(t);
    let opens = 0, closes = 0, busy = false;
    settings(() => opens++, () => closes++, () => busy);
    assert.ok(key('o', { ctrlKey: true }).defaultPrevented);
    key('o', { ctrlKey: true, repeat: true });
    state.dialog = field('#settings'); key('o', { metaKey: true });
    state.dialog = {}; key('o', { ctrlKey: true });
    state.dialog = null; field('#editor').hidden = false; key('o', { ctrlKey: true });
    field('#editor').hidden = true; busy = true; key('o', { ctrlKey: true });
    assert.deepEqual([opens, closes], [1, 1]);
});

test('mod shortcuts use existing buttons and leave input and IME keys alone', t => {
    const { field, key } = setup(t);
    field('#mods-dialog').open = true;
    mods();
    key('x'); key('g'); key('v'); key('f');
    field('#visibility-mod').value = 'fadein'; key('f');
    field('#visibility-mod').value = 'hidden'; key('f');
    key('x', { repeat: true }); key('x', { isComposing: true });
    key('x', { target: { closest: () => ({}) } });
    assert.equal(field('#arrangement-buttons [data-mod="random"]').clicks, 1);
    assert.equal(field('#visibility-buttons [data-visibility="fadein"]').clicks, 1);
    assert.equal(field('#visibility-buttons [data-visibility="hidden"]').clicks, 2);
    assert.equal(field('#visibility-buttons [data-visibility="flashlight"]').clicks, 1);
    assert.equal(field('#mode-buttons [data-value="auto"]').clicks, 1);
});
