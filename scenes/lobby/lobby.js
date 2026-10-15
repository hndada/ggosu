import { tr } from '../../preferences/language.js';
import { createChat } from '/lobby/chat.js';
import { mount as mountMods, mountVisibility } from '/settings/mods.js';
import { modsText } from '/songlist/analysis.js';

export function createLobby({ entries, selection, stopPreview }) {
  const app = () => window.go?.main?.App;
  const panel = document.createElement('dialog'); panel.id = 'multi-panel';
  panel.setAttribute('aria-label', tr('멀티플레이 대기실'));
  panel.innerHTML = tr`<header><h2>멀티플레이</h2><button type="button" data-action="close">곡 목록으로</button></header>
    <form id="multi-connect"><label>서버 <input name="endpoint" value="ws://127.0.0.1:8090/ws" required></label>
    <label>닉네임 <input name="name" maxlength="32" value="Player" required></label><button>접속</button></form>
    <p><button type="button" data-action="disconnect">연결 종료</button><span id="multi-message" role="status"></span></p>
    <section id="multi-browser"><form id="multi-create"><label>방 이름 <input name="name" value="연습방" maxlength="80" required></label>
    <label>인원 <input name="capacity" type="number" value="4" min="2" max="8" required></label><button>방 만들기</button>
    <label>비밀번호 (선택) <input name="password" type="password" maxlength="64" autocomplete="new-password" placeholder="비워 두면 공개 방"></label>
    <button type="button" data-action="refresh">새로고침</button></form><ul id="multi-rooms"></ul></section>
    <section id="multi-room" hidden><h3 id="multi-title"></h3><p id="multi-song"></p><p id="multi-policy"></p>
    <div id="multi-actions"><button type="button" data-action="start">Go</button>
    <button type="button" data-action="cancel">시작 취소</button><button type="button" data-action="reset">다음 곡 대기</button>
    <button type="button" data-action="wait">플레이 포기 · 대기실</button>
    <button type="button" data-action="leave">방 나가기</button></div>
    <table><thead><tr><th>사용자</th><th>채보</th><th>상태</th><th>점수</th></tr></thead><tbody id="multi-members"></tbody></table>
    <form id="multi-select"><label>내 채보 <select name="chart"></select></label>
    <label>Time Rate <input name="rate" type="number" min="0.5" max="2.0" step="any" value="1" required></label>
    <input name="mod" type="hidden" value=""><div class="mod-buttons" role="group" aria-label="노트 배치 · 함께 선택 가능">
    <button type="button" data-mod="mirror" aria-pressed="false">Mirror <span class="mod-state" aria-hidden="true">OFF</span></button>
    <button type="button" data-mod="random" aria-pressed="false">Random <span class="mod-state" aria-hidden="true">OFF</span></button></div>
    <input name="visibility" type="hidden" value=""><div class="mod-buttons" role="group" aria-label="Visibility · 하나만 선택">
    <button type="button" data-visibility="hidden" aria-pressed="false">Hidden</button>
    <button type="button" data-visibility="fadein" aria-pressed="false">Fade In</button>
    <button type="button" data-visibility="cover" aria-pressed="false">Cover</button>
    <button type="button" data-visibility="flashlight" aria-pressed="false">Flashlight</button></div>
    <label><input name="pitch" type="checkbox" checked>Pitch</label><button id="multi-choose">채보 선택</button></form><h4>곡 제안</h4><ul id="multi-proposals"></ul></section>`;
  document.body.append(panel);
  const joinDialog = document.createElement('dialog'); joinDialog.id = 'multi-join';
  joinDialog.setAttribute('aria-label', tr('비밀번호 방 입장'));
  joinDialog.innerHTML = tr`<form id="multi-join-form"><h2 id="multi-join-name">비밀번호 방 입장</h2>
    <label>비밀번호 <input name="password" type="password" maxlength="64" autocomplete="off" required></label>
    <p id="multi-join-status" role="status" aria-live="polite"></p>
    <button id="multi-join-submit" type="submit">입장</button> <button id="multi-join-cancel" type="button">취소</button></form>`;
  document.body.append(joinDialog);
  const joinForm = joinDialog.querySelector('form'), joinStatus = joinDialog.querySelector('#multi-join-status'), joinButton = joinDialog.querySelector('#multi-join-submit');
  const joinCancel = joinDialog.querySelector('#multi-join-cancel');
  let joinRoom = '', joinPending = '';
  const clearJoin = () => { joinForm.elements.password.value = ''; joinRoom = ''; joinPending = ''; joinStatus.textContent = ''; joinButton.disabled = joinCancel.disabled = false; };
  joinDialog.addEventListener('close', clearJoin);
  joinDialog.addEventListener('keydown', e => e.stopPropagation());
  joinCancel.onclick = () => joinDialog.close();
  joinDialog.addEventListener('cancel', e => { if (joinPending) e.preventDefault(); });
  const $ = s => panel.querySelector(s), message = text => { $('#multi-message').textContent = text; };
  const renderMods = mountMods($('#multi-select').elements.mod, [...panel.querySelectorAll('[data-mod]')]);
  const renderVisibility = mountVisibility($('#multi-select').elements.visibility, [...panel.querySelectorAll('[data-visibility]')]);
  let state = {}, cursor = 0, next = 0, have = '', launching = '', busy = false, refreshing = false;
  const chat = createChat(document.body, text => {
    const id = `lobby-chat-${++next}`;
    app().MultiSend({ type:'chat', id, text }).catch(e => chat.error(id, String(e)));
    return id;
  });
  async function send(q) { await app().MultiSend({id:`lobby-${++next}`,...q}); }
  function join(r) {
    if (!r.locked) { run(() => send({ type: 'join', room_id: r.id })); return; }
    clearJoin(); joinRoom = r.id;
    joinDialog.querySelector('#multi-join-name').textContent = tr`${r.name} 입장`;
    joinDialog.showModal(); joinForm.elements.password.focus();
  }
  joinForm.onsubmit = async e => {
    e.preventDefault(); if (joinPending) return;
    const id = `lobby-${++next}`, password = joinForm.elements.password.value;
    joinPending = id; joinButton.disabled = joinCancel.disabled = true; joinStatus.textContent = tr('입장 확인 중…'); joinForm.elements.password.value = '';
    try { await app().MultiSend({ type: 'join', id, room_id: joinRoom, password }); }
    catch (e) { if (joinPending === id) { joinPending = ''; joinButton.disabled = joinCancel.disabled = false; joinStatus.textContent = e?.message ?? String(e); } }
  };
  const button = action => $(`[data-action="${action}"]`);
  async function run(fn) { try { await fn(); await poll(); } catch(e) { console.error(e); message(e?.message ?? String(e)); } }
  function chartOptions() {
    const field = $('#multi-select').elements.chart, chosen = selection();
    field.replaceChildren(...entries().map((e,i) => {
      const o = document.createElement('option'); o.value = i;
      o.textContent = `${e.title} / ${e.difficulty} / ${e.lanes}K`; return o;
    }));
    const i = entries().findIndex(e => e.path === chosen?.chartID && e.index === chosen?.index);
    if(i >= 0) field.value = i;
    $('#multi-select').elements.rate.value = chosen?.rate ?? 1;
    $('#multi-select').elements.mod.value = chosen?.mod ?? '';
    $('#multi-select').elements.visibility.value = chosen?.visibility ?? '';
    renderMods();
    renderVisibility();
    $('#multi-select').elements.pitch.checked = chosen?.pitch !== false;
  }
  function render() {
    const r = state.room, host = r?.host_id === state.player_id;
    if (!state.connected && joinDialog.open) joinDialog.close();
    $('#multi-connect').hidden = !!state.connected;
    $('#multi-browser').hidden = !state.connected || !!r;
    $('#multi-room').hidden = !r;
    button('disconnect').disabled = !state.connected;
    chat.setPlayer(state.player_id ?? '', r?.members?.find(m=>m.id===state.player_id)?.name ?? ''); chat.setRoom(r?.id, r?.name, r?.phase, !!state.connected && !state.error, !!state.running, !!state.running && r?.phase==='playing');
    $('#multi-rooms').replaceChildren(...(state.rooms ?? []).map(r => {
      const li=document.createElement('li'), b=document.createElement('button');
      b.textContent=`${r.locked ? '🔒 ' : ''}${r.name} · ${r.players}/${r.capacity} · ${phase(r.phase)}${r.title ? ' · '+r.title : ''}`;
      b.disabled=r.players>=r.capacity; b.onclick=()=>join(r); li.append(b); return li;
    }));
    if(!r) return;
    $('#multi-title').textContent=`${r.name}${r.locked ? tr(' · 비밀번호 방') : ''} · ${phase(r.phase)}`;
    $('#multi-song').textContent=r.selection ? tr`${r.selection.title} · ${r.selection.lanes}K · ${r.selection.rate}배 · Pitch ${r.selection.pitch ? 'On' : 'Off'} · ${modsText(r.selection.mods)}` : tr('방장이 채보를 선택하세요.');
    $('#multi-policy').textContent=r.notice || tr('Go를 누르면 채보를 가진 사람들이 모두 참가합니다. 채보가 없는 사람은 대기실에 남습니다.');
    button('start').disabled=!host || r.phase!=='lobby' || !r.selection || r.members.filter(m=>m.connected).length<2 || !r.members.some(m=>m.has_chart) || busy;
    button('cancel').hidden=!host || !['preparing','arming'].includes(r.phase);
    button('reset').hidden=!host || r.phase!=='finished';
    button('wait').hidden=r.phase!=='playing' || r.members.find(m=>m.id===state.player_id)?.status!=='playing';
    $('#multi-select').hidden=r.phase!=='lobby'; $('#multi-choose').textContent=host ? tr('채보 선택') : tr('곡 제안');
    $('#multi-members').replaceChildren(...r.members.map(m=>{
      const tr=document.createElement('tr');
      for(const text of [m.name+(m.id===r.host_id?tr(' · 방장'):'')+(m.id===state.player_id?tr(' · 나'):''),m.has_chart?tr('있음'):tr('없음'),m.status==='playing'?tr('플레이 중'):m.ready?tr('로딩 완료'):status(m.status),m.participant ? m.score.Points.toLocaleString() : '—']) {
        const td=document.createElement('td');td.textContent=text;tr.append(td);
      } return tr;
    }));
    $('#multi-proposals').replaceChildren(...(r.proposals??[]).map(p=>{
      const li=document.createElement('li');li.textContent=tr`${p.name}: ${p.selection.title} · ${p.selection.lanes}K · ${p.selection.rate}배 `;
      if(host && r.phase==='lobby'){const b=document.createElement('button');b.textContent=tr('채택');b.onclick=()=>run(()=>send({type:'select',selection:p.selection}));li.append(b);}return li;
    }));
  }
  async function poll() {
    if(refreshing || !app()?.MultiState) return;
    refreshing=true;
    try {
      state=await app().MultiState(cursor); cursor=state.sequence??0; render();
      for(const {event:e} of state.events??[]){
        if (e.id === joinPending && e.type === 'ack') { joinDialog.close(); message(tr('입장했습니다.')); }
        else if (e.id === joinPending && e.type === 'error') { joinPending = ''; joinButton.disabled = joinCancel.disabled = false; joinStatus.textContent = e.error; joinForm.elements.password.focus(); }
        if(e.type==='welcome') message(tr('접속했습니다.'));
        if(e.type==='chat') chat.receive(e.message);
        if(e.type==='chat_history') chat.history(e.room_id,e.messages??[]);
        if(e.type==='ack') chat.ack(e.id);
        if(e.type==='error'){chat.error(e.id,e.error);message(e.error);}
      }
      if(state.error){message(state.error);chat.disconnected();if(joinDialog.open)joinDialog.close();}
      const r=state.room;
      if(!r){have='';launching='';return;}
      const available=entries().some(e=>e.hash===r.selection?.chart_hash);
      const key=`${r.id}:${r.revision}:${available}`;
      if(r.selection && r.phase==='lobby' && have!==key){
        have=key;
        await send({type:'have',revision:r.revision,chart_hash:r.selection.chart_hash,has_chart:available});
      }
      const me=r.members.find(m=>m.id===state.player_id);
      if(r.phase==='preparing' && me?.participant && !me.ready && launching!==r.match_id){
        launching=r.match_id; busy=true; stopPreview();
        app().MultiPlay().catch(e=>message(tr`플레이 준비 실패: ${e?.message??e}`)).finally(()=>{busy=false;});
      }
      if(r.phase==='lobby') launching='';
    } finally { refreshing=false; }
  }
  $('#multi-connect').onsubmit=e=>{e.preventDefault();run(async()=>{cursor=0;await app().MultiConnect(e.target.elements.endpoint.value,e.target.elements.name.value);message(tr('접속 중'));});};
  $('#multi-create').onsubmit=e=>{e.preventDefault();const form=e.target,password=form.elements.password.value;form.elements.password.value='';run(()=>send({type:'create',name:form.elements.name.value,capacity:form.elements.capacity.valueAsNumber,password}));};
  $('#multi-select').onsubmit=e=>{
    e.preventDefault(); const form=e.target, chart=entries()[Number(form.elements.chart.value)]; if(!chart)return;
    run(async()=>{stopPreview();message(tr('채보 확인 중'));await app().MultiChoose({folder:chart.folder,chartID:chart.path,index:chart.index,mode:'live',mod:form.elements.mod.value,visibility:form.elements.visibility.value,rate:form.elements.rate.valueAsNumber,pitch:form.elements.pitch.checked},state.room.host_id!==state.player_id);message('');});
  };
  for(const [action,type] of Object.entries({refresh:'list',start:'start',cancel:'cancel',reset:'reset',leave:'leave'})) button(action).onclick=()=>run(()=>send({type}));
  button('wait').onclick=()=>run(()=>send({type:'wait',match_id:state.room.match_id}));
  button('disconnect').onclick=()=>run(async()=>{await app().MultiDisconnect();cursor=0;have='';chat.disconnected();if(joinDialog.open)joinDialog.close();});
  button('close').onclick=()=>panel.close(); panel.addEventListener('keydown',e=>e.stopPropagation());
  panel.addEventListener('close',()=>{ $('#multi-create').elements.password.value=''; if(joinDialog.open)joinDialog.close(); });
  const timer=setInterval(()=>poll().catch(e=>{console.error(e);message(String(e));}),250);
  window.addEventListener('pagehide',()=>clearInterval(timer),{once:true});
  return {open(){
    if(!app()?.MultiConnect){message(tr('네이티브 멀티플레이는 데스크톱 앱에서 사용할 수 있어요.'));$('#multi-connect').hidden=true;$('#multi-browser').hidden=true;}
    chartOptions(); if(!panel.open)panel.show(); poll().catch(e=>message(String(e)));
  }, joined(){return !!state.room;}};
}
function phase(s){return {lobby:tr('대기실'),preparing:tr('플레이 준비 중'),arming:tr('동시 시작 예약'),playing:tr('플레이 중'),finished:tr('결과')}[s]??'';}
function status(s){return {waiting:tr('대기'),finished:tr('완주'),aborted:tr('포기 · 대기실'),left:tr('퇴장'),timeout:tr('응답 시간 초과')}[s]??s;}
