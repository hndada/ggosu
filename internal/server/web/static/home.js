import { normalizeScore } from '/songlist/analysis.js';

const status = document.querySelector('#records-status');
const list = document.querySelector('#records');

async function records() {
    const rows = [];
    let skipped = 0;
    for (let i = 0; i < localStorage.length; i++) {
        const key = localStorage.key(i);
        if (!key.startsWith('ggosu:record:')) continue;
        try {
            const r = JSON.parse(localStorage.getItem(key));
            if (!r.id || !r.metadata || !r.score || !r.replay || !Number.isFinite(r.score.Points) || !Number.isFinite(Date.parse(r.date))) throw new Error('invalid record');
            rows.push(r);
        } catch { skipped++; }
    }
    rows.sort((a, b) => Date.parse(b.date) - Date.parse(a.date));
    status.textContent = rows.length ? `${rows.length}개 기록 중 최근 ${Math.min(5, rows.length)}개` : '아직 저장된 플레이 기록이 없어요.';
    if (skipped) status.textContent += ` 읽을 수 없는 기록 ${skipped}개는 제외했어요.`;
    if (!rows.length) return;
    let entries = [];
    try {
        const response = await fetch('/api/catalog');
        if (!response.ok) throw new Error('catalog unavailable');
        entries = (await response.json()).entries;
    } catch { status.textContent += ' 채보 연결을 불러오지 못했어요.'; }
    for (const r of rows.slice(0, 5)) {
        const li = document.createElement('li');
        const e = entries.find(e => e.hash === r.replay.chart_hash);
        const title = r.metadata.title || '제목 없는 곡';
        if (e) {
            const a = document.createElement('a');
            a.href = '/play?' + new URLSearchParams({ chart: e.path });
            a.textContent = title;
            li.append(a);
        } else li.append(document.createTextNode(title));
        const mode = r.mode === 'auto' ? 'Auto' : r.mode === 'replay' ? '리플레이' : '일반';
        const p = document.createElement('p');
        p.textContent = `${new Date(r.date).toLocaleString('ko-KR')} · ${normalizeScore(r.score).Points.toLocaleString('ko-KR')}점 · ${mode} · ${r.completed ? '완주' : '미완주'}`;
        li.append(p); list.append(li);
    }
}

records().catch(() => { status.textContent = '브라우저 저장소에 접근할 수 없어 기록을 표시하지 못했어요.'; });
