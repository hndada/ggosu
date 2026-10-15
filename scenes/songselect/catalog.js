import { tr } from '../../preferences/language.js';
import { keyCounts, visibleCharts } from '/songlist/list.js';
import { lamps, CLEAR_LAMPS, clearLamp } from '/songlist/analysis.js';
import { levelAt } from '/songlist/level.js';

const $ = selector => document.querySelector(selector);
const list = $('#song-list');
const reducedMotion = matchMedia('(prefers-reduced-motion: reduce)');
const tickers = new Map();

// Measure only when the list changes size or content; playback runs in the browser.
// A content change measures just the rows it touched.
function scrollTitles(titles = list.querySelectorAll('.chart-title')) {
    for (const [title, ticker] of tickers) {
        if (!title.isConnected) { ticker.animation.cancel(); tickers.delete(title); }
    }
    for (const title of titles) {
        const width = title.parentElement.clientWidth;
        const distance = width > 0 && !reducedMotion.matches ? Math.max(0, title.scrollWidth - width) : 0;
        const ticker = tickers.get(title);
        if (ticker?.distance === distance) continue;
        ticker?.animation.cancel(); tickers.delete(title);
        if (distance <= 1) continue;
        const hold = 1600, travel = distance / 24 * 1000, duration = 2 * (hold + travel);
        const start = 'translateX(0)', end = `translateX(-${distance}px)`;
        const animation = title.animate([
            { transform: start, offset: 0 },
            { transform: start, offset: hold / duration },
            { transform: end, offset: .5 },
            { transform: end, offset: .5 + hold / duration },
            { transform: start, offset: 1 },
        ], { duration, iterations: Infinity, easing: 'linear' });
        tickers.set(title, { distance, animation });
    }
}
new ResizeObserver(() => scrollTitles()).observe(list);
reducedMotion.addEventListener('change', () => scrollTitles());
document.fonts.ready.then(() => scrollTitles());

function titleText(text, selected = false) {
    const name = document.createElement(selected ? 'strong' : 'span');
    name.className = 'chart-name'; name.title = text;
    const title = document.createElement('span'); title.className = 'chart-title'; title.textContent = text;
    name.append(title);
    return name;
}

