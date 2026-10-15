import test from 'node:test';
import assert from 'node:assert/strict';
import { createChat } from './chat.js';
import { loadChat, saveChat } from '../settings/chat.js';
import { defaultChat } from '../../preferences/chat.js';

function setup(t) {
  const old = {document: globalThis.document, window: globalThis.window, localStorage:globalThis.localStorage};
  t.after(() => { listeners.pagehide?.(); Object.assign(globalThis, old); });
  class Node {
    constructor(tag) { this.tag = tag; this.children = []; this.attributes = {}; this.listeners = {}; this.style = {}; this.dataset = {}; this.value = ''; this.clientHeight = 100; this._scroll = 0; }
    append(...nodes) { for (const n of nodes) { const node = typeof n === 'string' ? {textContent:n} : n; node.parent = this; this.children.push(node); } }
    replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
    setAttribute(k,v) { this.attributes[k] = v; }
    addEventListener(k,fn) { this.listeners[k] = fn; }
    contains(n) { return this === n || this.children.some(c => c.contains?.(n)); }
    focus() { document.activeElement = this; }
    get scrollHeight() { return this.children.length * 20; }
    get scrollTop() { return this._scroll; }
    set scrollTop(v) { this._scroll = Math.max(0, Math.min(v, Math.max(0, this.scrollHeight-this.clientHeight))); }
    getBoundingClientRect() { const top = this.parent?.className === 'room-chat-log' ? this.parent.children.indexOf(this)*20-this.parent.scrollTop : 0; return {top,bottom:top+20}; }
    requestSubmit() { this.onsubmit?.({preventDefault(){}}); }
  }
  const body = new Node('body'), nodes = [], list = new Node('main'), sent = [], listeners = {};
  globalThis.document = {body, activeElement:body, getElementById: id => id === 'songselect' ? list : null, createElement(tag) { const n = new Node(tag); nodes.push(n); return n; }};
  globalThis.window = {innerWidth:1280, addEventListener(k,fn) { listeners[k] = fn; }};
  const storage=new Map(); globalThis.localStorage={getItem:key=>storage.get(key)??null,setItem:(key,value)=>storage.set(key,value)};
  const api = createChat(body, text => { sent.push(text); return `send-${sent.length}`; });
  const node = name => nodes.find(n => n.className === `room-chat-${name}`);
  api.setPlayer('me'); api.setRoom('r','방','lobby');
  const draft = text => { const input = nodes.find(n=>n.tag==='textarea'); input.value=text; input.oninput(); return input; };
  const receive = (id, text='내용') => api.receive({id,room_id:'r',player_id:'other',name:'친구',time:'2026-10-14T10:00:00Z',text});
  return {api,node,list,sent,draft,receive, input:nodes.find(n=>n.tag==='textarea'),submit:nodes.find(n=>n.type==='submit'),listeners};
}

test('draft, unread messages and previous reading position survive drawer and phase changes', t => {
  const f = setup(t); f.draft('작성 중'); f.receive(1); f.api.hide(); f.receive(2);
  assert.match(f.node('toggle').textContent, /새 메시지 1/);
  f.api.setRoom('r','방','playing'); assert.equal(f.node('panel').hidden,true);
  f.api.show(); assert.equal(f.input.value,'작성 중');
  assert.equal(f.list.style.marginRight,'376px');
  for(let id=3;id<=100;id++) f.receive(id);
  const log = f.node('log'); log.scrollTop=1000; log.onscroll();
  const anchor = log.children[50].dataset.id;
  f.receive(101); assert.equal(log.children.length,100);
  assert.equal(log.children[49].dataset.id,anchor); assert.equal(log.scrollTop,980);
  f.api.hide(); f.api.show(); assert.equal(log.scrollTop,980);
  f.node('new').onclick(); assert.equal(log.scrollTop,log.scrollHeight-log.clientHeight);
  assert.equal(f.node('new').hidden,true);
});

test('ack clears only the submitted draft, room drafts stay separate and disconnection blocks sending', t => {
  const f=setup(t); f.draft('첫 메시지'); f.node('compose').requestSubmit();
  assert.deepEqual(f.sent,['첫 메시지']); assert.equal(f.input.value,'첫 메시지');
  f.draft('다음 초안'); f.api.ack('send-1'); assert.equal(f.input.value,'다음 초안');
  f.api.setRoom('s','다른 방','lobby'); assert.equal(f.input.value,'');
  f.draft('다른 초안'); f.api.setRoom('r','방','finished'); assert.equal(f.input.value,'다음 초안');
  f.api.disconnected(); f.api.hide(); f.api.show(); assert.equal(f.input.disabled,true);
  f.node('compose').requestSubmit(); assert.equal(f.sent.length,1);
  f.api.setRoom('r','방','lobby',false); assert.equal(f.submit.disabled,true);
  f.api.setRoom('r','방','lobby',true); assert.equal(f.input.disabled,false);
  f.node('compose').requestSubmit(); f.api.error('send-2','잠시 기다려 주세요');
  assert.equal(f.input.value,'다음 초안'); assert.match(f.node('status').textContent,/기다려/);
});

test('emoji use the server rune limit and IME Enter does not send', t => {
  const f=setup(t); f.draft('🎵'.repeat(301));
  assert.equal([...f.input.value].length,300); assert.equal(f.node('count').textContent,'300 / 300');
  f.input.listeners.compositionstart(); f.input.listeners.keydown({key:'Enter',preventDefault(){},isComposing:true});
  assert.equal(f.sent.length,0);
  f.input.listeners.compositionend(); f.input.listeners.keydown({key:'Enter',preventDefault(){}});
  assert.equal(f.sent.length,1);
});

test('ticker and popups follow settings, auto-hide permits F8 reopen and deferred alerts flush only once', async t => {
  const f=setup(t); await loadChat();
  await saveChat({...defaultChat(),ticker:true});
  f.api.hide(); f.receive(1);
  assert.equal(f.node('ticker').hidden,false); assert.equal(f.node('popup').hidden,false);
  f.api.setRoom('r','방','playing'); f.receive(2);
  f.api.setRoom('r','방','finished'); assert.match(f.node('popup').textContent,/친구/);
  f.node('popup').onclick(); assert.equal(f.node('panel').hidden,false); assert.equal(f.node('popup').hidden,true);
  f.api.setRoom('r','방','playing'); assert.equal(f.node('panel').hidden,true);
  f.api.show(); f.api.setRoom('r','방','playing'); assert.equal(f.node('panel').hidden,false);
  await saveChat({...defaultChat(),messagePopup:false,mentionPopup:false});
  f.api.hide(); f.receive(3); assert.equal(f.node('popup').hidden,true); assert.equal(f.node('ticker').hidden,true);
  await saveChat(defaultChat());
});
