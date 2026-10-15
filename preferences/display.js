import { tr } from './language.js';
export function defaultDisplay() { return { laneLines: false, noteThickness: 100, ghost: 'target', ghostScore: 900000, tps: 240, frameLimit: 'vsync', dim: 75, background: true, storyboard: true, hud: 'standard', skin: 'argon', comboPosition: 'center', judgementPosition: 'center', timingThreshold: 'cool', timeDisplay: 'pie', windowMode: 'windowed', resolution: '1600x900', letterbox: false, letterboxWidth: 80, letterboxHeight: 80, letterboxX: 0, letterboxY: 0, cursorSize: 100, laneDim: 0, noteColour: '', judgementShow: '', hideCombo: false, abort: false }; }

const HUD_ITEMS = ['health', 'song', 'comparison', 'ghost', 'score', 'counts', 'combo', 'judgement', 'time', 'gauges', 'meter'];

export function validateLayout(l = {}) {
    if (!l || typeof l !== 'object' || Array.isArray(l)) throw new Error('Invalid HUD layout');
    if (!within(l.hitPosition ?? 0, 240, 640) && (l.hitPosition ?? 0) !== 0) throw new Error('Invalid hit position');
    const result = {};
    if (l.hitPosition) result.hitPosition = l.hitPosition;
    for (const id of HUD_ITEMS) {
        if (l[id] === undefined) continue;
        const p = l[id];
        if (!p || !within(p.x, -1280, 1280) || !within(p.y, -720, 720) || ((p.scale ?? 0) !== 0 && !within(p.scale, 25, 200))) throw new Error('Invalid HUD position or scale');
        if (p.x || p.y || p.scale && p.scale !== 100) result[id] = { x: p.x, y: p.y, ...(p.scale && p.scale !== 100 ? {scale:p.scale} : {}) };
    }
    for (const id of Object.keys(l)) if (id !== 'hitPosition' && !HUD_ITEMS.includes(id)) throw new Error('Unknown HUD item');
    return result;
}

const within = (v, lo, hi) => Number.isInteger(v) && v >= lo && v <= hi;

