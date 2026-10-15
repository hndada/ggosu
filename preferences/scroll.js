// osu!lazer mania scroll speed (1..40): one unit is this many px/s on the play
// field's 1280x720 reference layout (settings.PixelsPerScroll).
export const PIXELS_PER_SCROLL = (768 - 124.8) * 720 / 768 * 1000 / 11485;
export const clampScroll = v => Math.min(40, Math.max(1, v));
// Matches the play field's 1280x720 reference layout. BPM does not affect
// scrolling; chart scroll multipliers and stops can vary exposure during play.
export function exposureMS(profile, rate, display = {}) {
    if (profile.scrollMode === 'exposure') {
        const ms = profile.exposureMS + (profile.exposureOffset ?? 0);
        return Number.isFinite(ms) ? Math.min(5000, Math.max(100, ms)) : null;
    }
    if (!Number.isFinite(rate) || rate <= 0) return null;
    const line = display.hudLayout?.hitPosition || (720 - (display.skin === 'classic' ? 124.8 : 110) * 720 / 768);
    const speed = clampScroll(profile.scrollSpeed + (profile.scrollSpeedOffset ?? 0)) * PIXELS_PER_SCROLL;
    if (!Number.isFinite(speed)) return null;
    return (line - 104) * 1000 / (speed * rate);
}
