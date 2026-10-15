import { problem } from './keytest.js';
import { clampScroll } from './scroll.js';

// Browser-local counterparts of settings.Config. Scroll and cover are shared;
// keys and offsets belong to one key count. Chart offsets use the source hash.
const key = 'ggosu:settings';
const defaults = ['SPACE', 'F,J', 'F,SPACE,J', 'D,F,J,K', 'D,F,SPACE,J,K', 'S,D,F,J,K,L', 'S,D,F,SPACE,J,K,L', 'A,S,D,F,SPACE,J,K,L', 'A,S,D,F,SPACE,J,K,L,SEMICOLON', 'A,S,D,F,V,N,J,K,L,SEMICOLON'];
function read(storage) {
    const text = storage.getItem(key);
    if (text === null) return {};
    const config = JSON.parse(text);
    if (!config || typeof config !== 'object' || Array.isArray(config)) throw new Error('잘못된 브라우저 설정입니다.');
    return config;
}
function write(config, storage) { storage.setItem(key, JSON.stringify(config)); }
function count(lanes) {
    if (!Number.isInteger(lanes) || lanes < 1 || lanes > 18) throw new Error('키 수는 1~18이어야 합니다.');
}
function between(v, lo, hi) { return Number.isFinite(v) && v >= lo && v <= hi; }
export function defaultProfile(lanes) {
    count(lanes);
    const keys = (defaults[lanes - 1] || 'A,S,D,F,G,H,J,K,L,SEMICOLON,Q,W,E,R,T,Y,U,I').split(',').slice(0, lanes);
    return { keys, scrollSpeed: 8, scrollMode: 'speed', exposureMS: 750, speedChange: 1, scrollSpeedOffset: 0, exposureOffset: 0, cover: { sudden: .2, hidden: .1, lift: .1 } };
}
function validate(lanes, p) {
    count(lanes);
    if (!Array.isArray(p.keys) || p.keys.length !== lanes || p.keys.some(k => typeof k !== 'string')) throw new Error('레인 수에 맞게 키를 지정해 주세요.');
    const issue = problem(p.keys);
    if (issue) throw new Error(issue);
    if (!['speed', 'exposure'].includes(p.scrollMode) || !between(p.scrollSpeed, 1, 40) || !Number.isInteger(p.exposureMS) || !between(p.exposureMS, 100, 5000) || !between(p.speedChange, 0, 2) || !between(p.scrollSpeedOffset, -39, 39) || !Number.isInteger(p.exposureOffset) || !between(p.exposureOffset, -4900, 4900)) throw new Error('스크롤 설정 범위를 확인해 주세요.');
    if (!p.cover || ['sudden', 'hidden', 'lift'].some(k => !between(p.cover[k], 0, 1)) || ['suddenOn', 'hiddenOn', 'liftOn'].some(k => p.cover[k] != null && typeof p.cover[k] !== 'boolean')) throw new Error('레인 커버 설정 범위를 확인해 주세요.');
    return p;
}
export function storedProfile(lanes, storage = localStorage) {
    const c = read(storage), p = { ...defaultProfile(lanes), ...c.profiles?.[lanes], ...c.scroll };
    return validate(lanes, p);
}
export function saveProfile(lanes, value, storage = localStorage) {
    const p = structuredClone(value);
    p.keys = p.keys?.map(k => typeof k === 'string' ? k.trim().toUpperCase() : k);
    validate(lanes, p);
    const c = read(storage);
    c.profiles = { ...c.profiles, [lanes]: p };
    const { scrollSpeed, scrollMode, exposureMS, speedChange, cover } = p;
    c.scroll = { scrollSpeed, scrollMode, exposureMS, speedChange, cover };
    write(c, storage);
    return p;
}
export function resetProfile(lanes, storage = localStorage) {
    const p = storedProfile(lanes, storage), d = defaultProfile(lanes);
    return saveProfile(lanes, { ...p, keys: d.keys, scrollSpeedOffset: 0, exposureOffset: 0 }, storage);
}
export function effectiveProfile(p) {
    return { ...p, scrollSpeed: clampScroll(p.scrollSpeed + p.scrollSpeedOffset), exposureMS: Math.min(5000, Math.max(100, p.exposureMS + p.exposureOffset)), scrollSpeedOffset: 0, exposureOffset: 0 };
}
// Keep in-play adjustments without discarding the key count's own offsets.
export function keepProfile(lanes, played, storage = localStorage) {
    const p = storedProfile(lanes, storage);
    return saveProfile(lanes, { ...p, scrollSpeed: clampScroll(played.scrollSpeed - p.scrollSpeedOffset), exposureMS: Math.min(5000, Math.max(100, played.exposureMS - p.exposureOffset)), cover: played.cover ?? p.cover }, storage);
}
export function storedOffsets(hash = '', storage = localStorage) {
    const c = read(storage);
    return validOffsets({ globalMS: c.globalMS ?? 0, chartMS: c.charts?.[hash] ?? 0 });
}
function validOffsets(o) {
    if (['globalMS', 'chartMS'].some(k => !Number.isInteger(o[k]) || !between(o[k], -1000, 1000))) throw new Error('타이밍 조정은 -1000~1000 ms여야 합니다.');
    return o;
}
export function saveOffsets(hash, value, storage = localStorage) {
    const o = validOffsets(value), c = read(storage);
    c.globalMS = o.globalMS;
    if (hash) c.charts = { ...c.charts, [hash]: o.chartMS };
    write(c, storage);
    return { globalMS: o.globalMS, chartMS: hash ? o.chartMS : 0 };
}
export function storedName(storage = localStorage) { return read(storage).name || 'Player'; }
export function saveName(value, storage = localStorage) {
    const name = value.trim();
    if ([...name].length > 32) throw new Error('닉네임은 32자까지예요');
    const c = read(storage); c.name = name; write(c, storage);
    return name || 'Player';
}
