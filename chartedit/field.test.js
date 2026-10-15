import test from 'node:test';
import assert from 'node:assert/strict';
import { lanes } from './field.js';

test('lane geometry matches the play layout and scales width only', () => {
    const seven = lanes({ lanes: 7 });
    assert.equal(seven.lanes[0].width, 64); assert.equal(seven.lanes[3].width, 64 * 1.1); assert.equal(seven.width, 64 * 7.1);
    const half = lanes({ lanes: 7 }, .5);
    assert.equal(half.width, seven.width / 2);
    const bms = lanes({ lanes: 8, layout: [{ role: 'scratch', group: 0 }, ...Array.from({ length: 7 }, () => ({ role: 'key', group: 1 }))] });
    assert.equal(bms.lanes[0].width, 64 * 1.25); assert.equal(bms.lanes[1].x, 64 * 1.25 + 12);
    const wide = lanes({ lanes: 18 });
    assert.ok(Math.abs(wide.width - 820) < 1e-9);
});
