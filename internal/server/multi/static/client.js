import { createChat } from '/lobby/chat.js';
const el = id => document.getElementById(id);
let socket, playerID, room, simulation, clockTimer;
let requestID = 0, sequence = 0, offset = 0, bestRTT = Infinity, uncertainty = Infinity;
let nextClock = 0, localStart, armedMatch, through = -Infinity;
const samples = [], history = [];
const pending = new Map();
const number = id => Number(el(id).value);
const chat = createChat(document.body, text => send('chat', {text}));

function log(message) {
  const lines = el('log').textContent.split('\n').slice(-39);
  el('log').textContent = [...lines, message].join('\n');
}
function send(type, fields = {}) {
  if (socket?.readyState !== WebSocket.OPEN) return;
  const id = String(++requestID);
  const sent = performance.now() * 1e6;
  pending.set(id, { type, sent });
  socket.send(JSON.stringify({ type, id, ...(type === 'clock' ? { sent_ns: Math.round(sent) } : {}), ...fields }));
  return id;
}
function stopSimulation() {
  clearInterval(simulation);
  simulation = undefined;
}
function render() {
  const connected = socket?.readyState === WebSocket.OPEN && !!playerID;
  chat.setPlayer(playerID, room?.members?.find(m=>m.id===playerID)?.name ?? el('name').value);
  chat.setRoom(room?.id, room?.name, room?.phase, connected);
  const host = room?.host_id === playerID;
  const lobby = room?.phase === 'lobby';
  el('connect').disabled = !!socket;
  el('disconnect').disabled = !socket;
  for (const id of ['create', 'join']) el(id).disabled = !connected || !!room;
  el('refresh').disabled = !connected;
  el('leave').disabled = !room;
  el('select').disabled = !host || !lobby;
  for (const id of ['ready', 'unready']) el(id).disabled = !lobby || !room?.selection;
  el('start').disabled = !host || !lobby;
  el('reset').disabled = !host || room?.phase !== 'finished';
  const active = room?.phase === 'playing' && room.members.find(p => p.id === playerID)?.status === 'playing';
  el('simulate').disabled = !active || !!simulation || !Number.isFinite(bestRTT) || room?.verify_results;
  el('abort').disabled = !active;
  el('state').textContent = room ? JSON.stringify(room, null, 2) : '방에 참가하세요.';
  el('summary').textContent = room ? `${room.name} / ${room.phase}${room.notice ? ` / ${room.notice}` : ''}` : '방에 참가하세요.';
  el('members').replaceChildren();
  for (const m of room?.members ?? []) {
    let diff = m.id === playerID ? '내 최신 점수' : '내 이력 대기';
    if (m.id !== playerID && m.score_at_ns <= through) {
      const point = history.findLast(p => p.at <= m.score_at_ns);
      if (point) diff = `${point.points - m.score.Points >= 0 ? '+' : ''}${point.points - m.score.Points}`;
    }
    const tr = document.createElement('tr');
    for (const value of [m.name, m.ready ? '준비' : '대기', m.status, m.score.Points, (m.score_at_ns / 1e9).toFixed(3), diff]) {
      const td = document.createElement('td'); td.textContent = String(value); tr.append(td);
    }
    el('members').append(tr);
  }
}

