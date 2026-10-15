// Toast notifications after osu!lazer's SimpleNotification in NotificationOverlayToastTray, in the
// bottom-right corner instead of the top-right, newest nearest the corner. Values from lazer:
// - 320 px wide, 20 px from the window edges, 3 px above and below each toast, at least 80 px tall
//   for the message, filename and action on separate lines.
// - Enters by fading in over 200 ms while sliding in from its own width over 500 ms OutQuint; a white
//   layer at 0.8 fades out over 2000 ms OutQuart. Other toasts move over 150 ms OutQuart.
// - After 12 s (12 s more while hovered) it moves 400 px right and fades out over 600 ms OutQuint.
// - Left click runs its action and fades out over 100 ms; right click or the check button flings it
//   left (DragContainer.FlingLeft) while it fades out over 600 ms Easing.In. A close plays pop-out.
// The tray is a manual popover so it stays above modal dialogs, like the cursor.
// theme.css holds the transitions; these durations must match it.
const SHOW = 12000, FORWARD = 600, FLING = 600, FADE = 100;
const SVG = 'http://www.w3.org/2000/svg';
let tray = null, raise = () => {}, closed = () => {};

// onTop runs after the tray is raised, so the cursor can be raised above it again.
// onClose runs when the player closes a toast (not when it times out).
export function mount({ onTop = () => {}, onClose = () => {} } = {}) {
    tray = document.createElement('div');
    tray.id = 'toasts';
    tray.popover = 'manual';
    document.body.append(tray);
    raise = onTop; closed = onClose;
    // A dialog opened after the tray would cover it; show the tray again on top.
    new MutationObserver(() => { if (tray.childElementCount) top(); }).observe(document.body, { subtree: true, attributeFilter: ['open'] });
}

function top() {
    if (tray.matches(':popover-open')) tray.hidePopover();
    tray.showPopover();
    raise();
}

function icon(viewBox, path) {
    const svg = document.createElementNS(SVG, 'svg');
    svg.setAttribute('viewBox', viewBox); svg.setAttribute('aria-hidden', 'true');
    const p = document.createElementNS(SVG, 'path');
    p.setAttribute('d', path); p.setAttribute('fill', 'currentColor');
    svg.append(p);
    return svg;
}
// An info circle (SimpleNotification's default icon) and a check (Notification.CloseButtonIcon).
const INFO = 'M8 0a8 8 0 1 1 0 16A8 8 0 0 1 8 0zm-1 7v5h2V7zm1-4a1.25 1.25 0 1 0 0 2.5A1.25 1.25 0 0 0 8 3z';
const CHECK = 'M4.4 10.6 1.4 7.6 0 9l4.4 4.4L12 5.8l-1.4-1.4z';

// Shows a toast. text may hold line breaks; activate runs on a left click.
export function post(text, activate = () => {}) {
    if (!tray) return;
    const toast = document.createElement('div');
    toast.className = 'toast';
    toast.setAttribute('role', 'status');
    const main = document.createElement('div');
    main.className = 'toast-main';
    const badge = document.createElement('span');
    badge.className = 'toast-icon';
    badge.append(icon('0 0 16 16', INFO));
    const body = document.createElement('p');
    body.className = 'toast-text';
    for (const line of text.split('\n')) {
        const row = document.createElement('span');
        row.textContent = line;
        row.title = line;
        body.append(row);
    }
    const close = document.createElement('button');
    close.type = 'button'; close.className = 'toast-close';
    close.append(icon('0 0 12 16', CHECK));
    const flash = document.createElement('span');
    flash.className = 'toast-flash';
    main.append(badge, body, close, flash);
    toast.append(main);
    // Start from the entry state, then let theme.css transition to rest after the first frame.
    toast.dataset.state = 'enter';

    flow(() => tray.append(toast));
    top();
    requestAnimationFrame(() => requestAnimationFrame(() => { if (toast.dataset.state === 'enter') toast.dataset.state = 'shown'; }));

    let timer = 0, done = false;
    const dismiss = () => { timer = setTimeout(() => toast.matches(':hover') ? dismiss() : forward(), SHOW); };
    dismiss();
    function finish() {
        done = true; clearTimeout(timer);
    }
    // Timed out: out of the flow, then right and away.
    function forward() {
        if (done) return;
        finish();
        // The tray is anchored at its bottom, so hold the toast by its distance from there.
        const at = tray.clientHeight - toast.offsetTop - toast.offsetHeight;
        flow(() => { toast.style.position = 'absolute'; toast.style.bottom = `${at}px`; });
        toast.dataset.state = 'forward';
        setTimeout(() => flow(() => toast.remove()), FORWARD);
    }
    function fade() {
        if (done) return;
        finish(); closed();
        toast.dataset.state = 'fade';
        setTimeout(() => flow(() => toast.remove()), FADE);
    }
    function fling() {
        if (done) return;
        finish(); closed();
        toast.dataset.state = 'fling';
        // DragContainer: at least 0.3 px/ms left plus up to 0.5 more, falling at 0.005 px/ms²,
        // turning 0.1° per px moved left.
        const vx = -0.3 - 0.5 * Math.random(), start = performance.now();
        const step = now => {
            const t = Math.min(FLING, now - start), x = vx * t, y = 0.0025 * t * t;
            toast.style.transform = `translate(${x}px, ${y}px) rotate(${Math.min(0, x * 0.1)}deg)`;
            toast.style.opacity = String(1 - (t / FLING) ** 2);
            if (t < FLING) requestAnimationFrame(step);
            else flow(() => toast.remove());
        };
        requestAnimationFrame(step);
    }
    close.addEventListener('click', e => { e.stopPropagation(); fling(); });
    toast.addEventListener('click', () => { if (done) return; activate(); fade(); });
    toast.addEventListener('contextmenu', e => e.preventDefault());
    toast.addEventListener('pointerdown', e => { if (e.button === 2) fling(); });
}

// flow runs change, then moves the toasts that stay from where they were to where they are now.
function flow(change) {
    const stay = [...tray.children].filter(t => !t.style.position && !t.dataset.state?.match(/forward|fling/));
    const before = new Map(stay.map(t => [t, t.getBoundingClientRect().top]));
    change();
    for (const [t, top] of before) {
        if (!t.isConnected) continue;
        const dy = top - t.getBoundingClientRect().top;
        if (!dy) continue;
        // Jump back without a transition, then let theme.css glide it home.
        t.style.transition = 'none';
        t.style.translate = `0 ${dy}px`;
        t.getBoundingClientRect();
        t.style.transition = '';
        t.style.translate = '';
    }
}
