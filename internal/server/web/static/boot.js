import { storedDisplay } from '/preferences/display.js';
import { storedAudio } from '/preferences/audio.js';
import { storedProfile, effectiveProfile, storedOffsets, storedName } from '/preferences/profile.js';
const status = document.querySelector('#loading');
const start = document.querySelector('#start');
function report(message) {
    status.textContent = message;
    parent.postMessage({ type: 'gosu:error', data: JSON.stringify(message) }, location.origin);
}
try {
    const query = new URLSearchParams(location.search);
    const id = query.get('chart');
    const response = await fetch(`/api/chart/${encodeURIComponent(id)}`);
    if (!response.ok) throw new Error('채보를 읽지 못했습니다.');
    const chart = await response.json();
    const assets = Object.create(null);
    const paths = new Set();
	let encodedBytes = 0;
    for (const [key, asset] of Object.entries(chart.assets ?? {})) {
        if (asset.kind === 'video' || paths.has(asset.path)) continue;
        paths.add(asset.path);
        const file = await fetch(`/api/asset/${encodeURIComponent(id)}?id=${encodeURIComponent(key)}`);
        if (file.ok) { const data = new Uint8Array(await file.arrayBuffer()); encodedBytes += data.byteLength; if (encodedBytes > 256 * 1024 * 1024) throw new Error('웹 음원·이미지 용량 제한을 초과했습니다.'); assets[asset.path] = data; }
    }
    const replayID = query.get('replay');
    const replay = replayID ? localStorage.getItem(`ggosu:record:${replayID}`) : '';
    if (query.get('mode') === 'replay' && !replay) throw new Error('리플레이 기록이 없습니다.');
    window.gosuConfig = { chart: JSON.stringify(chart), assets, health: Number(query.get('health') ?? 3), healthFail: Number(query.get('healthFail') ?? 0), mode: query.get('mode') || 'live', speed: Number(query.get('speed') || 1), pitch: query.get('pitch') !== 'false', mod: query.get('mod') || '', visibility: query.get('visibility') || '', seed: Number(query.get('seed') || 0), replay, display: JSON.stringify(storedDisplay()) };
    window.gosuConfig.audio = JSON.stringify(storedAudio());
    window.gosuConfig.profile = JSON.stringify(effectiveProfile(storedProfile(chart.lanes)));
    window.gosuConfig.offsets = JSON.stringify(storedOffsets(query.get('hash')));
    window.gosuConfig.name = storedName();
    if (query.has('sectionStart')) window.gosuConfig.section = JSON.stringify({ start: Number(query.get('sectionStart')), end: Number(query.get('sectionEnd')) });
    const go = new Go();
    const result = await WebAssembly.instantiateStreaming(fetch('/game.wasm'), go.importObject);
    status.textContent = '준비됐습니다. 시작 버튼을 누르면 오디오와 게임이 시작됩니다.';
    start.disabled = false;
    start.addEventListener('click', async () => {
        start.disabled = true; document.querySelector('#game-preparation').hidden = true;
        window.focus();
        try { await go.run(result.instance); }
        catch (e) { report(e?.message ?? String(e)); }
    }, { once: true });
} catch (e) { report(e?.message ?? String(e)); }