export function validateDisplay(d) {
    const noteThickness = d.noteThickness ?? 0;
    if (noteThickness !== 0 && !within(noteThickness, 50, 200)) throw new Error(tr('노트 두께를 확인해 주세요.'));
	if (typeof (d.laneLines ?? false) !== 'boolean') throw new Error(tr('레인 구분선 설정을 확인해 주세요.'));
    if (!['', 'off', 'target', 'max'].includes(d.ghost ?? '') || !within(d.ghostScore ?? 0, 0, 100000000)) throw new Error(tr('Ghost 설정을 확인해 주세요.'));
    if ((d.tps ?? 0) !== 0 && d.tps !== -1 && !within(d.tps, 30, 1000)) throw new Error(tr('TPS 설정을 확인해 주세요.'));
    if (!['', 'vsync', 'unlimited'].includes(d.frameLimit ?? '')) throw new Error(tr('FPS 설정을 확인해 주세요.'));
    if (!Number.isInteger(d.dim) || d.dim < 0 || d.dim > 100 || typeof d.background !== 'boolean' || typeof d.storyboard !== 'boolean' || !['', 'standard', 'focus'].includes(d.hud ?? '')) throw new Error(tr('배경 밝기와 HUD 설정을 확인해 주세요.'));
    for (const p of [d.comboPosition, d.judgementPosition]) if (!['', 'center', 'below', 'left'].includes(p ?? '')) throw new Error(tr('콤보·판정 위치를 확인해 주세요.'));
    if (!['', 'kool', 'cool', 'good', 'off'].includes(d.timingThreshold ?? '')) throw new Error(tr('Early / Late 표시 기준을 확인해 주세요.'));
    if (!['', 'argon', 'triangles', 'classic', 'circles', 'o2jam'].includes(d.skin ?? '')) throw new Error(tr('스킨을 확인해 주세요.'));
    if (!['', 'bar', 'pie'].includes(d.timeDisplay ?? '')) throw new Error(tr('시간 표시 방식을 확인해 주세요.'));
    if (!['', 'windowed', 'borderless', 'fullscreen'].includes(d.windowMode ?? '')) throw new Error(tr('창 모드를 확인해 주세요.'));
    if (!['', '1024x640', '1280x720', '1366x768', '1600x900', '1920x1080', '2560x1440', '3840x2160'].includes(d.resolution ?? '')) throw new Error(tr('해상도를 확인해 주세요.'));
    // Zero sizes and cursor size come from settings saved before these fields existed.
    const size = v => v || 80, cursor = d.cursorSize || 100;
    if (typeof (d.letterbox ?? false) !== 'boolean' || !within(size(d.letterboxWidth), 20, 100) || !within(size(d.letterboxHeight), 20, 100) || !within(d.letterboxX ?? 0, -50, 50) || !within(d.letterboxY ?? 0, -50, 50) || !within(cursor, 50, 200)) throw new Error(tr('letterbox·커서 크기를 확인해 주세요.'));
    if (!within(d.laneDim ?? 0, 0, 100) || !['', 'snap', 'assist'].includes(d.noteColour ?? '') || !['', 'cool', 'good', 'miss', 'off'].includes(d.judgementShow ?? '') || typeof (d.hideCombo ?? false) !== 'boolean' || typeof (d.abort ?? false) !== 'boolean') throw new Error(tr('판정 표시 설정을 확인해 주세요.'));
    const layout = validateLayout(d.hudLayout);
    return { noteThickness: noteThickness || 100, laneLines: d.laneLines ?? false, ...(Object.keys(layout).length ? { hudLayout: layout } : {}), ghost: d.ghost || 'target', ghostScore: d.ghostScore || 900000, tps: d.tps || 240, frameLimit: d.frameLimit || 'vsync', dim: d.dim, background: d.background, storyboard: d.storyboard, hud: d.hud || 'standard', skin: d.skin || 'argon', comboPosition: d.comboPosition || 'center', judgementPosition: d.judgementPosition || 'center', timingThreshold: d.timingThreshold || 'cool', timeDisplay: d.timeDisplay || 'pie', windowMode: d.windowMode || 'windowed', resolution: d.resolution || '1600x900', letterbox: d.letterbox ?? false, letterboxWidth: size(d.letterboxWidth), letterboxHeight: size(d.letterboxHeight), letterboxX: d.letterboxX ?? 0, letterboxY: d.letterboxY ?? 0, cursorSize: cursor, laneDim: d.laneDim ?? 0, noteColour: d.noteColour ?? '', judgementShow: d.judgementShow ?? '', hideCombo: d.hideCombo ?? false, abort: d.abort ?? false };
}

export function saveDisplay(d, storage = globalThis.localStorage) {
    const saved = validateDisplay(d), text = JSON.stringify(saved);
    storage.setItem('ggosu:display:backup', text);
    storage.setItem('ggosu:display', text);
    return saved;
}

export function storedDisplay(storage = globalThis.localStorage, report = () => {}) {
    let text;
    const parse = raw => {
        const value = JSON.parse(raw);
        if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('Invalid display settings');
        return validateDisplay({ ...defaultDisplay(), ...value });
    };
    try { text = storage.getItem('ggosu:display'); }
    catch { return defaultDisplay(); }
    if (text === null) {
        try {
            const raw = storage.getItem('ggosu:display:backup');
            if (raw === null) return defaultDisplay();
            const restored = parse(raw); storage.setItem('ggosu:display', JSON.stringify(restored));
            report(tr('누락된 화면 설정을 백업에서 복구했습니다.')); return restored;
        } catch { return defaultDisplay(); }
    }
    try { return parse(text); }
    catch {
        let restored = defaultDisplay(), backup = false;
        try { const raw = storage.getItem('ggosu:display:backup'); if (raw !== null) { restored = parse(raw); backup = true; } } catch { /* An invalid backup cannot replace defaults. */ }
        try {
            storage.setItem('ggosu:display:invalid:' + Date.now() + ':' + Math.random(), text);
            storage.setItem('ggosu:display', JSON.stringify(restored));
            report(tr`화면 설정을 ${backup ? tr('백업') : tr('기본값')}으로 복구했습니다. 원본은 별도로 보존했습니다.`);
        } catch { /* Storage denial leaves the original untouched. */ }
        return restored;
    }
}
