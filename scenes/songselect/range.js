import { tr } from '../../preferences/language.js';
// Range window widget over a distribution: drag to choose a level or length range instead of typing numbers.
import { toRange, toColumns } from '/songlist/range.js';

// The widget. update() redraws for new data; onChange receives a value range or null.
export function rangeWindow(host, { name, axis, format, onChange, unit = tr('채보') }) {
    host.className = 'range';
    const title = document.createElement('span'); title.className = 'range-title'; title.textContent = tr(name);
    const chart = document.createElement('div'); chart.className = 'range-chart';
    const win = document.createElement('div'); win.className = 'range-window'; win.tabIndex = 0;
    win.setAttribute('role', 'slider'); win.setAttribute('aria-label', tr`${name} 범위`);
    const ticks = document.createElement('div'); ticks.className = 'range-axis';
    const out = document.createElement('span'); out.className = 'range-value';
    host.replaceChildren(title, chart, ticks, out);
    let d = null, values = [], lo = 0, hi = 0, bars = [];

    function emit() { onChange(d ? toRange(d, lo, hi) : null); }
    function draw() {
        if (!d) { chart.replaceChildren(); ticks.replaceChildren(); out.textContent = tr('숫자 값 없음'); return; }
        // With no limit the chart stays neutral; the window appears once a range is chosen.
        const all = lo === 0 && hi === d.n - 1;
        win.style.left = `${lo / d.n * 100}%`;
        win.style.width = `${(hi - lo + 1) / d.n * 100}%`;
        win.hidden = all;
        bars.forEach((b, i) => b.classList.toggle('in', !all && i >= lo && i <= hi));
        const total = values.slice(lo, hi + 1).reduce((a, b) => a + b, 0);
        out.textContent = all ? tr`전체 · ${total}${tr(unit)}` : `${format(d, lo, hi)} · ${total}${tr(unit)}`;
        win.setAttribute('aria-valuetext', out.textContent);
    }
    function update(domain, columnCounts, range) {
        d = domain; values = columnCounts ?? [];
        if (!d) { draw(); return; }
        [lo, hi] = toColumns(d, range);
        const max = Math.max(1, ...values);
        chart.style.gridTemplateColumns = ticks.style.gridTemplateColumns = `repeat(${d.n}, minmax(0, 1fr))`;
        bars = values.map(c => { const b = document.createElement('div'); b.className = 'range-bar'; b.style.height = `${c ? Math.max(8, c / max * 100) : 0}%`; return b; });
        chart.replaceChildren(win, ...bars);
        ticks.replaceChildren(...values.map((_, i) => { const s = document.createElement('span'); s.textContent = axis(d, i); return s; }));
        draw();
    }

    const column = e => { const r = chart.getBoundingClientRect(); return Math.min(d.n - 1, Math.max(0, Math.floor((e.clientX - r.left) / r.width * d.n))); };
    let mode = null, anchor = 0, from = [0, 0];
    chart.addEventListener('mousedown', e => e.preventDefault()); // no text selection while dragging
    chart.addEventListener('pointerdown', e => {
        if (!d) return;
        const c = column(e), r = win.getBoundingClientRect(), edge = Math.max(6, r.width / 6);
        if (lo === 0 && hi === d.n - 1) { mode = 'new'; anchor = c; lo = hi = c; draw(); }
        else if (c >= lo && c <= hi && e.clientX - r.left < edge && hi > lo) mode = 'lo';
        else if (c >= lo && c <= hi && r.right - e.clientX < edge && hi > lo) mode = 'hi';
        else if (c >= lo && c <= hi) { mode = 'move'; anchor = c; from = [lo, hi]; }
        else { mode = 'new'; anchor = c; lo = hi = c; draw(); }
        chart.setPointerCapture(e.pointerId); win.focus();
    });
    chart.addEventListener('pointermove', e => {
        if (!mode) return;
        const c = column(e);
        if (mode === 'lo') lo = Math.min(c, hi);
        if (mode === 'hi') hi = Math.max(c, lo);
        if (mode === 'new') { lo = Math.min(anchor, c); hi = Math.max(anchor, c); }
        if (mode === 'move') { const s = Math.min(d.n - 1 - from[1], Math.max(-from[0], c - anchor)); lo = from[0] + s; hi = from[1] + s; }
        draw();
    });
    // Release is watched on the whole page so a missed pointerup on the chart cannot leave the drag running.
    for (const type of ['pointerup', 'pointercancel']) window.addEventListener(type, () => { if (mode) { mode = null; emit(); } });
    chart.addEventListener('dblclick', () => { if (d) { lo = 0; hi = d.n - 1; draw(); emit(); } });
    // Arrow keys belong to the window while it has focus, not to the song list.
    win.addEventListener('keydown', e => {
        const s = e.key === 'ArrowLeft' ? -1 : e.key === 'ArrowRight' ? 1 : 0;
        if (!s || !d || e.ctrlKey || e.altKey) return;
        e.preventDefault(); e.stopPropagation();
        if (e.shiftKey) hi = Math.min(d.n - 1, Math.max(lo, hi + s));
        else if (lo + s >= 0 && hi + s <= d.n - 1) { lo += s; hi += s; }
        draw(); emit();
    });
    return { update };
}
