import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const source = readFileSync(new URL('./medley.js', import.meta.url), 'utf8')
    .replace("'../../preferences/language.js'", JSON.stringify(new URL('../../preferences/language.js', import.meta.url).href));
const tick = () => new Promise(resolve => setImmediate(resolve));
let version = 0;
const chart = (title, extra = {}) => ({ path: `${title}/chart.osu`, folder: title, index: 0, title, artist: 'Artist', difficulty: 'Hard', lanes: 7, rating: 12, length: 120, ...extra });

async function setup(t, rows, draw = async () => rows.slice(0, 3)) {
    const old = { document: globalThis.document, window: globalThis.window };
    t.after(() => Object.assign(globalThis, old));
    const fields = new Map(), state = { rows, busy: false, error: '', launches: [], focused: null };
    function node(tag = 'div') {
        return {
            tag, children: [], listeners: {}, dataset: {}, attributes: {}, value: '', textContent: '', disabled: false, hidden: false,
            get valueAsNumber() { return this.value === '' ? NaN : Number(this.value); },
            append(...nodes) { this.children.push(...nodes); },
            replaceChildren(...nodes) { this.children = nodes; },
            addEventListener(k, fn) { this.listeners[k] = fn; },
            setAttribute(k, v) { this.attributes[k] = v; },
            focus() { state.focused = this; },
            matches(selector) { return selector.split(', ').includes(this.tag); },
            querySelector(selector) {
                const index = selector.match(/data-index="(\d+)"/)?.[1];
                for (const child of this.children) {
                    if (index != null ? child.dataset?.index === index : child.tag === 'button') return child;
                    const found = child.querySelector?.(selector);
                    if (found) return found;
                }
                return null;
            },
        };
    }
    const $ = selector => {
        if (selector === 'dialog[open]') return null;
        if (!fields.has(selector)) fields.set(selector, node());
        return fields.get(selector);
    };
    const keyboard = {};
    globalThis.document = { querySelector: $, createElement: node, createTextNode: text => ({ textContent: text }), addEventListener: (k, fn) => { keyboard[k] = fn; } };
    globalThis.window = { go: { main: { App: { MedleyPick: draw } } } };
    $('#medley-low').value = '1'; $('#medley-high').value = '99';
    const ui = await import('data:text/javascript;base64,' + Buffer.from(source + `\n// ${version++}`).toString('base64'));
    ui.mount({ entries: () => state.rows, busy: () => state.busy, error: () => state.error, playOptions: () => ({ rate: 1.25, mod: 'mirror' }), launch: s => state.launches.push(s), back() {} });
    ui.open();
    function click(button) { if (!button.disabled && button.attributes['aria-disabled'] !== 'true') button.listeners.click(); }
    function pick(title, difficulty = 'Hard') {
        const row = $('#medley-candidates').children.find(li => li.attributes['aria-label'].includes(`${title} —`) && li.attributes['aria-label'].includes(`[${difficulty}]`));
        assert.ok(row, `missing ${title} ${difficulty}`);
        click(row);
    }
    return { $, ui, state, keyboard, click, pick };
}

test('manual picks enforce three distinct songs and replace a selected difficulty', async t => {
    const a = chart('A'), hard = chart('A', { path: 'A/extra.osu', difficulty: 'Extra' });
    const { $, state, click, pick } = await setup(t, [a, hard, chart('B'), chart('C'), chart('D')]);
    assert.equal($('#medley-play').disabled, true);
    pick('A'); pick('B'); pick('C'); pick('D');
    assert.equal($('#medley-songs').children.length, 3);
    pick('A', 'Extra');
    click($('#medley-play'));
    assert.deepEqual(state.launches[0].medley.map(e => e.chartID), ['A/extra.osu', 'B/chart.osu', 'C/chart.osu']);
    assert.equal(state.launches[0].rate, 1.25);
    assert.equal(state.launches[0].mod, 'mirror');
    pick('B'); pick('D');
    click($('#medley-play'));
    assert.deepEqual(state.launches[1].medley.map(e => e.folder), ['A', 'C', 'D']);
    click($('#medley-clear'));
    assert.equal($('#medley-play').disabled, true);
});

test('O2Jam files in one folder are separate songs, their difficulties still replace', async t => {
    const rows = [chart('A', { path: 'o2jam/a.ojn', folder: 'o2jam' }), chart('B', { path: 'o2jam/b.ojn', folder: 'o2jam' }), chart('A', { path: 'o2jam/a.ojn', folder: 'o2jam', index: 1, difficulty: 'Extra' })];
    const { $, state, click, pick } = await setup(t, rows);
    pick('A'); pick('B'); pick('A', 'Extra'); click($('#medley-play'));
    assert.equal(state.launches[0].medley.length, 2);
    assert.equal(state.launches[0].medley[0].index, 1);
});

