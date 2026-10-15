import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const source = readFileSync(new URL('./records.js', import.meta.url), 'utf8')
    .replace("'../../preferences/language.js'", JSON.stringify(new URL('../../preferences/language.js', import.meta.url).href))
    .replace("'/songlist/analysis.js'", JSON.stringify(new URL('../../songlist/analysis.js', import.meta.url).href))
    .replace("'/songselect/analysis.js'", JSON.stringify(new URL('./analysis.js', import.meta.url).href))
    .replace("'/settings/offsets.js'", JSON.stringify(new URL('../settings/offsets.js', import.meta.url).href));
const { scores, history, brief, demoScores, pauseSummary, showResult, showSubmission } = await import('data:text/javascript;base64,' + Buffer.from(source).toString('base64'));
const record = (id, points, date, mode = 'live') => ({
    id, date, mode, completed: true, rate: 1, scoreVersion: 2, accuracy: .9,
    score: { Points: points, Total: 10, MaxCombo: 8 },
});

function setup(t, app) {
    const previous = { document: globalThis.document, window: globalThis.window };
    t.after(() => { globalThis.document = previous.document; globalThis.window = previous.window; });
    const elements = new Map();
    function node(tag) {
        return { tag, children: [], listeners: {}, attributes: {}, hidden: false, textContent: '', dataset: {}, style: {},
            focus() {},
            append(...children) { this.children.push(...children); },
            replaceChildren(...children) { this.children = children; },
            addEventListener(name, fn) { this.listeners[name] = fn; },
            setAttribute(name, value) { this.attributes[name] = value; },
            classes: new Set(),
            get classList() { const c = this.classes; return { add: (...n) => n.forEach(x => c.add(x)), remove: (...n) => n.forEach(x => c.delete(x)), contains: n => c.has(n) }; },
        };
    }
    function element(selector) {
        if (!elements.has(selector)) elements.set(selector, node('div'));
        return elements.get(selector);
    }
    globalThis.document = { querySelector: element, createElement: node };
    globalThis.window = { go: { main: { App: app } } };
    element('#history-order').value = 'points';
    element('#lane-mod').value = '';
    return element;
}

test('local scores filter live plays and sort by score or latest without changing saved order', () => {
    const rows = [record('new', 700000, '2026-10-10'), record('best', 900000, '2026-10-09'), record('auto', 1100000, '2026-10-08', 'auto')];
    assert.deepEqual(scores(rows, 'live', 'points').map(r => r.id), ['best', 'new']);
    assert.deepEqual(scores(rows, 'live', 'date').map(r => r.id), ['new', 'best']);
    assert.deepEqual(scores(rows, 'all', 'points').map(r => r.id), ['auto', 'best', 'new']);
    assert.deepEqual(rows.map(r => r.id), ['new', 'best', 'auto']);
});

test('submission status exposes retry for failed and unsent live records and hides it after acceptance', async t => {
    const $ = setup(t, {}), r = record('live', 1100000, '2026-10-13');
    showSubmission(r);
    assert.equal($('#result-submission').hidden, false);
    assert.equal($('#submission-retry').hidden, false);
    showSubmission({ ...r, submission: { state: 'pending' } });
    assert.equal($('#submission-retry').hidden, true);
    await showSubmission({ ...r, submission: { state: 'failed', error: 'offline' } });
    assert.equal($('#result-submission-status').textContent, '서버 제출 실패');
    window.runtime = { Environment: async () => ({ buildType: 'debug' }) };
    await showSubmission({ ...r, submission: { state: 'failed', error: 'offline' } });
    assert.match($('#result-submission-status').textContent, /offline/);
    delete window.runtime;
    assert.equal($('#submission-retry').hidden, false);
    showSubmission({ ...r, submission: { state: 'sent' } });
    assert.equal($('#submission-retry').hidden, true);
    showSubmission({ ...r, mode: 'auto' });
    assert.equal($('#result-submission').hidden, true);
    assert.equal(r.score.Points, 1100000);
});

