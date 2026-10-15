import { tr } from '../../preferences/language.js';
import { validatePlayback } from '/preferences/playback.js';
import { wrap, paint } from './slider.js';
import { mount as mountMods, mountVisibility, mountO2Jam } from './mods.js';

let ready = false, dirty = false, pending = 0, revision = 0;
let queue = Promise.resolve();
const rates = [1, 1.1, 1.25, 1.5];
// Give the standard rates more room without changing the scale when custom entry toggles.
const stops = [.5, ...rates, 2], positions = [0, .3, .42, .56, .75, 1];
function interpolate(value, from, to) {
    for (let i = 1; i < from.length; i++) {
        if (value <= from[i]) return to[i - 1] + (to[i] - to[i - 1]) * (value - from[i - 1]) / (from[i] - from[i - 1]);
    }
    return to.at(-1);
}
export function needsSave() { return !ready || dirty || pending > 0; }
export function loaded() { return ready; }
export function setBusy(busy) {
    document.getElementById('play-controls').disabled = busy || !ready;
    document.getElementById('rate-controls').disabled = busy || !ready;
}

// Save each valid edit in order, including edits made before the previous save finishes.
export async function mount(changed) {
    const fields = ['mode', 'lane-mod', 'time-rate', 'pitch', 'health-mode', 'health-fail', 'visibility-mod', 'o2jam-mod', 'o2jam-pills'].map(id => document.getElementById(id));
    const note = document.getElementById('playback-status');
    const rate = fields[2], slider = document.getElementById('rate-slider'), custom = document.getElementById('rate-custom');
    const presets = document.getElementById('rate-presets');
    const buttons = [...presets.querySelectorAll('button')];
    let free = false;
    const nearest = value => rates.reduce((a, b) => Math.abs(b - value) < Math.abs(a - value) ? b : a, 1);
    const renderRate = () => {
        const value = rate.valueAsNumber;
        rate.style.width = `${Math.max(1, String(rate.value).length)}ch`;
        document.getElementById('rate-input').hidden = !free;
        document.getElementById('rate-value').hidden = free;
        custom.setAttribute('aria-pressed', String(free));
        document.getElementById('rate-custom-state').textContent = free ? 'ON' : 'OFF';
        if (Number.isFinite(value) && value >= .5 && value <= 2) {
            slider.value = interpolate(value, stops, positions);
            slider.setAttribute('aria-valuenow', String(value));
            slider.setAttribute('aria-valuetext', tr`${value}배`);
            document.getElementById('rate-value').textContent = `${Number.isInteger(value) ? value.toFixed(1) : value}×`;
            paint(slider);
        }
        for (const b of buttons) {
            const preset = Number(b.dataset.rate);
            // Match the slider's 50px nub centre on the same scale in both modes.
            b.style.left = `calc((100% - 50px) * ${interpolate(preset, stops, positions)} + 25px)`;
            b.setAttribute('aria-pressed', String(preset === value));
        }
    };
    const chooseRate = value => {
        rate.value = value;
        rate.dispatchEvent(new Event('input', { bubbles: true }));
    };
    wrap(slider);
    const app = window.go.main.App;
    const read = () => validatePlayback({ mode: fields[0].value, mod: fields[1].value, rate: fields[2].valueAsNumber, pitch: fields[3].checked, health: Number(fields[4].value), healthFail: Number(fields[5].value), visibility: fields[6].value, o2jam: fields[7].value, pills: fields[8].checked });
    try {
        const p = await app.PlaybackSettings();
        fields[0].value = p.mode; fields[1].value = p.mod;
        fields[6].value = p.visibility ?? '';
        fields[7].value = p.o2jam ?? ''; fields[8].checked = !!p.pills;
        fields[2].value = p.rate; fields[3].checked = p.pitch; fields[4].value = p.health ?? 3; fields[5].value = p.healthFail ?? 0;
        free = !rates.includes(p.rate);
        try { free ||= localStorage.getItem('ggosu:ui:custom-rate') === 'true'; } catch { /* Storage is optional. */ }
        ready = true;
    } catch (e) { note.textContent = tr`설정 읽기 실패: ${e?.message ?? e}`; }
    renderRate();
    mountMods(fields[1], [...document.getElementById('arrangement-buttons').querySelectorAll('button')], document.getElementById('arrangement-value'));
    mountVisibility(fields[6], [...document.getElementById('visibility-buttons').querySelectorAll('button')], document.getElementById('visibility-value'));
    mountO2Jam(fields[7], fields[8], document.getElementById('mod-o2jam'), document.getElementById('mod-pills'), document.getElementById('o2jam-value'));
    const healthButtons = [...document.getElementById('health-buttons').querySelectorAll('button')];
    const failButtons = [...document.getElementById('health-fail-buttons').querySelectorAll('button')];
    const renderHealth = () => {
        const level = Number(fields[4].value), o2jam = fields[7].value !== '';
        // The O2Jam mod brings its own life bar, so the stage gauge waits until it is off.
        for (const b of healthButtons) b.disabled = o2jam;
        for (const b of failButtons) {
            b.disabled = o2jam || level === 0 || Number(b.dataset.value) > level;
            b.setAttribute('aria-pressed', String(b.dataset.value === String(fields[5].value)));
        }
    };
    fields[4].addEventListener('input', () => {
        const level = Number(fields[4].value);
        if (level > 0 && Number(fields[5].value) > level) fields[5].value = level;
        renderHealth();
    });
    fields[7].addEventListener('input', renderHealth);
    renderHealth();
    for (const [field, id] of [[fields[0], 'mode-buttons'], [fields[4], 'health-buttons'], [fields[5], 'health-fail-buttons'], [fields[3], 'pitch-buttons']]) {
        const choices = [...document.getElementById(id).querySelectorAll('button')];
        const render = () => {
            const value = field === fields[3] ? String(field.checked) : String(field.value);
            for (const b of choices) b.setAttribute('aria-pressed', String(b.dataset.value === value));
        };
        for (const b of choices) b.addEventListener('click', () => {
            if (field.disabled || b.disabled) return;
            if (field === fields[3]) field.checked = b.dataset.value === 'true';
            else field.value = b.dataset.value;
            field.dispatchEvent(new Event('input', { bubbles: true }));
            field.dispatchEvent(new Event('change', { bubbles: true }));
        });
        field.addEventListener('input', render);
        render();
    }
    changed();
    const save = () => {
        dirty = true;
        let p;
        try { p = read(); }
        catch (e) { ++revision; note.textContent = e.message; changed(); return; }
        const rev = ++revision;
        ++pending; changed();
        queue = queue.then(() => app.SavePlayback(p)).then(() => {
            if (rev === revision) { dirty = false; note.textContent = tr('저장했습니다.'); }
        }).catch(e => { if (rev === revision) note.textContent = tr`설정 저장 실패: ${e?.message ?? e}`; }).finally(() => {
            --pending;
            changed();
        });
    };
    for (const field of fields) field.addEventListener('input', save);
    // A failed save can be retried by committing the field again.
    for (const field of fields) field.addEventListener('change', () => { if (dirty && pending === 0) save(); });
    rate.addEventListener('input', renderRate);
    for (const b of buttons) b.addEventListener('click', () => chooseRate(Number(b.dataset.rate)));
    slider.addEventListener('input', () => {
        const value = interpolate(slider.valueAsNumber, positions, stops);
        chooseRate(free ? Math.round(value * 100) / 100 : nearest(value));
    });
    slider.addEventListener('keydown', e => {
        let value = rate.valueAsNumber;
        const direction = { ArrowLeft: -1, ArrowDown: -1, ArrowRight: 1, ArrowUp: 1, PageDown: -1, PageUp: 1 }[e.key];
        if (e.key === 'Home') value = free ? .5 : rates[0];
        else if (e.key === 'End') value = free ? 2 : rates.at(-1);
        else if (direction) {
            if (free) value = Math.max(.5, Math.min(2, Math.round((value + direction * (e.key.startsWith('Page') ? .1 : .01)) * 100) / 100));
            else value = rates[Math.max(0, Math.min(rates.length - 1, rates.indexOf(nearest(value)) + direction))];
        } else return;
        e.preventDefault();
        chooseRate(value);
    });
    custom.addEventListener('click', () => {
        if (document.getElementById('rate-controls').disabled) return;
        free = !free;
        try { localStorage.setItem('ggosu:ui:custom-rate', String(free)); } catch { /* Storage is optional. */ }
        if (!free && !rates.includes(rate.valueAsNumber)) chooseRate(nearest(rate.valueAsNumber));
        renderRate();
    });
}
