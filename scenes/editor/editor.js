import { tr } from '../../preferences/language.js';
import { Draft, ms, ns, snap, ticks, position, issues } from '/chartedit/edit.js';
import { mountSync } from '/editor/sync.js';
import { click } from '/chartedit/sync.js';
import { eventTime, inputPosition } from '/chartedit/beatfit.js';
import { FIELD, lanes } from '/chartedit/field.js';

const root = document.querySelector('#editor');
const $ = id => root.querySelector('#edit-' + id);
const svgNS = 'http://www.w3.org/2000/svg';
// Imported beat lengths leave float noise (199.9998); show two decimals at most.
const num = x => String(Number(x.toFixed(2)));
// An untouched field keeps the stored value instead of its rounded display.
const kept = (input, stored) => input.valueAsNumber === Number(num(stored)) ? stored : input.valueAsNumber;
let draft, entry, catalog, onClose, loading = false, busy = false, ticket = 0;
const ZOOMS = [25, 50, 75, 100, 150, 200, 300, 400];
let cursor = 0, tool = 'select', timingIndex = 0, zoom = 100, duration = 10000, gesture;
let playing = false, testing = false, context, startedAt, startedCursor, sources = [], frame, playTicket = 0, activeRate = 1;
let buffers = new Map(), peaks = [], waveLength = 0, audioText = '', bookmarks = [];
let previewTiming, gridWidth = 1000, widthScale = 1, reference, referenceKey = '';
let mapLayer = document.createElementNS(svgNS, 'g');
const pressed = new Set();
const sel = e => ({ folder: e.folder, chartID: e.path, index: e.index });

// Transport follows Quaver's EditorFooter: restart, previous bookmark, play/pause, next bookmark, skip to end.
const glyph = d => `<svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="${d}"/></svg>`;
const icon = { first: glyph('M3 2h2v12H3zM14 2v12L6 8z'), prev: glyph('M8 2v12L1 8zM15 2v12L8 8z'), play: glyph('M4 2v12l10-6z'), pause: glyph('M4 2h3v12H4zM10 2h3v12h-3z'), next: glyph('M1 2v12l7-6zM8 2v12l7-6z'), last: glyph('M2 2v12l8-6zM11 2h2v12h-2z') };
root.innerHTML = tr`
<header><button id="edit-back" type="button">곡 선택</button><h2 id="edit-title">Edit</h2><span id="edit-saved"></span><button id="edit-save" type="button">사본 저장</button><button id="edit-export" type="button">.osu 내보내기</button><button id="edit-fold" type="button" aria-pressed="false">목록 접기</button></header>
<nav id="edit-toolbar" aria-label="편집 도구">
<button id="edit-sync" type="button">음원 분석·장치 보정</button>
<button id="edit-select" type="button" aria-pressed="true">선택 [Q]</button><button id="edit-tap" type="button" aria-pressed="false">Tap [W]</button><button id="edit-hold" type="button" aria-pressed="false">Hold [E]</button>
<button id="edit-mirror" type="button">좌우 반전</button>
<label>Snap <select id="edit-snap"><option value="1">1/1 beat</option><option value="2">1/2 beat</option><option value="3">1/3 beat</option><option value="4" selected>1/4 beat</option><option value="6">1/6 beat</option><option value="8">1/8 beat</option><option value="12">1/12 beat</option><option value="16">1/16 beat</option><option value="0">자유</option></select></label>
<label>Width <select id="edit-width"><option>50</option><option>75</option><option selected>100</option><option>125</option><option>150</option><option>200</option></select> %</label>
</nav>
<div id="edit-body"><aside id="edit-charts" aria-label="같은 곡의 채보"><h3>채보</h3><ul id="edit-chart-list"></ul><h3>북마크</h3><button id="edit-bookmark" type="button">현재 위치 추가</button><ul id="edit-bookmarks"></ul></aside>
<div id="edit-field"><svg id="edit-grid" viewBox="0 0 1000 720" preserveAspectRatio="none" role="img" aria-label="세로 채보 레인" tabindex="0"></svg></div>
<svg id="edit-map" viewBox="0 0 44 720" preserveAspectRatio="none" role="img" aria-label="곡 전체 미니맵" tabindex="0"></svg>
<aside id="edit-inspector"><h3>선택 노트</h3><p id="edit-selected"></p>
<form id="edit-note"><label>레인<input id="edit-lane" type="number" min="1" step="1" required></label><label>종류<select id="edit-kind"><option value="tap">Tap</option><option value="hold">Hold</option><option value="roll">Roll</option><option value="mine">Mine</option><option value="lift">Lift</option><option value="fake">Fake</option></select></label><label>시작 (ms)<input id="edit-at" type="number" min="-86400000" max="86400000" step="any" required></label><label>끝 (ms)<input id="edit-end" type="number" min="-86400000" max="86400000" step="any" required></label></form>
<h3>타이밍</h3><form id="edit-timing"><label>이벤트<select id="edit-point"></select></label><label>위치 (ms)<input id="edit-timing-at" type="number" min="-86400000" max="86400000" step="any" required></label><label>BPM<input id="edit-bpm" type="number" min="1" max="2000" step="any" required></label><label>스크롤 배율<input id="edit-scroll" type="number" min="0" max="100" step="any" required></label><label>박자 (분자)<input id="edit-meter" type="number" min="1" max="32" step="1" required></label><label>정지 (ms)<input id="edit-stop" type="number" min="0" max="86400000" step="any" required></label><button id="edit-point-add" type="button">현재 위치에 추가</button><button id="edit-point-delete" type="button">삭제</button></form>
<h3>채보 정보</h3><form id="edit-meta"><label>난이도 이름<input id="edit-name" required maxlength="160"></label><label>제작자<input id="edit-creator" maxlength="160"></label></form>
<h3 id="edit-check">검사</h3><ol id="edit-issues"></ol><p id="edit-audio-status"></p></aside></div>
<footer id="edit-transport"><div id="edit-deck"><button id="edit-first" type="button" aria-label="처음">${icon.first}</button><button id="edit-prev" type="button" aria-label="이전 북마크">${icon.prev}</button><button id="edit-play" type="button" aria-label="재생">${icon.play}</button><button id="edit-next" type="button" aria-label="다음 북마크">${icon.next}</button><button id="edit-last" type="button" aria-label="끝">${icon.last}</button></div><button id="edit-test" type="button">Test [T]</button>
<label><input id="edit-metronome" type="checkbox">메트로놈</label><label><input id="edit-record" type="checkbox">Test 중 Tap 기록</label>
<label>위치 (ms) <input id="edit-position" type="number" min="-86400000" max="86400000" step="any" value="0"></label><span id="edit-time"></span>
<label>속도 <select id="edit-rate"><option value="0.5">0.5×</option><option value="0.75">0.75×</option><option value="1" selected>1×</option><option value="1.5">1.5×</option><option value="2">2×</option></select></label>
<p id="edit-message" role="status" aria-live="polite"></p></footer>
<dialog id="edit-leave"><p>저장하지 않은 변경이 있습니다.</p><button id="edit-keep" type="button">계속 편집</button><button id="edit-discard" type="button">변경 버리고 이동</button></dialog>`;