test('result header keeps modifiers compact while preserving seed, pauses and calibration details', t => {
    const element = setup(t, {});
    const r = { ...record('result', 1000000, '2026-10-12'), title: 'Recollection', difficulty: 'Hard', lanes: 7,
        pitch: false, mods: { kind: 'random', seed: 2660866540 }, offsets: { globalMS: 7, chartMS: 0 },
        calibration: { fromMS: 7, toMS: 13, meanMS: 6 }, pauses: [{ action: 'pause', elapsed_ns: 1e9 }, { action: 'resume', elapsed_ns: 4e9 }],
        score: { Points: 1000000, Total: 100, Possible: 100, MaxCombo: 90, Kool: 80, Cool: 15, Good: 3, Miss: 2 } };
    element('#result-run').open = true;
    showResult(r);
    assert.equal(element('#result-heading').textContent, 'RESULT');
    assert.equal(element('#result-state').textContent, 'CLEAR');
    assert.equal(element('#result-keys').textContent, '7KEY');
    assert.equal(element('#result-difficulty').textContent, 'HARD');
    assert.deepEqual(element('#result-flags').children.map(n => n.textContent), ['PITCH OFF', 'RANDOM']);
    assert.match(element('#result-mode').textContent, /seed 2660866540/);
    assert.match(element('#result-pauses').textContent, /1회 · 3.0초/);
    assert.match(element('#result-offsets').textContent, /7 ms/);
    assert.equal(element('#result-run').open, false);
    assert.equal(element('#result-calibration span').textContent, '타이밍 +6 ms 조정');
    assert.equal(element('#result-adjustment').textContent, '전체 7 → 13 ms · 평균 +6 ms');
    showResult(r, { past: true });
    assert.equal(element('#result-heading').textContent, 'RECORD');
    assert.equal(element('#result-calibration').hidden, true);
    showResult({ ...r, completed: false, health: { level: 3, failed: true }, pitch: true, mods: {}, calibration: undefined });
    assert.equal(element('#result-state').textContent, 'FAILED');
    assert.deepEqual(element('#result-flags').children.map(n => n.textContent), ['HARD GAUGE']);
    assert.equal(element('#result-adjustment').hidden, true);
});

test('result shows FAST and SLOW counts only when the record has them', t => {
    const element = setup(t, {});
    const r = { ...record('timing', 900000, '2026-10-13'), score: { Points: 900000, Total: 10, MaxCombo: 8, Kool: 6, Cool: 2, Good: 1, Miss: 1 } };
    showResult({ ...r, hitError: { meanMS: -3, count: 9, fast: 1200, slow: 4 } });
    assert.equal(element('#result-timing').hidden, false);
    assert.equal(element('#result-fast').textContent, (1200).toLocaleString());
    assert.equal(element('#result-slow').textContent, '4');
    showResult({ ...r, hitError: { meanMS: 0, count: 9 } });
    assert.equal(element('#result-timing').hidden, true);
    showResult(r);
    assert.equal(element('#result-timing').hidden, true);
});

test('result score tweens, cancels stale frames, and leaves the saved score intact', t => {
    const $ = setup(t, {});
    const previous = { requestAnimationFrame: globalThis.requestAnimationFrame, cancelAnimationFrame: globalThis.cancelAnimationFrame, performance: globalThis.performance };
    const frames = new Map();
    let id = 0;
    globalThis.performance = { now: () => 0 };
    globalThis.requestAnimationFrame = fn => { frames.set(++id, fn); return id; };
    globalThis.cancelAnimationFrame = id => frames.delete(id);
    t.after(() => Object.assign(globalThis, previous));
    const frame = now => {
        const pending = [...frames.values()]; frames.clear();
        for (const fn of pending) fn(now);
    };
    const r = { ...record('animated', 1100000, '2026-10-13'), score: { Points: 1100000, Total: 10, MaxCombo: 10, Kool: 10, Cool: 0, Good: 0, Miss: 0 } };
    showResult(r);
    assert.equal($('#result-points').textContent, '0');
    frame(130);
    const shown = Number($('#result-points').textContent.replaceAll(',', ''));
    assert.ok(shown > 0 && shown < r.score.Points);
    const stale = [...frames.values()][0];
    showResult({ ...r, score: { ...r.score, Points: 500000 } });
    stale(650);
    assert.equal($('#result-points').textContent, '0');
    frame(650);
    assert.equal($('#result-points').textContent, (500000).toLocaleString());
    assert.equal(frames.size, 0);
    assert.equal(r.score.Points, 1100000);
    showResult(r, { past: true });
    assert.equal($('#result-points').textContent, r.score.Points.toLocaleString());
    assert.equal(frames.size, 0);
    showResult(r);
    $('#result').hidden = true;
    frame(10);
    assert.equal(frames.size, 0);
    assert.equal($('#result-points').textContent, r.score.Points.toLocaleString());
    window.matchMedia = () => ({ matches: true });
    showResult(r);
    assert.equal(frames.size, 0);
    assert.equal($('#result-points').textContent, r.score.Points.toLocaleString());
});

