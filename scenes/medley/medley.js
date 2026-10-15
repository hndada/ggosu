import { tr } from '../../preferences/language.js';
// Up to three chosen or random songs; the game arranges their highlights.
const $ = selector => document.querySelector(selector);
const pageSize = 100;
let options, picked = [], lanes = 0, drawing = false, page = 0;
let entries, pool = [], shown = [], busy = false, lastError = '', cursor = '';

function chartKey(e) { return JSON.stringify([e.path, e.index]); }
function songKey(e) { return /\.ojn$/i.test(e.path) ? e.path : e.folder; }
function sameChart(a, b) { return chartKey(a) === chartKey(b); }
function text(e) { return `${e.title}${e.artist ? ` — ${e.artist}` : ''} [${e.difficulty}]`; }
function range() {
    const low = $('#medley-low').valueAsNumber, high = $('#medley-high').valueAsNumber;
    return Number.isFinite(low) && Number.isFinite(high) && low <= high ? { low, high } : null;
}
function keyCounts() {
    const counts = new Map();
    // Count charts that can actually enter the pool, including the length limit.
    for (const e of entries) if (e.rating > 0 && e.length >= 60) counts.set(e.lanes, (counts.get(e.lanes) ?? 0) + 1);
    return [...counts].sort((a, b) => a[0] - b[0]);
}
function renderKeys() {
    const counts = keyCounts();
    if (!counts.some(([n]) => n === lanes)) lanes = counts.find(([n]) => n === 7)?.[0] ?? counts[0]?.[0] ?? 0;
    $('#medley-keys').replaceChildren(document.createTextNode(tr('키 수 ')), ...counts.map(([n, count]) => {
        const b = document.createElement('button');
        b.type = 'button'; b.disabled = busy || drawing; b.setAttribute('aria-pressed', String(n === lanes));
        b.textContent = `${n}K (${count})`;
        b.addEventListener('click', () => {
            if (lanes === n || options.busy() || drawing) return;
            lanes = n; refresh();
        });
        return b;
    }));
}
function renderSongs() {
    $('#medley-picked').textContent = tr`선택한 곡 · ${picked.length} / 3`;
    $('#medley-songs').replaceChildren(...picked.map((e, i) => {
        const li = document.createElement('li'), name = document.createElement('strong'), detail = document.createElement('span');
        name.className = 'medley-picked-name'; name.textContent = e.title;
        detail.className = 'medley-picked-chart'; detail.textContent = `Lv ${e.rating.toFixed(1)} · ${e.difficulty} · ×`;
        li.tabIndex = 0; li.setAttribute('role', 'button');
        li.setAttribute('aria-disabled', String(busy || drawing));
        li.setAttribute('aria-label', tr`${text(e)} 빼기`);
        const remove = () => {
            if (options.busy() || drawing) return;
            picked.splice(i, 1); renderSelection(); $('#medley-search').focus();
        };
        li.addEventListener('click', remove);
        li.addEventListener('keydown', e => {
            if (!['Enter', ' ', 'Delete'].includes(e.key)) return;
            e.preventDefault(); e.stopPropagation(); remove();
        });
        li.append(name, detail);
        return li;
    }));
    $('#medley-play').disabled = $('#medley-auto').disabled = !picked.length || busy || drawing || !range();
    $('#medley-draw').disabled = !pool.length || busy || drawing || !range();
    $('#medley-clear').disabled = !picked.length || busy || drawing;
    for (const id of ['#medley-low', '#medley-high']) $(id).disabled = busy || drawing;
}
function renderCandidates() {
    const query = $('#medley-search').value.trim().toLocaleLowerCase();
    const matches = pool.filter(e => !query || text(e).toLocaleLowerCase().includes(query));
    const pages = Math.max(1, Math.ceil(matches.length / pageSize));
    page = Math.min(page, pages - 1);
    shown = matches.slice(page * pageSize, (page + 1) * pageSize);
    if (!shown.some(e => chartKey(e) === cursor)) cursor = shown[0] ? chartKey(shown[0]) : '';
    const songs = new Set(pool.map(songKey));
    $('#medley-count').textContent = tr`후보 ${songs.size}곡 · ${pool.length}채보 / 검색 ${matches.length}채보`;
    $('#medley-page').textContent = `${page + 1} / ${pages}`;
    $('#medley-prev').disabled = page === 0;
    $('#medley-next').disabled = page + 1 === pages;
    $('#medley-candidates').replaceChildren(...shown.map((e, i) => {
        const li = document.createElement('li'), label = document.createElement('span'), level = document.createElement('span'), name = document.createElement('span'), title = document.createElement('span'), mark = document.createElement('span');
        const selected = picked.some(p => sameChart(p, e)), replacing = picked.some(p => songKey(p) === songKey(e));
        label.className = 'chart-label'; level.className = 'chart-level';
        level.textContent = String(Math.min(99, Math.floor(e.rating))).padStart(2, '0');
        level.title = `Lv ${e.rating.toFixed(1)}`;
        name.className = 'chart-name'; name.title = text(e);
        title.className = 'chart-title'; title.textContent = `${e.title} · ${e.difficulty}`; name.append(title);
        const chartMark = document.createElement('span'); chartMark.className = 'chart-mark'; chartMark.append(level);
        if (e.level) {
            const original = document.createElement('span'); original.className = 'source-level'; original.textContent = `(${e.level})`;
            chartMark.append(original);
        }
        label.append(chartMark, name);
        mark.className = 'medley-mark'; mark.textContent = selected ? '✓' : '';
        li.dataset.index = String(i); li.tabIndex = chartKey(e) === cursor ? 0 : -1;
        li.setAttribute('role', 'option'); li.setAttribute('aria-selected', String(selected));
        li.setAttribute('aria-disabled', String(busy || drawing || !replacing && picked.length >= 3));
        li.setAttribute('aria-label', `Lv ${e.rating.toFixed(1)} ${text(e)} · ${selected ? tr('빼기') : replacing ? tr('난이도 교체') : tr('담기')}`);
        li.addEventListener('focus', () => { cursor = chartKey(e); });
        li.addEventListener('click', () => choose(e));
        li.append(label, mark);
        return li;
    }));
}
function choose(e) {
    if (options.busy() || drawing) return;
    const at = picked.findIndex(p => songKey(p) === songKey(e));
    if (at >= 0) {
        if (sameChart(picked[at], e)) picked.splice(at, 1);
        else picked[at] = e;
    } else if (picked.length < 3) picked.push(e);
    cursor = chartKey(e); renderSelection();
    $('#medley-candidates').querySelector(`[data-index="${shown.findIndex(p => sameChart(p, e))}"]`)?.focus();
}
function move(delta) {
    if (!shown.length) return;
    const at = shown.findIndex(e => chartKey(e) === cursor);
    const next = Math.max(0, Math.min(shown.length - 1, at + delta));
    cursor = chartKey(shown[next]);
    for (const row of $('#medley-candidates').children) row.tabIndex = Number(row.dataset.index) === next ? 0 : -1;
    const row = $('#medley-candidates').children[next];
    row.focus(); row.scrollIntoView?.({ block: 'nearest' });
}
function renderSelection() { renderSongs(); renderCandidates(); }
function refresh() {
    entries = options.entries(); busy = options.busy(); page = 0;
    renderKeys();
    const r = range();
    pool = r ? entries.filter(e => e.lanes === lanes && e.rating > 0 && e.rating >= r.low && e.rating <= r.high && e.length >= 60) : [];
    const available = new Map(pool.map(e => [chartKey(e), e]));
    picked = picked.map(e => available.get(chartKey(e))).filter(Boolean);
    $('#medley-status').textContent = !r ? tr('Lv 범위를 확인해 주세요.') : !pool.length ? tr('조건에 맞는 곡이 없습니다.') : '';
    renderSelection();
}
async function draw() {
    const r = range();
    if (!lanes || !r || !pool.length || drawing || options.busy()) return;
    drawing = true; $('#medley-status').textContent = '';
    renderKeys(); renderSelection();
    try { picked = await window.go.main.App.MedleyPick({ lanes, ...r }); }
    catch (err) { $('#medley-status').textContent = err?.message ?? String(err); }
    finally {
        drawing = false;
        // A rescan while the request was pending may have removed a chart.
        const error = $('#medley-status').textContent;
        refresh();
        if (error) $('#medley-status').textContent = error;
    }
}
function play(mode) {
    if (!picked.length || options.busy() || drawing || !range()) return;
    const parts = picked.map(e => ({ folder: e.folder, chartID: e.path, index: e.index }));
    options.launch({ ...options.playOptions(), ...parts[0], mode, medley: parts });
}