el('connect').onclick = () => {
  const name = el('name').value.trim();
  if (!name) return log('닉네임을 입력하세요.');
  socket = new WebSocket(`${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/ws`);
  socket.onmessage = ({ data }) => {
    const e = JSON.parse(data);
    const request = pending.get(e.id);
    if (request) pending.delete(e.id);
    if (e.type === 'welcome') {
      if (e.version !== 6) { log('지원하지 않는 프로토콜 버전'); socket.close(); return; }
      playerID = e.player_id;
      chat.setPlayer(playerID);
      el('connection').textContent = `접속: ${playerID}`;
      send('hello', { name });
      send('list');
      send('clock');
      clockTimer = setInterval(() => {
        if (performance.now() >= nextClock) {
          send('clock'); nextClock = performance.now() + (samples.length < 3 ? 200 : 5000);
        }
      }, 200);
    } else if (e.type === 'chat') {
      chat.receive(e.message);
    } else if (e.type === 'chat_history') {
      chat.history(e.room_id, e.messages ?? []);
    } else if (e.type === 'clock' && request) {
      const now = performance.now() * 1e6, c = e.clock;
      if (c) {
        const rtt = (now - c.client_sent_ns) - (c.server_sent_ns - c.server_recv_ns);
        const sampleOffset = ((c.server_recv_ns - c.client_sent_ns) + (c.server_sent_ns - now)) / 2;
        if (rtt >= 0 && rtt < 2e9) samples.push({ at: now, rtt, offset: sampleOffset });
        while (samples.length > 16 || samples.length && now - samples[0].at > 30e9) samples.shift();
        if (samples.length >= 3) {
          const best = [...samples].sort((a,b) => a.rtt-b.rtt).slice(0,3);
          const offsets = best.map(s => s.offset).sort((a,b) => a-b);
          bestRTT = best[0].rtt / 1e6; offset = offsets[1];
          uncertainty = (best[2].rtt / 2 + offsets[2] - offsets[0]) / 1e6;
        }
      }
      el('clock').textContent = `단조 시계 동기화 ${samples.length}개 / 최소 RTT ${bestRTT.toFixed(2)} ms / 추정 불확실성 ${uncertainty.toFixed(2)} ms`;
    } else if (e.type === 'room') {
      if (room?.match_id !== e.room.match_id) {
        stopSimulation(); sequence = 0; history.length = 0; through = -Infinity;
      }
      room = e.room;
      el('room-id').value = room.id;
      if (room.phase === 'arming' && room.members.find(m => m.id === playerID)?.participant && armedMatch !== room.match_id) {
        localStart = room.start_ns - offset;
        if (uncertainty > 100 || localStart - performance.now() * 1e6 < 100e6) {
          log('시작 예약을 확인할 시계 정확도 또는 여유가 부족합니다.');
        } else {
          armedMatch = room.match_id;
          history.push({ at: room.selection.begin_ns, points: 0 });
          send('armed', { match_id: room.match_id, start_ns: room.start_ns });
        }
      }
      if (room.phase !== 'playing') stopSimulation();
    } else if (e.type === 'rooms') {
      el('rooms').replaceChildren();
      for (const r of e.rooms ?? []) {
        const li = document.createElement('li'), button = document.createElement('button');
        button.textContent = `${r.locked ? '🔒 ' : ''}${r.name} (${r.players}/${r.capacity}) — ${r.phase}`;
        button.disabled = !!room || r.phase !== 'lobby' || r.players >= r.capacity;
        button.onclick = () => { el('room-id').value = r.id; if (r.locked) el('join-password').focus(); else send('join', { room_id: r.id }); };
        li.append(button); el('rooms').append(li);
      }
    } else if (e.type === 'ack' && request?.type === 'leave') {
      room = undefined; stopSimulation(); send('list');
    } else if (e.type === 'error') {
      chat.error(e.id, e.error);
      log(`오류: ${e.error}`);
      if (e.error === 'removed from room by host') { room = undefined; stopSimulation(); }
      if (request?.type === 'progress' || request?.type === 'finish') stopSimulation();
    }
    if (e.type === 'ack') chat.ack(e.id);
    if (!['room', 'clock'].includes(e.type)) log(`${e.type}${e.id ? ` #${e.id}` : ''}`);
    render();
  };
  socket.onclose = () => {
    socket = undefined; playerID = undefined; room = undefined;
    clearInterval(clockTimer); stopSimulation(); pending.clear();
    bestRTT = Infinity; offset = 0;
    uncertainty = Infinity; samples.length = 0; history.length = 0;
    armedMatch = undefined; through = -Infinity; nextClock = 0;
    el('connection').textContent = '연결 종료'; render();
    chat.disconnected();
  };
  socket.onerror = () => log('서버 연결 실패');
  render();
};
el('disconnect').onclick = () => socket?.close();
el('create').onclick = () => { const password = el('create-password').value; el('create-password').value = ''; send('create', { name: el('room-name').value, capacity: number('capacity'), password }); };
el('refresh').onclick = () => send('list');
el('join').onclick = () => { const password = el('join-password').value; el('join-password').value = ''; send('join', { room_id: el('room-id').value, password }); };
el('leave').onclick = () => send('leave');
el('select').onclick = () => send('select', { selection: {
  title: el('title').value, chart_hash: el('hash').value, lanes: number('lanes'),
  rules: 'ggosu-keys-v3', score_version: 2, begin_ns: 0,
  duration_ns: number('duration') * 1e9, judgements: number('judgements'), rate: 1, mods: {},
} });
for (const id of ['ready', 'unready']) el(id).onclick = () => send('ready', {
  ready: id === 'ready', revision: room.revision, chart_hash: el('loaded-hash').value,
});
el('start').onclick = () => send('start');
el('reset').onclick = () => send('reset');
el('abort').onclick = () => { stopSimulation(); send('abort', { match_id: room.match_id }); render(); };
el('simulate').onclick = () => {
  const match = room.match_id;
  simulation = setInterval(() => {
    if (room?.match_id !== match) { stopSimulation(); return; }
    const c = room.selection;
    // Leave 50 ms behind the estimated server clock for this test sender.
    const elapsed = (performance.now() * 1e6 - localStart) / 1e6;
    if (elapsed < 0) return;
    const at = Math.min(c.duration_ns, c.begin_ns + Math.floor(elapsed * 1e6 * c.rate));
    const total = Math.floor(c.judgements * (at - c.begin_ns) / (c.duration_ns - c.begin_ns));
    for (let j = history.length; j <= total; j++) {
      history.push({ at: c.begin_ns + (c.duration_ns-c.begin_ns)*j/c.judgements,
        points: Math.floor(j*2200*500/c.judgements) });
    }
    through = at;
    const score = { Kool: total, Cool: 0, Good: 0, Miss: 0, Combo: total, MaxCombo: total,
      Points: Math.floor(total * 2200 * 500 / c.judgements), Total: total,
      Possible: c.judgements, Flow: 50, Precision: 20, Earned: total * 2200 };
    const finished = at === c.duration_ns;
    send(finished ? 'finish' : 'progress', { match_id: match, sequence: ++sequence, position_ns: at, score_at_ns: at, score });
    if (finished) stopSimulation();
    render();
  }, 200);
  render();
};
render();
