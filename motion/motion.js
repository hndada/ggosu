// osu!lazer feedback and transitions for the menu screens (decision 28 → 3, 29 → 2). This module
// only builds button parts and switches classes; every duration and curve lives in theme.css.
//
// Buttons follow ShearedButton: the button is sheared by 0.2 while its label stays upright, hover
// brightens it over 150 ms, pressing shrinks to 0.9 over 2 s, release springs back over 1 s, and a
// click flashes 90% white. Screens fade in like lazer screens (300 ms), the title slides out like
// MainMenu (400 ms), and popup dialogs grow from 0.7 like PopupDialog over a half-black backdrop. The
// settings window has its own SettingsPanel slide (settings/panel.js).

// The title menu, website credit and lane keyboard keep their own presentation.
const SKIP = '#title-menu button, #title-web, .key-lane';
const POPUP = 'dialog:not(#settings):not(#chart-menu)';
const SCREENS = ['#title', '#songselect', '#editor'];
const PARTS = new Set(['feel-hover', 'feel-flash', 'feel-label']);

// Every button gets a hover layer, a flash layer and an upright label holding its own content. They are
// their own tags, not spans, so rules like `#player > span` keep matching only the button's content.
// Code that later replaces a button's content (textContent, replaceChildren) is fixed up by the observer.
function feel(b) {
    if (b.matches(SKIP)) return;
    const loose = [...b.childNodes].filter(n => !(n instanceof Element && PARTS.has(n.className)));
    let label = b.querySelector(':scope > .feel-label');
    if (label && !loose.length && b.querySelector(':scope > .feel-hover') && b.querySelector(':scope > .feel-flash')) return;
    b.classList.add('feel');
    for (const cls of ['feel-hover', 'feel-flash']) {
        if (b.querySelector(`:scope > .${cls}`)) continue;
        const layer = document.createElement('feel-layer'); layer.className = cls; b.prepend(layer);
    }
    if (!label) { label = document.createElement('feel-label'); label.className = 'feel-label'; b.append(label); }
    label.append(...loose);
}

const button = e => {
    const b = e.target instanceof Element ? e.target.closest('button') : null;
    return b && !b.disabled && !b.matches(SKIP) ? b : null;
};

// Restart a class-driven transition from its starting state.
function restart(el, cls) {
    el.classList.add(cls);
    void el.offsetWidth;
    el.classList.remove(cls);
}

export function mount() {
    for (const b of document.querySelectorAll('button')) feel(b);
    document.addEventListener('pointerover', e => {
        const b = button(e);
        if (b && !b.contains(e.relatedTarget)) b.classList.add('feel-hovered');
    });
    document.addEventListener('pointerout', e => {
        const b = button(e);
        if (b && !b.contains(e.relatedTarget)) b.classList.remove('feel-hovered', 'feel-press');
    });
    document.addEventListener('pointerdown', e => {
        const b = button(e);
        if (b && e.button === 0) b.classList.add('feel-press');
    });
    const release = () => { for (const b of document.querySelectorAll('button.feel-press')) b.classList.remove('feel-press'); };
    document.addEventListener('pointerup', release);
    document.addEventListener('pointercancel', release);
    document.addEventListener('click', e => {
        const b = button(e);
        if (b?.classList.contains('feel')) restart(b, 'feel-flash-on');
    });

    new MutationObserver(records => {
        const buttons = new Set();
        for (const r of records) {
            const el = r.target;
            if (r.type === 'childList') {
                if (el instanceof HTMLButtonElement) buttons.add(el);
                for (const n of r.addedNodes) if (n instanceof Element) {
                    if (n instanceof HTMLButtonElement) buttons.add(n);
                    for (const b of n.querySelectorAll('button')) buttons.add(b);
                }
                continue;
            }
            // Screens fade in and popups grow in on the frame they appear, whoever showed them.
            if (r.attributeName === 'hidden' && !el.hidden && SCREENS.some(s => el.matches(s))) restart(el, 'entering');
            if (r.attributeName === 'open' && el.open && el.matches(POPUP)) { el.classList.add('popup'); restart(el, 'entering'); }
        }
        for (const b of buttons) feel(b);
    }).observe(document.body, { subtree: true, childList: true, attributes: true, attributeFilter: ['hidden', 'open'] });
}

// The title leaves like lazer's MainMenu: a copy slides left and fades over 400 ms while the real
// one is already hidden, so key handling and focus move on at once.
export function leave(title) {
    if (title.hidden) return;
    const ghost = title.cloneNode(true);
    ghost.classList.add('leaving-ghost');
    ghost.inert = true;
    ghost.setAttribute('aria-hidden', 'true');
    title.parentElement.append(ghost);
    title.hidden = true;
    void ghost.offsetWidth;
    ghost.classList.add('gone');
    ghost.addEventListener('transitionend', e => { if (e.target === ghost) ghost.remove(); });
    setTimeout(() => ghost.remove(), 500);
}
