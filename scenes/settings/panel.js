// The settings window slides in and out like osu!lazer's SettingsPanel (theme.css has the
// timings): it moves in from the left over 600 ms while fading in over 300 ms, and leaves the
// same way before it actually closes.
const TRANSITION = 600;
const reduce = () => matchMedia('(prefers-reduced-motion: reduce)').matches;
let dialog, leaving = 0;

export function mount(el) {
    dialog = el;
    // Esc and the close button both start the slide out instead of closing at once.
    dialog.addEventListener('cancel', e => {
        if (e.defaultPrevented || !e.cancelable) return;
        e.preventDefault(); close();
    });
    for (const form of dialog.querySelectorAll('form[method="dialog"]')) {
        form.addEventListener('submit', e => { e.preventDefault(); close(); });
    }
}

export function open() {
    if (leaving) { clearTimeout(leaving); leaving = 0; }
    if (!dialog.open) { dialog.classList.add('away'); dialog.showModal(); }
    void dialog.offsetWidth;
    dialog.classList.remove('away');
}

export function close() {
    if (!dialog.open || leaving) return;
    dialog.classList.add('away');
    leaving = setTimeout(() => { leaving = 0; dialog.close(); dialog.classList.remove('away'); }, reduce() ? 0 : TRANSITION);
}
