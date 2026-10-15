import { tr } from '../preferences/language.js';
import * as languageSettings from '/settings/language.js';
import * as preferences from '/settings/preferences.js';
import * as playback from '/settings/playback.js';
import * as editor from '/editor/editor.js';
import { Preview } from '/audio/preview.js';
import { createLobby } from '/lobby/lobby.js';
import { query, random } from '/songlist/search.js';
import { defaultView, keyCounts, layout, step, sibling, songStep } from '/songlist/list.js';
import { fields, counts as columnCounts, formats, axes } from '/songlist/range.js';
import { rangeWindow } from '/songselect/range.js';
import { levelAt, remember as rememberLevel } from '/songlist/level.js';
import { cue } from '/audio/sound.js';
import { mountControlSounds } from '/controlsounds.js';
import { mount as mountVolume } from '/audio/volume.js';
import { render as renderInfo } from '/songselect/info.js';
import { renderList as drawList, renderTabs as drawTabs, renderKeyFilter as drawKeyFilter, renderLampFilter as drawLampFilter, scrollToFraction } from '/songselect/catalog.js';
import { mount as mountShortcuts, hints, background, settings as settingsShortcut, mods as modShortcuts } from '/songselect/shortcuts.js';
import { showResult, showSubmission, history as loadHistory, clearHistory, compareResult } from '/songselect/records.js';
import * as offsets from '/settings/offsets.js';
import * as cursor from '/cursor.js';
import * as notification from '/notification.js';
import * as browser from '/browser.js';
import * as settingsPanel from '/settings/panel.js';
import * as songFolders from '/settings/folders.js';
import * as submissionSettings from '/settings/submission.js';
import { mountChatSettings } from '/settings/chat.js';
import * as confirm from '/songselect/confirm.js';
import * as motion from '/motion/motion.js';
import * as pulse from '/songselect/pulse.js';
import * as downloads from '/songselect/download.js';
import * as medley from '/medley/medley.js';
const $ = selector => document.querySelector(selector);
const status = $('#status');
const integrity = document.createElement('p');
integrity.id = 'integrity-status'; integrity.setAttribute('role', 'status'); integrity.hidden = true;
status.after(integrity);
// Each new notice shows for a few seconds; failures stay longer so they can be read.
let statusTimer = 0;
new MutationObserver(() => {
    clearTimeout(statusTimer);
    status.hidden = !status.textContent;
    if (status.textContent) statusTimer = setTimeout(() => { status.textContent = ''; }, /실패|오류|failed|could not|error/i.test(status.textContent) ? 8000 : 3000);
}).observe(status, { childList: true, characterData: true, subtree: true });
function integrityNotices(notices = []) { integrity.textContent = notices.join('\n'); integrity.hidden = notices.length === 0; }
const list = $('#song-list');
const search = $('#search-text');
const params = new URLSearchParams(window.location.search);
let initial = window.location.pathname === '/play' && ['q', 'chart', 'edit'].some(k => params.has(k)) ? params : null;
let entries = [];
let rows = [];       // Matches for the chosen key counts, sorted.
let matched = [];    // Matches for every key count, for the key tabs.
let shown = layout([], { view: 'chart', sort: 'level' });
let current = -1;    // Index into entries.
let collapsed = false; // The selected song's charts are folded under its title.
let lanes = new Set(); // Chosen key counts; the filter checks any number of them.
let playedHashes = null;
let progress = null;
let recent = false;
let progressTicket = 0;
let searchValid = true;
let canPlay = false; // There is no play button; Enter, double-click and the hint bar start play.
let installedPack = null;
const ranges = { level: null, length: null }; // Dragged windows as value ranges; null = no limit.
const lampFilter = new Set(); // Chosen clear lamps; empty = no limit.
let sortDesc = false;
let preSearch = null; // Selection before the search text was typed; Esc returns to it.
const randomHistory = [];
const lastByLanes = new Map(); // Last selection per key-count choice.
const lanesKey = (set = lanes) => [...set].sort((a, b) => a - b).join(',');
const laneText = () => lanesKey().split(',').map(n => n + 'K').join('·');
const preview = new Preview({
    chart: s => window.go.main.App.EditChart(s),
    url: (s, id) => window.go.main.App.PreviewAudio ? window.go.main.App.PreviewAudio(s, id) : '/preview/audio?' + new URLSearchParams({ chart: s.chartID, folder: s.folder, index: s.index, id }),
    notify: (text, details = '') => {
        const failed = /실패|failed|could not|error/i.test(text);
        $('#preview-status').textContent = failed ? text : '';
        $('#preview-status').title = failed ? details || text : '';
        $('#preview-status').hidden = !failed;
    },
});
let pending = false;
let scanning = false; // the song catalog is being read
let state = { running: false, result: null };
let shownID = '';
// The record on the result screen: the play that just ended, or one opened from the history table.
let shownRecord = null;
let stateTicket = 0;
let distributionTicket = 0;
let distributionReport = null;
let listedRate = 1;  // Time Rate the list levels were drawn at.
let levelTicket = 0;
let durationTicket = 0, durationTimer;

// Per-viewer conveniences only; the page works without storage.
function remembered(key, fallback) { try { const v = localStorage.getItem('ggosu:ui:' + key); return v == null ? fallback : JSON.parse(v); } catch { return fallback; } }
function remember(key, value) { try { localStorage.setItem('ggosu:ui:' + key, JSON.stringify(value)); } catch { /* Storage is optional. */ } }
const viewBySort = remembered('viewBySort', {});

function editing() { return preferences.editing() || offsets.editing(); }
// Moving the selection while settings are unsaved would reload the edited chart offset.
function locked() { return pending || state.running || editing(); }

