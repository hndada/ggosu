import { tr } from '../preferences/language.js';
// Volume overlay after osu!lazer's VolumeOverlay/VolumeMeter: a column of three-quarter rings at the
// left edge, master in the middle and larger, shown by the wheel and hidden a second after the last use.
const METERS = [
    { id: 'effects', name: tr('효과음'), size: 125 },
    { id: 'master', name: tr('마스터'), size: 150 },
    { id: 'music', name: tr('음악'), size: 125 },
    { id: 'keys', name: tr('키음'), size: 125 },
];
const STEP = 5;                 // 5 percentage points per step.
const ACCELERATION = 1.8, MAX_ACCELERATION = 5, ACCELERATION_RESET = 150;
const POP_OUT_DELAY = 1000, FADE = 100, DISPLAY_EASE = 400;
const SVG = 'http://www.w3.org/2000/svg';

// get(): {master, music, keys, effects} or null before settings load; set(id, value): false when it cannot change now.
export function mount(parent, { get, set, tick }) {
    const root = document.createElement('div');
    root.id = 'volume'; root.hidden = true; root.setAttribute('aria-label', tr('볼륨'));
    const meters = METERS.map(m => {
        const el = document.createElement('div');
        el.className = 'volume-meter'; el.dataset.volume = m.id;
        const svg = document.createElementNS(SVG, 'svg');
        svg.setAttribute('viewBox', '0 0 100 100'); svg.setAttribute('width', m.size); svg.setAttribute('height', m.size);
        // Both arcs start at the bottom and run clockwise for three quarters of the circle.
        const arc = cls => {
            const c = document.createElementNS(SVG, 'circle');
            for (const [k, v] of Object.entries({ class: cls, cx: 50, cy: 50, r: 38.25, fill: 'none', stroke: 'currentColor', 'stroke-width': 3, pathLength: 100, transform: 'rotate(90 50 50)' })) c.setAttribute(k, v);
            svg.append(c); return c;
        };
        arc('volume-track').setAttribute('stroke-dasharray', '75 100');
        const fill = arc('volume-fill');
        const value = document.createElement('b'); value.className = 'volume-value';
        const name = document.createElement('span'); name.className = 'volume-name'; name.textContent = m.name;
        const ring = document.createElement('div'); ring.className = 'volume-ring'; ring.append(svg, value);
        el.append(ring, name);
        el.addEventListener('click', () => { select(m.id); poke(); });
        root.append(el);
        return { ...m, el, fill, value, shown: null, from: 0, to: 0, start: 0 };
    });
    parent.append(root);

    let selected = 'master', hideTimer = 0, fadeTimer = 0, hovered = false, frame = 0;
    let acceleration = 1, lastAdjust = 0, accumulated = 0, lastTick = 0;
    root.addEventListener('pointerenter', () => { hovered = true; clearTimeout(hideTimer); });
    root.addEventListener('pointerleave', () => { hovered = false; poke(); });

    function select(id) { selected = id; for (const m of meters) m.el.toggleAttribute('data-selected', m.id === id); }
    function visible() { return !root.hidden && root.hasAttribute('data-shown'); }
    function place() {
        // Modal dialogs live above the document; follow the currently open layer.
        const layer = document.querySelector('dialog[open]') || parent;
        if (root.parentElement !== layer) { hovered = false; layer.append(root); }
    }
    function show() {
        if (!get()) return false;
        place();
        clearTimeout(fadeTimer);
        if (root.hidden) { select('master'); for (const m of meters) m.shown = null; draw(); root.hidden = false; }
        requestAnimationFrame(() => root.setAttribute('data-shown', ''));
        poke();
        return true;
    }
    function hide() {
        clearTimeout(hideTimer); root.removeAttribute('data-shown');
        fadeTimer = setTimeout(() => { root.hidden = true; }, FADE);
    }
    function poke() { clearTimeout(hideTimer); if (!hovered) hideTimer = setTimeout(hide, POP_OUT_DELAY); }

    // Values ease to their target over 400 ms (OutQuint), as the meter's displayed volume does.
    function draw() {
        frame = 0;
        const v = get(); if (!v) return;
        const now = performance.now();
        let moving = false;
        for (const m of meters) {
            if (m.to !== v[m.id] || m.shown === null) {
                m.from = m.shown ?? v[m.id]; m.to = v[m.id]; m.start = now;
            }
            const t = Math.min(1, (now - m.start) / DISPLAY_EASE);
            m.shown = m.from + (m.to - m.from) * (1 - (1 - t) ** 5);
            if (t < 1) moving = true;
            m.fill.setAttribute('stroke-dasharray', `${m.shown * 0.75} 100`);
            m.value.textContent = m.shown >= 99.5 ? 'MAX' : Math.round(m.shown);
            m.el.toggleAttribute('data-max', m.shown >= 99.5);
        }
        if (moving) frame = requestAnimationFrame(draw);
    }
    function redraw() { if (!frame) frame = requestAnimationFrame(draw); }

    // amount: wheel notches (precise input gives fractions). Rapid input accelerates ×1.8 per step up to ×5.
    function adjust(amount, accelerated = true) {
        const v = get(); if (!v || !amount) return;
        const now = performance.now();
        if (now - lastAdjust > ACCELERATION_RESET) { acceleration = 1; accumulated = 0; }
        lastAdjust = now;
        const delta = amount * (accelerated ? acceleration : 1) * STEP;
        if (accelerated) acceleration = Math.min(MAX_ACCELERATION, acceleration * ACCELERATION);
        let steps;
        if (Math.abs(amount) >= 1) steps = Math.sign(delta) * Math.max(STEP, Math.round(Math.abs(delta)));
        else { accumulated += delta; steps = Math.trunc(accumulated); accumulated -= steps; }
        const next = Math.max(0, Math.min(100, v[selected] + steps));
        if (next !== v[selected] && set(selected, next)) {
            redraw();
            if (now - lastTick > 30) { lastTick = now; tick?.(); }
        }
        poke();
    }

    return {
        // Wheel: the first notch only shows the overlay; later ones change the selected volume.
        wheel(e) {
            place();
            if (!visible()) { show(); return; }
            adjust(-e.deltaY / (e.deltaMode === 1 ? 3 : e.deltaMode === 2 ? 0.1 : 100));
        },
        // Alt+↑/↓ changes the volume, Alt+←/→ picks the meter (osu!'s volume and next/previous meter keys).
        key(e) {
            if (!e.altKey || !['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight'].includes(e.key)) return false;
            place();
            if (!visible() && !show()) return true;
            if (e.key === 'ArrowUp' || e.key === 'ArrowDown') adjust(e.key === 'ArrowUp' ? 1 : -1, false);
            else {
                const at = meters.findIndex(m => m.id === selected);
                select(meters[(at + (e.key === 'ArrowRight' ? 1 : meters.length - 1)) % meters.length].id);
                poke();
            }
            return true;
        },
        hide,
    };
}