const sync = mountSync(root, {
    context: () => context ||= new AudioContext(), pause,
    apply(p) {
        if (!draft || busy || playing || loading) return;
        draft.change(c => { const point = c.timing[timingIndex]; point.at_ns = ns(p.at); point.bpm = p.bpm; c.timing.sort((a, b) => a.at_ns - b.at_ns); timingIndex = c.timing.indexOf(point); });
        update(); message(tr('선택한 타이밍의 BPM·시작 위치를 적용했습니다. 기존 스크롤 배율·박자·정지와 노트 시각은 유지합니다. Undo로 복구할 수 있습니다.'));
    },
    listen(p) {
        cursor = p.at; previewTiming = [{ at_ns: ns(p.at), bpm: p.bpm, scroll: 1, meter: draft.c.timing[timingIndex].meter }];
        $('metronome').checked = true; message(tr('후보 메트로놈 미리듣기입니다. 채보는 변경하지 않았습니다.')); start().catch(e => message(e.message));
    },
    seek,
});
$('sync').onclick = () => { if (draft && !busy && !loading) sync.open(); };

function message(text) { $('message').textContent = text; }
function element(tag, attrs = {}, text) {
    const e = document.createElementNS(svgNS, tag);
    for (const [key, value] of Object.entries(attrs)) e.setAttribute(key, value);
    if (text != null) e.textContent = text;
    return e;
}
function line(parent, x1, y1, x2, y2, stroke = 'var(--line)', width = 1) { parent.append(element('line', { x1, y1, x2, y2, stroke, 'stroke-width': width, 'pointer-events': 'none' })); }
function text(parent, x, y, value, color = 'var(--dim)') { parent.append(element('text', { x, y, fill: color }, value)); }
const rect = (parent, x, y, width, height, fill, attrs = {}) => parent.append(element('rect', { x, y, width, height, fill, ...attrs }));
function bounds() {
    const bpm = position(draft.c.timing, cursor).bpm;
    const scale = 96 * zoom / 100 * bpm / 60000;
    return { scale, y: at => FIELD.line - (at - cursor) * scale, at: y => cursor + (FIELD.line - y) / scale, from: cursor - (720 - FIELD.line) / scale, to: cursor + FIELD.line / scale };
}
// The grid keeps play's 720-unit height without stretching. Lanes are centered like play beside a 160-unit label gutter;
// a reference chart sits to the right at the same time scale.
function field() {
    const r = $('grid').getBoundingClientRect();
    const width = r.width && r.height ? 720 * r.width / r.height : 1000;
    if (width !== gridWidth) { gridWidth = width; $('grid').setAttribute('viewBox', `0 0 ${width} 720`); }
    const f = lanes(draft.c, widthScale), ref = reference && lanes(reference, widthScale);
    const left = Math.max(160, (gridWidth - f.width - (ref ? 40 + ref.width : 0)) / 2);
    const place = (g, at) => ({ left: at, width: g.width, lanes: g.lanes.map(l => ({ x: at + l.x, width: l.width })) });
    return { ...place(f, left), ref: ref && place(ref, left + f.width + 40) };
}
const mapY = at => 720 - at / duration * 720;
function draw() {
    if (!draft || root.hidden) return;
    const c = draft.c, grid = $('grid'), map = $('map'), b = bounds(), f = field();
    grid.replaceChildren(); map.replaceChildren(mapLayer);
    if (peaks.length) {
        for (let i = 0; i < peaks.length; i++) {
            const at = i / peaks.length * waveLength, y = b.y(at);
            if (y >= 0 && y <= 720) line(grid, f.left + f.width / 2 - peaks[i] * f.width / 2, y, f.left + f.width / 2 + peaks[i] * f.width / 2, y, 'var(--line)', 2);
        }
    }
    for (const tick of ticks(previewTiming || c.timing, b.from, b.to, Number($('snap').value) || 4)) {
        const y = b.y(tick.at); line(grid, f.left - 15, y, f.left + f.width, y, tick.major ? 'var(--dim)' : 'var(--line)');
        if (tick.major) { const p = position(previewTiming || c.timing, tick.at); text(grid, f.left - 20, y - 3, `${Math.floor(p.bar) + 1}:${Math.floor((p.bar % 1 + 1) % 1 * p.meter + 1e-5) + 1}`); grid.lastChild.setAttribute('text-anchor', 'end'); }
    }
    for (const l of f.lanes) { line(grid, l.x, 0, l.x, FIELD.bottom); line(grid, l.x + l.width, 0, l.x + l.width, FIELD.bottom); }
    if (f.ref) {
        for (const l of f.ref.lanes) { line(grid, l.x, 0, l.x, FIELD.bottom); line(grid, l.x + l.width, 0, l.x + l.width, FIELD.bottom); }
        for (const n of reference.notes) {
            const l = f.ref.lanes[n.lane], at = ms(n.at_ns), end = ms(n.end_ns);
            if (!l || b.y(end) > 730 || b.y(at) < -10) continue;
            if (end > at) rect(grid, l.x + l.width * .25, b.y(end), l.width * .5, b.y(at) - b.y(end), 'var(--line)', { 'pointer-events': 'none' });
            rect(grid, l.x + 3, b.y(at) - 10, l.width - 6, 10, 'var(--line)', { stroke: 'var(--dim)', 'pointer-events': 'none' });
        }
        rect(grid, f.ref.left, FIELD.line, f.ref.width, 3, 'var(--dim)');
        text(grid, f.ref.left, FIELD.line + 32, reference.metadata.difficulty);
    }
    c.timing.forEach((p, i) => {
        const y = b.y(ms(p.at_ns)); if (y < 15 || y > 715) return;
        text(grid, f.left - 60, y, p.scroll === 1 ? num(p.bpm) : `${num(p.bpm)} ×${num(p.scroll)}`, 'var(--text)');
        grid.lastChild.dataset.point = i; grid.lastChild.setAttribute('text-anchor', 'end');
    });
    for (const n of c.notes) {
        const at = ms(n.at_ns), end = ms(n.end_ns), top = Math.max(0, b.y(end)), bottom = Math.min(720, b.y(at));
        const l = f.lanes[n.lane], x = l.x + 3, w = l.width - 6, selected = draft.selected.has(n.id);
        if (bottom >= 0 && top - 10 <= 720 && bottom >= top) {
            // Shapes follow internal/play drawField: half-width body, 5-unit tail, 10-unit head ending at its time.
            if (end > at) { rect(grid, l.x + l.width * .25, top, l.width * .5, Math.max(3, bottom - top), 'var(--dim)', { 'data-id': n.id }); rect(grid, x, b.y(end), w, 5, 'var(--dim)', { 'data-id': n.id }); }
            rect(grid, x, b.y(at) - 10, w, 10, selected ? 'var(--text)' : 'var(--dim)', { 'data-id': n.id, stroke: 'var(--text)', 'stroke-width': selected ? 3 : 1 });
            if (selected && end > at) rect(grid, x, b.y(end) - 3, w, 10, 'var(--text)', { 'data-id': n.id, 'data-tail': '1' });
            if (!['tap', 'hold'].includes(n.kind)) text(grid, x, b.y(at) - 13, n.kind, 'var(--text)');
        }
    }
    if (gesture) {
        const x1 = Math.min(gesture.x, gesture.lastX), y1 = Math.min(gesture.y, gesture.lastY);
        if (gesture.tool === 'select' && !gesture.id) rect(grid, x1, y1, Math.abs(gesture.x - gesture.lastX), Math.abs(gesture.y - gesture.lastY), 'none', { stroke: 'var(--text)' });
        const l = f.lanes[gesture.lane];
        if (gesture.tool === 'hold') rect(grid, l.x + l.width * .25, Math.min(gesture.y, gesture.lastY), l.width * .5, Math.max(10, Math.abs(gesture.y - gesture.lastY)), 'none', { stroke: 'var(--text)' });
    }
    // Like play, heads that pass the line hide under the key area.
    rect(grid, f.left, FIELD.line, f.width, FIELD.bottom - FIELD.line, 'var(--field)', { 'pointer-events': 'none' });
    rect(grid, f.left, FIELD.line, f.width, 3, 'var(--text)');
    f.lanes.forEach((l, lane) => {
        if (pressed.has(lane)) rect(grid, l.x + 2, FIELD.line + 4, l.width - 4, FIELD.bottom - FIELD.line - 4, 'var(--dim)');
        text(grid, l.x + 5, FIELD.line + 32, String(lane + 1), 'var(--text)');
    });
    // The minimap runs bottom (start) to top (end), the same direction notes travel in the field.
    rect(map, 0, Math.max(0, mapY(b.to)), 44, Math.min(720, mapY(b.from)) - Math.max(0, mapY(b.to)), 'none', { stroke: 'var(--text)', 'stroke-width': 2 });
    line(map, 0, mapY(cursor), 44, mapY(cursor), 'var(--text)', 3);
    for (const at of bookmarks) line(map, 0, mapY(at), 12, mapY(at), 'var(--text)', 2);
    const p = position(previewTiming || c.timing, cursor);
    $('time').textContent = tr`${(cursor / 1000).toFixed(3)} s · ${Math.floor(p.bar) + 1}마디 · ${num(p.bpm)} BPM · ${zoom}%`;
    if (document.activeElement !== $('position')) $('position').value = cursor.toFixed(3);
}
function rebuildMap() {
    mapLayer = document.createElementNS(svgNS, 'g');
    for (const n of draft.c.notes) rect(mapLayer, n.lane / draft.c.lanes * 44, Math.max(0, mapY(ms(n.end_ns))), Math.max(1, 44 / draft.c.lanes), Math.max(1.2, ms(n.end_ns - n.at_ns) / duration * 720), 'var(--dim)');
}
function update() {
    if (!draft) return;
    duration = draft.c.notes.reduce((end, n) => Math.max(end, ms(n.end_ns) + 3000), Math.max(10000, waveLength));
    rebuildMap();
    $('title').textContent = `${draft.c.metadata.title} / ${draft.c.metadata.difficulty} · ${draft.c.lanes}K`;
    $('saved').textContent = draft.dirty ? tr('● 저장 안 됨') : '';
    const errors = issues(draft.c), notes = draft.c.notes.filter(n => draft.selected.has(n.id));
    $('save').disabled = $('export').disabled = busy || loading || errors.length > 0;
    $('sync').disabled = busy || loading; $('record').disabled = busy || playing;
    $('mirror').disabled = busy || playing || !notes.length;
    $('selected').textContent = `${notes.length} / ${draft.c.notes.length}`;
    $('note').hidden = notes.length !== 1;
    for (const e of $('note').elements) e.disabled = busy || playing;
    if (notes.length === 1) {
        const n = notes[0]; $('lane').value = n.lane + 1; $('lane').max = draft.c.lanes;
        $('kind').value = n.kind; $('at').value = ms(n.at_ns); $('end').value = ms(n.end_ns);
    }
    $('point').replaceChildren(...draft.c.timing.map((p, i) => { const o = document.createElement('option'); o.value = i; o.textContent = `${ms(p.at_ns).toFixed(1)} ms · ${num(p.bpm)} BPM`; return o; }));
    timingIndex = Math.min(timingIndex, draft.c.timing.length - 1); $('point').value = timingIndex; timingForm();
    $('name').value = draft.c.metadata.difficulty; $('creator').value = draft.c.metadata.creator || '';
    for (const form of ['timing', 'meta']) for (const e of $(form).elements) e.disabled = busy || playing;
    $('point-delete').disabled = busy || playing || draft.c.timing.length === 1;
    $('issues').replaceChildren(...(errors.length ? errors.slice(0, 50).map(issue => {
        const li = document.createElement('li'), b = document.createElement('button'); b.type = 'button'; b.textContent = `${issue.at.toFixed(1)} ms: ${issue.text}`;
        b.onclick = () => { seek(issue.at); draft.selected = new Set([issue.id]); update(); }; li.append(b); return li;
    }) : []));
    $('check').textContent = tr`검사 ${errors.length}`;
    $('audio-status').textContent = audioText;
    $('play').innerHTML = playing ? icon.pause : icon.play; $('play').setAttribute('aria-label', playing ? tr('일시정지') : tr('재생'));
    $('test').textContent = testing ? tr('Test 종료 [Esc]') : 'Test [T]';
    draw();
}
function timingForm() {
    const p = draft.c.timing[timingIndex];
    $('timing-at').value = ms(p.at_ns); $('bpm').value = num(p.bpm); $('scroll').value = num(p.scroll); $('meter').value = p.meter; $('stop').value = ms(p.stop_ns || 0);
}
function setTool(name) { tool = name; for (const n of ['select', 'tap', 'hold']) $(n).setAttribute('aria-pressed', String(n === name)); }
function seek(at) {
    const was = playing; pause(); cursor = Math.max(-86400000, Math.min(duration, at)); draw(); if (was) start();
}
function xy(event, svg) {
    const r = svg.getBoundingClientRect();
    return { x: (event.clientX - r.left) / r.width * (svg === $('map') ? 44 : gridWidth), y: (event.clientY - r.top) / r.height * 720 };
}
const quantize = at => snap(draft.c.timing, at, Number($('snap').value));
function laneAt(x) {
    const f = field(), i = f.lanes.findIndex(l => x < l.x + l.width);
    return i < 0 ? f.lanes.length - 1 : i;
}
$('grid').addEventListener('pointerdown', e => {
    if (!draft || busy || playing || testing || e.button !== 0) return;
    if (e.target.dataset.point != null) { timingIndex = Number(e.target.dataset.point); $('point').value = timingIndex; timingForm(); return; }
    const p = xy(e, $('grid')); if (p.x < field().left) { seek(quantize(bounds().at(p.y))); return; }
    const id = Number(e.target.dataset.id), n = draft.c.notes.find(n => n.id === id);
    if (tool === 'select' && n) {
        if (e.ctrlKey || e.metaKey) { if (draft.selected.has(id)) draft.selected.delete(id); else draft.selected.add(id); }
        else if (!draft.selected.has(id)) draft.selected = new Set([id]);
    } else if (tool === 'select' && !e.ctrlKey && !e.metaKey) draft.selected.clear();
    gesture = { ...p, lastX: p.x, lastY: p.y, lane: laneAt(p.x), at: quantize(bounds().at(p.y)), tool, id: n?.id, tail: e.target.dataset.tail, additive: e.ctrlKey || e.metaKey };
    $('grid').setPointerCapture(e.pointerId); update();
});
$('grid').addEventListener('pointermove', e => {
    if (!gesture) return; const p = xy(e, $('grid')); gesture.lastX = p.x; gesture.lastY = p.y; draw();
});
$('grid').addEventListener('pointerup', e => {
    if (!gesture) return;
    const g = gesture, p = xy(e, $('grid')), end = quantize(bounds().at(p.y)); gesture = null;
    if (g.tool === 'tap') draft.add(g.lane, g.at);
    else if (g.tool === 'hold') { if (g.at === end) message(tr('Hold는 시작과 끝을 다른 박에 놓아 주세요.')); else draft.add(g.lane, Math.min(g.at, end), Math.max(g.at, end)); }
    else if (g.id && draft.selected.has(g.id)) {
        const n = draft.c.notes.find(n => n.id === g.id);
        if (g.tail && end > ms(n.at_ns)) draft.change(() => n.end_ns = ns(end));
        else if (!g.tail && (Math.abs(p.x - g.x) > 3 || Math.abs(p.y - g.y) > 3)) draft.move(end - g.at, laneAt(p.x) - g.lane);
    } else {
        if (!g.additive) draft.selected.clear();
        if (Math.abs(p.x - g.x) < 3 && Math.abs(p.y - g.y) < 3) cursor = end;
        else for (const n of draft.c.notes) {
            if (n.lane >= Math.min(g.lane, laneAt(p.x)) && n.lane <= Math.max(g.lane, laneAt(p.x)) && ms(n.at_ns) >= Math.min(g.at, end) && ms(n.at_ns) <= Math.max(g.at, end)) draft.selected.add(n.id);
        }
    }
    update();
});
$('grid').addEventListener('pointercancel', () => { gesture = null; draw(); });
// Keys and wheel follow Quaver's EditorInputConfig: wheel moves the position, Ctrl+wheel steps the snap,
// PageUp/PageDown step the lane scale (확대).
$('grid').addEventListener('wheel', e => {
    e.preventDefault(); if (!draft || !e.deltaY) return;
    if (e.ctrlKey) step($('snap'), e.deltaY < 0 ? 1 : -1); else seek(cursor + e.deltaY / bounds().scale);
}, { passive: false });
function zoomBy(delta) {
    const i = Math.max(0, Math.min(ZOOMS.length - 1, ZOOMS.indexOf(zoom) + delta));
    if (ZOOMS[i] !== zoom) { zoom = ZOOMS[i]; draw(); }
}
// Steps a select through its options in list order and runs its change handler.
function step(select, delta) {
    const i = Math.max(0, Math.min(select.options.length - 1, select.selectedIndex + delta));
    if (i !== select.selectedIndex) { select.selectedIndex = i; select.onchange?.(); }
}
$('map').addEventListener('pointerdown', e => seek((720 - xy(e, $('map')).y) / 720 * duration));
new ResizeObserver(() => draw()).observe($('grid'));
for (const n of ['select', 'tap', 'hold']) $(n).onclick = () => setTool(n);
$('mirror').onclick = () => { draft.mirror(); update(); };
$('width').onchange = () => { widthScale = Number($('width').value) / 100; draw(); };
$('snap').onchange = draw;
$('fold').onclick = () => { const folded = root.classList.toggle('compact'); $('fold').setAttribute('aria-pressed', String(folded)); };
function jump() { if ($('position').validity.valid && Number.isFinite($('position').valueAsNumber)) seek($('position').valueAsNumber); }
$('position').onchange = jump;
$('position').onkeydown = e => { if (e.key === 'Enter') { e.preventDefault(); jump(); } };
$('point').onchange = () => { timingIndex = Number($('point').value); timingForm(); seek(ms(draft.c.timing[timingIndex].at_ns)); };
$('note').onsubmit = e => {
    e.preventDefault(); if (busy || playing || !$('note').reportValidity()) return;
    const n = draft.c.notes.find(n => draft.selected.has(n.id)), at = $('at').valueAsNumber, end = $('end').valueAsNumber, kind = $('kind').value;
    if (['hold', 'roll'].includes(kind) && end <= at) { message(tr('롱노트 끝은 시작보다 뒤여야 합니다.')); return; }
    draft.change(() => { n.lane = $('lane').valueAsNumber - 1; n.at_ns = ns(at); n.end_ns = ns(['hold', 'roll'].includes(kind) ? end : at); n.kind = kind; }); update();
};
$('timing').onsubmit = e => {
    e.preventDefault(); if (busy || playing || !$('timing').reportValidity()) return;
    draft.change(c => { const p = c.timing[timingIndex]; Object.assign(p, { at_ns: ns($('timing-at').valueAsNumber), bpm: kept($('bpm'), p.bpm), scroll: kept($('scroll'), p.scroll), meter: $('meter').valueAsNumber, stop_ns: ns($('stop').valueAsNumber) }); c.timing.sort((a, b) => a.at_ns - b.at_ns); timingIndex = c.timing.indexOf(p); }); update();
};
$('point-add').onclick = () => {
    const at = quantize(cursor); if (draft.c.timing.some(p => p.at_ns === ns(at))) { message(tr('해당 위치에 타이밍 이벤트가 이미 있습니다.')); return; }
    draft.change(c => { const prev = c.timing.findLast(p => p.at_ns <= ns(at)) || c.timing[0]; const p = { ...prev, at_ns: ns(at), stop_ns: 0 }; c.timing.push(p); c.timing.sort((a, b) => a.at_ns - b.at_ns); timingIndex = c.timing.indexOf(p); }); update();
};
$('point-delete').onclick = () => { if (draft.c.timing.length < 2) return; draft.change(c => c.timing.splice(timingIndex, 1)); update(); };
for (const form of ['note', 'timing', 'meta']) $(form).addEventListener('change', e => { if (e.target !== $('point')) $(form).requestSubmit(); });
$('meta').onsubmit = e => { e.preventDefault(); if (busy || playing || !$('meta').reportValidity()) return; draft.change(c => { c.metadata.difficulty = $('name').value.trim(); c.metadata.creator = $('creator').value.trim(); }); update(); };

