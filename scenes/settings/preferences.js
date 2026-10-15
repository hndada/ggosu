import { tr } from '../../preferences/language.js';
import { mount } from '/settings/keytest.js';
import { problem } from '/preferences/keytest.js';
import { defaultDisplay } from '/preferences/display.js';
import { clampScroll, exposureMS } from '/preferences/scroll.js';
import * as cursor from '/cursor.js';
import * as tabs from '/settings/tabs.js';
import * as slider from '/settings/slider.js';
import * as choice from '/settings/choice.js';
import { mount as mountHUD } from '/settings/hudlayout.js';

const $ = s => document.querySelector(s);
// Sliders show their value in the <output> next to them.
const shown = id => { $(`output[for="${id}"]`).value = $('#' + id).value; slider.paint($('#' + id)); };
for (const input of document.querySelectorAll('#settings input[type="range"]')) slider.wrap(input);
choice.wrapAll($('#settings'));
let available = false, loaded = false, changed = false, saving = false, parentBusy = false;
let profile, keyTest;
let scrollRate = 1;
export function setScrollContext(rate) {
    scrollRate = rate;
    if (profile) showOffset();
}
function exposureText() {
    const ms = exposureMS(profile, scrollRate, display);
    return ms === null ? '— ms' : `${Math.round(ms)} ms`;
}
// Every key count has its own profile; the pickers in the key and scroll tabs share this one.
const LANES = Array.from({ length: 10 }, (_, i) => i + 1);
const PICKERS = '.lane-picker button, #key-count';
const COVERS = ['sudden', 'hidden', 'lift'];
let lanes = 4;
let onChange = () => {};
let ticket = 0;
let audio, audioDirty = false, audioReady = false, audioSaving = false;
let audioAvailable = false, audioLoad = 0;
const VOLUMES = ['master', 'music', 'keys', 'effects', 'hit', 'guide'];
const DEFAULT_AUDIO = { master: 100, music: 100, keys: 100, effects: 50, hit: 0, hitSound: 'click', guide: 0 };
let volumeSave = 0;
// Previews and menu sounds play through master, as the game does.
export function musicVolume() { return (audio?.music ?? 100) * (audio?.master ?? 100) / 100; }
export function effectsVolume() { return (audio?.effects ?? 50) * (audio?.master ?? 100) / 100; }
export function volumes() { return audioReady ? { ...audio } : null; }
// The volume overlay changes one volume at once and saves shortly after the last change.
export function setVolume(id, value) {
    if (!audioReady || parentBusy || audioSaving) return false;
    audio[id] = value; $('#audio-' + id).value = value; shown('audio-' + id);
    audioDirty = true; notify();
    clearTimeout(volumeSave); volumeSave = setTimeout(() => saveAudio(), 500);
    return true;
}
let display, displayDirty = false, displayReady = false, displaySaving = false;
let displayAvailable = false;
let hudLayout;
let displayRevision=0;
const DISPLAY = ['noteThickness', 'laneLines', 'ghost', 'ghostScore', 'tps', 'frameLimit', 'dim', 'background', 'storyboard', 'skin', 'hud', 'comboPosition', 'judgementPosition', 'timingThreshold', 'timeDisplay', 'windowMode', 'resolution', 'letterbox', 'letterboxWidth', 'letterboxHeight', 'letterboxX', 'letterboxY', 'cursorSize', 'laneDim', 'noteColour', 'judgementShow', 'hideCombo', 'abort'];

