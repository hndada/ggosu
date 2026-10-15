import { tr } from '../preferences/language.js';
// WSOLA: align overlapping waveform blocks in every channel, then crossfade.
// Matches stretchPCM in internal/play/stretch.go; original chart timestamps remain unchanged.
export function stretch(buffer, rate, context) {
    if (!Number.isFinite(rate) || rate < .5 || rate > 2) throw new Error(tr('배속은 0.5–2.0배여야 합니다.'));
    if (rate === 1) return buffer;
    const n = buffer.length, frames = Math.round(n / rate), channels = buffer.numberOfChannels;
    if (frames * channels * 4 > 512 * 1024 * 1024) throw new Error(tr('오디오 메모리 한도를 초과했습니다.'));
    const out = context.createBuffer(channels, Math.max(1, frames), buffer.sampleRate);
    if (!n) return out;
    const input = Array.from({ length: channels }, (_, ch) => buffer.getChannelData(ch));
    const output = Array.from({ length: channels }, (_, ch) => out.getChannelData(ch));
    const window = Math.min(Math.round(buffer.sampleRate * .02), n);
    const hop = Math.max(1, Math.floor(window / 2)), search = Math.round(buffer.sampleRate * .005);
    output.forEach((data, ch) => data.set(input[ch].subarray(0, Math.min(window, frames))));
    function similarity(a, b, count, step) {
        let cross = 0, x = 0, y = 0;
        for (let i = 0; i < count; i += step) {
            for (const data of input) {
                const u = data[a + i], v = data[b + i];
                cross += u * v; x += u * u; y += v * v;
            }
        }
        return x * y ? cross / Math.sqrt(x * y) : 0;
    }
    let previous = 0;
    for (let at = hop; at < frames; at += hop) {
        const center = Math.min(Math.max(0, Math.round(at * rate)), n - window);
        const lo = Math.max(0, center - search), hi = Math.min(n - window, center + search);
        const count = Math.min(hop, n - previous - hop);
        let best = center, score = -Infinity;
        for (let candidate = lo; candidate <= hi; candidate += 8) {
            const v = similarity(previous + hop, candidate, count, 8);
            if (v > score) { best = candidate; score = v; }
        }
        const coarse = best;
        for (let candidate = Math.max(lo, coarse - 8); candidate <= Math.min(hi, coarse + 8); candidate++) {
            const v = similarity(previous + hop, candidate, count, 4);
            if (v > score) { best = candidate; score = v; }
        }
        for (let i = 0; i < window && at + i < frames; i++) {
            for (let ch = 0; ch < channels; ch++) {
                const value = input[ch][best + i], weight = i / hop;
                output[ch][at + i] = i < hop ? output[ch][at + i] * (1 - weight) + value * weight : value;
            }
        }
        previous = best;
    }
    return out;
}