async function audio(e, c, token) {
    const app = window.go.main.App;
    context ||= new AudioContext();
    const ids = [...new Set([...(c.audio || []).map(a => a.sound.asset), ...c.notes.flatMap(n => [...(n.sounds || []), ...(n.tail_sounds || [])].map(s => s.asset))])];
    let skipped = 0;
    for (const id of ids.slice(0, 64)) {
        if (token !== ticket) return;
        const asset = c.assets?.[id]; if (asset?.kind !== 'audio') { skipped++; continue; }
        try {
            const url = app.EditorAudio ? app.EditorAudio(sel(e), id) : '/editor/audio?' + new URLSearchParams({ chart: e.path, folder: e.folder, index: e.index, id });
            const response = await fetch(url); if (!response.ok) throw new Error('audio unavailable');
            const bytes = await response.arrayBuffer(); if (bytes.byteLength > 64 << 20) throw new Error('audio too large');
            const buffer = await context.decodeAudioData(bytes);
            if (token !== ticket) return; buffers.set(id, buffer);
            if (id === 'music' || !peaks.length && (c.audio || []).some(a => a.sound.asset === id && a.at_ns === 0)) {
                const samples = buffer.getChannelData(0), count = Math.min(6000, Math.ceil(buffer.duration * 20));
                peaks = Array.from({ length: count }, (_, i) => {
                    let peak = 0; const start = Math.floor(i / count * samples.length), end = Math.floor((i + 1) / count * samples.length);
                    for (let j = start; j < end; j++) peak = Math.max(peak, Math.abs(samples[j])); return peak;
                }); waveLength = buffer.duration * 1000;
            }
        } catch { skipped++; }
        duration = Math.max(duration, waveLength); rebuildMap(); draw();
    }
    if (token !== ticket) return;
    sync.audio(buffers, c);
    audioText = tr`${buffers.size}개 오디오 로드${!peaks.length ? tr(' · 전체 음악 파형 없음') : ''}${skipped || ids.length > 64 ? tr` · ${skipped + Math.max(0, ids.length - 64)}개 미리듣기 제외 (뱅크·누락·용량)` : ''}`;
    update();
}
function pause() {
    ++playTicket;
    if (playing) cursor = displayCursor();
    playing = false; cancelAnimationFrame(frame);
    for (const s of sources) { try { s.stop(); } catch { /* A finished source is already stopped. */ } }
    sources = []; previewTiming = undefined; update();
}
function displayCursor() { return Math.max(startedCursor, startedCursor + (context.currentTime - startedAt - sync.device.audioMS / 1000) * 1000 * activeRate); }
async function start() {
    if (!draft || busy || playing) return;
    const token = ++playTicket;
    context ||= new AudioContext(); await context.resume();
    if (root.hidden || token !== playTicket) return;
    playing = true; startedAt = context.currentTime + .04; startedCursor = cursor;
    const rate = Number($('rate').value), c = draft.c; activeRate = rate;
    let metroFrom = cursor;
    const metronome = () => {
        if (!$('metronome').checked) return;
        const to = Math.min(duration, startedCursor + (context.currentTime - startedAt + 3) * 1000 * rate);
        if (to <= metroFrom) return;
        for (const beat of ticks(previewTiming || c.timing, metroFrom, to, 1)) {
            const at = startedAt + (beat.at - startedCursor) / 1000 / rate;
            if (at >= context.currentTime) sources.push(click(context, at, Math.round(beat.beat) % position(previewTiming || c.timing, beat.at).meter === 0));
        }
        metroFrom = to + .001;
    };
    metronome();
    const events = [...(c.audio || []), ...c.notes.flatMap(n => (n.sounds || []).map(sound => ({ at_ns: n.at_ns, sound })))];
    for (const event of events) {
        const buffer = buffers.get(event.sound.asset); if (!buffer) continue;
        const at = ms(event.at_ns), offset = Math.max(0, (cursor - at) / 1000);
        if (offset >= buffer.duration || at < cursor && !(c.audio || []).includes(event)) continue;
        const source = context.createBufferSource(), gain = context.createGain(); source.buffer = buffer; source.playbackRate.value = rate;
        gain.gain.value = event.sound.volume; source.connect(gain); gain.connect(context.destination);
        source.start(startedAt + Math.max(0, at - cursor) / 1000 / rate, offset); sources.push(source);
    }
    const tick = () => {
        if (!playing) return;
        metronome();
        cursor = displayCursor();
        if (cursor >= duration) { pause(); cursor = duration; draw(); return; }
        draw(); frame = requestAnimationFrame(tick);
    };
    update(); tick();
}
$('play').onclick = () => playing ? pause() : start().catch(e => message(e.message));
$('metronome').onchange = () => { if (playing) { pause(); start().catch(e => message(e.message)); } };
$('first').onclick = () => seek(0);
$('last').onclick = () => seek(duration);
$('prev').onclick = () => { const at = bookmarks.findLast(at => at < cursor - 1); if (at != null) seek(at); };
$('next').onclick = () => { const at = bookmarks.find(at => at > cursor + 1); if (at != null) seek(at); };
$('rate').onchange = () => { if (playing) { pause(); start().catch(e => message(e.message)); } };
function test() {
    if (testing) { testing = false; root.classList.remove('testing'); pause(); pressed.clear(); update(); return; }
    testing = true; root.classList.add('testing'); 
    start().catch(e => message(e.message)); $('grid').focus(); update();
}
$('test').onclick = test;
$('bookmark').onclick = () => { if (!bookmarks.includes(cursor)) bookmarks.push(cursor); bookmarks.sort((a, b) => a - b); renderBookmarks(); draw(); };
function renderBookmarks() {
    $('bookmarks').replaceChildren(...bookmarks.map(at => {
        const li = document.createElement('li'), b = document.createElement('button'), del = document.createElement('button');
        b.textContent = `${(at / 1000).toFixed(2)} s`; b.onclick = () => seek(at); del.textContent = '×'; del.setAttribute('aria-label', tr`${at} ms 북마크 삭제`);
        del.onclick = () => { bookmarks = bookmarks.filter(p => p !== at); renderBookmarks(); draw(); }; li.append(b, del); return li;
    }));
}
let leave;
function leaving(fn) {
    if (busy || loading) return;
    if (!draft.dirty) { fn(); return; }
    leave = fn; $('leave').showModal();
}
$('keep').onclick = () => $('leave').close();
$('discard').onclick = () => { $('leave').close(); leave?.(); };
$('back').onclick = () => leaving(() => {
    ++ticket; pause(); sync.reset(); testing = false; root.classList.remove('testing'); root.hidden = true;
    const records = document.querySelector('#record-tools'); if (records) records.hidden = false;
    document.querySelector('#songselect').hidden = false; onClose?.();
});
$('save').onclick = async () => {
    if (busy || loading || issues(draft.c).length) return;
    pause(); busy = true; update();
    try {
        const path = await window.go.main.App.SaveChart(sel(entry), draft.document());
        draft.saved = JSON.stringify(draft.document()); message(tr`사본 저장 완료: ${path} · 원본과 같은 곡 폴더에 저장했습니다.`);
    } catch (err) { message(tr`저장 실패: ${err?.message ?? err}`); }
    finally { busy = false; update(); }
};
// Export writes a new .osu copy; the draft stays unsaved because .ggc is the editor's own format.
$('export').onclick = async () => {
    if (busy || loading || issues(draft.c).length) return;
    pause(); busy = true; update();
    try {
        const r = await window.go.main.App.ExportChart(sel(entry), draft.document());
        message(tr`.osu 내보내기: ${r.path}${r.dropped?.length ? tr` · 제외 ${r.dropped.join(', ')}` : ''}`);
    } catch (err) { message(tr`내보내기 실패: ${err?.message ?? err}`); }
    finally { busy = false; update(); }
};
window.addEventListener('beforeunload', e => { if (!root.hidden && draft?.dirty) { e.preventDefault(); e.returnValue = ''; } });
window.addEventListener('blur', () => { if (!root.hidden) { pause(); pressed.clear(); draw(); } });
// Space toggles playback regardless of the focused control; only text fields keep it for typing.
const typing = e => e.target.matches('textarea, input:not([type]), input[type="text"]');
const spaceKey = e => e.code === 'Space' && !root.hidden && draft && !typing(e) && !document.querySelector('dialog[open]');
document.addEventListener('keydown', e => {
    if (!spaceKey(e)) return;
    e.preventDefault(); e.stopPropagation();
    if (!e.repeat && !busy && !loading) $('play').click();
}, true);
document.addEventListener('keyup', e => { if (spaceKey(e)) { e.preventDefault(); e.stopPropagation(); } }, true);
document.addEventListener('keydown', e => {
    if (root.hidden || !draft || busy || loading || e.isComposing || document.querySelector('dialog[open]')) return;
    if (e.target.matches('input,select,textarea')) return;
    if (testing) {
        if (e.key === 'Escape') { e.preventDefault(); test(); return; }
        const lane = '123456789abcdefghi'.indexOf(e.key.toLowerCase());
        if (lane >= 0 && lane < draft.c.lanes) {
            e.preventDefault();
            if (!e.repeat && !pressed.has(lane) && playing && $('record').checked) {
                const at = inputPosition(eventTime(context, e.timeStamp), startedAt, startedCursor, activeRate, sync.device.inputMS), target = quantize(at);
                if (target >= -86400000 && target <= duration && !draft.c.notes.some(n => n.lane === lane && ms(n.at_ns) <= target && ms(n.end_ns) >= target && n.kind !== 'fake')) { draft.add(lane, target); update(); }
            }
            pressed.add(lane); draw(); return;
        }
    }
    // Esc leaves the editor like the back button, asking first when there are unsaved edits.
    if (e.key === 'Escape') { e.preventDefault(); $('back').click(); return; }
    if (testing) return;
    const mod = e.ctrlKey || e.metaKey, key = e.key.toLowerCase();
    if (!mod && !e.altKey && !e.shiftKey && ['c', 'z', 'x', 'v', 'f5'].includes(key)) {
        e.preventDefault();
        if (e.repeat) return;
        if (key === 'c') $('play').click();
        else if (key === 'z') seek(draft.c.notes.length ? draft.c.notes.reduce((at, n) => Math.min(at, ms(n.at_ns)), Infinity) : 0);
        else if (key === 'v') seek(draft.c.notes.length ? draft.c.notes.reduce((at, n) => Math.max(at, ms(n.end_ns)), -Infinity) : duration);
        else if (key === 'x') { pause(); seek(0); start().catch(e => message(e.message)); }
        else test();
        return;
    }
    // Seeking repeats while held and works during playback, like Quaver.
    if (key === 'arrowup' || key === 'arrowdown') {
        e.preventDefault(); const beat = 60000 / position(draft.c.timing, cursor).bpm, delta = (key === 'arrowup' ? 1 : -1) * beat / (Number($('snap').value) || 4);
        if (e.altKey) { if (draft.selected.size) { draft.move(delta); update(); } }
        seek(cursor + delta); return;
    }
    if (key === 'arrowleft' || key === 'arrowright') {
        e.preventDefault(); const delta = key === 'arrowleft' ? -1 : 1;
        if (e.altKey) { draft.move(0, delta); update(); } else step($('snap'), delta);
        return;
    }
    if (key === 'pageup' || key === 'pagedown') { e.preventDefault(); zoomBy(key === 'pageup' ? 1 : -1); return; }
    if (key === 'home' || key === 'end') { e.preventDefault(); seek(key === 'home' ? 0 : duration); return; }
    if (e.repeat) return;
    // Digit keys place or remove a note at the snapped position on that lane, also during playback; Shift+digit picks the snap.
    const digit = /^Digit\d$/.test(e.code) ? Number(e.code.slice(5)) : -1;
    if (digit >= 0 && !mod && !e.altKey) {
        e.preventDefault();
        if (e.shiftKey) { const o = [...$('snap').options].find(o => o.value === String(digit)); if (o) { $('snap').value = o.value; draw(); } return; }
        const lane = (digit + 9) % 10, at = quantize(cursor);
        if (lane < draft.c.lanes && Math.abs(at) <= 86400000) { draft.toggle(lane, at); update(); }
        return;
    }
    if (key === '+' || key === '=' || key === '-') { e.preventDefault(); step($('rate'), key === '-' ? -1 : 1); return; }
    if (playing) return;
    if (mod && key === 's') { e.preventDefault(); $('save').click(); return; }
    if (mod && ['z', 'y', 'c', 'v', 'x', 'a'].includes(key)) {
        e.preventDefault();
        if (key === 'a') draft.selected = new Set(draft.c.notes.map(n => n.id));
        else if (key === 'c') draft.copy();
        else if (key === 'x') draft.cut();
        else if (key === 'v') draft.paste(quantize(cursor));
        else if (key === 'z' && !e.shiftKey) draft.restore(draft.undo, draft.redo); else draft.restore(draft.redo, draft.undo);
        update(); return;
    }
    if (mod || e.altKey) return;
    if (e.key === 'Delete') { e.preventDefault(); if (draft.selected.size) { draft.remove(); update(); } }
    else if (['q', 'w', 'e'].includes(key)) setTool({ q: 'select', w: 'tap', e: 'hold' }[key]);
    else if (key === 't') { e.preventDefault(); test(); }
});
document.addEventListener('keyup', e => { const lane = '123456789abcdefghi'.indexOf(e.key.toLowerCase()); if (pressed.delete(lane)) draw(); });

