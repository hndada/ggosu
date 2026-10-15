// Menu sounds: CC0 samples standing in for osu!lazer's UI samples (sounds/SOURCES.txt names each one).
const KINDS = ['hover', 'select', 'check-on', 'check-off', 'notch', 'pop-in', 'pop-out', 'back', 'error', 'tab', 'move', 'confirm', 'random', 'shutter'];
let context = null, buffers = null, last = -Infinity;

function ready() {
    try { context ??= new AudioContext(); } catch { return false; }
    buffers ??= Object.fromEntries(KINDS.map(k => [k, fetch(`/audio/sounds/${k}.ogg`)
        .then(r => r.ok ? r.arrayBuffer() : Promise.reject(new Error(r.statusText)))
        .then(b => context.decodeAudioData(b)).catch(() => null)]));
    return true;
}
// Decode ahead so the first hover is not late; the context may stay suspended until a gesture.
export function prepare() { ready(); }

// volume: the effects volume setting, 0–100. rate: playback frequency, 1 = as recorded.
export function cue(kind, volume, rate = 1) {
    if (!KINDS.includes(kind) || !volume || !ready()) return;
    if (context.state === 'suspended') context.resume().catch(() => {});
    last = performance.now();
    buffers[kind].then(buffer => {
        if (!buffer) return;
        const source = context.createBufferSource(), gain = context.createGain();
        source.buffer = buffer; source.playbackRate.value = rate;
        gain.gain.value = volume / 100;
        source.connect(gain).connect(context.destination);
        source.start();
    });
}
// When the last cue played, so a generic click sound can step aside for a specific one.
export function lastCue() { return last; }
