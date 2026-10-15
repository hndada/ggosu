import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const source = readFileSync(new URL('./info.js', import.meta.url), 'utf8')
    .replace("'../../preferences/language.js'", JSON.stringify(new URL('../../preferences/language.js', import.meta.url).href))
    .replace("'/songlist/analysis.js'", JSON.stringify(new URL('../../songlist/analysis.js', import.meta.url).href))
    .replace("'/songlist/level.js'", JSON.stringify(new URL('../../songlist/level.js', import.meta.url).href));
const { summary, overview, densityHeights, render } = await import('data:text/javascript;base64,' + Buffer.from(source).toString('base64'));

test('song duration shows measured original audio, loading and failure without substituting note length', t => {
    const previous = globalThis.document, nodes = new Map();
    t.after(() => { globalThis.document = previous; });
    globalThis.document = { getElementById(id) {
        if (!nodes.has(id)) nodes.set(id, { dataset: {}, setAttribute() {}, removeAttribute() {}, replaceChildren() {} });
        return nodes.get(id);
    } };
    const entry = { title: 'Keysounds', difficulty: 'Hard', length: 100, audioDuration: 185e9 };
    render(entry, [], null, null, '', 2);
    const duration = nodes.get('song-duration');
    assert.equal(duration.hidden, false);
    assert.equal(duration.textContent, '전체 길이 3:05');
    assert.equal(duration.title, '원곡 1배속 기준');
    render({ ...entry, durationPending: true }, [], null, null, '');
    assert.equal(duration.textContent, '전체 길이 계산 중…');
    render({ ...entry, audioDuration: 0, durationError: 'broken bank' }, [], null, null, '');
    assert.equal(duration.textContent, '전체 길이 미확인');
    assert.equal(duration.title, 'broken bank');
    render(null, [], null, null, '');
    assert.equal(duration.hidden, true);
    assert.equal(duration.textContent, '');
});

test('song info counts playable heads and LN starts, using the actual density interval for peak NPS', () => {
    const report = { start: 0, step: 2, total: 18, mines: 3, bins: [
        { tap: 6, hold: 2, lift: 0, mine: 3 }, { tap: 7, hold: 2, lift: 1, mine: 0 },
    ] };
    const s = summary({ length: 6 }, report);
    assert.equal(s.total, 18); assert.equal(s.taps, 14); assert.equal(s.holds, 4);
    assert.equal(s.average, 3); assert.equal(s.peak, 5);
    assert.equal(s.ratio, 4 / 18 * 100);
});

test('an empty chart has zero density and unavailable reports never leave fabricated values', () => {
    assert.equal(summary({ length: 0 }, null), null);
    assert.equal(summary({ length: 0 }, { step: 0, bins: [], total: 0 }), null);
    assert.deepEqual(summary({ length: 0 }, { step: 1, bins: [], total: 0 }), { total: 0, taps: 0, holds: 0, ratio: 0, average: 0, peak: 0 });
    assert.deepEqual(overview(null), []);
});

test('a long chart overview keeps the original timeline and averages each slice', () => {
    const bins = Array.from({ length: 600 }, () => ({ tap: 1, hold: 0, lift: 0 }));
    bins[301].tap = 61;
    const bars = overview({ start: -2, step: 2, bins });
    assert.equal(bars.length, 40);
    assert.equal(bars[0].from, -2); assert.equal(bars.at(-1).to, 1198);
    assert.equal(bars[0].nps, 0.5); assert.equal(bars[20].nps, 2.5);
    assert.ok(bars.every((b, i) => !i || bars[i - 1].to === b.from));
});

test('density heights are linear against the densest slice of the chart', () => {
    assert.deepEqual(densityHeights([]), []);
    assert.deepEqual(densityHeights([{ nps: 0 }, { nps: 0 }]), [0, 0]);
    assert.deepEqual(densityHeights([{ nps: 3 }, { nps: 6 }, { nps: 0 }]), [50, 100, 0]);
});