function selected() { return current < 0 ? null : entries[current] ?? null; }
// Levels follow the Time Rate being typed; an unfinished value shows 1x.
function timeRate() { return $('#time-rate').validity.valid ? $('#time-rate').valueAsNumber : 1; }
let sectionDomain = null; // The selected chart's practice-section columns, once its graph has loaded.
function selection(e = selected()) { return { folder: e.folder, chartID: e.path, index: e.index, mod: $('#lane-mod').value, visibility: $('#visibility-mod').value, o2jam: $('#o2jam-mod').value, pills: $('#o2jam-pills').checked, rate: $('#time-rate').valueAsNumber, pitch: $('#pitch').checked, health: Number($('#health-mode').value), healthFail: Number($('#health-fail').value), section: practiceSection(e) }; }
const lobby = createLobby({entries:()=>entries,selection:()=>selected()?selection():null,stopPreview:()=>preview.stop()});
// Title: single play shows song select; Esc there comes back here, Esc here asks to quit.
const WEBSITE = window.runtime?.Quit ? 'http://127.0.0.1:8080' : window.location.origin;
function showTitle() {
    $('#songselect').hidden = true; medley.close(); $('#title').hidden = false;
    update(); $('#title-single').focus(); showPlayer();
}
// Mini player on the title: the previewing song and how far into it the preview is.
let playerFrame = 0;
const clock = s => `${Math.floor(s / 60)}:${String(Math.floor(s % 60)).padStart(2, '0')}`;
function showPlayer() { playerFrame ||= requestAnimationFrame(drawPlayer); }
function drawPlayer() {
    playerFrame = 0;
    if ($('#title').hidden) return;
    const e = selected(), p = preview.progress();
    $('#title-player').hidden = !e;
    if (e) {
        $('#title-song').textContent = `${e.artist || tr('아티스트 미표기')} - ${e.title}`;
        $('#title-time').textContent = p ? `${clock(p.at)} / ${clock(p.end)}` : '';
        $('#title-progress span').style.width = p ? `${p.at / p.end * 100}%` : '0';
    }
    showPlayer();
}
// The wheel on the title changes the volume (osu!'s main menu); Ctrl+wheel never zooms the page anywhere.
const volume = mountVolume(document.body, { get: preferences.volumes, set: preferences.setVolume, tick: () => cue('notch', preferences.effectsVolume()) });
mountControlSounds(preferences.effectsVolume);
// Capture before screen navigation and dialog shortcuts consume the arrows.
document.addEventListener('keydown', e => {
    if (e.isComposing || !volume.key(e)) return;
    e.preventDefault(); e.stopImmediatePropagation();
}, true);
document.addEventListener('wheel', e => {
    if (e.ctrlKey) e.preventDefault();
    if ($('#title').hidden || document.querySelector('dialog[open]')) return;
    e.preventDefault(); volume.wheel(e);
}, { passive: false });
function showSongSelect() {
    volume.hide();
    motion.leave($('#title')); $('#songselect').hidden = false;
    refreshList(); update(); search.focus();
}
$('#title-single').onclick = showSongSelect;
$('#title-multi').onclick = () => { showSongSelect(); lobby.open(); };
medley.mount({
    entries: () => entries, busy: () => pending || state.running, error: () => /실패|오류|failed|could not|error/i.test(status.textContent) ? status.textContent : '',
    playOptions: () => ({ mod: $('#lane-mod').value, visibility: $('#visibility-mod').value, o2jam: $('#o2jam-mod').value, pills: $('#o2jam-pills').checked, rate: $('#time-rate').valueAsNumber, pitch: $('#pitch').checked, health: Number($('#health-mode').value), healthFail: Number($('#health-fail').value) }),
    launch: s => { cue('confirm', preferences.effectsVolume()); preview.stop(); launch(() => window.go.main.App.Play(s)); },
    back: () => { cue('back', preferences.effectsVolume()); showTitle(); },
});
$('#title-medley').onclick = () => { volume.hide(); motion.leave($('#title')); medley.open(); };
$('#title-medley').hidden = typeof window.go?.main?.App?.MedleyPick !== 'function';
$('#title-edit').onclick = async () => {
    showSongSelect();
    if (!selected()) { status.textContent = tr('채보를 먼저 고르세요'); return; }
    openEditor(true);
};
settingsPanel.mount($('#settings'));
$('#title-options').onclick = () => { settingsPanel.open(); preferences.opened(); };
settingsShortcut(() => { settingsPanel.open(); preferences.opened(); }, settingsPanel.close, () => pending || state.running);
$('#settings').addEventListener('close', () => { if (!$('#title').hidden) $('#title-options').focus(); });
$('#title-address').textContent = new URL(WEBSITE).host;
$('#title-web').onclick = () => {
    if (window.runtime?.BrowserOpenURL) window.runtime.BrowserOpenURL(WEBSITE);
    else window.open(WEBSITE, '_blank', 'noopener,noreferrer');
};
$('#title-quit').onclick = () => $('#quit-dialog').showModal();
$('#title-quit').hidden = !window.runtime?.Quit;
// Menu items follow the logo's ring, 60px apart: each button's corner nearest the centre stays 12px
// clear of the ring at hover size (r 161.5 x 1.1), so the outer buttons never slide over the logo.
{
    const shown = [...$('#title-menu').querySelectorAll('button:not([hidden])')];
    const ring = 161.5 * 1.1;
    const reach = shown.map((_, i) => {
        const edge = Math.max(0, Math.abs((i - (shown.length - 1) / 2) * 60) - 26);
        return Math.sqrt(Math.max(0, ring ** 2 - edge ** 2)) + 12;
    });
    const near = Math.min(...reach);
    $('#title-menu').style.marginLeft = `${near - 170}px`;
    shown.forEach((b, i) => { b.style.marginLeft = `${reach[i] - near}px`; b.addEventListener('pointerenter', () => b.focus()); });
}
// The title logo answers the mouse like osu!lazer's OsuLogo; timings live in theme.css.
{
    const logo = $('#title-logo'), flash = $('#title-flash');
    logo.addEventListener('pointerenter', () => logo.classList.add('hovered'));
    logo.addEventListener('pointerleave', () => logo.classList.remove('hovered', 'pressed'));
    logo.addEventListener('pointerdown', e => { if (e.button === 0) logo.classList.add('pressed'); });
    logo.addEventListener('pointerup', e => {
        if (e.button !== 0 || !logo.classList.contains('pressed')) return;
        logo.classList.remove('pressed');
        flash.classList.add('lit'); void flash.getBoundingClientRect(); flash.classList.remove('lit');
    });
}
$('#quit-dialog').addEventListener('close', () => { if ($('#quit-dialog').returnValue === 'quit') window.runtime?.Quit?.(); else $('#title-single').focus(); });
document.addEventListener('keydown', e => {
    if ($('#title').hidden || document.querySelector('dialog[open]') || e.isComposing || e.defaultPrevented) return;
    if (e.ctrlKey || e.metaKey || e.altKey || e.repeat || e.target.closest('input, textarea, select, [contenteditable]')) return;
    const command = { p: '#title-single', m: '#title-multi', e: '#title-edit', o: '#title-options', q: '#title-quit' }[e.key.toLowerCase()];
    if (command && !$(command).hidden) { e.preventDefault(); $(command).click(); return; }
    const menu = [...$('#title-menu').querySelectorAll('button:not([hidden])')], at = menu.indexOf(document.activeElement);
    if (e.key === 'Escape') { e.preventDefault(); if (!$('#title-quit').hidden) $('#quit-dialog').showModal(); }
    else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') { e.preventDefault(); menu[(at + (e.key === 'ArrowDown' ? 1 : menu.length - 1)) % menu.length]?.focus(); }
});
let warmKey = '', warmTimer = 0, warmRequest = Promise.resolve();
function warmSelection(e) {
    // Native game processes share the persistent PCM cache. Wait for a stable
    // selection so moving through the catalog does not decode every song.
    if (!state.desktop) return;
    const key = e ? `${e.folder}\n${e.path}` : '';
    if (key === warmKey) return;
    warmKey = key; clearTimeout(warmTimer);
    const send = s => { warmRequest = warmRequest.then(() => fetch('/preview/warm', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(s) })).catch(() => {}); };
    send(null);
    if (e) {
        const s = { folder: e.folder, chartID: e.path, index: e.index };
        warmTimer = setTimeout(() => send(s), 600);
    }
}
function update() {
    if (pending || state.running || $('#songselect').hidden || document.querySelector('dialog[open]')) background();
    if (timeRate() !== listedRate) { listedRate = timeRate(); refreshList(); loadLevel(); return; }
    const e = selected();
    warmSelection(!pending && !state.running && !$('#songselect').hidden ? e : null);
    preview.setVolume(preferences.musicVolume());
    preview.setPlayback($('#time-rate').valueAsNumber, $('#pitch').checked);
    preferences.setScrollContext($('#time-rate').valueAsNumber);
    // Minimizing or switching windows keeps the preview going; only play, edit and leaving the screens stop it.
    preview.select(!pending && !state.running && !($('#songselect').hidden && $('#title').hidden && !medley.isOpen()) ? e : null);
    if (state.running || !e) { $('#preview-status').textContent = ''; $('#preview-status').hidden = true; }
    renderInfo(e, entries, distributionReport, progress, backgroundURL(e), timeRate(), scanning);
    $('#catalog-loading').hidden = !scanning;
    $('#list-pane').setAttribute('aria-busy', String(scanning));
    list.hidden = scanning;
    $('#list-summary').hidden = $('#list-position').hidden = scanning;
    showBackground(e);
    const busy = pending || state.running;
    $('#playing-status').hidden = !(state.desktop && state.running);
    $('#playing-label').textContent = state.activity === 'hud' ? tr('게임 창에서 HUD · Hit position 편집 중') : tr('게임 창에서 플레이 중');
    $('#playing-chart').textContent = e?.title ?? '';
    $('#o2jam-mod').disabled = $('#o2jam-pills').disabled = $('#visibility-mod').disabled = $('#health-fail').disabled = $('#health-mode').disabled = $('#mode').disabled = $('#lane-mod').disabled = $('#time-rate').disabled = $('#pitch').disabled = busy || editing() || !playback.loaded();
    playback.setBusy(busy || editing());
    canPlay = !busy && !preferences.needsSave() && !playback.needsSave() && !!e && $('#time-rate').validity.valid && !offsets.needsSave();
    $('#hud-edit').disabled = false;
    $('#chart-edit').disabled = locked() || !e;
    preferences.setBusy(busy);
    $('#downloads-open').disabled = busy || editing();
    offsets.setBusy(busy, !!e);
    $('#retry').disabled = $('#watch').disabled = $('#back').disabled = busy;
    for (const b of $('#history-box').querySelectorAll('button, select')) b.disabled = busy || !e;
    renderHints();
    medley.update();
}
async function sync() {
    const ticket = ++stateTicket;
    const next = await window.go.main.App.State();
    if (ticket !== stateTicket) return;
    const ended = state.running && !next.running, desktop = next.desktop !== state.desktop;
    state = next;
    if (desktop) renderSection();
    if (ended) preferences.reload();
    integrityNotices(state.notices);
    if (!state.running) confirm.back();
    // A run without a result (HUD editor, 0-point play) leaves no "playing" note behind.
    if (!state.running && status.textContent === tr('플레이 중')) status.textContent = '';
    if (state.running) {
        $('#result').hidden = true; confirm.undim();
        if (!medley.isOpen()) $('#songselect').hidden = false;
        status.textContent = tr('플레이 중');
    } else if (state.error) {
        status.textContent = tr`플레이 오류: ${state.error}`;
    } else if (state.result && shownID !== state.result.id) {
        const r = state.result, s = state.selection;
        shownID = state.result.id;
        shownRecord = state.result;
        preview.stop();
        showResult(state.result);
        confirm.enter();
        if (state.result.calibration) offsetForm.load();
        status.textContent = '';
        await history();
        await compareResult(r, s, () => shownRecord);
        await Promise.all([loadProgress(), loadPlayer()]);
        refreshList();
    }
    if (shownRecord && state.result?.id === shownRecord.id && !$('#result').hidden) {
        shownRecord = state.result;
        showSubmission(shownRecord);
    } else if (shownRecord && !$('#result').hidden && typeof window.go.main.App.Submission === 'function') {
        const r = shownRecord;
        try { r.submission = await window.go.main.App.Submission(r.id); }
        catch (e) { r.submission = { state: 'failed', error: e?.message ?? String(e) }; }
        if (shownRecord === r && !$('#result').hidden) showSubmission(r);
    }
    update();
    if (installedPack && !locked() && !scanning) void selectInstalled();
}
function history() {
 const e = selected();
 return loadHistory(e, e ? selection(e) : null, update, openRecord, historyScope);
}
// Local, rival or global ranking; only the local list has a sort order to change.
let historyScope = 'local';
for (const b of $('#history-scope').querySelectorAll('button')) b.addEventListener('click', () => {
    historyScope = b.value;
    for (const other of $('#history-scope').querySelectorAll('button')) other.setAttribute('aria-pressed', String(other === b));
    $('#history-order').hidden = historyScope !== 'local';
    history();
});
// One press moves to the next sort order.
const historyOrders = { points: tr('점수순'), date: tr('날짜순') };
$('#history-order').addEventListener('click', () => {
    const b = $('#history-order'), names = Object.keys(historyOrders);
    b.value = names[(names.indexOf(b.value) + 1) % names.length];
    b.textContent = tr(historyOrders[b.value]);
    history();
});
function openRecord(r) {
    const e = selected();
    if (!e || locked()) return;
    shownRecord = r;
    preview.stop();
    showResult(r, { past: true });
    compareResult(r, selection(e), () => shownRecord);
    update();
}
async function loadDistribution() {
    const ticket = ++distributionTicket, e = selected();
    distributionReport = null;
    renderInfo(e, entries, null, progress, backgroundURL(e), timeRate(), scanning);
    if (!e) return;
    try {
        const report = await window.go.main.App.Distribution(selection(e));
        if (ticket !== distributionTicket) return;
        distributionReport = report;
        renderInfo(e, entries, report, progress, backgroundURL(e), timeRate(), scanning);
    } catch { /* the info graph stays empty */ }
    renderSection();
}
// Practice section (F1): a range window over the selected chart's note density, remembered per chart.
const sectionKey = e => `section:${e.folder}|${e.path}|${e.index}`;
const sectionClock = s => `${s < 0 ? '-' : ''}${clock(Math.abs(s))}`;
const sectionWidget = rangeWindow($('#section-range'), {
    name: tr('구간'), unit: tr('노트'),
    axis: (d, i) => i > 0 && Math.floor((d.start + i * d.width) / 60) !== Math.floor((d.start + (i - 1) * d.width) / 60) ? clock(Math.floor((d.start + i * d.width) / 60) * 60) : '',
    format: (d, lo, hi) => `${sectionClock(d.start + lo * d.width)}–${sectionClock(d.start + (hi + 1) * d.width)}`,
    onChange: r => { const e = selected(); if (e) remember(sectionKey(e), r); renderHints(); },
});
function renderSection() {
    const e = selected(), r = distributionReport;
    $('#section-range').closest('section').hidden = false;
    sectionDomain = e && r?.step > 0 && r.bins.length ? { start: r.start, width: r.step, n: r.bins.length } : null;
    sectionWidget.update(sectionDomain, r?.bins.map(b => b.tap + b.hold + b.lift), sectionDomain ? remembered(sectionKey(e), null) : null);
    renderHints();
}
// Seconds on the song timeline; only the selected chart's loaded graph defines one.
function practiceSection(e) {
    const r = e === selected() && sectionDomain ? remembered(sectionKey(e), null) : null;
    return r ? { start: r.min ?? sectionDomain.start, end: r.max ?? sectionDomain.start + sectionDomain.n * sectionDomain.width } : undefined;
}
// Measure one selection after navigation settles; never scan every keysound bank.
function loadDuration() {
    const ticket = ++durationTicket, e = selected();
    clearTimeout(durationTimer);
    if (!e || e.durationMeasured) return;
    e.durationPending = true; e.audioDuration = 0; e.durationError = '';
    update();
    const s = selection(e);
    durationTimer = setTimeout(async () => {
        try {
            const end = await window.go.main.App.AudioDuration(s);
            e.audioDuration = end;
            e.durationMeasured = true;
        } catch (error) {
            e.durationError = String(error?.message || error);
        } finally {
            e.durationPending = false;
            if (ticket === durationTicket && e === selected()) update();
        }
    }, 200);
}
// The scan rated the preset Time Rates; any other rate is rated for the selected chart when chosen.
async function loadLevel() {
    const ticket = ++levelTicket, e = selected(), rate = timeRate();
    if (!e || !(e.rating > 0) || levelAt(e, rate).value != null && !levelAt(e, rate).estimated || !window.go.main.App.Level) return;
    try {
        const value = await window.go.main.App.Level({ ...selection(e), rate });
        rememberLevel(e.hash, rate, value);
        if (ticket === levelTicket) { refreshList(); }
    } catch { /* the estimate or the 1x level stays */ }
}
$('#lane-mod').addEventListener('change', () => { update(); history(); });
$('#visibility-mod').addEventListener('change', () => { update(); history(); });
$('#time-rate').addEventListener('input', update);
$('#pitch').addEventListener('change', update);

