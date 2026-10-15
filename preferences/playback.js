import { tr } from './language.js';
export const defaultPlayback = () => ({ health: 3, healthFail: 0, mode: 'live', mod: '', rate: 1, pitch: true });

export function validatePlayback(p) {
	const visibility = p?.visibility ?? '';
	if (!['', 'hidden', 'fadein', 'cover', 'flashlight'].includes(visibility)) throw new Error(tr('Visibility 옵션을 확인해 주세요.'));
	const o2jam = p?.o2jam ?? '', pills = !!p?.pills;
	if (!['', 'easy', 'normal', 'hard'].includes(o2jam)) throw new Error(tr('O2Jam 옵션을 확인해 주세요.'));
    const health = p?.health ?? 3, healthFail = p?.healthFail ?? 0;
    if (!Number.isInteger(health) || health < 0 || health > 3 || !Number.isInteger(healthFail) || healthFail < 0 || healthFail > 3 || health > 0 && healthFail > health) throw new Error(tr('체력 시작 단계와 게임 오버 단계를 확인해 주세요.'));
    if (!p || !['live', 'auto'].includes(p.mode) || !['', 'mirror', 'random', 'mirror+random'].includes(p.mod) || !Number.isFinite(p.rate) || p.rate < 0.5 || p.rate > 2 || typeof p.pitch !== 'boolean') throw new Error(tr('플레이 옵션을 확인해 주세요. Time Rate는 0.5~2.0배입니다.'));
    return { health, healthFail, mode: p.mode, mod: p.mod, rate: p.rate, pitch: p.pitch, ...(visibility ? { visibility } : {}), ...(o2jam ? { o2jam, pills } : {}) };
}

export function storedPlayback(storage = globalThis.localStorage) {
    const text = storage.getItem('ggosu:playback');
    return text === null ? defaultPlayback() : validatePlayback(JSON.parse(text));
}

export function savePlayback(p, storage = globalThis.localStorage) {
    const saved = validatePlayback(p);
    storage.setItem('ggosu:playback', JSON.stringify(saved));
    return saved;
}
