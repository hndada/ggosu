import test from 'node:test';
import assert from 'node:assert/strict';
import { Draft, ns, ms, snap, position, ticks, issues, mirrorLanes } from './edit.js';

const chart = () => ({ version: 1, lanes: 4, metadata: { title: 'Test', difficulty: 'Easy' }, source: { format: 'osu' }, assets: {}, notes: [{ lane: 1, at_ns: ns(1000), end_ns: ns(2000), kind: 'hold', sounds: [{ asset: 'hit', volume: 1 }] }], timing: [{ at_ns: 0, bpm: 120, scroll: 1, meter: 4 }] });
test('copy, paste and undo retain LN duration, sounds and original audio timestamps', () => {
    const original = chart(), d = new Draft(original);
    d.selected.add(d.c.notes[0].id); d.copy(); d.paste(3000);
    assert.equal(ms(d.c.notes[1].end_ns), 4000); assert.deepEqual(d.c.notes[1].sounds, original.notes[0].sounds);
    d.move(125, 1); assert.equal(ms(d.c.notes[1].at_ns), 3125);
    d.restore(d.undo, d.redo); assert.equal(ms(d.c.notes[1].at_ns), 3000);
    d.restore(d.undo, d.redo); assert.equal(d.c.notes.length, 1); assert.equal(d.dirty, false);
    d.restore(d.redo, d.undo); assert.equal(d.c.notes.length, 2);
    assert.equal(original.notes.length, 1); assert.ok(!('id' in d.document().notes[0]));
});
test('lane toggle removes the covering note or adds a tap; cut keeps the clipboard', () => {
    const d = new Draft(chart());
    d.toggle(1, 1500); assert.equal(d.c.notes.length, 0);
    d.toggle(2, 500); assert.equal(d.c.notes[0].kind, 'tap'); assert.equal(ms(d.c.notes[0].at_ns), 500);
    d.cut(); assert.equal(d.c.notes.length, 0); d.paste(800); assert.equal(ms(d.c.notes[0].at_ns), 800);
    d.restore(d.undo, d.redo); d.restore(d.undo, d.redo); d.restore(d.undo, d.redo); d.restore(d.undo, d.redo);
    assert.equal(d.c.notes[0].kind, 'hold');
});
test('beat snap changes at BPM boundaries and excludes frozen stop intervals', () => {
    const t = [{ at_ns: 0, bpm: 120, scroll: 1, meter: 4 }, { at_ns: ns(2000), bpm: 240, scroll: 2, meter: 4, stop_ns: ns(500) }];
    assert.equal(snap(t, 1980, 4), 2000); assert.equal(snap(t, 2100, 4), 2000);
    assert.equal(snap(t, 2580, 4), 2562.5); assert.equal(position(t, 2500).beat, 4);
    assert.equal(position(t, 2750).beat, 5);
    assert.equal(ticks(t, 2001, 2499, 4).length, 0);
    assert.equal(snap(t, 2580, 0), 2580);
});
test('overlapping LN and duplicate taps are flagged while fake notes are exempt', () => {
    const c = chart(); c.notes.push({ lane: 1, at_ns: ns(1500), end_ns: ns(1500), kind: 'tap' });
    assert.equal(issues(c).length, 1);
    c.notes[1].kind = 'fake'; assert.equal(issues(c).length, 0);
    c.notes[1].kind = 'tap'; c.notes[1].at_ns = c.notes[1].end_ns = ns(2000); assert.equal(issues(c).length, 0);
    c.notes.push({ ...c.notes[1] }); assert.equal(issues(c).length, 1);
});
test('Scroll events between beats do not reset the snap grid', () => {
    const t = [{ at_ns: 0, bpm: 120, scroll: 1, meter: 4 }, { at_ns: ns(1300), bpm: 120, scroll: 2, meter: 4 }];
    assert.equal(snap(t, 1390, 4), 1375);
    assert.equal(ticks(t, 1301, 1500, 4)[0].at, 1375);
});
test('lead-in ticks and first stop retain the same beat origin', () => {
    const t = [{ at_ns: 0, bpm: 120, scroll: 1, meter: 4, stop_ns: ns(300) }];
    assert.equal(snap(t, -100, 4), -125);
    assert.equal(ticks(t, 300, 301, 4)[0].beat, 0);
    assert.equal(ticks(t, -130, -120, 4)[0].at, -125);
});
test('mirror preserves scratch lanes and player groups, and moves reject lane escapes', () => {
    const c = chart(); c.lanes = 6; c.layout = [{ role: 'scratch' }, { role: 'key' }, { role: 'key' }, { role: 'scratch', group: 1 }, { role: 'key', group: 1 }, { role: 'key', group: 1 }];
    assert.deepEqual(mirrorLanes(c), [0, 2, 1, 3, 5, 4]);
    const d = new Draft(c); d.selected.add(d.c.notes[0].id); d.move(100, -2);
    assert.equal(d.dirty, false); d.mirror(); assert.equal(d.c.notes[0].lane, 2);
});