export function needsSave() { return available && (!loaded || changed || saving) || audioAvailable && (!audioReady || audioDirty || audioSaving) || displayAvailable && (!displayReady || displayDirty || displaySaving); }
export function editing() { return available && (changed || saving) || audioAvailable && (audioDirty || audioSaving) || displayAvailable && (displayDirty || displaySaving); }
export function setBusy(value) {
    const wasBusy = parentBusy;
    parentBusy = value;
    if (wasBusy && !value && displayAvailable) reloadDisplay();
    if (wasBusy && !value && audioAvailable) reloadAudio();
    if (displayAvailable) for (const el of document.querySelectorAll('#display-settings :is(input, button, select), #hud-settings :is(input, select), #skin-picker button')) el.disabled = value || displaySaving || !displayReady;
    if (audioAvailable) for (const el of $('#audio-settings').querySelectorAll('input, button, select')) el.disabled = value || audioSaving || !audioReady;
    if (!available) return;
    for (const el of document.querySelectorAll(PICKERS)) el.disabled = value || saving;
    for (const el of $('#preferences').querySelectorAll('input, button, select')) if (!el.matches(PICKERS)) el.disabled = value || saving || !loaded;
    $('#profile-scroll').disabled = value || saving || !loaded || profile?.scrollMode === 'exposure';
    $('#profile-exposure').disabled = value || saving || !loaded || profile?.scrollMode !== 'exposure';
}
function notify() { setBusy(parentBusy); onChange(); }
function dirty() { changed = true; notify(); }
// What play uses: the shared value plus this key count's offset, within the play limits.
const effective = p => ({ scroll: clampScroll(Math.round((p.scrollSpeed + (p.scrollSpeedOffset ?? 0)) * 10) / 10), exposureMS: Math.min(5000, Math.max(100, p.exposureMS + (p.exposureOffset ?? 0))) });
function showOffset() {
    const e = effective(profile);
    $('#scroll-offset-row').hidden = profile.scrollMode === 'exposure';
    $('#exposure-offset-row').hidden = profile.scrollMode !== 'exposure';
    for (const id of ['#scroll-speed-row', '#scroll-speed-keys']) $(id).hidden = profile.scrollMode === 'exposure';
    for (const id of ['#exposure-row', '#exposure-keys']) $(id).hidden = profile.scrollMode !== 'exposure';
    $('#scroll-effective').value = `= ${e.scroll}`;
    $('#exposure-effective').value = `= ${e.exposureMS} ms`;
    $('#scroll-exposure').value = `≈ ${exposureText()}`;
}
function render(p) {
    profile = { ...p, scrollSpeedOffset: p.scrollSpeedOffset ?? 0, exposureOffset: p.exposureOffset ?? 0 };
    $('#profile-scroll').value = p.scrollSpeed;
    $('#profile-scroll-mode').value = p.scrollMode;
    $('#profile-exposure').value = p.exposureMS;
    $('#profile-speed-change').value = p.speedChange;
    $('#profile-scroll-offset').value = profile.scrollSpeedOffset;
    $('#profile-exposure-offset').value = profile.exposureOffset;
    for (const k of COVERS) {
        $(`#cover-${k}`).value = Math.round((p.cover?.[k] ?? 0) * 1000) / 10;
        $(`#cover-${k}-on`).checked = !!p.cover?.[`${k}On`];
    }
    showOffset();
    if (LANES.includes(lanes)) keyTest.setKeys(p.keys);
    else keyTest.stopCapture();
    changed = false; loaded = true; notify();
}
async function load() {
    const t = ++ticket; loaded = false; notify();
    try {
        const p = await window.go.main.App.Profile(lanes);
        if (t !== ticket) return;
        render(p); $('#profile-status').textContent = '';
    } catch (e) { if (t === ticket) $('#profile-status').textContent = tr`설정 읽기 실패: ${e?.message ?? e}`; }
}
async function saveProfile() {
    if (parentBusy || saving || !loaded || !changed || !$('#preferences').reportValidity()) return false;
    const issue = problem(profile.keys);
    if (issue) { $('#profile-status').textContent = tr`설정 저장 실패: ${issue}`; return false; }
    saving = true; notify();
    try {
        render(await window.go.main.App.SaveProfile(lanes, profile));
        $('#profile-status').textContent = tr('저장했습니다. 다음 플레이부터 적용합니다.');
        return true;
    } catch (e) { $('#profile-status').textContent = tr`설정 저장 실패: ${e?.message ?? e}`; return false; }
    finally { saving = false; notify(); }
}
// PgUp/PgDn on song select: the same steps as in game, saved at once for the current key count.
export async function nudgeScroll(faster, fine = false) {
    if (!available || !loaded || parentBusy || saving) return '';
    // The shared value moves; the shown value includes this key count's offset.
    if (profile.scrollMode === 'exposure') profile.exposureMS = faster ? Math.max(100, profile.exposureMS - 25) : Math.min(5000, profile.exposureMS + 25);
    else profile.scrollSpeed = clampScroll(Math.round((profile.scrollSpeed + (faster ? 1 : -1) * (fine ? .1 : 1)) * 10) / 10);
    changed = true;
    if (!await saveProfile()) return $('#profile-status').textContent;
    const e = effective(profile);
    return profile.scrollMode === 'exposure' ? tr`노출 시간 ${e.exposureMS} ms` : tr`스크롤 속도 ${e.scroll} · 기준 노출 ${exposureText()}`;
}
function showLanes() {
    for (const b of document.querySelectorAll('.lane-picker button')) b.setAttribute('aria-pressed', String(Number(b.value) === lanes));
    $('#key-count').value = String(lanes);
    $('#key-binding').hidden = !LANES.includes(lanes);
}
// Picking a key count shows its saved layout without starting key capture.
async function pick(n) {
    if ((n === lanes && loaded) || saving || parentBusy) return;
    keyTest.stopCapture(); lanes = n; showLanes(); await load();
}
// The settings dialog just opened: show the saved values again.
export function opened() {
    tabs.refresh();
    if (available && !changed && !saving) load();
}
// Play may have changed the scroll speed (PgUp/PgDn); show what the menu saved.
export function reload() { if (available && !changed && !saving) load(); }
export function select(n) {
	 hudLayout?.setLanes(n);
    if (!available || changed || saving || lanes === n) return;
    lanes = n; showLanes(); load();
}
export function init(callback) {
    onChange = callback;
    tabs.mount();
    available = typeof window.go?.main?.App?.Profile === 'function';
    audioAvailable = typeof window.go?.main?.App?.AudioSettings === 'function';
    displayAvailable = typeof window.go?.main?.App?.DisplaySettings === 'function';
    if (displayAvailable) {
        hudLayout = mountHUD({current:()=>display??defaultDisplay(),change:d=>{display=d;displayDirty=true;displayRevision++;notify();},save:()=>saveDisplay(false,true)});
        $('#display-settings').hidden = false; $('#hud-settings').hidden = false;
        // Fields missing from older settings take their defaults.
        window.go.main.App.DisplaySettings().then(d => { display = { ...defaultDisplay(), ...d }; renderDisplay(); }).catch(e => { $('#display-status').textContent = tr`설정 읽기 실패: ${e}`; });
        for (const id of DISPLAY) $('#display-' + id).addEventListener('input', () => {
            const input = $('#display-' + id);
            if (id === 'comboPosition' && display.hudLayout) delete display.hudLayout.combo;
            if (id === 'judgementPosition' && display.hudLayout) delete display.hudLayout.judgement;
            display[id] = id === 'tps' || id === 'ghostScore' ? Number(input.value) : input.type === 'range' ? input.valueAsNumber : input.tagName === 'SELECT' ? input.value : input.checked;
            if (input.type === 'range') shown('display-' + id);
            if (id === 'letterbox') $('#display-letterbox-area').hidden = !input.checked;
            if (id === 'cursorSize') cursor.setSize(input.valueAsNumber);
            // The menu background follows the dim slider while it moves.
            if (id === 'dim') { $('#backdrop').classList.add('instant'); $('#backdrop').dataset.dim = input.value; }
            displayDirty = true; notify();
            displayRevision++;
            hudLayout?.render();
            if (profile) showOffset();
        });
        // Changes apply at once; a slider saves when it is let go.
        for (const id of DISPLAY) $('#display-' + id).addEventListener('change', () => saveDisplay());
        $('#display-dim').addEventListener('change', () => $('#backdrop').classList.remove('instant'));
        // A skin picture sets the hidden select, which saves like every other display field.
        for (const b of document.querySelectorAll('#skin-picker button')) b.addEventListener('click', () => {
            const select = $('#display-skin');
            if (select.disabled || select.value === b.value) return;
            select.value = b.value;
            select.dispatchEvent(new Event('input')); select.dispatchEvent(new Event('change'));
        });
        $('#display-settings').addEventListener('submit', e => { e.preventDefault(); saveDisplay(); });
        $('#display-reset').addEventListener('click', () => { display = defaultDisplay(); displayDirty = true; saveDisplay(true); });
    }
    if (audioAvailable) {
        $('#audio-settings').hidden = false;
        reloadAudio();
        for (const id of VOLUMES) $('#audio-' + id).addEventListener('input', () => { audio[id] = $('#audio-' + id).valueAsNumber; shown('audio-' + id); audioDirty = true; notify(); });
        for (const id of VOLUMES) $('#audio-' + id).addEventListener('change', () => saveAudio());
        $('#audio-hitSound').addEventListener('change', () => { audio.hitSound = $('#audio-hitSound').value; audioDirty = true; saveAudio(); });
        $('#audio-settings').addEventListener('submit', e => { e.preventDefault(); saveAudio(); });
        $('#audio-reset').addEventListener('click', () => { audio = { ...DEFAULT_AUDIO }; audioDirty = true; saveAudio(true); });
    }
    if (!available) return;
    $('#preferences').hidden = false;
    keyTest = mount({
        root: $('#key-binding'), grid: $('#lane-keys'), pad: $('#key-pad'), status: $('#key-status'), chord: $('#key-chord'),
        chordReset: $('#key-chord-reset'), test: $('#key-test'), dialog: $('#settings'),
        onBind: (i, key) => { profile.keys[i] = key; dirty(); },
        onComplete: () => saveProfile(),
    });
    $('#key-again').addEventListener('click', () => { keyTest.capture(0); $('#key-pad').focus(); });
    // All supported counts are visible together, without an extra fold.
    for (const picker of document.querySelectorAll('.lane-picker')) {
        for (const n of LANES) {
            const b = document.createElement('button'); b.type = 'button'; b.value = n; b.textContent = `${n}K`;
            b.addEventListener('click', () => pick(n));
            picker.append(b);
        }
    }
    const count = $('#key-count');
    for (const n of LANES) count.add(new Option(`${n}K`, n));
    count.addEventListener('change', () => pick(Number(count.value)));
    showLanes();
    // Leaving mid-way drops the half-entered keys so play is not blocked by an unsaved profile.
    $('#settings').addEventListener('close', () => { if (changed && !saving) load(); });
    // Number fields save when committed (Enter or leaving the field); the select saves at once.
    const field = (id, key, read) => {
        $(id).addEventListener('input', () => { profile[key] = read(); dirty(); });
        $(id).addEventListener('change', () => saveProfile());
        // Arrow-key adjustments set .value in code, so the browser may not
        // generate change on blur. Commit the dirty profile explicitly.
        $(id).addEventListener('blur', () => saveProfile());
        $(id).addEventListener('keydown', e => { if (e.key === 'Enter') { e.preventDefault(); saveProfile(); } });
    };
    field('#profile-scroll', 'scrollSpeed', () => $('#profile-scroll').valueAsNumber);
    field('#profile-scroll-mode', 'scrollMode', () => $('#profile-scroll-mode').value);
    field('#profile-exposure', 'exposureMS', () => $('#profile-exposure').valueAsNumber);
    field('#profile-speed-change', 'speedChange', () => $('#profile-speed-change').valueAsNumber);
    field('#profile-scroll-offset', 'scrollSpeedOffset', () => $('#profile-scroll-offset').valueAsNumber);
    field('#profile-exposure-offset', 'exposureOffset', () => $('#profile-exposure-offset').valueAsNumber);
    for (const k of COVERS) {
        field(`#cover-${k}`, 'cover', () => ({ ...profile.cover, [k]: $(`#cover-${k}`).valueAsNumber / 100 }));
        field(`#cover-${k}-on`, 'cover', () => ({ ...profile.cover, [`${k}On`]: $(`#cover-${k}-on`).checked }));
    }
    for (const id of ['#profile-scroll', '#profile-scroll-offset']) $(id).addEventListener('keydown', e => {
        const direction = { ArrowUp: 1, ArrowDown: -1, PageUp: 1, PageDown: -1 }[e.key];
        const input = $(id);
        const page = e.key.startsWith('Page');
        if (!direction || input.disabled || e.ctrlKey || (e.altKey && !page) || e.metaKey || !Number.isFinite(input.valueAsNumber)) return;
        e.preventDefault();
        input.value = Math.min(Number(input.max), Math.max(Number(input.min), Math.round((input.valueAsNumber + direction * ((page ? e.altKey : e.shiftKey) ? .1 : 1)) * 10) / 10));
        input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    for (const id of ['#profile-scroll', '#profile-scroll-mode', '#profile-exposure', '#profile-scroll-offset', '#profile-exposure-offset']) $(id).addEventListener('input', showOffset);
    $('#preferences').addEventListener('submit', e => { e.preventDefault(); saveProfile(); });
    $('#profile-reset').addEventListener('click', async () => {
        if (parentBusy || saving) return;
        saving = true; notify();
        try { render(await window.go.main.App.ResetProfile(lanes)); keyTest.say(tr`${lanes}K 키·스크롤 설정을 초기화했습니다.`); }
        catch (e) { $('#profile-status').textContent = tr`초기화 실패: ${e?.message ?? e}`; }
        finally { saving = false; notify(); }
    });
    load();
}
function renderAudio() {
    audio = { ...DEFAULT_AUDIO, ...audio };
    for (const id of VOLUMES) { $('#audio-' + id).value = audio[id]; shown('audio-' + id); }
    $('#audio-hitSound').value = audio.hitSound || 'click';
    audioReady = true; audioDirty = false; notify();
}
async function saveAudio(reset = false) {
    if (parentBusy || audioSaving || !audioReady || (!reset && !$('#audio-settings').reportValidity())) return;
    audioSaving = true; notify();
    try { audio = await window.go.main.App.SaveAudio(audio); renderAudio(); $('#audio-status').textContent = tr('저장했습니다.'); }
    catch (e) { $('#audio-status').textContent = tr`설정 저장 실패: ${e?.message ?? e}`; }
    finally { audioSaving = false; notify(); }
}
function renderDisplay() {
	 hudLayout?.render();
	$('#display-laneLines').checked = display.laneLines ?? false;
    $('#display-ghost').value = display.ghost || 'target';
    $('#display-ghostScore').value = display.ghostScore || 900000;
    const ticks = $('#display-tps');
    const tps = String(display.tps || 240);
    if (![...ticks.options].some(o => o.value === tps)) ticks.add(new Option(tps, tps));
    ticks.value = tps;
    $('#display-frameLimit').value = display.frameLimit || 'vsync';
    $('#display-dim').value = display.dim; shown('display-dim');
    $('#backdrop').dataset.dim = display.dim;
    $('#display-background').checked = display.background;
    $('#display-storyboard').checked = display.storyboard;
    $('#display-skin').value = display.skin || 'argon';
    for (const b of document.querySelectorAll('#skin-picker button')) b.setAttribute('aria-pressed', String(b.value === $('#display-skin').value));
    $('#display-hud').value = display.hud || 'standard';
    $('#display-noteThickness').value = display.noteThickness || 100; shown('display-noteThickness');
    $('#display-timeDisplay').value = display.timeDisplay || 'pie';
    $('#display-comboPosition').value = display.comboPosition || 'center';
    $('#display-judgementPosition').value = display.judgementPosition || 'center';
    $('#display-timingThreshold').value = display.timingThreshold || 'cool';
    $('#display-laneDim').value = display.laneDim ?? 0; shown('display-laneDim');
    $('#display-noteColour').value = display.noteColour ?? '';
    $('#display-judgementShow').value = display.judgementShow ?? '';
    $('#display-hideCombo').checked = display.hideCombo ?? false;
    $('#display-abort').checked = display.abort ?? false;
    $('#display-windowMode').value = display.windowMode || 'windowed';
    $('#display-resolution').value = display.resolution || '1600x900';
    $('#display-letterbox').checked = display.letterbox;
    $('#display-letterbox-area').hidden = !display.letterbox;
    for (const id of ['letterboxWidth', 'letterboxHeight', 'letterboxX', 'letterboxY', 'cursorSize']) { $('#display-' + id).value = display[id]; shown('display-' + id); }
    cursor.setSize(display.cursorSize);
    displayReady = true; displayDirty = false; notify();
    if (profile) showOffset();
}
async function saveDisplay(reset = false, live = false) {
    if (!displayReady || (!reset && !$('#display-settings').reportValidity())) return false;
    if (!live && parentBusy || displaySaving) return;
    displaySaving = true; notify();
    const revision=displayRevision, snapshot=structuredClone(display);
    try {
        const saved=await window.go.main.App.SaveDisplay(snapshot);
        if(revision===displayRevision) { display=saved;renderDisplay(); }
        else setTimeout(()=>saveDisplay(false,true),250);
        $('#display-status').textContent=tr('저장했습니다.');return true;
    }
    catch (e) { $('#display-status').textContent = tr`설정 저장 실패: ${e?.message ?? e}`;return false; }
    finally { displaySaving = false; notify(); }
}

export function openHUD() { tabs.show($('#tab-hud-layout')); hudLayout?.sync(); }

let displayLoad = 0;
function reloadDisplay() {
    const ticket = ++displayLoad;
    displayReady = false;
    window.go.main.App.DisplaySettings().then(d => {
        if (ticket !== displayLoad) return;
        display = { ...defaultDisplay(), ...d }; renderDisplay();
    }).catch(e => { $('#display-status').textContent = `HUD settings reload failed: ${e}`; });
}
window.addEventListener('ggosu:display-changed', () => { if (displayAvailable) { reloadDisplay(); notify(); } });

function reloadAudio() {
    const ticket = ++audioLoad;
    audioReady = false;
    window.go.main.App.AudioSettings().then(a => {
        if (ticket !== audioLoad) return;
        audio = a; renderAudio();
    }).catch(e => { $('#audio-status').textContent = tr`설정 읽기 실패: ${e}`; });
}
window.addEventListener('ggosu:audio-changed', () => { if (audioAvailable) reloadAudio(); });