test('visible scores show saved modifiers and open the saved result', async t => {
    const rows = [record('live', 900000, '2026-10-09'), record('auto', 1100000, '2026-10-10', 'auto'),
        { ...record('mods', 800000, '2026-10-08'), rate: 1.25, pitch: false, mods: { kind: 'mirror+random', seed: 123, visibility: 'hidden', o2jam: 'hx', pills: true } },
        { ...record('visibility', 700000, '2026-10-07'), mods: { visibility: 'flashlight' } }];
    let opened;
    const element = setup(t, { History: async () => rows });
    const selection = { chartID: 'chart', folder: 'song', index: 2 };
    await history({ path: 'chart' }, selection, () => {}, r => { opened = r; });
    const list = element('#history');
    assert.equal(list.hidden, false); assert.equal(list.children.length, 3);
    assert.equal(list.children[0].children[0].children[0].textContent, '900,000');
    assert.equal(list.children[0].children[0].children.length, 1);
    const modRow = list.children[1].children[0];
    assert.equal(modRow.children[0].textContent, '800,000');
    assert.equal(modRow.children[1].textContent, '1.25× · PITCH OFF · MIRROR+RANDOM · HD · O2JAM HX + PILLS');
    assert.match(modRow.attributes['aria-label'], /MIRROR\+RANDOM · HD/);
    assert.equal(list.children[2].children[0].children[1].textContent, 'FL');
    assert.match(brief(rows[2])[2], /seed 123/);
    assert.equal(brief(rows[3])[2], 'Flashlight');
    assert.match(element('#best').textContent, /900,000/);
    assert.equal(element('#history-message').textContent, '');
    list.children[0].children[0].listeners.click();
    assert.equal(opened, rows[0]);
    assert.equal(list.children[0].children.length, 1);
    assert.equal(brief(rows[0])[0], '90.00% · 8콤보');
    assert.match(brief(rows[0])[1], /^\d{4}\.\d\d\.\d\d \d\d:\d\d$/);
    assert.equal(brief(rows[0]).length, 2);
    assert.equal(brief({ ...rows[0], rate: 1.5, completed: false })[2], '미완주 · 1.5x');
});

test('an older chart history cannot replace the latest chart scores', async t => {
    let resolveOld;
    const latest = record('latest', 700000, '2026-10-10');
    const element = setup(t, { History: s => s.chartID === 'old' ? new Promise(resolve => { resolveOld = resolve; }) : Promise.resolve([latest]) });
    const old = history({ path: 'old' }, { chartID: 'old' }, () => {}, () => {});
    await history({ path: 'new' }, { chartID: 'new' }, () => {}, () => {});
    resolveOld([record('old', 1000000, '2026-10-09')]); await old;
    assert.equal(element('#history').children.length, 1);
    assert.equal(element('#history').children[0].children[0].children[0].textContent, '700,000');
});

test('empty, unselected and failed histories explain the state and clear previous scores', async t => {
    const app = { History: async () => [] }, element = setup(t, app);
    await history({}, {}, () => {}, () => {});
    assert.equal(element('#history').hidden, true);
    assert.match(element('#history-message').textContent, /스코어 없음/);
    app.History = async () => [record('auto', 1100000, '2026-10-10', 'auto')];
    await history({}, {}, () => {}, () => {});
    assert.match(element('#history-message').textContent, /스코어 없음/);
    await history(null, null, () => {}, () => {});
    assert.equal(element('#history').children.length, 0);
    assert.match(element('#history-message').textContent, /채보를 선택/);
    app.History = async () => { throw Error('disk unavailable'); };
    await history({}, {}, () => {}, () => {});
    assert.equal(element('#history-message').textContent, '로컬 스코어를 읽지 못함');
    window.runtime = { Environment: async () => ({ buildType: 'debug' }) };
    await history({}, {}, () => {}, () => {});
    assert.match(element('#history-message').textContent, /disk unavailable/);
    assert.equal(element('#history').hidden, true);
});

test('demo rankings are fixed per chart, ranked by score and never above the maximum', () => {
    const global = demoScores('global', 'songs/a.osu/0');
    assert.deepEqual(global, demoScores('global', 'songs/a.osu/0'));
    assert.equal(global.length, 8);
    assert.equal(demoScores('rival', 'songs/a.osu/0').length, 3);
    assert.deepEqual(demoScores('local', 'x'), []);
    for (let i = 0; i < global.length; i++) {
        assert.equal(global[i].rank, i + 1);
        assert.ok(global[i].points <= 1100000 && global[i].points > 0);
        if (i) assert.ok(global[i].points < global[i - 1].points);
    }
});


test('zero-point health failure stays listed and pause events include canceled countdown', () => {
    const row = { ...record('failed', 0, '2026-10-12'), completed: false, health: { level: 3, failed: true, value: 0 } };
    assert.equal(scores([row], 'all', 'date').length, 1);
    assert.equal(pauseSummary([{ action: 'pause', elapsed_ns: 1000000000 }, { action: 'resume-request', elapsed_ns: 2000000000 }, { action: 'resume-cancel', elapsed_ns: 3000000000 }, { action: 'end', elapsed_ns: 6000000000 }]), '일시정지 1회 · 5.0초 · 재개 취소 1회');
});