export function open() {
    $('#medley').hidden = false;
    refresh();
    $('#medley-search').focus();
}
export function close() { $('#medley').hidden = true; }
export function isOpen() { return !$('#medley').hidden; }
export function update() {
    if (!isOpen()) return;
    if (entries !== options.entries()) refresh();
    if (busy !== options.busy()) { busy = options.busy(); renderKeys(); renderSelection(); }
    const error = options.error();
    if (error && error !== lastError) $('#medley-status').textContent = error;
    lastError = error;
}

// options supplies the scanned list, playback state/options and game launcher.
export function mount(o) {
    options = o;
    for (const id of ['#medley-low', '#medley-high']) $(id).addEventListener('change', refresh);
    $('#medley-search').addEventListener('input', () => { page = 0; renderCandidates(); });
    $('#medley-prev').addEventListener('click', () => { if (page > 0) page--; renderCandidates(); });
    $('#medley-next').addEventListener('click', () => { page++; renderCandidates(); });
    const clear = () => {
        if (options.busy() || drawing) return;
        picked = []; renderSelection(); $('#medley-search').focus();
    };
    $('#medley-clear').addEventListener('click', clear);
    $('#medley-draw').addEventListener('click', () => void draw());
    $('#medley-play').addEventListener('click', () => play('live'));
    $('#medley-auto').addEventListener('click', () => play('auto'));
    $('#medley-back').addEventListener('click', () => o.back());
    document.addEventListener('keydown', e => {
        if (!isOpen() || document.querySelector('dialog[open]') || e.isComposing || e.defaultPrevented) return;
        // Buttons keep native Enter/Space activation; Tab reaches every list action.
        const interactive = e.target.matches?.('input, button, select, textarea');
        const searching = e.target === $('#medley-search');
        if (e.key === 'Enter' && (!interactive || searching || e.ctrlKey)) { e.preventDefault(); play(e.ctrlKey ? 'auto' : 'live'); }
        else if ((e.key === 'ArrowUp' || e.key === 'ArrowDown') && (!interactive || searching)) {
            e.preventDefault(); move(e.key === 'ArrowUp' ? -1 : 1);
        } else if (e.key === ' ' && !interactive && shown.some(p => chartKey(p) === cursor)) {
            e.preventDefault(); choose(shown.find(p => chartKey(p) === cursor));
        } else if (e.key === 'Backspace' && e.ctrlKey) { e.preventDefault(); clear(); }
        else if (e.key === 'F2') { e.preventDefault(); void draw(); }
        else if (e.key === 'Escape') { e.preventDefault(); o.back(); }
    });
}
