import { tr } from '/preferences/language.js';
import { normalizeScore, summarize, playerStats } from '/songlist/analysis.js';
import { storedDisplay, saveDisplay } from '/preferences/display.js';
import { storedPlayback, savePlayback } from '/preferences/playback.js';
import { storedAudio, saveAudio } from '/preferences/audio.js';
import { createSubmissions, sendScore } from '/preferences/submission.js';
import { storedLanguage, saveLanguage } from '/preferences/language.js';
import { storedProfile, saveProfile, resetProfile, keepProfile, storedOffsets, saveOffsets, storedName, saveName } from '/preferences/profile.js';
const prefix = 'ggosu:record:';
let catalog = { entries: [], errors: [] };
let state = { running: false, result: null, error: '' };
let frame;
let ready = false;
let gamePaused = false, resumeIn = 0;
let latestJSON = '';
try { let date = 0; for (let i = 0; i < localStorage.length; i++) { const key = localStorage.key(i); if (!key.startsWith(prefix)) continue; const text = localStorage.getItem(key); try { const r = JSON.parse(text); const at = Date.parse(r.date); if (at > date) { latestJSON = text; date = at; } } catch { /* Keep other valid records available. */ } } } catch { /* Playback still works when storage is unavailable. */ }
const listeners = new Set();
const area = document.createElement('section'); area.id = 'web-play';
area.hidden = true;
const pause = document.createElement('button'); pause.type = 'button'; pause.textContent = tr('일시정지');
const resume = document.createElement('button'); resume.type = 'button'; resume.textContent = tr('재개');
const restart = document.createElement('button'); restart.type = 'button'; restart.textContent = tr('빠른 재시작');
pause.disabled = resume.disabled = restart.disabled = true;
const stop = document.createElement('button'); stop.type = 'button'; stop.textContent = tr('플레이 종료');
const note = document.createElement('p'); note.textContent = tr('게임 안의 시작 버튼을 눌러 주세요. 포커스를 잃으면 자동으로 정지합니다.');
note.id = 'playback-status'; note.setAttribute('role', 'status');
const toolbar = document.createElement('div'); toolbar.id = 'web-controls';
    toolbar.append(pause, resume, restart, stop);
    area.append(note, toolbar);
document.querySelector('main').append(area);
function changed() { for (const fn of listeners) fn(); }
function view(r) {
    return { submission: submissionStatus(r.id), health: r.health, pauses: r.replay.pauses ?? [], id: r.id, date: r.date, title: r.metadata.title, difficulty: r.metadata.difficulty, lanes: r.replay.lanes, mode: r.mode, completed: r.completed, practice: r.practice, rate: r.rate, pitch: r.pitch !== false, score: normalizeScore(r.score), accuracy: r.accuracy, warnings: r.warnings ?? [], offsets: r.replay.offsets ?? { globalMS: 0, chartMS: 0 }, mods: r.replay.mods ?? {}, scoreVersion: r.version, verification: r.verification };
}
const scoreServer = () => localStorage.getItem('ggosu:score-server') ?? location.origin;
function submissionRecord(id) {
    const text = localStorage.getItem(prefix + id);
    const r = text === null ? null : JSON.parse(text);
    if (r && r.id !== id) throw new Error(tr('기록 ID가 저장 위치와 다릅니다.'));
    return r;
}
const submissionStorage = { get length() { return localStorage.length; }, key: i => localStorage.key(i), getItem: k => localStorage.getItem(k), setItem: (k, v) => localStorage.setItem(k, v) };
const submissions = createSubmissions({ storage: submissionStorage, server: scoreServer, read: async id => submissionRecord(id), send: sendScore,
    changed(id, submission) {
        if (state.result?.id === id) state = { ...state, result: { ...state.result, submission } };
        changed();
    },
});
function submissionStatus(id) {
    try { return submissions.status(id); }
    catch (e) { return { state: 'failed', error: tr`제출 상태 읽기 실패: ${e.message}` }; }
}
function store(r) {
    if (!((r.completed && r.score?.Points > 0) || r.health?.failed)) throw new Error(tr('완주 또는 체력 탈락 기록만 저장할 수 있습니다.'));
    latestJSON = JSON.stringify(r); jsonText.hidden = true; localStorage.setItem(prefix + r.id, latestJSON); }
