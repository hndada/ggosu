import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const source = readFileSync(new URL('./offsets.js', import.meta.url), 'utf8')
    .replace("'../../preferences/language.js'", JSON.stringify(new URL('../../preferences/language.js', import.meta.url).href));
let version = 0;
const turn = () => new Promise(resolve => setImmediate(resolve));
function deferred() {
    let resolve, reject;
    const promise = new Promise((ok, fail) => { resolve = ok; reject = fail; });
    return { promise, resolve, reject };
}
async function setup(t, app) {
    const previous = { document: globalThis.document, window: globalThis.window };
    t.after(() => { globalThis.document = previous.document; globalThis.window = previous.window; });
    const elements = new Map();
    function element(selector) {
        if (!elements.has(selector)) elements.set(selector, {
            value: 0, hidden: true, disabled: false, textContent: '', listeners: {},
            get valueAsNumber() { return Number(this.value); },
            reportValidity() { return true; },
            addEventListener(name, fn) { this.listeners[name] = fn; },
        });
        return elements.get(selector);
    }
    globalThis.document = { querySelector: element };
    globalThis.window = { go: { main: { App: app } } };
    const offsets = await import('data:text/javascript;base64,' + Buffer.from(source + '\n//' + ++version).toString('base64'));
    const pending = [];
    let busy = false;
    const form = offsets.mount({
        selection: () => ({ chartID: 'selected', index: 0 }),
        busy: () => busy,
        update: () => offsets.setBusy(busy, true),
        setPending: value => { busy = value; pending.push(value); offsets.setBusy(busy, true); },
    });
    return { offsets, form, element, pending };
}

test('offset changes block play until save completes, then release the parent pending state', async t => {
    const saving = deferred();
    let request;
    const ui = await setup(t, {
        Offsets: async () => ({ globalMS: 10, chartMS: 20 }),
        SaveOffsets: (s, o) => { request = { s, o }; return saving.promise; },
    });
    await ui.form.load();
    assert.equal(ui.offsets.needsSave(), false);
    ui.element('#global-offset').value = 30;
    ui.element('#global-offset').listeners.input();
    assert.equal(ui.offsets.editing(), true);
    assert.equal(ui.offsets.needsSave(), true);
    ui.element('#offsets').listeners.submit({ preventDefault() {} });
    assert.deepEqual(ui.pending, [true]);
    assert.equal(ui.element('#global-offset').disabled, true);
    assert.deepEqual(request, { s: { chartID: 'selected', index: 0 }, o: { globalMS: 30, chartMS: 20, judgeMS: 0 } });
    saving.resolve(request.o);
    await turn();
    assert.deepEqual(ui.pending, [true, false]);
    assert.equal(ui.offsets.editing(), false);
    assert.equal(ui.offsets.needsSave(), false);
    assert.equal(ui.element('#global-offset').disabled, false);
});

test('number edits commit on blur and Enter without depending on a native change event', async t => {
    const calls = [];
    const ui = await setup(t, {
        Offsets: async () => ({ globalMS: 0, chartMS: 0 }),
        SaveOffsets: async (s, o) => { calls.push(o); return o; },
    });
    await ui.form.load();
    ui.element('#global-offset').value = 20;
    ui.element('#global-offset').listeners.input();
    ui.element('#global-offset').listeners.blur();
    await turn();
    assert.equal(ui.offsets.needsSave(), false);
    assert.deepEqual(calls, [{ globalMS: 20, chartMS: 0, judgeMS: 0 }]);
    ui.element('#chart-offset').value = -5;
    ui.element('#chart-offset').listeners.input();
    ui.element('#chart-offset').listeners.keydown({ key: 'Enter', preventDefault() {} });
    await turn();
    ui.element('#chart-offset').listeners.blur();
    assert.equal(calls.length, 2);
    assert.deepEqual(calls[1], { globalMS: 20, chartMS: -5, judgeMS: 0 });
});

test('an older offset request cannot overwrite the latest selected chart', async t => {
    const old = deferred(), latest = deferred();
    let calls = 0;
    const ui = await setup(t, { Offsets: () => ++calls === 1 ? old.promise : latest.promise });
    const first = ui.form.load(), second = ui.form.load();
    latest.resolve({ globalMS: 1, chartMS: 2 });
    await second;
    old.resolve({ globalMS: 90, chartMS: 99 });
    await first;
    assert.equal(ui.element('#global-offset').valueAsNumber, 1);
    assert.equal(ui.element('#chart-offset').valueAsNumber, 2);
    assert.equal(ui.offsets.needsSave(), false);
});

test('failed save releases the pending state and preserves the unsaved values', async t => {
    const ui = await setup(t, {
        Offsets: async () => ({ globalMS: 0, chartMS: 0 }),
        SaveOffsets: async () => { throw new Error('disk unavailable'); },
    });
    await ui.form.load();
    ui.element('#chart-offset').value = 15;
    ui.element('#offsets').listeners.submit({ preventDefault() {} });
    await turn();
    assert.deepEqual(ui.pending, [true, false]);
    assert.equal(ui.offsets.needsSave(), true);
    assert.equal(ui.element('#chart-offset').valueAsNumber, 15);
    assert.match(ui.element('#offset-status').textContent, /disk unavailable/);
});

test('the browser bridge without offset settings remains playable', async t => {
    const ui = await setup(t, {});
    await ui.form.load();
    assert.equal(ui.offsets.needsSave(), false);
    assert.equal(ui.offsets.editing(), false);
    assert.equal(ui.element('#offsets').hidden, true);
});
