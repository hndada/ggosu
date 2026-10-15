import test from 'node:test';
import assert from 'node:assert/strict';
import { createSubmissions, eligible, sendScore } from './submission.js';

const play = id => ({ id, version: 2, mode: 'live', completed: true, score: { Points: 1100000 }, replay: { rules: 'ggosu-keys-v3', mods: { visibility: 'hidden' } } });
const receipt = id => ({ id, verified: true, received: '2026-10-13T12:00:00Z' });
function storage() {
    const data = new Map();
    return { get length() { return data.size; }, key: i => [...data.keys()][i] ?? null, getItem: k => data.get(k) ?? null, setItem: (k, v) => data.set(k, v) };
}

test('failed submissions preserve records, survive reload, retry, and do not duplicate accepted scores', async () => {
    const local = storage(), r = play('a');
    const records = new Map([[r.id, r]]);
    let calls = 0, online = false;
    const options = { storage: local, server: () => 'http://server', read: async id => records.get(id), send: async (server, q) => { calls++; assert.equal(q.record.replay.mods.visibility, 'hidden'); if (!online) throw new Error('offline'); return receipt(q.record.id); } };
    const first = createSubmissions(options);
    assert.equal(first.enqueue(r).state, 'pending');
    await first.flush();
    assert.equal(first.status('a').state, 'failed');
    assert.equal(records.get('a'), r);
    online = true;
    const afterReload = createSubmissions(options);
    await afterReload.flush();
    assert.equal(afterReload.status('a').state, 'sent');
    assert.equal(afterReload.enqueue(r).state, 'sent');
    await afterReload.flush();
    assert.equal(calls, 2);
});

test('Auto, replay, canceled and legacy plays do not submit; failures and rate/mod plays do', async () => {
    const r = play('a');
    for (const changes of [{ mode: 'auto' }, { mode: 'replay' }, { completed: false }, { version: 1 }, { replay: { rules: 'old' } }]) assert.equal(eligible({ ...r, ...changes }), false);
    assert.equal(eligible({ ...r, completed: false, score: { Points: 0 }, health: { failed: true } }), true);
    assert.equal(eligible({ ...r, practice: true, rate: 2 }), true);
    const outbox = createSubmissions({ storage: storage(), server: () => '', read: async () => r, send: async () => { throw new Error('must not send'); } });
    assert.equal(outbox.enqueue(r).state, 'disabled');
    assert.equal(outbox.enqueue({ ...r, mode: 'auto' }), null);
    await outbox.flush();
});

test('a new play queued during an in-flight request is delivered without another user action', async () => {
    const local = storage(), records = new Map(['a', 'b'].map(id => [id, play(id)]));
    let release, finished;
    const wait = new Promise(resolve => { release = resolve; });
    const sent = new Promise(resolve => { finished = resolve; });
    const calls = [];
    const outbox = createSubmissions({ storage: local, server: () => 'http://server', read: async id => records.get(id),
        send: async (_, q) => { calls.push(q.record.id); if (q.record.id === 'a') await wait; return receipt(q.record.id); },
        changed(id, s) { if (id === 'b' && s.state === 'sent') finished(); },
    });
    outbox.enqueue(records.get('a'));
    await new Promise(resolve => setImmediate(resolve));
    outbox.enqueue(records.get('b'));
    release(); await sent;
    assert.deepEqual(calls, ['a', 'b']);
});

test('mismatched receipts and HTTP failures are surfaced, and the upload contains the replay', async () => {
    const r = play('a'), local = storage();
    const outbox = createSubmissions({ storage: local, server: () => 'http://server', read: async () => r, send: async () => receipt('wrong') });
    outbox.enqueue(r); await outbox.flush();
    assert.equal(outbox.status('a').state, 'failed');
    const got = await sendScore('http://server/', { player: 'Player', record: r }, async (url, opts) => {
        assert.equal(url, 'http://server/api/scores'); assert.equal(opts.method, 'POST'); assert.equal(opts.headers['X-Gosu-Score'], '1');
        assert.deepEqual(JSON.parse(opts.body).record.replay, r.replay);
        return { ok: true, json: async () => receipt('a') };
    });
    assert.equal(got.id, 'a');
    await assert.rejects(sendScore('http://server', { record: r }, async () => ({ ok: false, status: 422, text: async () => 'bad replay' })), /422.*bad replay/);
});