const key = row => `${row.folder}|${row.path}|${row.index}`;
function renderCharts() {
    $('chart-list').replaceChildren(...catalog.filter(row => row.folder === entry.folder && row.title === entry.title).map(row => {
        const li = document.createElement('li'), b = document.createElement('button'), side = document.createElement('button'); b.type = side.type = 'button'; b.textContent = `${row.difficulty} · ${row.lanes}K`;
        b.textContent += ` · ${(row.format || '').toUpperCase()}`;
        b.setAttribute('aria-current', String(row.path === entry.path && row.index === entry.index)); b.title = row.path;
        b.onclick = () => leaving(() => open(row, catalog, onClose).catch(err => message(tr`열기 실패: ${err.message}`)));
        side.textContent = tr('옆'); side.title = tr('옆에 참고 채보로 보기'); side.setAttribute('aria-pressed', String(referenceKey === key(row)));
        side.onclick = () => toggleReference(row).catch(err => message(tr`참고 채보 열기 실패: ${err.message}`));
        li.append(b, side); return li;
    }));
}
async function toggleReference(row) {
    if (referenceKey === key(row)) { reference = undefined; referenceKey = ''; }
    else { const token = ticket, c = await window.go.main.App.EditChart(sel(row)); if (token !== ticket) return; reference = c; referenceKey = key(row); }
    renderCharts(); draw();
}

export async function open(e, entries, close) {
    if (loading || busy) return;
    loading = true; const token = ++ticket;
    try {
        const c = await window.go.main.App.EditChart(sel(e));
        if (token !== ticket) return;
        pause(); sync.reset(); buffers = new Map(); peaks = []; waveLength = 0; audioText = '';
        draft = new Draft(c); entry = e; catalog = entries; onClose = close; cursor = 0; bookmarks = []; pressed.clear(); testing = false; root.classList.remove('testing');
        document.querySelector('#songselect').hidden = true; root.hidden = false;
        const records = document.querySelector('#record-tools'); if (records) records.hidden = true;
        reference = undefined; referenceKey = '';
        renderCharts();
        renderBookmarks(); setTool('select'); timingIndex = 0; update();
        message('');
        audio(e, c, token).catch(err => { if (token === ticket) { audioText = tr`오디오 읽기 실패: ${err.message}`; update(); } });
        $('grid').focus();
    } finally { loading = false; update(); }
}