// ---- Song list ----

function sortKey() { return recent ? 'recent' : $('#search-sort').value; }
function currentView() { return viewBySort[sortKey()] ?? defaultView(sortKey()); }
function criteria() {
    const q = { text: search.value, sort: sortKey(), desc: sortDesc, rate: $('#adjusted-level').checked ? timeRate() : 1 };
    q.levelRange = ranges.level;
    q.lengthRanges = ranges.length ? [ranges.length] : [];
    q.playedHashes = playedHashes;
    q.recent = recent;
    q.progress = progress;
    q.lamps = lampFilter;
    searchValid = !(recent || lampFilter.size) || playedHashes != null;
    return q;
}
function renderList() { drawList(shown, { current, progress, recent, view: currentView(), collapsed, rate: timeRate() }); }
function renderTabs() { drawTabs(entries, matched, lanes, toggleLanes); }
function listStatus() {
    if (scanning) return '';
    if (!searchValid) return recent ? tr('기록을 읽지 못해 최근 플레이 사용 불가') : tr('기록을 읽지 못해 클리어 램프 필터 사용 불가');
    if (!entries.length) return tr('채보 없음 · songs 폴더 [F5]');
    if (!rows.length) return matched.length ? tr`${laneText()} 결과 없음` : tr('결과 없음');
    return '';
}
// Recompute matches and layout. Keeps the selection when it is still listed, else selects the first row.
function refreshList({ keep = true } = {}) {
    renderSort();
    const q = criteria();
    matched = searchValid ? query(entries, q) : [];
    const known = new Set(entries.map(e => e.lanes));
    for (const n of lanes) if (!known.has(n)) lanes.delete(n);
    if (!lanes.size) lanes = mostCommonLanes();
    rows = matched.filter(r => lanes.has(r.entry.lanes));
    renderRanges(q);
    shown = layout(rows, { view: currentView(), sort: q.sort, rate: q.rate, progress });
    const previous = current;
    const last = lastByLanes.get(lanesKey());
    if (!keep || !shown.order.includes(current)) current = shown.order.includes(last) ? last : shown.order[0] ?? -1;
    renderTabs(); renderList();
    $('#list-status').textContent = listStatus();
    if (current !== previous) selectionChanged();
    else update();
}
function mostCommonLanes() {
    const counts = keyCounts(entries, entries.map((entry, index) => ({ entry, index })));
    const saved = [remembered('lanes', [])].flat().filter(n => counts.has(n)); // older saves hold one number
    if (saved.length) return new Set(saved);
    const top = [...counts].sort((a, b) => b[1] - a[1] || a[0] - b[0])[0]?.[0];
    return new Set(top == null ? [] : [top]);
}
function select(index, sound = 'move') {
    if (index < 0 || index === current) return;
    cue(sound, preferences.effectsVolume());
    current = index; collapsed = false; lastByLanes.set(lanesKey(), index);
    renderList(); $('#list-status').textContent = listStatus();
    selectionChanged();
}
// The chart's own background fills the top of the detail pane and fades in once loaded.
let shownBackground = '';
function backgroundURL(e) {
    if (!e?.background) return '';
    // The browser version supplies its own asset route; the desktop app serves /preview/background.
    if (window.go?.main?.App?.backgroundURL) return window.go.main.App.backgroundURL(e);
    return '/preview/background?' + new URLSearchParams({ chart: e.path, folder: e.folder, index: e.index, id: e.background });
}
function showBackground(e) {
    const url = backgroundURL(e), img = $('#bg'), old = $('#bg-old');
    if (url === shownBackground) return;
    shownBackground = url;
    // osu! lazer: the previous picture fades out (250 ms) under the new one fading in (500 ms, OutQuint).
    // Nothing changes until the new picture is decoded, so the screen never shows a half-loaded or blank frame.
    const next = new Image();
    if (url) next.src = url;
    (url ? next.decode() : Promise.resolve()).then(async () => {
        if (shownBackground !== url) return;
        if (img.classList.contains('shown')) {
            old.classList.remove('gone'); old.src = img.src; old.hidden = false;
            await old.decode().catch(() => {});
            if (shownBackground !== url) return;
            requestAnimationFrame(() => old.classList.add('gone'));
        } else old.hidden = true;
        // The new picture starts from zero at once instead of fading out from the old one's opacity.
        img.classList.add('instant'); img.classList.remove('shown');
        if (!url) { img.hidden = true; img.removeAttribute('src'); img.classList.remove('instant'); return; }
        img.src = url; img.hidden = false; void img.offsetWidth; img.classList.remove('instant');
        requestAnimationFrame(() => img.classList.add('shown'));
    }, () => { if (shownBackground === url) { img.classList.remove('shown'); img.hidden = true; } });
}
// The list and header follow every key press; reads that wait on the backend start once the selection rests,
// so holding an arrow key never queues one request per chart passed.
let settleTimer = 0;
function selectionChanged() {
    if (selected()) preferences.select(selected().lanes);
    ++distributionTicket; distributionReport = null; renderSection();
    clearHistory();
    update(); offsetForm.load();
    clearTimeout(settleTimer);
    settleTimer = setTimeout(() => { history(); loadDistribution(); loadLevel(); loadDuration(); }, 150);
}
function setLanes(next) {
    if (locked() || !next.size || lanesKey(next) === lanesKey()) return;
    cue('tab', preferences.effectsVolume());
    lanes = next; remember('lanes', [...next]);
    refreshList({ keep: false });
}
// A key tab or filter checkbox adds or removes one key count; the last one stays checked.
function toggleLanes(n) {
    const next = new Set(lanes);
    if (next.has(n)) next.delete(n); else next.add(n);
    if (!next.size || locked()) { renderRanges(criteria()); return; } // puts the checkbox back
    setLanes(next);
}
// Tab steps through single key counts; from a mixed choice it starts at the lowest one.
function cycleLanes(delta) {
    const keys = [...keyCounts(entries, [])].map(([n]) => n);
    if (!keys.length) return;
    const at = lanes.size === 1 ? keys.indexOf([...lanes][0]) : delta > 0 ? -1 : 0;
    setLanes(new Set([keys[(at + delta + keys.length) % keys.length]]));
}
function setView(view) {
    cue('tab', preferences.effectsVolume());
    viewBySort[sortKey()] = view; remember('viewBySort', viewBySort);
    refreshList();
}
// The sort window mirrors the hidden select; Shift+F4 flips the direction.
const sortButtons = [...$('#search-sort').options].map(o => sortButton(o.textContent, () => { $('#search-sort').value = o.value; refreshList(); }, $('#sort-options')));
function sortButton(text, run, parent) {
    const b = document.createElement('button'); b.type = 'button'; b.textContent = text;
    b.addEventListener('click', () => { if (locked() || recent) return; cue('tab', preferences.effectsVolume()); run(); });
    parent.append(b); return b;
}
function setSortDesc(desc) {
    if (locked() || recent || sortDesc === desc) return;
    cue('tab', preferences.effectsVolume());
    sortDesc = desc; refreshList();
}
$('#sort-asc').addEventListener('click', () => setSortDesc(false));
$('#sort-desc').addEventListener('click', () => setSortDesc(true));
let sortText = tr('정렬');
function renderSort() {
    const sort = $('#search-sort');
    sortButtons.forEach((b, i) => b.setAttribute('aria-pressed', String(i === sort.selectedIndex)));
    for (const b of sortButtons) b.disabled = recent;
    $('#sort-asc').setAttribute('aria-pressed', String(!sortDesc)); $('#sort-desc').setAttribute('aria-pressed', String(sortDesc));
    $('#sort-asc').disabled = $('#sort-desc').disabled = recent;
    $('#sort-note').hidden = !recent;
    sortText = recent ? tr('정렬: 최근 플레이') : tr`정렬: ${sort.selectedOptions[0].textContent}${sortDesc ? tr(' 내림차순') : ''}`;
    queueMicrotask(renderHints); // renderSort can run before the action table below exists
}
// Sort and filter share one window, as do game mode and mods; each key focuses its own part.
function openWindow(id, focus) { const d = $(id); if (!d.open) d.showModal(); $(focus)?.focus(); }
function openSort() { openWindow('#advanced', '#sort-options button:not(:disabled)'); }
function pickRandom() {
    if (locked() || !searchValid) return;
    const row = random(rows); if (!row) return;
    if (current >= 0) randomHistory.push(current);
    select(row.index, 'random');
    status.textContent = tr('랜덤 선택 · 되돌리기 Shift+F2');
}
function undoRandom() {
    if (locked()) return;
    while (randomHistory.length) {
        const index = randomHistory.pop();
        if (shown.order.includes(index)) { select(index); return; }
    }
    status.textContent = tr('되돌릴 랜덤 없음');
}

