import test from 'node:test';
import assert from 'node:assert/strict';
import { exposureMS, PIXELS_PER_SCROLL } from './scroll.js';

const profile = { scrollSpeed: 8, scrollMode: 'speed', exposureMS: 750 };
test('exposure follows effective scroll, rate, skin and dragged hit line without chart data', () => {
    const ms = exposureMS(profile, 1), px = 8 * PIXELS_PER_SCROLL;
    assert.equal(ms, 512.875 * 1000 / px);
    assert.equal(exposureMS({ ...profile, scrollSpeed: 16 }, 1), ms / 2);
    assert.equal(exposureMS({ ...profile, scrollSpeedOffset: 8 }, 1), ms / 2);
    assert.equal(exposureMS(profile, .5), ms * 2);
    assert.equal(exposureMS(profile, 2), ms / 2);
    assert.equal(exposureMS(profile, 1, { skin: 'classic' }), 499 * 1000 / px);
    assert.equal(exposureMS(profile, 1, { hudLayout: { hitPosition: 500 } }), 396 * 1000 / px);
    assert.equal(exposureMS({ ...profile, scrollSpeed: 100 }, 1), 512.875 * 1000 / (40 * PIXELS_PER_SCROLL));
    for (const rate of [0, undefined, NaN]) assert.equal(exposureMS(profile, rate), null);
    assert.equal(exposureMS({ ...profile, scrollSpeed: NaN }, 1), null);
});

test('exposure mode displays the applied milliseconds including per-key-count offsets', () => {
    const p = { ...profile, scrollMode: 'exposure', exposureOffset: -100 };
    assert.equal(exposureMS(p, 2), 650);
    assert.equal(exposureMS({ ...p, exposureMS: NaN }, 1), null);
    assert.equal(exposureMS({ ...p, exposureOffset: 9000 }, 1), 5000);
});
