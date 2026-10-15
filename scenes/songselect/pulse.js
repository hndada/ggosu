// The selected row pulses with the previewing song, after osu!lazer Panel.PulsatingBox: on a beat its
// highlight brightens to 0.8 over 40 ms (Easing.Out), then settles back to 0.4 over one beat, doubled
// until at least 250 ms. The selected chart flashes every beat and its open song every bar factor
// (getFlashInterval), 40 ms early. With no preview playing it keeps time at 60 BPM, as lazer does.
// Full combo and perfect lamps keep the same time: their glow flashes on every beat like the selected chart,
// and a shine runs down them on the open song's beats.
// The highlight colours are --pulse-* in theme.css; the beat sets the timing, so it is set here.
import { beatAt, flashInterval } from '/audio/beat.js';

const EARLY = 40, PEAK = 40;
const OUT = 'cubic-bezier(0.5, 1, 0.89, 1)'; // osu! Easing.Out (OutQuad)
const LAMPS = '.combo-lamp[data-state="fullcombo"], .combo-lamp[data-state="perfect"]';
const SHINE = [
    { opacity: 0, transform: 'translateY(0)' },
    { opacity: 1, offset: 0.1 },
    { opacity: 1, offset: 0.45 },
    { opacity: 0, transform: 'translateY(14px)', offset: 0.55 },
    { opacity: 0, transform: 'translateY(14px)' },
];
const reducedMotion = matchMedia('(prefers-reduced-motion: reduce)');

export function mount({ list, screen, beat }) {
    const last = new Map(); // row → beat it last flashed on
    function stop(row) {
        for (const a of row.getAnimations()) if (a.id === 'pulse') a.cancel();
        last.delete(row);
    }
    function flash(row, length, since) {
        const from = getComputedStyle(row).backgroundColor;
        stop(row);
        const peak = row.hasAttribute('aria-expanded') ? 'var(--pulse-peak-song)' : 'var(--pulse-peak)';
        // The settled value is the row's own background in theme.css, so the end keyframe is left implicit.
        const a = row.animate([
            { backgroundColor: from, easing: OUT },
            { backgroundColor: peak, offset: PEAK / (PEAK + length), easing: OUT },
        ], { duration: PEAK + length, delay: -since });
        a.id = 'pulse';
    }
    let lampBeat = '';
    function lamps(point, index, since, length) {
        const shine = index % flashInterval(point.meter, 1) === 0;
        for (const lamp of list.querySelectorAll(LAMPS)) {
            lamp.querySelector('.lamp-flash')?.animate([{ opacity: 1, easing: 'cubic-bezier(0.2, 0, 0.3, 1)' }, { opacity: 0 }], { duration: length, delay: -since });
            if (shine) lamp.querySelector('.lamp-shine')?.animate(SHINE, { duration: length, delay: -since, easing: 'ease-in-out' });
        }
    }
    function frame(now) {
        requestAnimationFrame(frame);
        const rows = screen.hidden || screen.classList.contains('leaving') || reducedMotion.matches ? []
            : [...list.querySelectorAll('li[aria-selected="true"], li[aria-expanded="true"]')]
                .filter(row => !row.matches('.activated, .released'));
        for (const row of last.keys()) if (!rows.includes(row)) stop(row);
        if (screen.hidden || reducedMotion.matches) return;
        const playing = beat();
        const time = playing ? playing.time + EARLY * playing.rate : now + EARLY;
        const { point, index, since } = beatAt(playing?.points ?? [], time);
        const key = `${point.at}:${index}`;
        let length = point.length;
        while (length < 250) length *= 2;
        if (lampBeat !== key) { lampBeat = key; lamps(point, index, since, length); }
        for (const row of rows) {
            if (last.get(row) === key) continue;
            last.set(row, key);
            const depth = row.hasAttribute('aria-expanded') ? 1 : 0;
            if (index % flashInterval(point.meter, depth) === 0) flash(row, length, since);
        }
    }
    requestAnimationFrame(frame);
}
