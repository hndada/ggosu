import { tr } from '../../preferences/language.js';
// Key setup screen; pure helpers live in /preferences/keytest.js.
import { WINDOW, keyName, classify, lanesOf, duplicates, conflict, problem, pattern, judge, summary } from '/preferences/keytest.js';

const FALL = 1000, LEAD = 1500;
const ms = v => `${v > 0 ? '+' : ''}${v}ms`;

export function mount({ root, grid, pad, status, chord, chordReset, test, dialog, onBind, onComplete }) {
    let keys = [], columns = [], holds = []; // holds[lane]: last { start, end }, lights the lane while held
    const held = new Map(); // event.code -> lanes it lit
    let max = 0, run = null, frame = 0, cursor = null; // cursor: next lane to capture, null when only testing
    let sequential = false, before = [];
    const say = text => { status.textContent = text; };
    // Lanes waiting for a key are blank and do not light until assigned.
    const live = () => cursor === null || !sequential ? keys : keys.map((k, i) => i < cursor ? k : null);
    const label = key => key === 'SEMICOLON' ? ';' : key === 'SPACE' ? 'Space' : key;
    const prompt = () => tr`${keys.length}K 레인 ${cursor + 1}: 사용할 키를 누르세요. ${sequential ? tr('Backspace는 이전 레인, ') : ''}Esc는 취소입니다.`;

    function mark() {
        const dup = duplicates(live());
        columns.forEach((c, i) => {
            const pending = cursor !== null && (i === cursor || sequential && i > cursor);
            c.name.textContent = pending ? '…' : label(keys[i]);
            c.lane.setAttribute('aria-label', tr`레인 ${i + 1}: ${label(keys[i])}, 키 변경`);
            c.lane.setAttribute('aria-current', String(i === cursor));
            c.lane.setAttribute('aria-invalid', String(!pending && (dup.has(i) || classify(keys[i]) !== 'ok')));
        });
    }
    function setKeys(next) {
        stop(); release();
        keys = [...next]; holds = keys.map(() => null); max = 0; cursor = null;
        const focused = grid.contains(document.activeElement);
        grid.style.gridTemplateColumns = `repeat(${keys.length}, minmax(0, 1fr))`;
        grid.style.width = `${keys.length * 76}px`;
        columns = keys.map((key, i) => {
            const lane = document.createElement('button'); lane.type = 'button'; lane.className = 'key-lane';
            lane.dataset.tone = keys.length % 2 && i === Math.floor(keys.length / 2) ? 'special' : Math.min(i, keys.length - 1 - i) % 2 ? 'pink' : 'white';
            const arrow = document.createElement('span'); arrow.className = 'key-arrow'; arrow.textContent = '↓'; arrow.setAttribute('aria-hidden', 'true');
            const track = document.createElement('span'); track.className = 'key-track';
            const name = document.createElement('span'); name.className = 'key-name';
            const number = document.createElement('span'); number.className = 'key-number'; number.textContent = i + 1; number.setAttribute('aria-hidden', 'true');
            lane.addEventListener('click', () => capture(i, false));
            lane.append(arrow, track, name, number);
            return { lane, track, name };
        });
        grid.replaceChildren(...columns.map(c => c.lane));
        if (focused) pad.focus();
        say(tr('바꿀 레인을 선택하세요.'));
        mark(); draw();
    }
    function capture(from = 0, all = true) {
        if (run || !keys.length || test.disabled) return;
        stopCapture(); release(); before = [...keys]; sequential = all;
        cursor = from; mark(); say(prompt());
        pad.focus();
    }
    function stopCapture() {
        if (cursor === null) return;
        keys = [...before]; cursor = null; mark();
        say(tr('키 변경을 취소했습니다.'));
    }
    function bind(name) {
        if (name === 'BACKSPACE' && sequential) {
            if (cursor > 0) cursor--;
            mark(); say(prompt()); return;
        }
        const kind = classify(name);
        if (kind !== 'ok') {
            say(tr`${name}: ${kind === 'reserved' ? tr('재생 제어용 예약 키라') : tr('지원하지 않는 키라')} 할당하지 않았습니다. ${prompt()}`);
            return;
        }
        const other = sequential ? conflict(keys, cursor, name) : keys.findIndex((k, i) => i !== cursor && k === name);
        if (other >= 0) { say(tr`${name}는 이미 레인 ${other + 1}에 있습니다. 중복 없이 다른 키를 누르세요.`); return; }
        const at = cursor;
        keys[at] = name;
        if (sequential && ++cursor < keys.length) { mark(); say(prompt()); return; }
        cursor = null; mark();
        say(`${keys.length}K: ${keys.join(' · ')}`);
        if (sequential) keys.forEach((key, i) => onBind(i, key));
        else onBind(at, name);
        onComplete(keys.length);
    }

    function describe(name, t) {
        const lanes = lanesOf(keys, name);
        if (run) {
            const hits = lanes.flatMap(lane => {
                const i = judge(run.notes, lane, t - run.t0);
                if (i < 0) return [];
                run.notes[i].result = Math.round(t - run.t0 - run.notes[i].at);
                return [tr`레인 ${lane + 1} ${ms(run.notes[i].result)}`];
            });
            if (hits.length) say(hits.join(', '));
            return;
        }
        const kind = classify(name);
        if (!lanes.length) say(kind === 'reserved' ? tr`${name}: 재생 제어용 예약 키입니다.` : kind === 'unsupported' ? tr`${name}: 레인에 쓸 수 없는 키입니다.` : tr`${name}: 어느 레인에도 할당되지 않았습니다.`);
        else say(tr`${name} → 레인 ${lanes.map(l => l + 1).join(', ')}${lanes.length > 1 ? tr(' (중복 할당)') : ''}`);
    }
    function down(e) {
        if (e.isComposing || test.disabled || !dialog.open || (e.target !== pad && !columns.some(c => c.lane === e.target))) return;
        if (e.key === 'Escape' && cursor !== null) { e.preventDefault(); e.stopPropagation(); stopCapture(); release(); return; }
        if (e.key === 'Tab') { stopCapture(); return; }
        if (e.key === 'Escape' || e.ctrlKey || e.altKey || e.metaKey) return;
        if (e.target !== pad && cursor === null && (e.key === 'Enter' || e.key === ' ')) return;
        e.preventDefault();
        if (e.repeat || held.has(e.code)) return;
        const name = keyName(e.code);
        const capturing = cursor !== null && !run;
        if (capturing) bind(name);
        const lanes = lanesOf(live(), name);
        held.set(e.code, lanes);
        for (const lane of lanes) holds[lane] = { start: e.timeStamp, end: null };
        max = Math.max(max, held.size);
        if (!capturing) describe(name, e.timeStamp);
        tick();
    }
    function close(lanes, at) {
        for (const lane of lanes) {
            const s = holds[lane];
            if (s && s.end === null) s.end = at;
        }
    }
    function up(e) {
        if (!held.has(e.code)) return;
        close(held.get(e.code), e.timeStamp); held.delete(e.code); tick();
    }
    function release() {
        for (const lanes of held.values()) close(lanes, performance.now());
        held.clear(); tick();
    }

    function block(className, bottom, height) {
        const el = document.createElement('div'); el.className = className;
        el.style.bottom = `${bottom}px`; el.style.height = `${height}px`;
        return el;
    }
    function draw() {
        frame = 0;
        const now = performance.now();
        let active = held.size > 0;
        if (run) {
            for (const n of run.notes) if (n.result === undefined && now - run.t0 - n.at > WINDOW) n.result = 'miss';
            if (now - run.t0 > run.notes.at(-1).at + WINDOW + 300) finish();
            else active = true;
        }
        columns.forEach((c, lane) => {
            const h = c.track.clientHeight || 1, last = holds[lane];
            const nodes = run ? run.notes.filter(n => n.lane === lane && n.result === undefined).map(n => block('key-note', (n.at - (now - run.t0)) / FALL * h, 6)) : [];
            c.track.replaceChildren(...nodes);
            c.lane.toggleAttribute('data-held', last?.end === null);
        });
        chord.textContent = tr`동시 입력 ${held.size}개 · 최대 ${max}개`;
        if (active && dialog.open) frame = requestAnimationFrame(draw);
    }
    const tick = () => { if (!frame) frame = requestAnimationFrame(draw); };

    function start() {
        if (cursor !== null) { say(tr('키 입력을 마친 뒤 테스트하세요.')); return; }
        if (!keys.length || problem(keys)) { say(problem(keys) || tr('키 바인딩을 불러오지 못했습니다.')); return; }
        release();
        run = { notes: pattern(keys.length), t0: performance.now() + LEAD };
        test.textContent = tr('테스트 중지');
        say(tr('떨어지는 노트가 아래 선에 닿을 때 해당 레인 키를 누르세요.'));
        pad.focus(); tick();
    }
    function finish() {
        const s = summary(run.notes);
        run = null; test.textContent = tr('미니 테스트');
        const lean = s.mean > 0 ? tr('늦게') : s.mean < 0 ? tr('빠르게') : tr('정확히');
        say(s.hits ? tr`결과: ${s.total}개 중 ${s.hits}개 판정, 평균 ${ms(s.mean)} (${lean} 누름), 편차 ±${s.spread}ms. 화면만 보고 치는 테스트라 음악 오프셋과는 다를 수 있습니다.` : tr`결과: ${s.total}개 모두 놓쳤습니다.`);
    }
    function stop() {
        if (!run) return;
        run = null; test.textContent = tr('미니 테스트'); say(tr('테스트를 중지했습니다.'));
    }

    root.addEventListener('keydown', down);
    document.addEventListener('keyup', up);
    window.addEventListener('blur', release);
    dialog.addEventListener('close', () => { stop(); stopCapture(); release(); });
    test.addEventListener('click', () => run ? stop() : start());
    chordReset.addEventListener('click', () => { max = held.size; draw(); });
    return { setKeys, capture, stopCapture, capturing: () => cursor !== null, say };
}
