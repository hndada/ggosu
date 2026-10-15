// Control sounds after osu!lazer: HoverClickSounds on buttons, OsuCheckbox, OsuSliderBar notch ticks
// and overlay pop-in/out on dialogs. Screens that play their own cue for a click keep it; the generic
// select sound is skipped when another cue already played for the same click.
import { cue, lastCue, prepare } from '/audio/sound.js';

const DEBOUNCE = 50;        // OsuGameBase.SAMPLE_DEBOUNCE_TIME, shared by every hover sound.
const NOTCH_DEBOUNCE = 30;  // OsuSliderBar: at most one notch tick per 30 ms, never twice for one value.
// Chart list rows ([role="option"]) stay silent on hover: sweeping the pointer over the list rattled.
const pressable = 'button, select, summary, a[href], [role="tab"]';

// volume(): the effects volume, 0–100.
export function mountControlSounds(volume) {
    prepare();
    let hoverAt = -Infinity, notchAt = -Infinity, notchValue = null, hovered = null;
    // pointerover bubbles; play once when the pointer enters a new control, not while it moves inside it.
    document.addEventListener('pointerover', e => {
        const el = e.target.closest?.(pressable);
        if (el === hovered) return;
        hovered = el;
        // No hover sounds while dragging or scrolling with a button held.
        if (!el || el.disabled || e.buttons) return;
        const now = performance.now();
        if (now - hoverAt < DEBOUNCE) return;
        hoverAt = now;
        cue('hover', volume(), 0.98 + Math.random() * 0.04); // pitch variation 0.02
    });
    document.addEventListener('click', e => {
        const el = e.target.closest?.('button, summary, [role="tab"], [role="option"]');
        if (!el || el.disabled || performance.now() - lastCue() < 5) return;
        cue('select', volume(), 0.99 + Math.random() * 0.02); // pitch variation 0.01
    });
    document.addEventListener('change', e => {
        const el = e.target;
        if (el instanceof HTMLInputElement && el.type === 'checkbox') cue(el.checked ? 'check-on' : 'check-off', volume());
    });
    document.addEventListener('input', e => {
        const el = e.target;
        if (!(el instanceof HTMLInputElement) || el.type !== 'range') return;
        const now = performance.now(), value = el.valueAsNumber;
        if (now - notchAt <= NOTCH_DEBOUNCE || value === notchValue) return;
        notchAt = now; notchValue = value;
        const min = Number(el.min || 0), max = Number(el.max || 100);
        const normalized = max > min ? (value - min) / (max - min) : 0;
        // Higher values tick higher; the extremes drop half an octave.
        let rate = 0.99 + Math.random() * 0.02 + normalized * 0.2;
        if (value === min || value === max) rate -= 0.5;
        cue('notch', volume(), rate);
    });
    // Dialogs: pop in on open, pop out on close, whatever opened or closed them.
    new MutationObserver(records => {
        for (const r of records) {
            if (r.target.tagName !== 'DIALOG') continue;
            const open = r.target.hasAttribute('open');
            if (open !== (r.oldValue !== null)) cue(open ? 'pop-in' : 'pop-out', volume());
        }
    }).observe(document.body, { subtree: true, attributes: true, attributeFilter: ['open'], attributeOldValue: true });
}
