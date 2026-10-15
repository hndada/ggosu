import { tr } from '../../preferences/language.js';
import { chatOptions, loadChat, watchChat, mountChatSettings } from '../settings/chat.js';
import { alertFor } from '../../preferences/chat.js';
import { cue } from '../../audio/sound.js';

// One room drawer survives phase changes; the transport remains its owner's job.
export function createChat(root, send) {
  const toggle = document.createElement('button');
  toggle.type = 'button'; toggle.className = 'room-chat-toggle';
  toggle.setAttribute('aria-controls', 'room-chat-panel');
  const panel = document.createElement('aside');
  panel.id = 'room-chat-panel'; panel.className = 'room-chat-panel'; panel.hidden = true;
  panel.setAttribute('aria-label', tr('방 채팅'));
  const head = document.createElement('header'); head.className = 'room-chat-head';
  const title = document.createElement('h2'); title.textContent = tr('방 채팅');
  const context = document.createElement('p'); context.className = 'room-chat-context';
  const close = document.createElement('button'); close.type = 'button'; close.className = 'room-chat-close'; close.textContent = tr('접기 F8');
  const settings = document.createElement('button'); settings.type = 'button'; settings.className = 'room-chat-settings'; settings.textContent = tr('설정');
  head.append(title, settings, close, context);
  const resize = document.createElement('div'); resize.className = 'room-chat-resize'; resize.tabIndex = 0;
  resize.setAttribute('role', 'separator'); resize.setAttribute('aria-label', tr('채팅창 너비')); resize.setAttribute('aria-orientation', 'vertical');
  const body = document.createElement('div'); body.className = 'room-chat-body';
  const log = document.createElement('div'); log.className = 'room-chat-log';
  log.setAttribute('role', 'log'); log.setAttribute('aria-label', tr('방 대화 이력'));
  log.setAttribute('aria-live', 'off'); log.tabIndex = 0;
  const latest = document.createElement('button'); latest.type = 'button'; latest.className = 'room-chat-new'; latest.hidden = true;
  const empty = document.createElement('p'); empty.className = 'room-chat-empty'; empty.textContent = tr('아직 대화가 없어요. 첫 메시지를 보내 보세요.');
  body.append(empty, log, latest);
  const form = document.createElement('form'); form.className = 'room-chat-compose';
  // osu!lazer StandAloneChatDisplay: one 30 px box, Enter sends (Shift+Enter still breaks a line).
  const input = document.createElement('textarea'); input.rows = 1;
  input.setAttribute('aria-label', tr('채팅 메시지')); input.placeholder = tr('방에 메시지 보내기');
  const actions = document.createElement('div'); actions.className = 'room-chat-actions';
  const hint = document.createElement('span'); hint.className = 'room-chat-hint'; hint.textContent = tr('Enter 전송 · Shift+Enter 줄바꿈');
  const count = document.createElement('span'); count.className = 'room-chat-count';
  const submit = document.createElement('button'); submit.type = 'submit'; submit.textContent = tr('전송');
  actions.append(count, submit);
  const status = document.createElement('p'); status.className = 'room-chat-status'; status.setAttribute('role', 'status');
  form.append(input, actions, hint, status); panel.append(resize, head, body, form); root.append(toggle, panel);
  const ticker = document.createElement('button'); ticker.type='button'; ticker.className='room-chat-ticker'; ticker.hidden=true;
  const popup = document.createElement('button'); popup.type='button'; popup.className='room-chat-popup'; popup.hidden=true; popup.setAttribute('aria-live','polite');
  root.append(ticker,popup);

  // wanted is the player's choice (open by default); the drawer shows only inside a room.
  let roomID = '', self = '', open = false, wanted = true, connected = false, composing = false, request, width = 360;
  let prefs = chatOptions(), playing = false, delegated = false, selfName = '', pendingAlert, popupTimer, tickerTimer, settingsDialog;
  const stopAlerts = () => { clearTimeout(popupTimer); clearTimeout(tickerTimer); popup.hidden=ticker.hidden=true; pendingAlert=undefined; };
  const preview = m => `${m.name} · ${[...m.text.replace(/\s+/g,' ')].slice(0,90).join('')}`;
  async function ring() {
    try {
      const app = window.go?.main?.App;
      const audio = app?.AudioSettings ? await app.AudioSettings() : JSON.parse(localStorage.getItem('ggosu:audio') || '{"master":100,"effects":50}');
      cue('confirm', (audio.master ?? 100) * (audio.effects ?? 50) / 100);
    } catch (e) { status.textContent=tr`알림음 재생 실패: ${e?.message ?? e}`; }
  }
  function showPopup(m, count=1) {
    popup.textContent=`${count>1 ? tr`새 메시지 ${count} · ` : ''}${preview(m)}`; popup.hidden=false;
    clearTimeout(popupTimer); popupTimer=setTimeout(()=>{popup.hidden=true;},5000);
  }
  function alert(m) {
    const a=alertFor(prefs,{text:m.text,name:selfName,own:m.player_id===self,visible:open && state()?.follow,playing});
    if (a.ticker && !open && !delegated) { ticker.textContent=preview(m); ticker.hidden=false; clearTimeout(tickerTimer); tickerTimer=setTimeout(()=>{ticker.hidden=true;},5000); }
    if (a.defer) { pendingAlert={message:m,count:(pendingAlert?.count??0)+1,popup:a.popup||!!pendingAlert?.popup,sound:a.sound||!!pendingAlert?.sound}; return; }
    if (!delegated) { if (a.popup) showPopup(m); if(a.sound) void ring(); }
  }
  ticker.onclick=popup.onclick=()=>{show();};
  settings.onclick=()=>{
    if(document.getElementById('panel-chat')) { window.dispatchEvent(new CustomEvent('ggosu:chat-settings')); return; }
    if(!settingsDialog) {
      settingsDialog=document.createElement('dialog'); settingsDialog.className='chat-settings-dialog'; settingsDialog.setAttribute('aria-label',tr('채팅 · 알림 설정'));
      const heading=document.createElement('h2'); heading.textContent=tr('채팅 · 알림');
      const done=document.createElement('button'); done.type='button'; done.textContent=tr('닫기'); done.onclick=()=>settingsDialog.close();
      settingsDialog.append(heading,done); mountChatSettings(settingsDialog); root.append(settingsDialog);
    }
    settingsDialog.showModal();
  };
  const rooms = new Map();
  const state = () => rooms.get(roomID);
  const nearBottom = () => log.scrollHeight - log.clientHeight - log.scrollTop < 12;
  const fitInput = () => { input.style.height = '0px'; input.style.height = `${Math.min(112, Math.max(40, input.scrollHeight))}px`; };
  function size() {
    panel.style.width = `${width}px`;
    resize.setAttribute('aria-valuemin', '280'); resize.setAttribute('aria-valuemax', '480'); resize.setAttribute('aria-valuenow', String(width));
    const list = document.getElementById('songselect');
    if (list) list.style.marginRight = open && window.innerWidth > 900 ? `${width + 16}px` : '';
  }
  // Retain a visible message, including its offset, when old rows are trimmed or rewrapped.
  function anchor() {
    const top = log.getBoundingClientRect().top;
    const row = [...log.children].find(n => n.getBoundingClientRect().bottom > top);
    return row ? {id: row.dataset.id, offset: row.getBoundingClientRect().top - top} : null;
  }
  function restore(a) {
    const row = a && [...log.children].find(n => n.dataset.id === a.id);
    if (row) log.scrollTop += row.getBoundingClientRect().top - log.getBoundingClientRect().top - a.offset;
    else log.scrollTop = 0;
    if (state()) { state().scroll = log.scrollTop; state().anchor = anchor(); }
  }
  const update = () => {
    const s = state(), unread = s?.unread ?? 0;
    toggle.hidden = !roomID || open;
    toggle.textContent = tr`채팅 F8${unread ? tr` · 새 메시지 ${unread}` : ''}`;
    toggle.setAttribute('aria-expanded', String(open));
    latest.hidden = !open || !unread; latest.textContent = tr`새 메시지 ${unread} · 최신으로`;
    empty.hidden = !!s?.messages.length;
    count.textContent = `${[...input.value].length} / 300`;
    input.disabled = !roomID || !connected; submit.disabled = input.disabled || !!request || !input.value.trim();
  };
  const clearUnread = () => { if (state()) state().unread = 0; update(); };
  // osu!lazer ChatLine: time (only when the minute changes) | name right-aligned in the sender's colour | text.
  const minute = m => Math.floor(new Date(m.time).getTime() / 60000);
  const colour = id => { let h = 0; for (const c of String(id)) h = (h * 31 + c.codePointAt(0)) >>> 0; return h % 32; };
  function line(m, previous) {
    const item = document.createElement('div'); item.className = 'room-chat-message'; item.dataset.id = String(m.id);
    item.dataset.self = String(m.player_id === self);
    const time = document.createElement('span'); time.className = 'room-chat-time';
    if (!previous || minute(previous) !== minute(m)) time.textContent = new Date(m.time).toLocaleTimeString('ko-KR', {hour:'2-digit',minute:'2-digit',hour12:false});
    const name = document.createElement('span'); name.className = 'room-chat-name'; name.dataset.colour = String(colour(m.player_id || m.name));
    name.textContent = m.name;
    const text = document.createElement('span'); text.className = 'room-chat-text';
    for (const [i, part] of m.text.split('\n').entries()) { if (i) text.append(document.createElement('br')); text.append(part); }
    item.append(time, name, text); return item;
  }
  const lines = messages => messages.map((m, i) => line(m, messages[i - 1]));
  function reveal() {
    open = true; panel.hidden = false;
    ticker.hidden=true; if(state()?.follow) stopAlerts();
    size(); fitInput();
    const s = state(); if (s) { if (s.follow) log.scrollTop = log.scrollHeight; else restore(s.anchor); }
    if (nearBottom()) clearUnread(); update();
  }
  function conceal() {
    const s = state(); if (s) { s.scroll = log.scrollTop; s.follow = nearBottom(); s.anchor = anchor(); }
    const focused = panel.contains(document.activeElement);
    open = false; panel.hidden = true; size(); update(); if (focused) toggle.focus();
  }
  function show() { wanted = true; if (roomID) reveal(); }
  function hide() { wanted = false; conceal(); }
  toggle.onclick = () => open ? hide() : show();
  close.onclick = hide;
  let dragging = false;
  resize.addEventListener('pointerdown', e => { dragging = true; resize.setPointerCapture(e.pointerId); e.preventDefault(); });
  resize.addEventListener('pointermove', e => {
    if (!dragging) return;
    const a = anchor(); width = Math.min(480, Math.max(280, window.innerWidth - e.clientX)); size();
    if (state()?.follow) log.scrollTop = log.scrollHeight; else restore(a);
  });
  resize.addEventListener('pointerup', () => { dragging = false; });
  resize.addEventListener('lostpointercapture', () => { dragging = false; });
  resize.addEventListener('keydown', e => {
    if (!['ArrowLeft', 'ArrowRight'].includes(e.key)) return;
    e.preventDefault(); e.stopPropagation(); const a = anchor();
    width = Math.min(480, Math.max(280, width + (e.key === 'ArrowLeft' ? 20 : -20))); size();
    if (state()?.follow) log.scrollTop = log.scrollHeight; else restore(a);
  });
  window.addEventListener('resize', () => { const a = state()?.anchor; size(); if (open && state()) { if (state().follow) log.scrollTop = log.scrollHeight; else restore(a); } });
  // F8 is osu!'s chat key; it also works while typing.
  window.addEventListener('keydown', e => {
    if (e.key !== 'F8' || e.repeat || !roomID) return;
    e.preventDefault(); e.stopPropagation();
    if (open) hide(); else { show(); input.focus(); }
  }, true);
  log.onscroll = () => {
    if (!open || !state()) return;
    state().scroll = log.scrollTop; state().follow = nearBottom();
    state().anchor = anchor();
    if (state().follow) clearUnread();
  };
  latest.onclick = () => { log.scrollTop = log.scrollHeight; if (state()) { state().follow = true; state().scroll = log.scrollTop; } clearUnread(); };
  input.oninput = () => { if (!composing) input.value = [...input.value].slice(0, 300).join(''); if (state()) state().draft = input.value; fitInput(); update(); };
  input.addEventListener('compositionstart', () => { composing = true; });
  input.addEventListener('compositionend', () => { composing = false; input.oninput(); });
  panel.addEventListener('keydown', e => {
    if (e.key === 'Escape' && !e.isComposing && !composing) { e.preventDefault(); e.stopPropagation(); hide(); }
    else if (e.target === input) e.stopPropagation();
  });
  input.addEventListener('keydown', e => {
    if (e.key === 'Enter' && !e.shiftKey && !e.isComposing && !composing && e.keyCode !== 229) {
      e.preventDefault(); form.requestSubmit();
    }
  });
  form.onsubmit = e => {
    e.preventDefault();
    const text = input.value.trim(); if (!text || request || !roomID || !connected || composing || [...text].length > 300) return;
    let id;
    try { id = send(text); } catch (e) { status.textContent = e?.message ?? String(e); return; }
    if (!id) { status.textContent = tr('연결 상태를 확인하세요.'); return; }
    request = {id, text, roomID}; status.textContent = tr('전송 중'); update();
  };
  const api = {
    setPlayer(id, name='') { self = id; selfName=name; },
    setRoom(id = '', name = '', phase = '', online = true, native = false, active = phase==='playing') {
      connected = !!id && online;
      const wasPlaying=playing; playing=active; delegated=native;
      if(playing && !wasPlaying && prefs.autoHide && open) conceal();
      if(!playing && wasPlaying && pendingAlert && !delegated) { const p=pendingAlert; pendingAlert=undefined; if(p.popup && (prefs.messagePopup||prefs.mentionPopup)) showPopup(p.message,p.count); if(p.sound && prefs.mentionSound) void ring(); }
      if (roomID !== id) {
        stopAlerts();
        const previous = state(); if (previous) { previous.draft = input.value; if (open) { previous.anchor = anchor(); previous.follow = nearBottom(); } }
        roomID = id; request = undefined;
        if (id && !rooms.has(id)) rooms.set(id, {messages:[],draft:'',scroll:0,follow:true,unread:0});
        const s = state(); input.value = s?.draft ?? ''; log.replaceChildren(...lines(s?.messages ?? [])); fitInput();
        if (open && s) { if (s.follow) log.scrollTop = log.scrollHeight; else restore(s.anchor); }
        status.textContent = '';
      }
      context.textContent = id ? `${name} · ${online ? ({lobby:tr('대기실'),preparing:tr('플레이 준비'),arming:tr('시작 대기'),playing:tr('플레이 중'),finished:tr('결과')}[phase] ?? phase) : tr('연결 종료')}` : tr('방에 참가하면 대화를 보낼 수 있어요.');
      if (id && wanted && !open && !(playing && prefs.autoHide)) reveal(); else if (!id && open) conceal(); else update();
    },
    history(id, messages) {
      if (id !== roomID) return;
      const s = state(), existing = new Map(s.messages.map(m => [m.id,m]));
      const a = open ? anchor() : s.anchor;
      for (const m of messages) if (m.room_id === id) existing.set(m.id,m);
      s.messages = [...existing.values()].sort((a,b) => a.id-b.id).slice(-100);
      log.replaceChildren(...lines(s.messages)); if (open) { if (s.follow) log.scrollTop = log.scrollHeight; else restore(a); } update();
    },
    receive(m) {
      if (!m || m.room_id !== roomID) return;
      const s = state(); if (s.messages.some(v => v.id === m.id)) return;
      const follow = s.follow;
      const a = open ? anchor() : s.anchor;
      s.messages.push(m);
      // Dropping the oldest line shifts the alternating rows, so the log is redrawn then.
      if (s.messages.length > 100) { s.messages.shift(); log.replaceChildren(...lines(s.messages)); if (open && !follow) restore(a); }
      else log.append(line(m, s.messages.at(-2)));
      if (m.player_id !== self && (!open || !follow)) s.unread++;
      if (open && follow) { log.scrollTop = log.scrollHeight; s.scroll = log.scrollTop; }
      alert(m);
      update();
    },
    ack(id) {
      if (!request || request.id !== id) return;
      if (input.value.trim() === request.text && roomID === request.roomID) { input.value = ''; if (state()) state().draft = ''; fitInput(); }
      request = undefined; status.textContent = ''; update();
    },
    error(id, text) {
      if (!request || request.id !== id) return;
      request = undefined; status.textContent = text; update();
    },
    disconnected() { connected = false; request = undefined; stopAlerts(); status.textContent = tr('연결이 종료됐어요. 입력 중인 내용은 유지됩니다.'); update(); },
    show, hide,
  };
  const unwatch=watchChat(value=>{const before=prefs; prefs=value; if(!prefs.ticker)ticker.hidden=true; if(!prefs.messagePopup&&!prefs.mentionPopup)popup.hidden=true; if(playing&&prefs.autoHide&&!before.autoHide&&open)conceal();});
  window.addEventListener('pagehide',()=>{unwatch();stopAlerts();},{once:true});
  loadChat().catch(e=>{status.textContent=tr`채팅 설정 읽기 실패: ${e?.message ?? e}`;});
  api.setRoom(); return api;
}
