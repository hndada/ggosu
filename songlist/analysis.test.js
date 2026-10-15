import test from 'node:test';
import assert from 'node:assert/strict';
import { comparison, sameMods, modsText, eligible, normalizeScore, summarize, achievement, lamps, playerStats } from './analysis.js';

test('browser player pp weights the best verified current play per chart and counts local-day presses', () => {
    const date = new Date(2026, 9, 14, 12), yesterday = new Date(2026, 9, 13, 12);
    const record = (hash, accuracy) => ({ version: 2, verification: 'verified', date: date.toISOString(), mode: 'live', completed: true, practice: false, rate: 1, accuracy, score: { Possible: 100 }, replay: { chart_hash: hash, rules: 'ggosu-keys-v2', duration_ns: 10e9, events: [{ down: true }, { down: false }] } });
    const stats = playerStats([record('a', 1), record('a', .9), record('b', .9), { ...record('c', 1), mode: 'auto' }, { ...record('d', 1), verification: 'unavailable' }, { ...record('e', 1), date: yesterday.toISOString(), practice: true }], date);
    assert.ok(Math.abs(stats.pp - 147.5) < 1e-9);
    assert.equal(stats.keysToday, 4);
});

test('combined mods show both names and compare only equal combined mappings', () => {
    const both = { kind: 'mirror+random', seed: 0, mapping: [2, 0, 1] };
    assert.equal(modsText(both), 'Mirror + Random · seed 0');
    assert.match(modsText(both, true), /1→3, 2→1, 3→2/);
    assert.equal(sameMods(both, { ...both, seed: 42 }), true);
    assert.equal(sameMods(both, { ...both, mapping: [0, 2, 1] }), false);
    assert.equal(sameMods(both, { ...both, kind: 'random' }), false);
});

const base = { id: 'previous', date: '2026-10-08T01:00:00Z', scoreVersion: 2, mode: 'live', completed: true, practice: false, rate: 1, accuracy: .25, score: { Points: 175000, MaxCombo: 1, Kool: 0, Cool: 0, Good: 1, Miss: 1 }, mods: {} };
const current = { ...base, id: 'current', date: '2026-10-08T02:00:00Z', accuracy: 1, score: { Points: 1050000, MaxCombo: 2, Kool: 1, Cool: 1, Good: 0, Miss: 0 } };

test('visibility records compare only matching effects and show their names', () => {
    const hidden = { visibility: 'hidden' };
    assert.equal(sameMods({}, hidden), false);
    assert.equal(sameMods(hidden, { visibility: 'fadein' }), false);
    assert.equal(sameMods(hidden, { ...hidden }), true);
    assert.equal(modsText(hidden), 'Hidden');
    assert.equal(modsText({ kind: 'mirror', visibility: 'flashlight' }), 'Mirror / Flashlight');
    const r = { ...current, mods: hidden };
    assert.equal(comparison([base, r], r).status, 'first');
    assert.equal(comparison([{ ...base, mods: hidden }, r], r).status, 'compared');
});

test('combo lamp is independent of points, gives perfect priority, and persists across retries', () => {
    assert.equal(lamps({ bestPoints: 1050000 }).combo, 'none');
    assert.equal(lamps({ bestPoints: 700000, fullCombo: true }).combo, 'fullcombo');
    assert.equal(lamps({ fullCombo: true, perfect: true }).combo, 'perfect');
    assert.equal(lamps({ completed: true }).combo, 'clear');
    assert.equal(lamps({ completed: false }).combo, 'none');
    assert.equal(lamps({ completed: true, gauge: 2 }).combo, 'gauge2');
    assert.equal(lamps({ completed: true, gauge: 3, fullCombo: true }).combo, 'fullcombo');
    const r = { ...current, hash: 'chart', rules: 'ggosu-keys-v1', score: { ...current.score, Total: 2, Possible: 2 } };
    const missed = { ...r, date: '2026-10-09T00:00:00Z', score: { ...r.score, Points: 1080000, Miss: 1, MaxCombo: 1 } };
    const retry = { ...r, completed: false, date: '2026-10-10T00:00:00Z', score: { ...r.score, Points: 0 } };
    const state = lamps(summarize([r, missed, retry])[0]);
    assert.equal(state.combo, 'perfect');
});

test('catalogue achievements survive newer practice and incomplete attempts', () => {
    const r = { ...current, hash: 'chart', rules: 'ggosu-keys-v1', score: { ...current.score, Total: 2, Possible: 2 } };
    const latest = { ...r, date: '2026-10-09T00:00:00Z', completed: false, practice: true, score: { ...r.score, Points: 0 } };
    const s = summarize([latest, r])[0];
    assert.equal(s.lastPlayed, latest.date);
    assert.equal(s.bestPoints, r.score.Points);
    assert.equal(achievement(s), '퍼펙트');
    assert.equal(s.perfect, true); // COOL counts toward perfect.
    assert.equal(achievement(summarize([{ ...r, score: { ...r.score, Kool: 2, Cool: 0 } }])[0]), '퍼펙트');
    assert.equal(achievement(summarize([{ ...r, score: { ...r.score, Kool: 1, Cool: 1, Good: 0 } }])[0]), '퍼펙트');
    assert.equal(achievement(summarize([{ ...r, score: { ...r.score, Kool: 1, Cool: 0, Good: 1 } }])[0]), '풀콤보');
    assert.equal(achievement(summarize([{ ...r, score: { ...r.score, Cool: 0, Miss: 1, MaxCombo: 1 } }])[0]), '완주');
    assert.equal(achievement(undefined), '미플레이');
});

