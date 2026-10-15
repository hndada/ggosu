import { tr } from './language.js';
// A browser outbox keeps only delivery metadata; replay data stays in the local record.
const prefix = 'ggosu:submission:';
export function eligible(r) { return r?.mode === 'live' && r.version === 2 && r.replay?.rules === 'ggosu-keys-v3' && (r.completed && r.score?.Points > 0 || r.health?.failed === true); }

export function createSubmissions({ storage, server, read, send, changed = () => {} }) {
    let running;
    const status = id => {
        const text = storage.getItem(prefix + id);
        return text === null ? null : JSON.parse(text).status;
    };
    const save = (id, job) => { storage.setItem(prefix + id, JSON.stringify(job)); changed(id, job.status); };
    const flush = () => {
        if (running) return running;
        running = (async () => {
            const keys = Array.from({ length: storage.length }, (_, i) => storage.key(i)).filter(k => k?.startsWith(prefix));
            for (const key of keys) {
                if (!server()) break;
                const id = key.slice(prefix.length);
                let job;
                try {
                    job = JSON.parse(storage.getItem(key));
                    if (!job || job.status.state === 'sent') continue;
                    const r = await read(id);
                    if (!eligible(r)) throw new Error(tr('제출할 일반 플레이 기록이 없습니다.'));
                    const receipt = await send(job.server, { player: job.player, record: r });
                    if (receipt?.id !== id || receipt.verified !== true || !Number.isFinite(Date.parse(receipt.received))) throw new Error(tr('서버 제출 확인 정보가 기록과 다릅니다.'));
                    job.status = { state: 'sent', receipt };
                } catch (e) {
                    if (!job) { changed(id, { state: 'failed', error: tr`대기열 읽기 실패: ${e.message}` }); continue; }
                    job.status = { state: 'failed', error: e.message };
                }
                const latest = JSON.parse(storage.getItem(key));
                if (latest?.revision !== job.revision) continue;
                try { save(id, job); }
                catch (e) { changed(id, { state: 'failed', error: tr`제출 상태 저장 실패: ${e.message}` }); }
            }
        })().finally(() => { running = null; });
        return running;
    };
    const enqueue = (r, player = 'Player') => {
        if (!eligible(r)) return null;
        const url = server();
        if (!url) return { state: 'disabled' };
        const oldText = storage.getItem(prefix + r.id), old = oldText && JSON.parse(oldText);
        if (old?.status.state === 'sent' && old.server === url) return old.status;
        const job = { server: url, player, revision: (old?.revision ?? 0) + 1, status: { state: 'pending' } };
        save(r.id, job);
        // A play can finish while another request is in flight. Run a fresh pass
        // afterwards to include the newly enqueued record without busy polling.
        const previous = running;
        if (previous) void previous.then(flush).catch(e => changed(r.id, { state: 'failed', error: e.message }));
        else void flush().catch(e => changed(r.id, { state: 'failed', error: e.message }));
        return job.status;
    };
    return { enqueue, flush, status };
}

export async function sendScore(server, q, fetcher = globalThis.fetch) {
    const controller = new AbortController(), timer = setTimeout(() => controller.abort(), 10000);
    try {
        const body = JSON.stringify(q);
        if (new TextEncoder().encode(body).length > 32 * 1024 * 1024) throw new Error(tr('리플레이가 32 MiB를 넘습니다.'));
        const r = await fetcher(server.replace(/\/$/, '') + '/api/scores', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Gosu-Score': '1' }, body, signal: controller.signal, redirect: 'error' });
        if (!r.ok) throw new Error(tr`점수 제출 HTTP ${r.status}: ${(await r.text()).slice(0, 4096)}`);
        return await r.json();
    } finally { clearTimeout(timer); }
}
