import { tr } from '../../preferences/language.js';
// Mirror and Random are independent; the hidden field carries the combined value.
export function mount(field, buttons, note) {
    const render = () => {
        const active = field.value.split('+');
        for (const b of buttons) {
            const on = active.includes(b.dataset.mod);
            b.setAttribute('aria-pressed', String(on));
            const state = b.querySelector('.mod-state');
            if (state) state.textContent = on ? 'ON' : 'OFF';
        }
        if (note) note.textContent = { '': tr('변경 없음'), mirror: 'Mirror', random: 'Random', 'mirror+random': 'Mirror + Random' }[field.value];
    };
    for (const b of buttons) b.addEventListener('click', () => {
        if (field.disabled) return;
        const active = new Set(field.value.split('+').filter(Boolean));
        if (active.has(b.dataset.mod)) active.delete(b.dataset.mod);
        else active.add(b.dataset.mod);
        field.value = ['mirror', 'random'].filter(mod => active.has(mod)).join('+');
        field.dispatchEvent(new Event('input', { bubbles: true }));
        field.dispatchEvent(new Event('change', { bubbles: true }));
    });
    field.addEventListener('input', render);
    field.addEventListener('change', render);
    render();
    return render;
}

// The O2Jam event mod cycles OFF → EASY → NORMAL → HARD on one button, as osu! mod variants do.
// Pills only apply while it is on.
export function mountO2Jam(field, pills, button, pillButton, note) {
    const levels = ['', 'easy', 'normal', 'hard'];
    const render = () => {
        const on = field.value !== '';
        const label = on ? field.value.toUpperCase() : 'OFF';
        button.setAttribute('aria-pressed', String(on));
        button.querySelector('.mod-state').textContent = label;
        pillButton.disabled = !on;
        pillButton.setAttribute('aria-pressed', String(on && pills.checked));
        pillButton.querySelector('.mod-state').textContent = on && pills.checked ? 'ON' : 'OFF';
        if (note) note.textContent = on ? label + (pills.checked ? ' + PL' : '') : 'OFF';
    };
    const commit = target => {
        target.dispatchEvent(new Event('input', { bubbles: true }));
        target.dispatchEvent(new Event('change', { bubbles: true }));
    };
    button.addEventListener('click', () => {
        if (field.disabled) return;
        field.value = levels[(levels.indexOf(field.value) + 1) % levels.length];
        commit(field);
    });
    pillButton.addEventListener('click', () => {
        if (pills.disabled || field.value === '') return;
        pills.checked = !pills.checked;
        commit(pills);
    });
    for (const f of [field, pills]) {
        f.addEventListener('input', render);
        f.addEventListener('change', render);
    }
    render();
    return render;
}

// Visibility effects are mutually exclusive. Clicking the active effect turns it off.
export function mountVisibility(field, buttons, note) {
    const labels = { '': 'OFF', hidden: 'Hidden', fadein: 'Fade In', cover: 'Cover', flashlight: 'Flashlight' };
    const render = () => {
        for (const b of buttons) {
            const on = field.value === b.dataset.visibility;
            b.setAttribute('aria-pressed', String(on));
            const state = b.querySelector('.mod-state');
            if (state) state.textContent = on ? 'ON' : 'OFF';
        }
        if (note) note.textContent = labels[field.value] ?? 'OFF';
    };
    for (const b of buttons) b.addEventListener('click', () => {
        if (field.disabled) return;
        field.value = field.value === b.dataset.visibility ? '' : b.dataset.visibility;
        field.dispatchEvent(new Event('input', { bubbles: true }));
        field.dispatchEvent(new Event('change', { bubbles: true }));
    });
    field.addEventListener('input', render);
    field.addEventListener('change', render);
    render();
    return render;
}
