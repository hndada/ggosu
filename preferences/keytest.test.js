import test from 'node:test';
import assert from 'node:assert/strict';
import { keyName, classify, lanesOf, duplicates, conflict, problem, pattern, judge, summary } from './keytest.js';

test('key names match the backend lane key names', () => {
    assert.deepEqual(['KeyD', 'Digit7', 'Space', 'Semicolon', 'F7'].map(keyName), ['D', '7', 'SPACE', 'SEMICOLON', 'F7']);
    assert.deepEqual(['D', '7', 'SPACE', 'SEMICOLON', 'P', 'ESCAPE', 'SHIFTLEFT'].map(classify), ['ok', 'ok', 'ok', 'ok', 'reserved', 'reserved', 'unsupported']);
});

test('duplicate keys light every lane they are bound to and block saving', () => {
    const keys = ['D', 'F', 'D', 'K'];
    assert.deepEqual(lanesOf(keys, 'D'), [0, 2]);
    assert.deepEqual(lanesOf(keys, 'Q'), []);
    assert.deepEqual([...duplicates(keys)].sort(), [0, 2]);
    assert.match(problem(keys), /D가 레인 1, 3에 중복/);
    assert.match(problem(['D', 'P']), /예약 키/);
    assert.match(problem(['D', 'TAB']), /지원하지 않는 키/);
    assert.equal(problem(['D', 'F', 'J', 'K']), '');
});

test('test pattern walks every lane and adds chords up to 8K', () => {
    assert.deepEqual(pattern(3).map(n => n.lane), [0, 1, 2, 1, 0, 0, 1, 2, 0, 1, 2]);
    assert.deepEqual(pattern(1).map(n => n.lane), [0]);
    assert.equal(pattern(10).length, 19);
    const times = pattern(4).map(n => n.at);
    assert.deepEqual(times, [...times].sort((a, b) => a - b));
});

test('judging takes the closest open note in the lane and reports the offset', () => {
    const notes = [{ lane: 0, at: 0 }, { lane: 0, at: 250 }, { lane: 1, at: 240 }];
    assert.equal(judge(notes, 0, 200), 1);
    assert.equal(judge(notes, 0, 401), -1);
    assert.equal(judge(notes, 2, 0), -1);
    notes[1].result = -50; notes[2].result = 10; notes[0].result = 'miss';
    assert.equal(judge(notes, 0, 250), -1);
    assert.deepEqual(summary(notes), { hits: 2, total: 3, mean: -20, spread: 30 });
});

test('sequential capture only rejects keys already assigned before the cursor', () => {
    const keys = ['F', 'D', 'J', 'K'];
    assert.equal(conflict(keys, 0, 'D'), -1);
    assert.equal(conflict(['F', 'D', 'J', 'K'], 1, 'D'), -1);
    assert.equal(conflict(['F', 'D', 'J', 'K'], 2, 'F'), 0);
    assert.equal(conflict(['F', 'D', 'J', 'K'], 3, 'K'), -1);
});
