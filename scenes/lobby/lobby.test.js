import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const source = readFileSync(new URL('./lobby.js', import.meta.url), 'utf8')
    .replace("'../../preferences/language.js'", JSON.stringify(new URL('../../preferences/language.js', import.meta.url).href))
    .replace(/import[^\n]*createChat[^\n]*\n/, `const createChat = () => Object.fromEntries(['setPlayer','setRoom','receive','history','ack','error','disconnected'].map(k => [k, () => {}]));\n`)
    .replace(/import[^\n]*mountVisibility[^\n]*\n/, `const mountMods = () => () => {}, mountVisibility = () => () => {};\n`)
    .replace(/import[^\n]*modsText[^\n]*\n/, `const modsText = () => '';\n`);
const { createLobby } = await import('data:text/javascript;base64,' + Buffer.from(source).toString('base64'));

async function setup(t) {
    const old = { document: globalThis.document, window: globalThis.window, setInterval: globalThis.setInterval };
    t.after(() => Object.assign(globalThis, old));
    class Node {
        constructor(tag) {
            this.tag = tag; this.children = []; this.nodes = new Map(); this.listeners = {}; this.open = false;
            this.value = ''; this.checked = true; this.disabled = false;
            this.elements = new Proxy({}, { get: (obj, key) => obj[key] ??= Object.assign(new Node('input'), { value: key === 'rate' ? '1' : '' }) });
        }
        get valueAsNumber() { return Number(this.value); }
        setAttribute() {}
        append(...nodes) { this.children.push(...nodes); }
        replaceChildren(...nodes) { this.children = nodes; }
        querySelector(key) { if (!this.nodes.has(key)) this.nodes.set(key, new Node(key.includes('form') ? 'form' : 'div')); return this.nodes.get(key); }
        querySelectorAll() { return []; }
        addEventListener(key, fn) { this.listeners[key] = fn; }
        focus() { this.focused = true; }
        show() { this.open = true; }
        showModal() { this.open = true; }
        close() { this.open = false; this.listeners.close?.(); }
    }
    let state = { connected: true, player_id: 'host', sequence: 1, events: [], rooms: [
        { id: 'private', name: 'Private', locked: true, players: 1, capacity: 4, phase: 'lobby' },
        { id: 'public', name: 'Public', locked: false, players: 1, capacity: 4, phase: 'lobby' },
    ] };
    const sent = [], body = new Node('body');
    let poll;
    globalThis.document = { body, createElement: tag => new Node(tag) };
    globalThis.window = { go: { main: { App: { MultiConnect: async () => {}, MultiState: async () => state, MultiSend: async q => sent.push(q) } } }, addEventListener() {} };
    globalThis.setInterval = fn => { poll = fn; return 1; };
    const lobby = createLobby({ entries: () => [], selection: () => null, stopPreview() {} });
    lobby.open(); await new Promise(resolve => setImmediate(resolve));
    return { panel: body.children[0], dialog: body.children[1], sent, poll: () => poll(), state: next => { state = { ...state, ...next }; } };
}

test('private room asks for password, clears it after send, keeps errors visible and closes on accepted join', async t => {
    const f = await setup(t), form = f.dialog.querySelector('form');
    f.panel.querySelector('#multi-rooms').children[0].children[0].onclick();
    assert.equal(f.dialog.open, true);
    assert.equal(f.sent.length, 0);
    form.elements.password.value = 'wrong';
    await form.onsubmit({ preventDefault() {} });
    assert.deepEqual({ ...f.sent[0], id: '' }, { type: 'join', id: '', room_id: 'private', password: 'wrong' });
    assert.equal(form.elements.password.value, '');
    assert.equal(f.dialog.querySelector('#multi-join-cancel').disabled, true);
    f.state({ sequence: 2, events: [{ event: { type: 'error', id: f.sent[0].id, error: '비밀번호가 일치하지 않습니다' } }] });
    await f.poll();
    assert.equal(f.dialog.open, true);
    assert.match(f.dialog.querySelector('#multi-join-status').textContent, /비밀번호/);
    assert.equal(f.dialog.querySelector('#multi-join-submit').disabled, false);
    form.elements.password.value = 'correct';
    await form.onsubmit({ preventDefault() {} });
    f.state({ sequence: 3, room: { id: 'private', name: 'Private', host_id: 'other', locked: true, phase: 'lobby', revision: 0, members: [] }, events: [{ event: { type: 'ack', id: f.sent[1].id } }] });
    await f.poll();
    assert.equal(f.dialog.open, false);
    assert.equal(form.elements.password.value, '');
    assert.match(f.panel.querySelector('#multi-title').textContent, /비밀번호 방/);
});

test('public room joins directly and creation sends an optional password without retaining the input', async t => {
    const f = await setup(t);
    f.panel.querySelector('#multi-rooms').children[1].children[0].onclick();
    await new Promise(resolve => setImmediate(resolve));
    assert.equal(f.dialog.open, false);
    assert.equal(f.sent[0].type, 'join');
    assert.equal(f.sent[0].room_id, 'public');
    assert.equal(f.sent[0].password, undefined);
    const form = f.panel.querySelector('#multi-create');
    form.elements.name.value = 'Private'; form.elements.capacity.value = '4'; form.elements.password.value = 'secret';
    form.onsubmit({ target: form, preventDefault() {} });
    await new Promise(resolve => setImmediate(resolve));
    assert.equal(f.sent[1].type, 'create');
    assert.equal(f.sent[1].password, 'secret');
    assert.equal(form.elements.password.value, '');
});
