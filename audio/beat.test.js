import test from 'node:test';
import assert from 'node:assert/strict';
import { grid, beatAt, flashInterval } from './beat.js';

test('scroll-only timing points keep the beat grid', () => {
    const points = grid([
        { at_ns: 100e6, bpm: 120, meter: 4 },
        { at_ns: 900e6, bpm: 120, meter: 4, scroll: 2 },
        { at_ns: 2100e6, bpm: 150, meter: 3 },
        { at_ns: 3000e6, bpm: 0, meter: 4 },
    ]);
    assert.deepEqual(points, [{ at: 100, length: 500, meter: 4 }, { at: 2100, length: 400, meter: 3 }]);
});

test('beat index and time since the beat follow BeatSyncedContainer', () => {
    const points = [{ at: 100, length: 500, meter: 4 }, { at: 2100, length: 400, meter: 3 }];
    assert.deepEqual(beatAt(points, 1350), { point: points[0], index: 2, since: 250 });
    assert.deepEqual(beatAt(points, 2150), { point: points[1], index: 0, since: 50 });
    // Before the first point, beats count back from it.
    assert.deepEqual(beatAt(points, 0), { point: points[0], index: -1, since: 400 });
    // No song: 60 BPM from zero.
    assert.equal(beatAt([], 2500).index, 2);
    assert.equal(beatAt([], 2500).point.length, 1000);
});

test('deeper layers flash less often and stay on the bar', () => {
    assert.equal(flashInterval(4, 0), 1);
    assert.equal(flashInterval(4, 1), 2);
    assert.equal(flashInterval(4, 2), 4);
    assert.equal(flashInterval(3, 1), 3);
    assert.equal(flashInterval(3, 2), 6);
    assert.equal(flashInterval(6, 2), 6);
});