function item(text, right, attrs = {}) {
    const li = document.createElement('li');
    const left = document.createElement('span'); left.textContent = text;
    const end = document.createElement('span'); end.textContent = right;
    li.append(left, end);
    for (const [k, v] of Object.entries(attrs)) li.setAttribute(k, v);
    return li;
}
// Rows that stay on screen keep their element so theme.css transitions play when the selection moves;
// a rebuilt row would start in its final state. Kept rows are never detached, which would also cancel them.
// A node that is already the listed row is left untouched.
function patch(nodes, old) {
    const changed = [];
    const rows = nodes.map(([key, fresh]) => {
        const li = old.get(key);
        if (li === fresh) return li;
        changed.push(li ?? fresh);
        if (!li) { fresh.rowKey = key; return fresh; }
        for (const a of [...li.attributes]) if (!fresh.hasAttribute(a.name)) li.removeAttribute(a.name);
        for (const a of fresh.attributes) if (li.getAttribute(a.name) !== a.value) li.setAttribute(a.name, a.value);
        const title = li.querySelector('.chart-title'), next = fresh.querySelector('.chart-title');
        if (title && next && title.textContent === next.textContent) next.replaceWith(title);
        li.firstChild.replaceChildren(...fresh.firstChild.childNodes);
        li.lastChild.replaceWith(fresh.lastChild);
        return li;
    });
    const keep = new Set(rows);
    for (const li of [...list.children]) if (!keep.has(li)) li.remove();
    rows.forEach((li, i) => { if (list.children[i] !== li) list.insertBefore(li, list.children[i] ?? null); });
    scrollTitles(changed.flatMap(li => [...li.querySelectorAll('.chart-title')]));
}
// What the rows were last drawn from; while it holds, a move redraws only the rows it changes.
let drawn = {};
let positions = [], count = 0, selectedPosition = 0, positionFrame = 0;
function updatePosition() {
    positionFrame = 0;
    const lo = list.scrollTop, hi = lo + list.clientHeight;
    const firstAfter = test => {
        let a = 0, b = positions.length;
        while (a < b) { const m = (a + b) >>> 1; if (test(positions[m].node)) b = m; else a = m + 1; }
        return a;
    };
    const first = firstAfter(n => n.offsetTop + n.offsetHeight > lo);
    const end = firstAfter(n => n.offsetTop >= hi);
    const from = positions[first]?.first ?? 0, to = positions[Math.max(first, end - 1)]?.last ?? 0;
    $('#list-range').textContent = count ? tr`선택 ${selectedPosition} / ${count} · 표시 ${from}–${to}` : '0 / 0';
    const bar = $('#list-range-window');
    const visible = count ? to - from + 1 : 0;
    bar.style.height = `${count ? visible / count * 100 : 0}%`;
    // A minimum-height bar travels within the track and still reaches both ends.
    const travel = Math.max(0, bar.parentElement.clientHeight - bar.offsetHeight);
    const fraction = count > visible ? Math.max(0, Math.min(1, (from - 1) / (count - visible))) : 0;
    bar.style.top = `${fraction * travel}px`;
}
// osu!lazer Carousel.GetPanelXOffset: rows bend right away from the middle of the list, as if they ran
// along a circle three half-heights in radius. It follows the scroll each frame, without easing.
let bendFrame = 0;
function bend() {
    bendFrame = 0;
    const half = list.clientHeight / 2, rows = list.children;
    if (!half || !rows.length) return;
    const top = list.scrollTop, bottom = top + list.clientHeight;
    let a = 0, b = rows.length;
    while (a < b) { const m = (a + b) >>> 1; if (rows[m].offsetTop + rows[m].offsetHeight > top) b = m; else a = m + 1; }
    const shown = [];
    for (let i = a; i < rows.length && rows[i].offsetTop < bottom; i++) {
        const row = rows[i], dist = Math.abs(1 - (row.offsetTop - top + row.offsetHeight / 2) / half);
        shown.push([row, (3 - Math.sqrt(Math.max(0, 9 - dist * dist))) * half]);
    }
    for (const [row, x] of shown) row.style.left = `${x.toFixed(1)}px`;
}
function scheduleBend() { if (!bendFrame) bendFrame = requestAnimationFrame(bend); }
function schedulePosition() { if (!positionFrame) positionFrame = requestAnimationFrame(updatePosition); scheduleBend(); }
list.addEventListener('scroll', schedulePosition, { passive: true });
new ResizeObserver(schedulePosition).observe(list);
export function renderList(shown, { current, progress, recent, view, collapsed, rate = 1 }) {
    // Two digits fit the column; the tooltip carries the exact value. Levels follow the Time Rate.
    function levelText(n) {
        return n != null && n >= 0 ? String(Math.min(99, Math.floor(n))).padStart(2, '0') : '--';
    }
    function chartItem(e, index, text, cls = '') {
        const li = item(text, '', { role: 'option', 'data-index': index, 'aria-selected': String(index === current) });
        const name = titleText(text, index === current);
        const level = document.createElement('span');
        const { value: n, estimated } = levelAt(e, rate), rated = e.rating > 0 && n != null;
        level.className = 'chart-level'; level.textContent = levelText(n);
        level.setAttribute('aria-label', level.textContent === '--' ? tr('레벨 정보 없음') : tr`레벨 ${n.toFixed(1)}`);
        level.title = level.textContent === '--' ? tr('레벨 정보 없음') : [rated ? tr`난이도 ${n.toFixed(1)}${rate !== 1 ? ` (${rate}x${estimated ? tr(' 추정') : ''})` : ''} (잠정)` : '', e.level ? tr`원본 레벨 ${e.level}` : ''].filter(Boolean).join(' · ');
        li.firstChild.className = 'chart-label';
        const end = li.lastChild, s = progress?.get(e.hash);
        end.className = 'chart-progress';
        const states = lamps(s);
        const lamp = document.createElement('span'); lamp.className = 'chart-lamp combo-lamp';
        for (const part of ['lamp-flash', 'lamp-shine']) { const span = document.createElement('span'); span.className = part; lamp.append(span); }
        lamp.dataset.state = progress == null ? 'unavailable' : states.combo;
        lamp.title = progress == null ? tr('기록 확인 불가') : tr(states.comboLabel);
        lamp.setAttribute('role', 'img'); lamp.setAttribute('aria-label', lamp.title);
        const points = document.createElement('span'); points.className = 'chart-points';
        points.textContent = s?.bestPoints == null ? '' : `${Math.floor(s.bestPoints / 1000).toLocaleString()}K`;
        if (s?.bestPoints != null) points.title = tr`최고 ${s.bestPoints.toLocaleString()}점 · 기본·Mirror·Random 포함`;
        const mark = document.createElement('span'); mark.className = 'chart-mark';
        mark.append(points, lamp, level);
        const original = String(e.level ?? '').trim();
        if (e.rating > 0 && original) {
            const source = document.createElement('span'); source.className = 'source-level';
            source.textContent = `(${original})`; source.title = tr`원본 레벨 ${original}`;
            source.setAttribute('aria-label', source.title);
            mark.append(source);
        }
        li.firstChild.replaceChildren(mark, name);
        if (recent && s) {
            const date = document.createElement('time'); date.dateTime = s.lastPlayed;
            date.textContent = new Date(s.lastPlayed).toLocaleString('ko-KR', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' });
            end.append(date);
        }
        end.title = s ? tr`마지막 플레이 ${new Date(s.lastPlayed).toLocaleString()}` : '';
        if (cls) li.className = cls;
        return li;
    }

    const old = new Map([...list.children].map(li => [li.rowKey, li]));
    const same = drawn.shown === shown && drawn.progress === progress && drawn.recent === recent && drawn.rate === rate;
    const kept = key => same ? old.get(key) : undefined;
    // A chart row only changes when it gains or loses the selection.
    const chart = (e, index, text, cls) => (index !== current && index !== drawn.current && kept(`c:${index}`)) || chartItem(e, index, text, cls);
    const nodes = [];
    if (shown.view === 'chart') {
        for (const it of shown.items) {
            if (it.type === 'separator') nodes.push([`s:${it.label}`, kept(`s:${it.label}`) || item(`— ${it.label}`, '', { role: 'presentation' })]);
            else nodes.push([`c:${it.index}`, chart(it.entry, it.index, `${it.entry.title} · ${it.entry.difficulty}`)]);
        }
    } else {
        for (const g of shown.groups) {
            if (g.charts.length === 1) { const c = g.charts[0]; nodes.push([`c:${c.index}`, chart(c.entry, c.index, `${g.title} · ${c.entry.difficulty}`)]); continue; }
            const open = !collapsed && g.charts.some(c => c.index === current);
            const header = kept(`g:${g.folder}`);
            if (header?.getAttribute('aria-expanded') === String(open)) nodes.push([`g:${g.folder}`, header]);
            else {
                const group = item(g.title, '', { role: 'option', 'data-index': g.charts[0].index, 'aria-selected': 'false', 'aria-expanded': String(open) });
                group.firstChild.className = 'chart-label'; group.firstChild.replaceChildren(titleText(g.title));
                nodes.push([`g:${g.folder}`, group]);
            }
            if (!open) continue;
            const v = visibleCharts(g, current);
            if (v.before) nodes.push([`m:${g.folder}:before`, item(tr`↑ ${v.before}개 더`, '', { role: 'presentation', class: 'more' })]);
            for (const c of v.charts) nodes.push([`c:${c.index}`, chart(c.entry, c.index, c.entry.difficulty, 'chart-in-song')]);
            if (v.after) nodes.push([`m:${g.folder}:after`, item(tr`↓ ${v.after}개 더`, '', { role: 'presentation', class: 'more' })]);
        }
    }
    patch(nodes, old);
    drawn = { shown, progress, recent, rate, current };
    count = shown.order.length;
    const ranks = new Map(shown.order.map((index, i) => [index, i + 1]));
    selectedPosition = ranks.get(current) ?? 0;
    const groups = new Map((shown.groups ?? []).map(g => [g.folder, g.charts]));
    positions = [...list.children].flatMap(node => {
        const group = node.rowKey?.startsWith('g:') ? groups.get(node.rowKey.slice(2)) : null;
        const first = group ? ranks.get(group[0].index) : ranks.get(Number(node.dataset.index));
        if (first == null) return [];
        return [{ node, first, last: group ? ranks.get(group.at(-1).index) : first }];
    });
    schedulePosition();
    follow(list.querySelector('[aria-selected="true"]'));
    $('#view-song').setAttribute('aria-pressed', String(view === 'song'));
    $('#view-chart').setAttribute('aria-pressed', String(view === 'chart'));
    $('#library-all').setAttribute('aria-pressed', String(!recent));
    $('#library-recent').setAttribute('aria-pressed', String(recent));
    $('#search-sort').disabled = recent;
    $('#list-summary').textContent = progress == null ? tr('기록 읽기 실패 [F5]') : '';
}
// osu! lazer's carousel keeps the selection centred and eases toward it (ScrollContainer DistanceDecayJump 0.01/ms).
let following = 0, target = 0;
function follow(row) {
    if (!row) return;
    const offset = row.getBoundingClientRect().top - list.getBoundingClientRect().top;
    glide(list.scrollTop + offset - (list.clientHeight - row.offsetHeight) / 2);
}
// The window height stands for the whole list (osu!'s right-mouse absolute scroll): 0 = top, 1 = bottom.
export function scrollToFraction(f) {
    glide(Math.max(0, Math.min(1, f)) * (list.scrollHeight - list.clientHeight));
}
// A running glide only changes its target, so dragging keeps one smooth motion.
function glide(to) {
    target = Math.max(0, Math.min(list.scrollHeight - list.clientHeight, to));
    if (matchMedia('(prefers-reduced-motion: reduce)').matches) { cancelAnimationFrame(following); following = 0; list.scrollTop = target; return; }
    if (following) return;
    // The browser rounds scrollTop to device pixels, so the glide ends within a pixel; a scroll it did not make
    // (wheel, scrollbar, keys) hands the list back to the user.
    let last = performance.now(), set = list.scrollTop;
    const step = now => {
        if (Math.abs(list.scrollTop - set) > 1) { following = 0; return; }
        const gap = (target - list.scrollTop) * Math.exp(-0.01 * Math.max(0, now - last));
        last = now;
        if (Math.abs(gap) < 1) { list.scrollTop = target; following = 0; return; }
        list.scrollTop = target - gap; set = list.scrollTop;
        following = requestAnimationFrame(step);
    };
    following = requestAnimationFrame(step);
}
// The key tabs above the list and the sort/filter window show the same buttons; each click adds or removes one key count.
function keyButtons(entries, matched, lanes, toggle) {
    return [...keyCounts(entries, matched)].map(([n, count]) => {
        const b = document.createElement('button');
        b.type = 'button'; b.setAttribute('aria-pressed', String(lanes.has(n)));
        b.textContent = `${n}K (${count})`;
        b.addEventListener('click', () => toggle(n));
        return b;
    });
}
export function renderTabs(entries, matched, lanes, toggle) {
    $('#key-tabs').replaceChildren(document.createTextNode(tr('키 수 [F7] ')), ...keyButtons(entries, matched, lanes, toggle));
}
export function renderKeyFilter(entries, matched, lanes, toggle) {
    $('#key-filter').replaceChildren(document.createTextNode(tr('키 수 [F7] ')), ...keyButtons(entries, matched, lanes, toggle));
}
// One button per clear lamp; counts are the charts passing every other condition. None pressed = no limit.
export function renderLampFilter(rows, progress, chosen, toggle) {
    const counts = new Map(CLEAR_LAMPS.map(([k]) => [k, 0]));
    if (progress) for (const { entry } of rows) { const k = clearLamp(progress.get(entry.hash)); counts.set(k, counts.get(k) + 1); }
    $('#lamp-filter').replaceChildren(document.createTextNode(tr('클리어 램프 ')), ...CLEAR_LAMPS.map(([k, label]) => {
        const b = document.createElement('button');
        b.type = 'button'; b.setAttribute('aria-pressed', String(chosen.has(k)));
        b.textContent = progress ? `${tr(label)} (${counts.get(k)})` : tr(label);
        b.addEventListener('click', () => toggle(k));
        return b;
    }));
}
