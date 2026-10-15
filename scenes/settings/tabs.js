// Settings section tabs, a vertical list like Quaver's options sidebar.
// Click or ↑↓ on a tab shows its panel; the last tab is remembered per viewer.
const KEY = 'ggosu:ui:settingsTab';
const tabs = () => [...document.querySelectorAll('#settings-tabs [role="tab"]')];
const panel = tab => document.getElementById(tab.getAttribute('aria-controls'));
// A tab whose forms are all unavailable (browser build) is hidden.
function usable(tab) {
    const p = panel(tab), outer = p.closest('form');
    return outer ? !outer.hidden : [...p.querySelectorAll('form')].some(f => !f.hidden);
}

export function show(tab, focus = false) {
    for (const t of tabs()) {
        const on = t === tab;
        t.setAttribute('aria-selected', String(on));
        t.tabIndex = on ? 0 : -1;
        panel(t).hidden = !on;
    }
    if (focus) tab.focus();
    try { localStorage.setItem(KEY, tab.id); } catch { /* Storage is optional. */ }
}

// Called whenever the dialog opens: forms may have become available since the last time.
export function refresh() {
    for (const t of tabs()) t.hidden = !usable(t);
    const visible = tabs().filter(t => !t.hidden);
    let saved = null;
    try { saved = localStorage.getItem(KEY); } catch { /* Storage is optional. */ }
    const current = visible.find(t => t.getAttribute('aria-selected') === 'true') ?? visible.find(t => t.id === saved) ?? visible[0];
    if (current) show(current);
}

export function mount() {
    for (const t of tabs()) t.addEventListener('click', () => show(t));
    document.querySelector('#settings-tabs').addEventListener('keydown', e => {
        if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
        const visible = tabs().filter(t => !t.hidden), at = visible.indexOf(document.activeElement);
        if (at < 0) return;
        e.preventDefault();
        show(visible[(at + (e.key === 'ArrowDown' ? 1 : visible.length - 1)) % visible.length], true);
    });
    refresh();
}
