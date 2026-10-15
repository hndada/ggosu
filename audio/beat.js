// Beats of the previewing song, after osu!lazer BeatSyncedContainer. A chart's timing list holds
// complete states, so a point that only changes scroll keeps the beat grid of the one before it.
// Times are in milliseconds on the song timeline.

// TimingControlPoint.DEFAULT: 60 BPM in 4/4 from time zero, used while no song plays.
const DEFAULT = { at: 0, length: 1000, meter: 4 };
// BeatSyncedContainer.MinimumBeatLength.
const MINIMUM = 50;

export function grid(timing = []) {
    const points = [];
    for (const t of timing) {
        if (!(t.bpm > 0)) continue;
        const length = 60000 / t.bpm, meter = t.meter > 0 ? t.meter : 4, last = points.at(-1);
        if (last && last.length === length && last.meter === meter) continue;
        points.push({ at: (t.at_ns || 0) / 1e6, length, meter });
    }
    return points;
}

// BeatSyncedContainer.Update: the timing point at time (the first one before it starts),
// the beat index and how far past that beat time is.
export function beatAt(points, time) {
    let point = points[0] ?? DEFAULT;
    for (const p of points) { if (p.at <= time) point = p; else break; }
    let length = point.length;
    while (length < MINIMUM) length *= 2;
    let index = Math.trunc((time - point.at) / length);
    if (time < point.at) index--;
    let until = (point.at - time) % length;
    if (until <= 0) until += length;
    return { point, index, since: length - until };
}

function smallestPrimeFactor(n) {
    for (let i = 2; i <= n / i; i++) if (n % i === 0) return i;
    return n;
}

// Panel.PulsatingBox.getFlashInterval: deeper carousel layers flash every few beats, staying on the bar.
export function flashInterval(beatsPerBar, depth) {
    let beats = 1, remaining = beatsPerBar;
    for (let i = 0; i < depth; i++) {
        let factor = smallestPrimeFactor(remaining);
        if (factor <= 1) factor = 2;
        beats *= factor;
        if (remaining % factor === 0) remaining /= factor;
    }
    return beats;
}
