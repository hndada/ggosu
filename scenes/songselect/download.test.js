import test from 'node:test';
import assert from 'node:assert/strict';
import { mount } from './download.js';

const tick = () => new Promise(resolve => setImmediate(resolve));
const pack = (id, extra = {}) => ({ key: `osu:osu:${id}`, source: 'osu', id, title: `Song ${id}`, artist: 'Artist', keys: [4], available: true, installed: false, ...extra });
function setup(t, api) {
    const old = { document: globalThis.document, window: globalThis.window, setTimeout: globalThis.setTimeout, clearTimeout: globalThis.clearTimeout };
    t.after(() => Object.assign(globalThis, old));
    class Element {
        constructor(tag) { this.tag = tag; this.children = []; this.listeners = {}; this.value = ''; this.dataset = {}; this.disabled = false; this.hidden = false; }
        append(...children) { this.children.push(...children); }
        replaceChildren(...children) { this.children = children; }
        addEventListener(k, fn) { this.listeners[k] = fn; }
        querySelectorAll(tag) { return this.children.flatMap(c => [...(c.tag === tag ? [c] : []), ...c.querySelectorAll(tag)]); }
        removeAttribute() {}
        focus() { document.activeElement = this; }
        showModal() { this.open = true; }
        close() { this.open = false; }
    }
    const elements = new Map(), timers = new Map(); let timerID = 0;
    const $ = s => { if (!elements.has(s)) elements.set(s, new Element('div')); return elements.get(s); };
    globalThis.document = { querySelector: $, createElement: tag => new Element(tag) };
    $('#download-catalog').value = 'osu'; $('#download-keys').value = '0';
    globalThis.window = { go: { main: { App: api } }, addEventListener() {} };
    globalThis.setTimeout = fn => { timers.set(++timerID, fn); return timerID; };
    globalThis.clearTimeout = id => timers.delete(id);
    const installed = [], ui = mount({ busy: () => false, text: () => 'Search', installed: async r => installed.push(r) });
    return { $, ui, installed, buttons: () => $('#download-list').querySelectorAll('button'), poll: async () => { const fn = [...timers.values()][0]; timers.clear(); fn?.(); await tick(); } };
}

test('search selection downloads, reports progress and reloads/selects the installed result once', async t => {
    let state = { phase: 'idle' }, starts = 0;
    const result = { source: 'osu', id: '1', title: 'Song 1', charts: 2, hashes: ['hash'] };
    const f = setup(t, {
        DownloadInfo: async () => ({ archive: 'http://server', canInstall: true, canConfigure: true }),
        SearchSongs: async q => { assert.equal(q.text, 'Search'); return { items: [pack('1')], offset: 0, more: false, total: -1 }; },
        StartInstall: async key => { starts++; assert.equal(key, 'osu:osu:1'); return state = { phase: 'download', received: 100, total: 1000, title: 'Song 1' }; },
        InstallState: async () => state,
    });
    await f.ui.open(); await tick();
    assert.equal(f.buttons()[0].disabled, false);
    f.buttons()[0].listeners.click(); f.buttons()[0].listeners.click(); await tick();
    assert.equal(starts, 1);
    assert.equal(f.$('#download-progress').value, 100);
    state = { phase: 'done', result };
    await f.poll();
    assert.deepEqual(f.installed, [result]);
    assert.equal(f.buttons()[0].textContent, '설치됨');
    assert.equal(f.buttons()[0].disabled, true);
    assert.match(f.$('#download-status').textContent, /설치 완료/);
});

test('unavailable, already-installed and remote-browser packages cannot start installation', async t => {
    const f = setup(t, {
        DownloadInfo: async () => ({ archive: 'http://server', canInstall: false, canConfigure: false }),
        SearchSongs: async () => ({ items: [pack('1'), pack('2', { available: false, reason: '미보관' }), pack('3', { installed: true })], more: false, total: 3 }),
        StartInstall: async () => { throw new Error('must not install'); },
    });
    await f.ui.open(); await tick();
    assert.ok(f.buttons().every(b => b.disabled));
    assert.equal(f.$('#download-limit').hidden, false);
    assert.equal(f.$('#download-server').readOnly, true);
});

test('newer search wins when an earlier response arrives late', async t => {
    let resolveFirst, n = 0;
    const first = new Promise(resolve => { resolveFirst = resolve; });
    const f = setup(t, {
        DownloadInfo: async () => ({ canInstall: true, archive: 'http://server' }), InstallState: async () => ({ phase: 'idle' }),
        SearchSongs: async () => ++n === 1 ? first : { items: [pack('new')], more: false, total: -1 },
    });
    await f.ui.open();
    f.$('#download-query').value = 'New'; f.$('#download-form').listeners.submit({ preventDefault() {} }); await tick();
    resolveFirst({ items: [pack('old')], more: false, total: -1 }); await tick();
    assert.match(f.$('#download-list').children[0].children[0].textContent, /Song new/);
});

test('failed and cancelled installs leave the list usable and never report installation', async t => {
    let state = { phase: 'idle' };
    const f = setup(t, {
        DownloadInfo: async () => ({ canInstall: true, archive: 'http://server' }),
        SearchSongs: async () => ({ items: [pack('1')], more: false, total: 1 }),
        StartInstall: async () => state = { phase: 'download', received: 0, total: 10 }, InstallState: async () => state,
        CancelInstall: async () => { state = { phase: 'cancelled', error: '취소했습니다' }; },
    });
    await f.ui.open(); await tick(); f.buttons()[0].listeners.click(); await tick();
    await f.$('#download-cancel').listeners.click(); await tick();
    assert.equal(f.buttons()[0].disabled, false);
    assert.equal(f.installed.length, 0);
    assert.match(f.$('#download-status').textContent, /취소/);
});
