import { tr } from './language.js';
export function defaultAudio() { return { master: 100, music: 100, keys: 100, effects: 50, hit: 0, hitSound: 'click', guide: 0 }; }

function normalize(value) {
    const audio = { ...defaultAudio(), ...value };
    for (const key of Object.keys(defaultAudio())) {
        if (key === 'hitSound') continue;
        if (!Number.isInteger(audio[key]) || audio[key] < 0 || audio[key] > 100) throw new Error(tr('볼륨은 0~100이어야 합니다.'));
    }
    if (!['click', 'wood', 'glass', 'pluck'].includes(audio.hitSound)) throw new Error(tr('볼륨은 0~100이어야 합니다.'));
    return audio;
}

export function storedAudio(storage = localStorage) {
    for (const key of ['ggosu:audio', 'ggosu:audio:backup']) {
        try {
            const text = storage.getItem(key);
            if (text) return normalize(JSON.parse(text));
        } catch { /* Try the saved backup, then the default volume. */ }
    }
    return defaultAudio();
}

export function saveAudio(value, storage = localStorage) {
    const audio = normalize(value), text = JSON.stringify(audio);
    storage.setItem('ggosu:audio:backup', text);
    storage.setItem('ggosu:audio', text);
    return audio;
}
