// In-window cursor: theme.css hides the system cursor and this draws the arrow the
// game window draws (internal/play/screen.go). It is a manual popover so it stays
// above modal dialogs, which open in the top layer.
const POINTS = '0,0 0,16 4,12.5 6.8,18.6 9.3,17.5 6.6,11.6 11.6,11.6';
const UNIT = 22 / 20; // CSS px per arrow unit at 100%; the arrow is 20 units tall.
const SVG = 'http://www.w3.org/2000/svg';
let el, unit = UNIT;

function arrow(className, edge) {
    const p = document.createElementNS(SVG, 'polygon');
    p.setAttribute('class', className);
    p.setAttribute('points', POINTS);
    p.setAttribute('fill', 'currentColor');
    if (edge) { p.setAttribute('stroke', 'currentColor'); p.setAttribute('stroke-width', '2'); p.setAttribute('stroke-linejoin', 'round'); }
    return p;
}

// raise shows the cursor again above popovers and dialogs opened after it.
export function raise() {
    if (!el?.isConnected || !el.matches(':popover-open')) return;
    el.hidePopover(); el.showPopover();
}

export function mount() {
    el = document.createElement('div');
    el.id = 'cursor';
    el.popover = 'manual';
    el.setAttribute('aria-hidden', 'true');
    const svg = document.createElementNS(SVG, 'svg');
    svg.setAttribute('viewBox', '-1 -1 14 21');
    svg.append(arrow('cursor-edge', true), arrow('cursor-body', false));
    el.append(svg);
    document.body.append(el);
    setSize(100);
    // The box starts one pixel right of and below the pointer, so it never takes clicks.
    addEventListener('pointermove', e => {
        el.style.left = `${e.clientX + 1}px`; el.style.top = `${e.clientY + 1}px`;
        if (!el.matches(':popover-open')) el.showPopover();
    }, { passive: true });
    document.addEventListener('mouseout', e => { if (!e.relatedTarget && el.matches(':popover-open')) el.hidePopover(); });
    // A dialog opened after the cursor would cover it; show the cursor again on top.
    new MutationObserver(raise).observe(document.body, { subtree: true, attributeFilter: ['open'] });
}

// size is the cursor scale in percent, as in the display settings.
export function setSize(size) {
    if (!el) return;
    unit = UNIT * size / 100;
    el.style.width = `${14 * unit}px`; el.style.height = `${21 * unit}px`;
}