test('candidate filters match random eligibility; search and polling preserve manual picks and focus', async t => {
    const rows = [chart('A'), chart('B', { rating: 20 }), chart('Short', { length: 59 }), chart('Unknown', { rating: 0 }), chart('4K', { lanes: 4 })];
    const { $, ui, state, click, pick } = await setup(t, rows);
    assert.equal($('#medley-candidates').children.length, 2);
    pick('A');
    const focus = state.focused;
    ui.update(); ui.update();
    assert.equal(state.focused, focus);
    assert.ok($('#medley-candidates').children.some(li => li === focus));
    $('#medley-search').value = 'B'; $('#medley-search').listeners.input();
    assert.equal($('#medley-candidates').children.length, 1);
    click($('#medley-play'));
    assert.equal(state.launches[0].medley[0].folder, 'A');
    $('#medley-low').value = '15'; $('#medley-low').listeners.change();
    assert.equal($('#medley-songs').children.length, 0);
    assert.equal($('#medley-play').disabled, true);
    $('#medley-high').value = '10'; $('#medley-high').listeners.change();
    assert.match($('#medley-status').textContent, /Lv 범위/);
});

test('random draw replaces manual picks, blocks edits while pending and preserves picks on failure', async t => {
    const rows = [chart('A'), chart('B'), chart('C')];
    let resolve, fail = false, request;
    const { $, state, click, pick } = await setup(t, rows, r => {
        request = r;
        if (fail) return Promise.reject(new Error('추첨 실패'));
        return new Promise(done => { resolve = done; });
    });
    pick('A'); click($('#medley-draw'));
    assert.equal($('#medley-play').disabled, true);
    assert.equal($('#medley-low').disabled, true);
    pick('C');
    assert.equal($('#medley-songs').children.length, 1);
    resolve([rows[1], rows[2]]); await tick();
    assert.deepEqual(request, { lanes: 7, low: 1, high: 99 });
    click($('#medley-play'));
    assert.deepEqual(state.launches[0].medley.map(e => e.folder), ['B', 'C']);
    fail = true; click($('#medley-draw')); await tick();
    assert.equal($('#medley-songs').children.length, 2);
    assert.equal($('#medley-status').textContent, '추첨 실패');
    state.busy = true;
    // Even a keyboard-triggered play must obey the live busy state.
    click($('#medley-play'));
    assert.equal(state.launches.length, 1);
});

test('paging reaches every chart and native button Enter does not launch the game', async t => {
    const rows = Array.from({ length: 105 }, (_, i) => chart(`Song ${i}`));
    const { $, ui, state, click, pick, keyboard } = await setup(t, rows);
    assert.equal($('#medley-candidates').children.length, 100);
    click($('#medley-next'));
    assert.equal($('#medley-candidates').children.length, 5);
    pick('Song 104');
    let prevented = false;
    const button = document.createElement('button');
    keyboard.keydown({ key: 'Enter', target: button, preventDefault() { prevented = true; } });
    assert.equal(prevented, false);
    assert.equal(state.launches.length, 0);
    keyboard.keydown({ key: 'Enter', ctrlKey: true, target: button, preventDefault() {} });
    assert.equal(state.launches[0].mode, 'auto');
    state.rows = rows.slice(0, 100); ui.update();
    assert.equal($('#medley-play').disabled, true);
});

test('keyboard arrows navigate rows, Space toggles picks, Enter launches and Auto remains available', async t => {
    const { $, state, click, keyboard } = await setup(t, [chart('A'), chart('B'), chart('C')]);
    const key = (name, target = state.focused) => keyboard.keydown({ key: name, target, preventDefault() {} });
    key('ArrowDown', $('#medley-search'));
    assert.equal(state.focused.dataset.index, '1');
    key(' ');
    assert.equal($('#medley-songs').children.length, 1);
    assert.equal($('#medley-candidates').children[1].attributes['aria-selected'], 'true');
    key('ArrowDown'); key(' '); key('Enter');
    assert.deepEqual(state.launches[0].medley.map(e => e.folder), ['B', 'C']);
    click($('#medley-auto'));
    assert.equal(state.launches[1].mode, 'auto');
    key(' ');
    assert.equal($('#medley-songs').children.length, 1);
    keyboard.keydown({ key: 'Backspace', ctrlKey: true, target: state.focused, preventDefault() {} });
    assert.equal($('#medley-songs').children.length, 0);
});

test('selected song rows remove with click or keyboard without launching', async t => {
    const { $, state, click, pick } = await setup(t, [chart('A'), chart('B')]);
    pick('A'); pick('B');
    click($('#medley-songs').children[0]);
    assert.equal($('#medley-songs').children.length, 1);
    let stopped = false;
    $('#medley-songs').children[0].listeners.keydown({ key: 'Enter', preventDefault() {}, stopPropagation() { stopped = true; } });
    assert.equal($('#medley-play').disabled, true);
    assert.equal(state.launches.length, 0);
    assert.equal(stopped, true);
});
