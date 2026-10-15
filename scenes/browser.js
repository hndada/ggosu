// The menu is a game screen, not a web page: no text selection, image or link dragging,
// context menu, reload, print, save or find bar. Text fields keep their own editing.
const editable = t => t instanceof Element && !!t.closest('input, textarea, select, [contenteditable=""], [contenteditable="true"]');
const browserKeys = new Set(['F5', 'F3', 'F7', 'F12', 'Ctrl+r', 'Ctrl+Shift+r', 'Ctrl+p', 'Ctrl+s', 'Ctrl+f', 'Ctrl+g', 'Ctrl+u', 'Ctrl+j', 'Ctrl+h', 'Ctrl+o', 'Ctrl+Shift+i']);

export function fullscreen(notify) {
    let busy = false;
    document.addEventListener('keydown', e => {
        if (e.key !== 'Enter' || !e.altKey || e.ctrlKey || e.metaKey || e.shiftKey || e.isComposing) return;
        e.preventDefault(); e.stopImmediatePropagation();
        if (e.repeat || busy) return;
        busy = true;
        const app = window.go?.main?.App;
        const toggle = async () => {
            if (window.runtime?.WindowFullscreen && app?.SaveDisplay) {
                const d = await app.DisplaySettings();
                d.windowMode = ['fullscreen', 'borderless'].includes(d.windowMode) ? 'windowed' : 'fullscreen';
                await app.SaveDisplay(d);
                window.dispatchEvent(new Event('ggosu:display-changed'));
            } else if (document.fullscreenElement) {
                await document.exitFullscreen();
            } else {
                await document.documentElement.requestFullscreen();
            }
        };
        toggle().catch(notify).finally(() => { busy = false; });
    }, true);
}

export function block() {
    document.addEventListener('selectstart', e => { if (!editable(e.target)) e.preventDefault(); });
    document.addEventListener('dragstart', e => { if (!editable(e.target)) e.preventDefault(); });
    document.addEventListener('contextmenu', e => { if (!editable(e.target)) e.preventDefault(); });
    // Ctrl+A outside a field would select the whole page.
    document.addEventListener('keydown', e => {
        const key = `${e.ctrlKey || e.metaKey ? 'Ctrl+' : ''}${e.shiftKey && (e.ctrlKey || e.metaKey) ? 'Shift+' : ''}${e.key.length === 1 ? e.key.toLowerCase() : e.key}`;
        if (browserKeys.has(key) || key === 'Ctrl+a' && !editable(e.target)) e.preventDefault();
    });
    // A middle click would start autoscroll or open a link in a new window.
    document.addEventListener('mousedown', e => { if (e.button === 1) e.preventDefault(); });
    document.addEventListener('auxclick', e => { if (e.button === 1) e.preventDefault(); });
}