// Esc goes back one step: dialogs close themselves, then search text, then the selection before searching, then the title.
function escapeStep() {
    if (search.value) { search.value = ''; refreshList(); return; }
    if (preSearch) {
        const { index, scroll } = preSearch; preSearch = null;
        if (shown.order.includes(index)) select(index);
        list.scrollTop = scroll;
        renderHints();
        return;
    }
    if (!locked()) { cue('back', preferences.effectsVolume()); showTitle(); }
}
function filterLabel() {
    return ranges.level || ranges.length || lampFilter.size ? tr('필터 적용') : tr('필터');
}

function play(modeName) {
    if (!selected() || !canPlay || pending || state.running) return;
    cue('confirm', preferences.effectsVolume());
    const leaving = confirm.leave(selected(), timeRate());
    // Play resolves once the game window has drawn its first frame.
    launch(async () => {
        try { await window.go.main.App.Play({ ...selection(), mode: modeName }); leaving.ready(); }
        catch (e) { leaving.cancel(); throw e; }
    });
}
// Default key assignments. Kept in one table so they can become user settings.
const actions = [
    { key: 'Enter', label: tr('플레이'), run: () => play($('#mode').value) },
    { key: 'Ctrl+Enter', label: 'Auto', run: () => play('auto') },
    { key: 'Ctrl+A', run: () => $('#mode-buttons').querySelector(`[data-value="${$('#mode').value === 'auto' ? 'live' : 'auto'}"]`).click() },
    { key: 'ArrowUp', run: () => !locked() && select(step(shown.order, current, -1)) },
    { key: 'ArrowDown', run: () => !locked() && select(step(shown.order, current, 1)) },
    { key: 'ArrowLeft', run: () => !locked() && select(sibling(rows, current, -1, criteria().rate)) },
    { key: 'ArrowRight', run: () => !locked() && select(sibling(rows, current, 1, criteria().rate)) },
    { key: 'Shift+ArrowUp', run: () => !locked() && select(shown.view === 'song' ? songStep(shown.groups, current, -1) : step(shown.order, current, -10)) },
    { key: 'Shift+ArrowDown', run: () => !locked() && select(shown.view === 'song' ? songStep(shown.groups, current, 1) : step(shown.order, current, 10)) },
    { key: 'Home', run: () => !locked() && select(shown.order[0] ?? -1) },
    { key: 'End', run: () => !locked() && select(shown.order.at(-1) ?? -1) },
    { key: 'Tab', label: tr('배경 보기'), run: () => { if (!pending && !state.running) background(true); } },
    { key: 'F7', label: tr('키 수'), run: () => cycleLanes(1) },
    { key: 'Shift+F7', run: () => cycleLanes(-1) },
    { key: 'F1', label: () => selected() && practiceSection(selected()) ? tr('모드 · 구간') : tr('모드'), run: () => openWindow('#mods-dialog', '#mod-mirror') },
    { key: 'F2', label: tr('랜덤 선곡'), run: pickRandom },
    { key: 'Shift+F2', label: tr('랜덤 되돌리기'), run: undoRandom },
    { key: 'F3', label: tr('채보 메뉴'), run: openChartMenuAtRow },
    { key: 'F4', label: () => filterLabel() === tr('필터') ? sortText : `${sortText} · ${filterLabel()}`, run: openSort },
    { key: 'Shift+F4', run: () => setSortDesc(!sortDesc) },
    { key: 'F5', label: tr('목록 새로 읽기'), run: load },
    { key: 'F6', label: tr('보기 전환'), run: () => !locked() && setView(shown.view === 'song' ? 'chart' : 'song') },
    // F8 is left to the room chat, as in osu!.
    { key: 'F9', label: tr('곡 받기'), run: () => { void downloadPanel.open(); } },
    { key: 'F10', hint: 'F10 · Ctrl+O', label: tr('환경 설정'), run: () => { settingsPanel.open(); preferences.opened(); } },
    { key: 'Escape', run: escapeStep },
    { key: 'PageUp', run: () => nudgeScroll(true) },
    { key: 'PageDown', run: () => nudgeScroll(false) },
    { key: 'Alt+PageUp', run: () => nudgeScroll(true, true) },
    { key: 'Alt+PageDown', run: () => nudgeScroll(false, true) },
    { key: 'Ctrl+F3', run: () => nudgeScroll(true) },
    { key: 'Ctrl+F4', run: () => nudgeScroll(false) },
    { key: 'Ctrl++', run: () => nudgeScroll(true) },
    { key: 'Ctrl+=', run: () => nudgeScroll(true) },
    { key: 'Ctrl+-', run: () => nudgeScroll(false) },
];
async function nudgeScroll(faster, fine = false) {
    const text = await preferences.nudgeScroll(faster, fine);
    if (text) status.textContent = text;
}
// Only the main commands get buttons; the other keys still work.
// 모드 stays alone on the left with what it currently changes; everything else sits at the right end.
function renderHints() {
    hints(actions, ['F1'], '#hints');
    $('#hints').append(Object.assign(document.createElement('output'), { id: 'mod-summary', textContent: modSummary() }));
    hints(actions, ['Tab', 'F2', 'F4', 'F10'], '#hints-end');
}
// Short codes for every play option that differs from its default.
function modSummary() {
    const rate = $('#time-rate').valueAsNumber, health = $('#health-mode').value, fail = $('#health-fail').value, o2jam = $('#o2jam-mod').value;
    return [
        $('#mode').value === 'auto' && 'Auto',
        ...$('#lane-mod').value.split('+').map(m => ({ mirror: 'MR', random: 'RD' })[m]),
        { hidden: 'HD', fadein: 'FI', cover: 'CO', flashlight: 'FL' }[$('#visibility-mod').value],
        o2jam && `O2 ${o2jam.toUpperCase()}` + ($('#o2jam-pills').checked ? ' + PL' : ''),
        Number.isFinite(rate) && rate !== 1 && `${rate}×`,
        !$('#pitch').checked && 'Pitch OFF',
        !o2jam && health !== '3' && (health === '0' ? tr('체력 OFF') : tr`체력 ${health}단`),
        !o2jam && fail !== '0' && tr`${fail}단 0% 종료`,
    ].filter(Boolean).join(' · ');
}
$('#mods-dialog').addEventListener('input', renderHints);
mountShortcuts(actions, search);
modShortcuts();
search.addEventListener('input', () => {
    if (search.value && !preSearch && current >= 0) preSearch = { index: current, scroll: list.scrollTop };
    refreshList();
});
list.addEventListener('click', e => {
    const li = e.target.closest('li[data-index]');
    if (!li || locked()) return;
    // The selected song's title folds and unfolds its charts; the selection stays.
    const song = shown.groups?.find(g => g.charts[0].index === Number(li.dataset.index));
    if (li.hasAttribute('aria-expanded') && song?.charts.some(c => c.index === current)) { collapsed = !collapsed; renderList(); return; }
    select(Number(li.dataset.index));
});
list.addEventListener('dblclick', e => { if (e.target.closest('li[data-index]')) play($('#mode').value); });
const chartMenu = $('#chart-menu');
function openChartMenu(event, index) {
    if (locked() || !entries[index] || !shown.order.includes(index) || document.querySelector('dialog[open]')) return;
    event.preventDefault();
    select(index);
    const e = selected();
    $('#chart-menu-name').textContent = `${e.title} · ${e.difficulty} · ${e.lanes}K`;
    chartMenu.show();
    const r = chartMenu.getBoundingClientRect(), area = $('#songselect').getBoundingClientRect();
    chartMenu.style.left = `${Math.max(12, Math.min(event.clientX, area.right - r.width - 12))}px`;
    chartMenu.style.top = `${Math.max(12, Math.min(event.clientY, innerHeight - r.height - 12))}px`;
    $('#chart-edit').focus();
}
// F3 opens the same menu under the selected row.
function openChartMenuAtRow() {
    const r = (list.querySelector(`li[data-index="${current}"]`) ?? list).getBoundingClientRect();
    openChartMenu({ preventDefault() {}, clientX: r.left + 24, clientY: r.bottom }, current);
}
// Right button anywhere else on the screen treats the window height as one tall slider over the list; dragging keeps sliding.
const songselect = $('#songselect');
const slides = e => e.button === 2 && !locked() && !document.querySelector('dialog[open]') && !e.target.closest('#selection');
songselect.addEventListener('pointerdown', e => {
    if (!slides(e)) return;
    e.preventDefault();
    songselect.setPointerCapture(e.pointerId);
    scrollToFraction(e.clientY / innerHeight);
});
songselect.addEventListener('pointermove', e => {
    if (e.buttons & 2 && songselect.hasPointerCapture(e.pointerId)) scrollToFraction(e.clientY / innerHeight);
});
songselect.addEventListener('contextmenu', e => { if (!e.target.closest('#selection')) e.preventDefault(); });
$('#selection').addEventListener('contextmenu', e => openChartMenu(e, current));
document.addEventListener('pointerdown', e => { if (chartMenu.open && !chartMenu.contains(e.target)) chartMenu.close(); });
// A press and release both outside a window closes it the way Esc does, so its own cancel handling still runs.
const outside = (d, e) => { const r = d.getBoundingClientRect(); return e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom; };
let pressedOutside = null;
document.addEventListener('pointerdown', e => { pressedOutside = e.target instanceof HTMLDialogElement && outside(e.target, e) ? e.target : null; });
document.addEventListener('click', e => {
    const d = e.target;
    if (!(d instanceof HTMLDialogElement) || d !== pressedOutside || !d.open || !outside(d, e)) return;
    if (d.dispatchEvent(new Event('cancel', { cancelable: true }))) d.close();
});
list.addEventListener('scroll', () => { if (chartMenu.open) chartMenu.close(); });
$('#selection').addEventListener('scroll', () => { if (chartMenu.open) chartMenu.close(); });
window.addEventListener('resize', () => { if (chartMenu.open) chartMenu.close(); });
chartMenu.addEventListener('keydown', e => {
    if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); chartMenu.close(); }
    else if (['ArrowUp', 'ArrowDown', 'Home', 'End'].includes(e.key)) { e.preventDefault(); $('#chart-edit').focus(); }
});
$('#chart-edit').addEventListener('click', () => { chartMenu.close(); openEditor(); });
$('#filters').addEventListener('submit', e => e.preventDefault());
$('#search-sort').addEventListener('change', () => refreshList());
$('#adjusted-level').checked = remembered('adjustedLevel', true);
$('#adjusted-level').addEventListener('change', () => { remember('adjustedLevel', $('#adjusted-level').checked); refreshList(); });
$('#view-song').addEventListener('click', () => setView('song'));
$('#view-chart').addEventListener('click', () => setView('chart'));
for (const [id, value] of [['library-all', false], ['library-recent', true]]) {
    $('#' + id).addEventListener('click', () => { if (locked() || recent === value) return; recent = value; refreshList(); });
}
for (const d of document.querySelectorAll('dialog')) d.addEventListener('close', () => search.focus());
// Level and length are chosen by dragging a window over the distribution of the chosen key counts.
const rangeWidgets = Object.fromEntries(['level', 'length'].map(f => [f, rangeWindow($(`#${f}-range`), {
    name: f === 'level' ? tr('레벨') : tr('길이'), axis: axes[f], format: formats[f],
    onChange: r => { ranges[f] = r; refreshList(); },
})]));
// Columns are fixed; bar heights count the charts that pass every other condition.
function renderRanges(q) {
    const own = { level: { levelRange: null }, length: { lengthRanges: [] } };
    drawKeyFilter(entries, matched, lanes, toggleLanes);
    for (const f of ['level', 'length']) {
        const values = rows => rows.map(r => f === 'level' ? levelAt(r.entry, q.rate).value : fields[f].value(r.entry)).filter(v => v != null);
        const d = fields[f].domain();
        const others = searchValid ? query(entries, { ...q, ...own[f] }).filter(r => lanes.has(r.entry.lanes)) : [];
        rangeWidgets[f].update(d, d && columnCounts(d, values(others)), ranges[f]);
    }
    drawLampFilter(query(entries, { ...q, lamps: null }).filter(r => lanes.has(r.entry.lanes)), progress, lampFilter, toggleLamp);
}

