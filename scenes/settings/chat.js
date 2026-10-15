import { tr } from '../../preferences/language.js';
import { defaultChat, validateChat } from '../../preferences/chat.js';
import { wrap } from './choice.js';

let current = defaultChat(), loading;
const listeners = new Set();
const app = () => window.go?.main?.App;
export const chatOptions = () => ({...current});
export function watchChat(fn) { listeners.add(fn); return () => listeners.delete(fn); }
const notify = () => { for (const fn of listeners) fn(chatOptions()); };

export function loadChat() {
  loading ??= (async () => {
    if (typeof app()?.ChatSettings === 'function') current = validateChat(await app().ChatSettings());
    else {
      const raw = localStorage.getItem('ggosu:chat');
      current = raw ? validateChat(JSON.parse(raw)) : defaultChat();
    }
    notify();
  })().catch(e => { loading = undefined; throw e; });
  return loading.then(chatOptions);
}

export async function saveChat(value) {
  const clean = validateChat(value);
  if (typeof app()?.SaveChat === 'function') current = validateChat(await app().SaveChat(clean));
  else { localStorage.setItem('ggosu:chat', JSON.stringify(clean)); current = clean; }
  notify(); return chatOptions();
}

export function mountChatSettings(root) {
  const form = document.createElement('form'); form.className = 'chat-settings';
  // Same rows as the other settings tabs: short label + OFF/ON ShearedButtons; the longer note is hover text.
  const rows = [
    ['ticker',tr('채팅 ticker'),tr('채팅창이 접혀 있을 때 최신 메시지를 잠깐 표시합니다.')],
    ['autoHide',tr('플레이 중 자동 숨김'),tr('경기가 시작되면 접습니다. F8로 다시 열 수 있습니다.')],
    ['mentionPopup',tr('언급 팝업'),tr('닉네임이나 @닉네임을 포함한 메시지를 알립니다.')],
    ['messagePopup',tr('새 메시지 팝업'),tr('이름 언급 외의 새 메시지를 알립니다.')],
    ['mentionSound',tr('언급 알림음'),tr('효과음·마스터 볼륨을 따릅니다.')],
    ['instant',tr('플레이 중 즉시 팝업'),tr('끄면 팝업과 언급 알림음을 경기 종료까지 모아 둡니다.')],
  ];
  const fields = new Map();
  for (const [key, label, description] of rows) {
    const row = document.createElement('div'); row.className = 'settings-row'; row.title = description;
    const name = document.createElement('label'); name.textContent = label;
    const box = document.createElement('span');
    const input = document.createElement('input'); input.type = 'checkbox'; input.name = key; input.setAttribute('aria-label', label);
    box.append(input); row.append(name, box); form.append(row); wrap(input); input.disabled = true; fields.set(key,input);
  }
  const foot = document.createElement('p'), reset = document.createElement('button'); reset.type='button'; reset.textContent=tr('기본값으로');
  const status = document.createElement('span'); status.setAttribute('role','status'); foot.append(reset,status); form.append(foot); root.append(form);
  const render = prefs => { for (const [key,input] of fields) input.checked = prefs[key]; };
  const enable = value => { for (const input of fields.values()) input.disabled = !value; reset.disabled = !value; };
  watchChat(render);
  async function save(value) {
    enable(false);
    try { render(await saveChat(value)); status.textContent=tr('저장했습니다.'); }
    catch (e) { render(chatOptions()); status.textContent=tr`저장 실패: ${e?.message ?? e}`; }
    finally { enable(true); }
  }
  for (const [key,input] of fields) input.addEventListener('change', () => save({...chatOptions(),[key]:input.checked}));
  reset.onclick=()=>save(defaultChat()); form.onsubmit=e=>e.preventDefault();
  loadChat().then(prefs=>{render(prefs);enable(true);}).catch(e=>{status.textContent=tr`설정 읽기 실패: ${e?.message ?? e}`;});
  return form;
}
