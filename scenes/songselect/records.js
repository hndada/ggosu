import { tr } from '../../preferences/language.js';
import { best, eligible, comparison, modsText } from '/songlist/analysis.js';
import { offsetText } from '/settings/offsets.js';

const $ = selector => document.querySelector(selector);
let scoreFrame = 0, scoreTicket = 0;
function showPoints(points, past) {
    const ticket = ++scoreTicket;
    if (scoreFrame) cancelAnimationFrame(scoreFrame);
    scoreFrame = 0;
    const el = $('#result-points');
    if (past || !points || typeof requestAnimationFrame !== 'function' || window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) {
        el.textContent = points.toLocaleString();
        return;
    }
    const start = performance.now();
    el.textContent = '0';
    const frame = now => {
        if (ticket !== scoreTicket) return;
        scoreFrame = 0;
        const t = Math.min(1, Math.max(0, (now - start) / 650));
        if ($('#result').hidden || t === 1) { el.textContent = points.toLocaleString(); return; }
        el.textContent = Math.round(points * (1 - (1 - t) ** 5)).toLocaleString();
        scoreFrame = requestAnimationFrame(frame);
    };
    scoreFrame = requestAnimationFrame(frame);
}
function mode(r) {
    const name = { live: tr('일반 플레이'), auto: 'Auto', replay: tr('리플레이 재생') }[r.mode];
    return tr`${name} · ${r.rate}배 · Pitch ${r.pitch === false ? 'Off' : 'On'} · ${modsText(r.mods)}` + (r.practice ? tr(' · 배속 연습') : '') + (r.scoreVersion === 1 ? tr(' · 이전 점수 규칙') : '');
}
function flags(r) {
    const tags = [];
    if (r.mode === 'auto') tags.push('AUTO PLAY');
    if (r.mode === 'replay') tags.push('REPLAY');
    if (r.practice) tags.push('PRACTICE');
    if (r.rate !== 1) tags.push(`${r.rate.toFixed(2)}×`);
    if (r.pitch === false) tags.push('PITCH OFF');
    if (r.mods?.kind) tags.push(r.mods.kind.toUpperCase());
    if (r.mods?.visibility) tags.push({ hidden: 'HD', fadein: 'FI', cover: 'CO', flashlight: 'FL' }[r.mods.visibility]);
    if (r.mods?.o2jam) tags.push(`O2JAM ${r.mods.o2jam.toUpperCase()}` + (r.mods.pills ? ' + PILLS' : ''));
    else if (r.health?.level) tags.push(`${['', 'EASY', 'NORMAL', 'HARD'][r.health.level]} GAUGE`);
    if (r.scoreVersion === 1 || r.verification === 'legacy') tags.push('LEGACY');
    return tags;
}
export function pauseSummary(events = []) {
    let start = null, total = 0, count = 0, canceled = 0;
    for (const e of events) {
        if (e.action === 'pause') { start = e.elapsed_ns; count++; }
        if (e.action === 'resume-cancel') canceled++;
        if ((e.action === 'resume' || e.action === 'end') && start != null) { total += e.elapsed_ns-start; start = null; }
    }
    return count ? tr`일시정지 ${count}회 · ${(total/1e9).toFixed(1)}초${canceled ? tr` · 재개 취소 ${canceled}회` : ''}` : tr('일시정지 없음');
}
function percent(r) { return r.score.Total ? `${(r.accuracy * 100).toFixed(2)}%` : '—'; }
let submissionTicket = 0;
export function showSubmission(r) {
    const box = $('#result-submission');
    if (!box) return;
    const unsent = r?.mode === 'live' && r.scoreVersion === 2 && r.verification !== 'legacy' && (r.completed && r.score?.Points > 0 || r.health?.failed);
    const s = r?.submission ?? (unsent ? { state: 'unsent' } : null);
    box.hidden = !s;
    const status = $('#result-submission-status'), ticket = ++submissionTicket;
    status.textContent = !s ? '' : ({ unsent: tr('서버 제출 전'), pending: tr('서버에 점수 제출 중…'), sent: tr('서버에 점수를 제출했습니다.'), disabled: tr('서버 제출 꺼짐'), failed: tr('서버 제출 실패') }[s.state] ?? '');
    $('#submission-retry').hidden = !['unsent', 'failed', 'disabled'].includes(s?.state);
    $('#submission-retry').textContent = s?.state === 'unsent' ? tr('점수 제출') : tr('점수 제출 재시도');
    // Error details are for developers; release builds show only the failure.
    if (s?.state !== 'failed' || !s.error) return;
    return window.runtime?.Environment?.().then(env => {
        if (ticket === submissionTicket && env?.buildType !== 'production') status.textContent = tr`서버 제출 실패: ${s.error}`;
    }, () => {});
}
// past: a saved record opened from the history table, so this play's save and calibration notes do not apply.
export function showResult(r, { past = false } = {}) {
    showSubmission(r);
    $('#songselect').hidden = true;
    $('#result').hidden = false;
    $('#result-song').textContent = r.title;
    $('#result-keys').textContent = `${r.lanes}KEY`;
    $('#result-difficulty').textContent = (r.difficulty ?? '').toUpperCase();
    $('#result-state').textContent = r.health?.failed ? 'FAILED' : r.completed ? 'CLEAR' : 'QUIT';
    $('#result').dataset.completed = String(r.completed);
    $('#result-heading').textContent = past ? 'RECORD' : 'RESULT';
    const tags = flags(r);
    $('#result-flags').hidden = tags.length === 0;
    $('#result-flags').replaceChildren(...tags.map(text => { const span = document.createElement('span'); span.textContent = text; return span; }));
    $('#result-run').open = false;
    const health = r.health;
    $('#result-mode').textContent = mode(r) + (r.mods?.o2jam ? '' : health ? tr` · 체력 ${health.start && health.start !== health.level ? `${health.start}→` : ''}${health.level}단` + (health.start ? health.fail_level ? tr` · ${health.fail_level}단 0% 게임 오버` : tr(' · 계속 진행') : '') : '');
    $('#result-pauses').textContent = pauseSummary(r.pauses);
    $('#result-offsets').textContent = tr`보정 ${offsetText(r.offsets)}`;
    // The global offset moved by this play's mean hit error (positive = late).
    const c = r.calibration, mean = Math.round(c?.meanMS ?? 0);
    $('#result-calibration').hidden = !c || past;
    $('#result-adjustment-label').hidden = $('#result-adjustment').hidden = !c;
    if (c) {
        const delta = c.toMS-c.fromMS;
        $('#result-calibration span').textContent = tr`타이밍 ${delta > 0 ? '+' : ''}${delta} ms 조정`;
        $('#result-adjustment').textContent = tr`전체 ${c.fromMS} → ${c.toMS} ms · 평균 ${mean > 0 ? '+' : ''}${mean} ms`;
    }
    $('#calibration-undo').disabled = false;
    $('#comparison').hidden = true;
    $('#comparison-status').textContent = '';
    $('#comparison-heading').hidden = true;
    $('#comparison-status').classList.remove('improved');
    showPoints(r.score.Points, past);
    $('#result-accuracy').textContent = percent(r);
    $('#result-combo').textContent = r.score.MaxCombo.toLocaleString();
    const total = ['Kool', 'Cool', 'Good', 'Miss'].reduce((sum, name) => sum + r.score[name], 0);
    for (const name of ['Kool', 'Cool', 'Good', 'Miss']) {
        $(`#result-${name.toLowerCase()}`).textContent = r.score[name].toLocaleString();
        $(`#result-bar-${name.toLowerCase()}`).style.width = `${total ? r.score[name] / total * 100 : 0}%`;
    }
    // Cool/Good hits before/after the note (Kool excluded). Hidden when there are none or an unverified old record lacks the counts.
    const h = r.hitError, timed = (h?.fast ?? 0) + (h?.slow ?? 0) > 0;
    $('#result-timing').hidden = !timed;
    if (timed) { $('#result-fast').textContent = h.fast.toLocaleString(); $('#result-slow').textContent = h.slow.toLocaleString(); }
    $('#result-saved').textContent = !past && r.saved === false ? tr('기록 저장 실패') : '';
    $('#result-warnings').textContent = (r.warnings ?? []).join('\n');
    $('#result-heading').focus();
}
// Hover details for a score row: numbers first, then non-default play conditions only.
export function brief(r) {
    const d = new Date(r.date), pad = n => String(n).padStart(2, '0');
    const total = (r.offsets?.globalMS ?? 0) + (r.offsets?.chartMS ?? 0);
    const tags = [
        r.mode === 'auto' ? 'Auto' : r.mode === 'replay' ? tr('리플레이') : '',
        r.health?.failed ? (r.mods?.o2jam ? tr('O2Jam 탈락') : tr`체력 ${r.health.level}단 탈락`) : r.completed ? '' : tr('미완주'),
        r.pauses?.length ? pauseSummary(r.pauses) : '',
        r.rate !== 1 ? `${r.rate}x` : '', r.pitch === false ? 'Pitch Off' : '',
        r.mods?.kind || r.mods?.visibility || r.mods?.o2jam ? modsText(r.mods) : '', r.practice ? tr('연습') : '',
        total ? tr`보정 ${total > 0 ? '+' : ''}${total}ms` : '', r.scoreVersion === 1 ? tr('구 규칙') : '',
    ].filter(Boolean);
    return [tr`${percent(r)} · ${r.score.MaxCombo.toLocaleString()}콤보`,
        `${d.getFullYear()}.${pad(d.getMonth() + 1)}.${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`,
        tags.join(' · ')].filter(Boolean);
}
// An in-app panel beside the hovered row instead of the browser's delayed title tooltip.
function showTip(row, r) {
    const tip = $('#score-tip'), rect = row.getBoundingClientRect();
    tip.replaceChildren(...brief(r).map((text, i) => {
        const line = document.createElement('span'); line.className = i ? 'tip-sub' : 'tip-main'; line.textContent = text; return line;
    }));
    tip.hidden = false;
    const width = tip.offsetWidth, right = rect.right + 8 + width <= innerWidth;
    tip.style.left = `${right ? rect.right + 8 : Math.max(0, rect.left - 8 - width)}px`;
    tip.style.top = `${Math.max(0, Math.min(rect.top, innerHeight - tip.offsetHeight))}px`;
}
function hideTip() { $('#score-tip').hidden = true; }
let historyTicket = 0;
// osu!lazer's leaderboard placeholder: a warning-circle icon and one message where the list would be.
// A new message grows in from 0.8 (theme.css); the same message staying up does not replay.
function placeholder(text) {
    const s = $('#history-status'), message = $('#history-message');
    if (s.classList.contains('placeholder') && message.textContent === text) return;
    message.textContent = text;
    s.classList.add('placeholder', 'entering'); void s.offsetWidth; s.classList.remove('entering');
}
function clearStatus() { $('#history-status').classList.remove('placeholder'); $('#history-message').textContent = ''; }
export function scores(rows, mode, order) {
    return rows.filter(r => (r.score.Points > 0 || r.health?.failed) && (mode === 'all' || r.mode === 'live')).sort((a, b) => {
        if (order === 'points') {
            const points = b.score.Points - a.score.Points;
            if (points) return points;
            const accuracy = b.accuracy - a.accuracy;
            if (accuracy) return accuracy;
        }
        return Date.parse(b.date) - Date.parse(a.date) || a.id.localeCompare(b.id);
    });
}
// Rival and global rankings need a ranking server, which gosu does not have yet. Until then
// they list made-up demo scores, marked as such, so the list can be laid out and debugged.
const DEMO_NAMES = { rival: ['Kazuki', 'mint', 'Nobody'], global: ['Ayato', 'chocomint', 'Kazuki', 'Rin', 'mint', 'Haru', 'Nobody', 'Sora'] };
export function demoScores(scope, key) {
    let seed = 0;
    for (const c of `${scope}:${key}`) seed = (seed * 31 + c.charCodeAt(0)) >>> 0;
    const next = () => (seed = (seed * 1664525 + 1013904223) >>> 0) / 2 ** 32;
    let points = 1100000 - Math.floor(next() * 30000);
    return (DEMO_NAMES[scope] ?? []).map((name, i) => {
        if (i) points -= 5000 + Math.floor(next() * 40000);
        return { rank: i + 1, name, points, accuracy: (85 + points / 1100000 * 15).toFixed(2) };
    });
}
function showDemo(scope, key) {
    const rows = demoScores(scope, key);
    for (const r of rows) {
        const li = document.createElement('li'); li.className = 'demo';
        const rank = document.createElement('span'); rank.className = 'demo-rank'; rank.textContent = `#${r.rank}`;
        const name = document.createElement('span'); name.textContent = r.name;
        const points = document.createElement('strong'); points.textContent = r.points.toLocaleString();
        const accuracy = document.createElement('span'); accuracy.className = 'demo-accuracy'; accuracy.textContent = `${r.accuracy}%`;
        li.append(rank, name, points, accuracy); $('#history').append(li);
    }
    $('#history').hidden = rows.length === 0;
    clearStatus(); $('#history-message').textContent = tr('데모');
}
// Empties the table at once so the previous chart's records never show under a new selection.
export function clearHistory() {
    ++historyTicket;
    $('#history').hidden = true;
    $('#history').replaceChildren();
    hideTip();
    $('#best').textContent = '';
}
export async function history(e, s, update, open, scope = 'local') {
    clearHistory();
    const ticket = historyTicket;
    if (!e) { placeholder(tr('채보를 선택해 주세요')); update(); return; }
    try {
        const rows = await window.go.main.App.History(s);
        if (ticket !== historyTicket) return;
        const kind = $('#lane-mod').value;
        const normal = rows.filter(r => eligible(r) && (r.mods?.kind || '') === kind && (r.mods?.visibility || '') === ($('#visibility-mod')?.value || '') && (r.mods?.o2jam || '') === ($('#o2jam-mod')?.value || '') && !!r.mods?.pills === !!($('#o2jam-mod')?.value && $('#o2jam-pills')?.checked));
        if (normal.length) {
            const b = best(normal);
            $('#best').textContent = kind.includes('random') ? '' : tr`최고 ${b.score.Points.toLocaleString()} · ${percent(b)}`;
        }
        if (scope !== 'local') { showDemo(scope, `${e.folder}/${e.path}/${e.index}`); update(); return; }
        const visible = scores(rows, 'live', $('#history-order').value);
        for (const r of visible) {
            const li = document.createElement('li');
            const result = document.createElement('button');
            result.type = 'button'; result.className = 'score-open';
            // Saved play conditions stay visible beside the score; the hover panel adds details such as the random seed.
            for (const name of ['pointerenter', 'focus']) result.addEventListener(name, () => showTip(result, r));
            for (const name of ['pointerleave', 'blur']) result.addEventListener(name, hideTip);
            const tags = flags(r).join(' · ');
            result.setAttribute('aria-label', [tr`${r.score.Points.toLocaleString()}점 · 결과 보기`, tags].filter(Boolean).join(' · '));
            const points = document.createElement('strong'); points.textContent = r.score.Points.toLocaleString();
            result.append(points);
            if (tags) {
                const mods = document.createElement('span'); mods.className = 'score-mods'; mods.textContent = tags;
                result.append(mods);
            }
            result.addEventListener('click', () => { hideTip(); open(r); });
            li.append(result); $('#history').append(li);
        }
        $('#history').hidden = visible.length === 0;
        if (visible.length) clearStatus(); else placeholder(tr('스코어 없음'));
        update();
    } catch (e) {
        if (ticket !== historyTicket) return;
        // Error details are for developers; release builds show only the failure.
        const env = await window.runtime?.Environment?.().catch(() => null);
        if (ticket !== historyTicket) return;
        placeholder(env && env.buildType !== 'production' ? tr`로컬 스코어를 읽지 못함: ${e?.message ?? String(e)}` : tr('로컬 스코어를 읽지 못함'));
        update();
    }
}
const signed = (n, digits = 0) => `${n > 0 ? '+' : ''}${n.toFixed(digits)}`;
export async function compareResult(r, s, currentResult) {
    try {
        const rows = await window.go.main.App.History(s);
        if (currentResult()?.id !== r.id) return;
        const c = comparison(rows, r);
        // Plays that are not compared (Auto, replay, practice, unfinished...) show no comparison at all.
        if (c.status === 'excluded') return;
        $('#comparison-heading').hidden = false;
        if (c.status === 'first') {
            $('#comparison-status').textContent = tr('첫 기록');
        } else {
            $('#comparison-status').textContent = tr`${c.improved ? tr('최고 점수 갱신! ') : ''}이전 최고 ${c.previous.score.Points.toLocaleString()}점`;
            $('#comparison-status').classList.toggle('improved', c.improved);
            for (const key of ['points', 'combo', 'Kool', 'Cool', 'Good', 'Miss']) $('#delta-' + key).textContent = signed(c.delta[key]);
            $('#delta-accuracy').textContent = signed(c.delta.accuracy, 2);
            $('#comparison').hidden = false;
        }
    } catch (err) { if (currentResult()?.id === r.id) { $('#comparison-heading').hidden = false; $('#comparison-status').textContent = tr('비교 실패'); } }
}