function toggleLamp(k) {
    if (locked()) return;
    if (!lampFilter.delete(k)) lampFilter.add(k);
    refreshList();
}

async function loadProgress() {
    const ticket = ++progressTicket;
    try {
        const summaries = await window.go.main.App.Progress();
        if (ticket !== progressTicket) return;
        progress = new Map(summaries.map(s => [s.hash, s]));
        playedHashes = new Set(progress.keys());
    } catch (e) {
        if (ticket !== progressTicket) return;
        progress = playedHashes = null;
        status.textContent = tr`플레이 기록 읽기 실패: ${e?.message ?? String(e)}`;
    }
}
async function loadPlayer() {
    try {
        const p = await window.go.main.App.Player();
        $('#player-name').textContent = p.name;
        $('#player-pp').textContent = Math.round(p.pp).toLocaleString();
        $('#player-keys').textContent = p.keysToday.toLocaleString();
        const lobbyName = $('#multi-connect input[name="name"]');
        if (lobbyName) lobbyName.value = p.name;
    } catch (e) { status.textContent = tr`프로필 읽기 실패: ${e?.message ?? String(e)}`; }
}
$('#player').addEventListener('click', () => {
    $('#player-input').value = $('#player-name').textContent;
    $('#player-status').textContent = '';
    $('#player-dialog').showModal();
    $('#player-input').select();
});
$('#player-dialog form').addEventListener('submit', async e => {
    if (e.submitter?.value !== 'save') return;
    e.preventDefault();
    try {
        await window.go.main.App.SaveName($('#player-input').value);
        $('#player-dialog').close();
        await loadPlayer();
    } catch (err) { $('#player-status').textContent = err?.message ?? String(err); }
});
window.addEventListener('ggosu:records-changed', async () => {
    loadPlayer();
    await loadProgress();
    if (!locked()) { refreshList(); history(); }
});
async function load() {
    if (locked()) return;
    preview.stop(); // F5 retries repaired or previously unavailable audio, too.
    pending = scanning = true; update();
    $('#list-status').textContent = listStatus();
    try {
        if (!window.go?.main?.App?.Catalog) throw new Error(tr('Wails 앱에서 실행해 주세요.'));
        const catalog = await window.go.main.App.Catalog();
        const previous = selected();
        entries = catalog.entries;
        current = previous ? entries.findIndex(e => e.path === previous.path && e.index === previous.index) : -1;
        const start = initial;
        initial = null;
        if (start) {
            search.value = start.get('q') || '';
            const index = entries.findIndex(e => e.path === start.get('chart'));
            const found = index >= 0 ? entries[index].lanes : search.value ? query(entries, { text: search.value })[0]?.entry.lanes : null;
            if (index >= 0) current = index;
            if (found) lanes = new Set([found]);
        }
        await Promise.all([loadProgress(), loadPlayer()]);
        // Nothing chosen yet (a fresh launch): start on the chart played last.
        let resumed = false;
        if (current < 0 && progress?.size) {
            const last = [...progress.values()].reduce((a, b) => Date.parse(b.lastPlayed) > Date.parse(a.lastPlayed) ? b : a);
            const index = entries.findIndex(e => e.hash === last.hash);
            if (index >= 0) { current = index; lanes = new Set([entries[index].lanes]); resumed = true; }
        }
        integrityNotices((await window.go.main.App.State()).notices);
        pending = scanning = false;
        refreshList();
        loadDuration();
        if ((resumed || start?.has('chart')) && selected()) selectionChanged();
        status.textContent = catalog.errors.join('\n');
        await sync();
        if (start?.get('edit') === '1') {
            if (start.has('chart') && selected()?.path === start.get('chart')) await editor.open(selected(), entries, load);
            else status.textContent = tr('채보를 먼저 고르세요');
        } else if (start?.has('chart') && !entries.some(e => e.path === start.get('chart'))) {
            status.textContent = tr('채보를 찾을 수 없음');
        }
    } catch (e) { status.textContent = e?.message ?? String(e); }
    finally { pending = scanning = false; $('#list-status').textContent = listStatus(); update(); search.focus(); }
}
async function launch(start) {
    if (pending || state.running) return;
    pending = true; update();
    try { await start(); await sync(); }
    catch (e) { cue('error', preferences.effectsVolume()); status.textContent = tr`플레이 실패: ${e?.message ?? String(e)}`; }
    finally { pending = false; update(); }
}
$('#hud-edit').hidden = typeof window.go?.main?.App?.DisplaySettings !== 'function';
$('#hud-edit').addEventListener('click', () => preferences.openHUD());
$('#playing-hud').addEventListener('click',()=>{settingsPanel.open();preferences.opened();preferences.openHUD();});
$('#selection').addEventListener('submit', event => {
    event.preventDefault(); play($('#mode').value);
});
document.addEventListener('pointerdown', () => preview.unlock());
document.addEventListener('keydown', () => preview.unlock());
window.addEventListener('pagehide', () => { preview.stop(); warmSelection(null); });
// Leaving an editor opened from the title goes back to the title.
async function openEditor(fromTitle = false) {
    if (locked() || !selected()) return;
    pending = true; update();
    try { await editor.open(selected(), entries, async () => { await load(); if (fromTitle) showTitle(); }); }
    catch (err) { status.textContent = tr`Edit 열기 실패: ${err?.message ?? err}`; }
    finally { pending = false; update(); }
}
$('#retry').addEventListener('click', () => {
    if (shownRecord && shownRecord !== state.result && selected()) {
        // A past record restarts the selected chart with that record's rate, pitch and lane mod.
        const s = { ...selection(), mod: shownRecord.mods?.kind || '', visibility: shownRecord.mods?.visibility || '', o2jam: shownRecord.mods?.o2jam || '', pills: !!shownRecord.mods?.pills, rate: shownRecord.rate, pitch: shownRecord.pitch !== false, mode: 'live' };
        if (s.mod.includes('random')) s.seed = shownRecord.mods.seed ?? 0;
        launch(() => window.go.main.App.Play(s));
    } else launch(() => window.go.main.App.Retry());
});
$('#watch').addEventListener('click', () => {
    if (!shownRecord) return;
    const s = { ...(shownRecord === state.result ? state.selection : selection()), mode: 'replay', replayID: shownRecord.id };
    launch(() => window.go.main.App.Play(s));
});
$('#back').addEventListener('click', () => {
    $('#result').hidden = true; confirm.undim(); $('#songselect').hidden = false; history(); refreshList(); search.focus();
});
// Esc on the result screen returns to song select, like the back button.
document.addEventListener('keydown', e => {
    if (e.key !== 'Escape' || $('#result').hidden || document.querySelector('dialog[open]') || e.isComposing) return;
    e.preventDefault(); $('#back').click();
});
$('#calibration-undo').addEventListener('click', async () => {
    const c = shownRecord === state.result ? state.result?.calibration : null;
    if (!c || locked()) return;
    $('#calibration-undo').disabled = true;
    try {
        await window.go.main.App.SaveOffsets({}, { globalMS: c.fromMS, chartMS: 0 });
        $('#result-calibration span').textContent = tr('타이밍 조정 취소됨');
        $('#result-adjustment').textContent = tr`전체 ${c.fromMS} ms · 적용 취소`;
        offsetForm.load();
    } catch (e) { $('#calibration-undo').disabled = false; status.textContent = tr`되돌리기 실패: ${e?.message ?? e}`; }
});
window.runtime?.EventsOn('play:changed', () => { sync().catch(e => { status.textContent = String(e); }); });
cursor.mount();
motion.mount();
pulse.mount({ list: $('#song-list'), screen: $('#songselect'), beat: () => preview.beat() });
browser.block();
browser.fullscreen(e => { status.textContent = tr`전체화면 전환 실패: ${e}`; });
notification.mount({ onTop: cursor.raise, onClose: () => cue('pop-out', preferences.effectsVolume()) });
// F12 saves the menu window to data/screenshots. As in osu!lazer's ScreenshotManager the shutter
// sounds on the key press and a notification names the file; clicking it shows the file.
document.addEventListener('keydown', e => {
    const app = window.go?.main?.App;
    if (e.key !== 'F12' || e.repeat || !app?.Screenshot) return;
    cue('shutter', preferences.effectsVolume());
    app.Screenshot().then(path => {
        const name = path.split(/[\\/]/).pop();
        notification.post(`${tr('스크린샷이 저장되었어요!')}\n${name}\n${tr('클릭해서 보기')}`, () => app.RevealScreenshot?.(path).catch(() => {}));
    }).catch(() => {});
});
mountChatSettings($('#chat-settings'));
window.addEventListener('ggosu:language-changed', () => {
    refreshList();
});
// Offset loading calls update synchronously; mount only after the screen state
// (background, filters and hints) has been initialized.
const offsetForm = offsets.mount({ selection: () => selected() ? selection() : null, busy: () => pending || state.running, update, setPending: value => { pending = value; update(); } });
await languageSettings.mount();
preferences.init(update);
window.addEventListener('ggosu:chat-settings', () => { settingsPanel.open(); document.getElementById('tab-chat').click(); });
playback.mount(update);
songFolders.mount();
const downloadPanel = downloads.mount({ busy: () => locked() || scanning, text: () => search.value, installed: async r => { installedPack = r; await selectInstalled(); } });
$('#downloads-open').addEventListener('click', () => { void downloadPanel.open(); });
async function selectInstalled() {
    if (!installedPack || locked() || scanning) return;
    const r = installedPack; installedPack = null;
    search.value = ''; recent = false; ranges.level = null; ranges.length = null; lampFilter.clear(); searchValid = true;
    await load();
    const index = entries.findIndex(e => r.hashes.includes(e.hash));
    if (index >= 0) {
        lanes = new Set([entries[index].lanes]); current = index;
        refreshList(true); select(index);
        $('#result').hidden = true; $('#songselect').hidden = false;
    } else status.textContent = tr('설치한 곡이 목록에 아직 보이지 않습니다. F5로 다시 읽어 주세요.');
}
submissionSettings.mount();
$('#submission-retry').addEventListener('click', async () => {
    if (!shownRecord) return;
    const r = shownRecord, button = $('#submission-retry');
    button.disabled = true;
    try { r.submission = await window.go.main.App.RetrySubmission(r.id); }
    catch (e) { r.submission = { state: 'failed', error: e?.message ?? String(e) }; }
    finally { button.disabled = false; if (shownRecord?.id === r.id) showSubmission(r); }
    await sync();
});
// Adding or removing a song folder rescans the list.
window.addEventListener('ggosu:songs-changed', () => load());
// /play links (web, edit) open song select directly; a normal start shows the title.
if (initial) { $('#title').hidden = true; $('#songselect').hidden = false; } else { $('#title-single').focus(); showPlayer(); }
load();