async function request(url, options) { const r = await fetch(url, options); if (!r.ok) { const error = new Error(await r.text()); error.status = r.status; throw error; } return r.json(); }
const checked = new Map();
function notice(text) { state.notices = [...new Set([...(state.notices || []), text])].slice(-64); }
async function validateRecord(text) { return request('/api/validate', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: text }); }
async function readSaved(key) {
    const text = localStorage.getItem(key);
    if (text === null) return null;
    if (checked.has(text)) return checked.get(text);
    const task = (async () => {
        try {
            const r = await validateRecord(text);
            if (key !== prefix + r.id) { const error = new Error(tr('기록 ID가 저장 위치와 다릅니다.')); error.status = 400; throw error; }
            if (r.verification === 'repaired') {
                localStorage.setItem('ggosu:quarantine:' + crypto.randomUUID(), text);
                localStorage.setItem(key, JSON.stringify(r));
                notice(tr('리플레이에서 기록을 복구했습니다. 원본은 별도로 보존했습니다.'));
                checked.set(JSON.stringify(r), Promise.resolve(r));
            }
            return r;
        } catch (error) {
            if (error.status !== 400) { checked.delete(text); throw error; }
            localStorage.setItem('ggosu:quarantine:' + crypto.randomUUID(), text);
            localStorage.removeItem(key);
            notice(tr('잘못된 기록을 격리했습니다: ') + error.message);
            return null;
        }
    })();
    checked.set(text, task);
    return task;
}
function recordKeys() { return Array.from({ length: localStorage.length }, (_, i) => localStorage.key(i)).filter(key => key?.startsWith(prefix)); }
async function savedRecords() { return (await Promise.all(recordKeys().map(readSaved))).filter(Boolean); }
function chartHash(s) { return catalog.entries.find(e => e.path === s.chartID && e.index === s.index)?.hash ?? ''; }
function send(type) { frame?.contentWindow.postMessage({ type: `gosu:${type}` }, location.origin); }
function playbackNote() { note.textContent = resumeIn > 0 ? tr`${resumeIn}초 후 재개합니다. 게임 시간과 음악은 정지 중입니다.` : gamePaused ? tr('정지했습니다. 재개를 누르거나 게임에 포커스를 주고 Esc를 누르세요.') : tr('플레이 중입니다.'); }
pause.addEventListener('click', () => send('pause'));
resume.addEventListener('click', () => { send('resume'); frame?.contentWindow.focus(); });
restart.addEventListener('click', () => { send('restart'); frame?.contentWindow.focus(); });
stop.addEventListener('click', () => {
    if (ready) send('stop');
    else { frame?.remove(); state = { ...state, running: false, error: tr('플레이 시작을 취소했습니다.') }; area.hidden = true; changed(); }
});
window.addEventListener('message', async e => {
    if (e.origin !== location.origin || e.source !== frame?.contentWindow) return;
    try {
        let data = JSON.parse(e.data.data);
        if (e.data.type === 'gosu:ready') { ready = true; pause.disabled = resume.disabled = restart.disabled = false; note.textContent = tr('플레이 중입니다. 게임 영역을 클릭해 키보드 포커스를 주세요.'); }
        if (e.data.type === 'gosu:paused') { gamePaused = data; playbackNote(); }
        if (e.data.type === 'gosu:display') window.dispatchEvent(new Event('ggosu:display-changed'));
        if (e.data.type === 'gosu:audio') window.dispatchEvent(new Event('ggosu:audio-changed'));
        if (e.data.type === 'gosu:profile') {
            try { keepProfile(data.lanes, data.profile); }
            catch (error) { notice(tr('설정 저장 실패: ') + error.message); }
        }
        if (e.data.type === 'gosu:countdown') { resumeIn = data; playbackNote(); }
        if (e.data.type === 'gosu:result') {
            // The Go runtime has recalculated this result. Save it locally before
            // any HTTP call; history/import still validate at their own boundary.
            // A 0-point play is not kept: no saved record and no result screen.
            const zero = !((data.completed && data.score?.Points > 0) || data.health?.failed);
            state = { ...state, running: false, result: zero ? null : view(data), error: '' };
            if (!zero) try { store(data); } catch (error) { state.result.saved = false; state.result.warnings.push(tr`브라우저 저장 실패: ${error.message}`); }
            if (!zero) try { state.result.submission = submissions.enqueue(data, data.replay.player || 'Player'); }
            catch (error) { state.result.submission = { state: 'failed', error: error.message }; }
            frame.remove(); area.hidden = true; changed();
        }
        // The HUD editor closes without a record.
        if (e.data.type === 'gosu:closed') { state = { ...state, running: false, error: '' }; frame.remove(); area.hidden = true; changed(); }
        if (e.data.type === 'gosu:error') { state = { ...state, running: false, error: data }; frame.remove(); area.hidden = true; changed(); }
    } catch (error) { note.textContent = tr`게임 메시지 오류: ${error.message}`; if (e.data.type === 'gosu:result') { state = { ...state, running: false, error: error.message }; frame?.remove(); area.hidden = true; changed(); } }
});
const app = {
    async Profile(lanes) { return storedProfile(lanes); },
    async SaveProfile(lanes, value) { if (state.running) throw new Error(tr('플레이 종료 후 설정을 변경해 주세요')); return saveProfile(lanes, value); },
    async ResetProfile(lanes) { if (state.running) throw new Error(tr('플레이 종료 후 설정을 변경해 주세요')); return resetProfile(lanes); },
    async Offsets(s) { return storedOffsets(chartHash(s)); },
    async SaveOffsets(s, value) { if (state.running) throw new Error(tr('플레이 종료 후 설정을 변경해 주세요')); return saveOffsets(chartHash(s), value); },
    async Player() { return { name: storedName(), ...playerStats(await savedRecords()) }; },
    async SaveName(name) { return saveName(name); },
    async Language() { return storedLanguage(); },
    async SaveLanguage(value) { return saveLanguage(value); },
	async DownloadInfo() { return request('/api/download/settings'); },
	async SaveArchiveServer() { throw new Error(tr('웹 서버의 -archive-server 실행 옵션에서 변경해 주세요.')); },
	async SearchSongs(q) { return request('/api/download/search?' + new URLSearchParams({ catalog: q.catalog, q: q.text, keys: q.keys, offset: q.offset })); },
	async StartInstall(key) { if (state.running) throw new Error(tr('플레이 종료 후 설치해 주세요.')); return request('/api/download/start', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Gosu-Install': '1' }, body: JSON.stringify({ key }) }); },
	async InstallState() { return request('/api/download/state'); },
	async CancelInstall() { return request('/api/download/cancel', { method: 'POST', headers: { 'X-Gosu-Install': '1' } }); },
    async Submission(id) { return submissionStatus(id) ?? { state: 'unsent' }; },
    async ScoreServer() { return scoreServer(); },
    async SaveScoreServer(server) {
        if (server && server !== location.origin) throw new Error(tr('브라우저는 현재 사이트 서버에 제출합니다. 현재 주소를 쓰거나 비워 주세요.'));
        localStorage.setItem('ggosu:score-server', server); return server;
    },
    async RetrySubmission(id) {
        const r = submissionRecord(id);
        const status = submissions.enqueue(r, r?.replay.player || 'Player');
        if (!status) throw new Error(tr('일반 플레이 기록만 제출할 수 있습니다.'));
        return status;
    },
    async AudioSettings() { return storedAudio(); },
    async SaveAudio(value) { return saveAudio(value); },
    async PlaybackSettings() { return storedPlayback(); },
    async SavePlayback(p) { return savePlayback(p); },
    async DisplaySettings() { return storedDisplay(localStorage, notice); },
    async SaveDisplay(d) { const saved=saveDisplay(d); frame?.contentWindow.postMessage({type:'gosu:display-settings',data:JSON.stringify(saved)},location.origin); return saved; },
    async Catalog() { catalog = await request('/api/catalog'); checked.clear(); return catalog; },
    async State() { return state; },
    async EditChart(s) { return request('/api/chart/' + encodeURIComponent(s.chartID)); },
    async SaveChart(s, c) { return request('/api/edit/' + encodeURIComponent(s.chartID), { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Gosu-Editor': '1' }, body: JSON.stringify(c) }); },
    async ExportChart(s, c) { return request('/api/export/' + encodeURIComponent(s.chartID), { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Gosu-Editor': '1' }, body: JSON.stringify(c) }); },
    EditorAudio(s, id) { return '/api/asset/' + encodeURIComponent(s.chartID) + '?' + new URLSearchParams({ id }); },
    PreviewAudio(s, id) { return '/api/asset/' + encodeURIComponent(s.chartID) + '?' + new URLSearchParams({ id, preview: '1' }); },
    async Distribution(s) { return request('/api/distribution/' + encodeURIComponent(s.chartID)); },
    async AudioDuration(s) { return request('/api/duration/' + encodeURIComponent(s.chartID)); },
    async Level(s) { return request('/api/level/' + encodeURIComponent(s.chartID) + '?' + new URLSearchParams({ rate: s.rate })); },
    async Play(s) {
        if (state.running) throw new Error(tr('현재 플레이를 종료해 주세요.'));
        const entry = catalog.entries.find(e => e.path === s.chartID && e.index === s.index && e.folder === s.folder);
        if (!entry) throw new Error(tr('채보를 다시 선택해 주세요.'));
        const mode = s.mode || 'live';
        if (!['live', 'auto', 'replay', 'hud'].includes(mode)) throw new Error(tr('잘못된 플레이 모드입니다.'));
        const health = Number(s.health ?? 3), healthFail = Number(s.healthFail ?? 0);
        if (!Number.isInteger(health) || health < 0 || health > 3 || !Number.isInteger(healthFail) || healthFail < 0 || healthFail > 3 || health > 0 && healthFail > health) throw new Error(tr('체력 시작 단계와 게임 오버 단계를 확인해 주세요.'));
        let speed = Number(s.rate ?? 1), pitch = s.pitch !== false, mod = s.mod || '', seed = s.seed, visibility = s.visibility || '';
        if (!['', 'hidden', 'fadein', 'cover', 'flashlight'].includes(visibility)) throw new Error(tr('잘못된 Visibility입니다.'));
        if (!['', 'mirror', 'random', 'mirror+random'].includes(mod)) throw new Error(tr('잘못된 Mods입니다.'));
        if (mod.includes('random') && seed == null) seed = crypto.getRandomValues(new Uint32Array(1))[0];
        seed ??= 0;
        if (!Number.isInteger(seed) || seed < 0 || seed > 4294967295) throw new Error(tr('잘못된 Random seed입니다.'));
        if (mode === 'replay') {
            const r = await readSaved(prefix + s.replayID);
            if (!r || r.replay.chart_hash !== entry.hash) throw new Error(tr('리플레이와 채보가 맞지 않습니다.'));
            speed = r.rate;
            pitch = r.pitch !== false;
            mod = r.replay.mods?.kind || ''; seed = r.replay.mods?.seed ?? 0;
            visibility = r.replay.mods?.visibility || '';
        }
        if (!Number.isFinite(speed) || speed < .5 || speed > 2) throw new Error(tr('Time Rate는 0.5~2.0배여야 합니다.'));
        if (s.section && (!['live', 'auto'].includes(mode) || !Number.isFinite(s.section.start) || !Number.isFinite(s.section.end) || s.section.start >= s.section.end)) throw new Error(tr('연습 구간을 확인해 주세요.'));
        state = { notices: state.notices, running: true, selection: mode === 'hud' ? state.selection : { ...s, mode, mod, visibility, rate: speed, pitch, health, healthFail, seed }, result: null, error: '' }; ready = false; gamePaused = false; resumeIn = 0;
        jsonText.hidden = true;
        note.textContent = tr('게임 안의 시작 버튼을 눌러 주세요.'); pause.disabled = resume.disabled = restart.disabled = true;
        frame?.remove(); frame = document.createElement('iframe'); frame.title = tr`${entry.title} 플레이`;
        frame.width = '800'; frame.height = '600'; frame.allow = 'autoplay';
        const params = new URLSearchParams({ chart: s.chartID, hash: entry.hash, mode, speed, pitch, health, healthFail, mod, visibility, seed, replay: s.replayID || '' });
        if (s.section) { params.set('sectionStart', s.section.start); params.set('sectionEnd', s.section.end); }
        frame.src = '/game.html?' + params;
        area.append(frame); area.hidden = false; changed(); return tr('브라우저 게임을 준비합니다.');
    },
    // Like osu!lazer's skin editor: the HUD editor over looping Auto play.
    async EditHUD(s) { return app.Play({ ...s, mode: 'hud' }); },
    async Retry() { return app.Play(state.selection); },
    async History(s) {
        const entry = catalog.entries.find(e => e.path === s.chartID && e.index === s.index);
        const rows = [];
        for (const key of recordKeys()) {
            const r = await readSaved(key);
            if (r && r.replay.chart_hash === entry?.hash && ['ggosu-keys-v1', 'ggosu-keys-v2', 'ggosu-keys-v3'].includes(r.replay.rules)) rows.push(view(r));
        }
        return rows.sort((a, b) => new Date(b.date) - new Date(a.date));
    },
    async Progress() {
        const rows = [];
        for (const key of recordKeys()) {
            const r = await readSaved(key);
            if (r) rows.push({ ...view(r), hash: r.replay.chart_hash, rules: r.replay.rules });
        }
        return summarize(rows);
    },
    // Song select background images come from the chart's own assets.
    backgroundURL(e) { return `/api/asset/${encodeURIComponent(e.path)}?id=${encodeURIComponent(e.background)}&preview=1`; },
    async Played() {
        return (await app.Progress()).map(r => r.hash);
    }
};
window.go = { main: { App: app } };
window.runtime = { EventsOn(event, fn) { if (event === 'play:changed') listeners.add(fn); } };
const portability = document.createElement('section');
portability.id = 'record-tools';
const download = document.createElement('button'); download.type = 'button'; download.textContent = tr('최근 기록·리플레이 다운로드');
const showJSON = document.createElement('button'); showJSON.type = 'button'; showJSON.textContent = tr('리플레이 JSON 보기');
const jsonText = document.createElement('textarea'); jsonText.readOnly = true; jsonText.rows = 12; jsonText.cols = 80; jsonText.hidden = true; jsonText.setAttribute('aria-label', tr('리플레이 JSON'));
showJSON.addEventListener('click', () => { if (!latestJSON) { document.querySelector('#status').textContent = tr('최근 기록이 없습니다.'); return; } jsonText.value = JSON.stringify(JSON.parse(latestJSON), null, 2); jsonText.hidden = false; jsonText.focus(); });
download.addEventListener('click', () => {
    const text = latestJSON;
    if (!text) { document.querySelector('#status').textContent = tr('다운로드할 저장 기록이 없습니다.'); return; }
    const url = URL.createObjectURL(new Blob([text], { type: 'application/json' }));
    const a = document.createElement('a'); a.href = url; a.download = JSON.parse(text).id + '.json'; a.hidden = true; document.body.append(a); a.click(); a.remove(); setTimeout(() => URL.revokeObjectURL(url), 1000);
});
const upload = document.createElement('input'); upload.type = 'file'; upload.accept = '.json'; upload.setAttribute('aria-label', tr('기록·리플레이 가져오기'));
upload.addEventListener('change', async () => {
    try {
        const file = upload.files[0]; if (!file) return;
        if (file.size > 128 * 1024 * 1024) throw new Error(tr('기록이 너무 큽니다.'));
        const text = await file.text(), r = await validateRecord(text);
        if (r.verification === 'repaired') {
            localStorage.setItem('ggosu:quarantine:' + crypto.randomUUID(), text);
            notice(tr('가져온 결과를 리플레이에서 복구했습니다. 원본은 별도로 보존했습니다.'));
        }
        store(r); changed(); window.dispatchEvent(new Event('ggosu:records-changed'));
        document.querySelector('#status').textContent = tr('기록·리플레이를 가져왔습니다.') + (r.verification === 'repaired' ? tr(' 결과를 리플레이에서 복구했습니다.') : '');
    }
    catch (e) { document.querySelector('#status').textContent = tr`가져오기 실패: ${e.message}`; }
});
portability.append(download, showJSON, upload, jsonText); document.querySelector('#panel-online').append(portability);
window.addEventListener('online', () => { void submissions.flush().catch(e => notice(tr('점수 제출 재시도 실패: ') + e.message)); });
void submissions.flush().catch(e => notice(tr('점수 제출 대기열 읽기 실패: ') + e.message));
await import('/main.js');
