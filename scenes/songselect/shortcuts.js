import { tr } from '../../preferences/language.js';
const $ = selector => document.querySelector(selector);
let backgroundFocus = null;
export function background(show = false) {
    if (document.body.classList.contains('background-view') === show) return;
    if (show) backgroundFocus = document.activeElement;
    document.body.classList.toggle('background-view', show);
    $('#app').inert = show;
    if (!show) {
        if (!$('#songselect').hidden && backgroundFocus?.isConnected) backgroundFocus.focus({ preventScroll: true });
        backgroundFocus = null;
    }
}
function keyName(e) {
    const key = e.key === ' ' ? 'Space' : e.key.length === 1 ? e.key.toUpperCase() : e.key;
    return [(e.ctrlKey || e.metaKey) && 'Ctrl', e.altKey && 'Alt', e.shiftKey && key !== '+' && 'Shift', key].filter(Boolean).join('+');
}
export function hints(actions, hinted, nav) {
    // Big command buttons: the label first, its key small underneath.
    const buttons = hinted.flatMap(key => {
        const a = actions.find(x => x.key === key);
        const label = tr(typeof a.label === 'function' ? a.label() : a.label);
        if (!label) return [];
        const b = document.createElement('button'); b.type = 'button';
        b.append(Object.assign(document.createElement('span'), { textContent: label }), Object.assign(document.createElement('kbd'), { textContent: a.hint ?? (key === 'Escape' ? 'Esc' : key) }));
        b.addEventListener('click', () => a.run());
        return [b];
    });
    $(nav).replaceChildren(...buttons);
}
export function mount(actions, search) {
    const typing = el => el instanceof HTMLInputElement || el instanceof HTMLSelectElement || el instanceof HTMLTextAreaElement;
    document.addEventListener('keydown', e => {
        if (!document.body.classList.contains('background-view')) return;
        e.preventDefault(); e.stopImmediatePropagation();
        if (!e.isComposing && !e.repeat && (keyName(e) === 'Tab' || keyName(e) === 'Escape')) background();
    }, true);
    document.addEventListener('keydown', e => {
        if ($('#songselect').hidden || document.querySelector('dialog[open]') || e.isComposing || e.defaultPrevented) return;
        const name = keyName(e);
        const inSearch = e.target === search;
        // Search fields keep Select All and word deletion.
        if (inSearch && (name === 'Ctrl+Backspace' || name === 'Ctrl+A' && search.value)) return;
        // Other fields keep their own keys; Esc leaves them and returns to the list.
        if (typing(e.target) && !inSearch) {
            if (name === 'Escape') { e.target.blur(); e.preventDefault(); }
            return;
        }
        // Buttons keep Enter and Space for themselves.
        if (e.target instanceof HTMLButtonElement && (name === 'Enter' || name === 'Space')) return;
        const action = actions.find(a => a.key === name);
        if (action) { e.preventDefault(); if (!e.repeat || /^((Shift\+)?Arrow|Home|End|Page|Alt\+Page)/.test(name)) action.run(); return; }
        if (name === 'Ctrl+R') { e.preventDefault(); return; }
        // Typing anywhere starts a search; the character goes into the search box.
        if (!inSearch && e.key.length === 1 && !e.ctrlKey && !e.altKey && !e.metaKey) search.focus();
    });
}

// Settings are available from every menu; the editor owns its own Ctrl+O.
export function settings(open, close, busy) {
    document.addEventListener('keydown', e => {
        if (e.isComposing || e.altKey || e.shiftKey || !(e.ctrlKey || e.metaKey) || e.key.toLowerCase() !== 'o') return;
        if (busy() || !$('#editor').hidden || document.body.classList.contains('background-view')) return;
        const dialog = document.querySelector('dialog[open]');
        if (dialog && dialog !== $('#settings')) return;
        e.preventDefault(); e.stopImmediatePropagation();
        if (!e.repeat) { if (dialog) close(); else open(); }
    }, true);
}

export function mods() {
    document.addEventListener('keydown', e => {
        if (!$('#mods-dialog').open || e.isComposing || e.ctrlKey || e.metaKey || e.altKey || e.shiftKey) return;
        if (e.target.closest('input:not([type="hidden"]), textarea, select, [contenteditable]')) return;
        const key = e.key.toLowerCase();
        let selector = {
            x: '#arrangement-buttons [data-mod="random"]',
            g: '#visibility-buttons [data-visibility="flashlight"]',
            v: `#mode-buttons [data-value="${$('#mode').value === 'auto' ? 'live' : 'auto'}"]`,
        }[key];
        if (key === 'f') {
            const next = { fadein: 'hidden', hidden: '', '': 'fadein' }[$('#visibility-mod').value] ?? 'fadein';
            selector = `#visibility-buttons [data-visibility="${next || 'hidden'}"]`;
        }
        if (!selector) return;
        e.preventDefault();
        if (!e.repeat) $(selector).click();
    });
}