test('visits include practice, old rules of scoring and mods but only live current-rule input', () => {
    const r = { ...current, hash: 'chart', rules: 'ggosu-keys-v1', score: { ...current.score, Total: 2, Possible: 2 } };
    for (const changes of [{ completed: false }, { practice: true }, { rate: 2 }, { scoreVersion: 1 }, { score: { ...r.score, Possible: 0 } }]) {
        const rows = summarize([{ ...r, ...changes }]);
        assert.equal(rows.length, 1);
        assert.equal(achievement(rows[0]), '플레이함');
        assert.equal(rows[0].bestPoints, null);
    }
    for (const changes of [{ mode: 'auto' }, { mode: 'replay' }, { rules: 'other' }, { date: 'invalid' }]) assert.deepEqual(summarize([{ ...r, ...changes }]), []);
    assert.deepEqual(summarize([]), []);
});
test('catalogue points include Mirror and Random while achievements use the default layout', () => {
    const r = { ...current, hash: 'chart', rules: 'ggosu-keys-v2', score: { ...current.score, Total: 2, Possible: 2, Points: 865176 } };
    for (const kind of ['mirror', 'random']) {
        const modified = { ...r, mods: { kind } };
        const s = summarize([modified])[0];
        assert.equal(s.bestPoints, 865176);
        assert.equal(s.completed, false);
        assert.equal(s.fullCombo, false);
        assert.equal(s.perfect, false);
        const normal = { ...r, score: { ...r.score, Points: 800000 } };
        for (const rows of [[normal, modified], [modified, normal]]) {
            const mixed = summarize(rows)[0];
            assert.equal(mixed.bestPoints, 865176);
            assert.equal(mixed.perfect, true);
        }
    }
});
test('old browser counts display and compare as KOOL/COOL', () => {
    const old = { Points: 1050000, MaxCombo: 2, Perfect: 1, Great: 1, Good: 0, Miss: 0 };
    assert.deepEqual(normalizeScore(old), current.score);
    const c = comparison([{ ...base, score: { ...old, Perfect: 0 } }], current);
    assert.equal(c.delta.Kool, 1);
    assert.equal(c.delta.Cool, 0);
});
test('compare against the prior best, excluding the current and future records', () => {
    const c = comparison([base, current, { ...current, id: 'future', date: '2026-10-08T03:00:00Z', score: { ...current.score, Points: 1100000 } }], current);
    assert.equal(c.previous.id, base.id);
    assert.equal(c.improved, true);
    assert.equal(c.delta.points, 875000);
    assert.ok(Math.abs(c.delta.accuracy - 75) < 1e-9);
    assert.equal(c.delta.combo, 1);
    assert.equal(c.delta.Good, -1);
    assert.equal(comparison([current], current).status, 'first');
});
test('exclude Auto, replay, incomplete and rate practice, even at a better score', () => {
    for (const changes of [{ mode: 'auto' }, { mode: 'replay' }, { completed: false }, { rate: 2 }, { practice: true }, { scoreVersion: 1 }]) {
        const r = { ...current, ...changes };
        assert.equal(eligible(r), false);
        assert.equal(comparison([base], r).status, 'excluded');
        assert.equal(comparison([{ ...r, id: 'other' }], current).status, 'first');
    }
});
test('mod comparison uses actual permutation and ignores equal-layout seeds', () => {
    const a = { kind: 'random', seed: 123, mapping: [2, 1, 0] }, b = { ...a, seed: 456 };
    assert.equal(sameMods(a, b), true);
    assert.equal(sameMods(a, { ...b, mapping: [1, 0, 2] }), false);
    assert.equal(sameMods({}, { kind: 'mirror', mapping: [2, 1, 0] }), false);
    const r = { ...current, mods: a };
    assert.equal(comparison([{ ...base, mods: b }], r).status, 'compared');
    assert.equal(comparison([base], r).status, 'first');
});

test('clear lamp keeps the highest health stage standing at the end, current gauge table only', () => {
    const r = { ...current, hash: 'chart', rules: 'ggosu-keys-v1', score: { ...current.score, Total: 2, Possible: 2, Kool: 1, Cool: 0, Good: 0, Miss: 1, MaxCombo: 1, Points: 550000 } };
    const at = (level, value, gauge = 1) => ({ ...r, health: { level, value, failed: false, start: 3, gauge } });
    assert.equal(summarize([at(2, 500), at(1, 9000)])[0].gauge, 2);
    assert.equal(summarize([at(1, 0)])[0].gauge, 0);
    assert.equal(summarize([at(3, 100, 0)])[0].gauge, 0);
    assert.equal(lamps(summarize([at(3, 100)])[0]).combo, 'gauge3');
});
