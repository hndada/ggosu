import { tr } from '../../preferences/language.js';
let started = false;
window.addEventListener('message', async e => {
    if (e.source !== parent || e.origin !== location.origin || e.data?.type !== 'gosu:preview-start' || started) return;
    started = true;
    try {
        const lanes = Math.max(1, Math.min(18, Number(e.data.lanes) || 7));
        const notes = [];
        for (let i = 0; i < 24; i++) {
            const at = (i*.18+.5)*1e9;
            notes.push({at_ns:Math.round(at),end_ns:Math.round(at),lane:i%lanes,kind:'tap'});
        }
        // Space the fixture by lane so long notes never overlap another head.
        for (let k=0;k<Math.min(3,lanes);k++) notes.push({at_ns:5000000000,end_ns:6000000000,lane:k,kind:'hold'});
        const chart = {version:1,lanes,metadata:{title:'HUD PREVIEW',difficulty:'PREVIEW'},timing:[{at_ns:0,bpm:60,scroll:1,meter:4}],notes:notes.sort((a,b)=>a.at_ns-b.at_ns),assets:{}};
        const display = {...e.data.display,letterbox:false};
        window.gosuConfig = {chart:JSON.stringify(chart),assets:{},mode:'hud',preview:true,speed:1,pitch:true,health:3,display:JSON.stringify(display),audio:JSON.stringify({master:0,music:0,keys:0,effects:0})};
        const go = new Go();
        const wasm = await WebAssembly.instantiateStreaming(fetch('/game.wasm'),go.importObject);
        await go.run(wasm.instance);
    } catch (err) {
        document.querySelector('#preview-error').hidden=false;
        document.querySelector('#preview-error').textContent=tr`미리보기 실행 실패: ${err.message ?? err}`;
        parent.postMessage({type:'gosu:preview-error',data:String(err.message ?? err)},location.origin);
    }
});
// The editor's keys act the same with the preview focused: arrows nudge, Ctrl+Z / Ctrl+Y undo and redo, Esc closes.
window.addEventListener('keydown', e => {
    if (e.altKey) return;
    const undo = (e.ctrlKey || e.metaKey) && ['z','y'].includes(e.key.toLowerCase());
    if (!undo && !['ArrowLeft','ArrowRight','ArrowUp','ArrowDown','Escape'].includes(e.key)) return;
    e.preventDefault();
    parent.postMessage({type:'gosu:preview-key',key:e.key,ctrl:e.ctrlKey||e.metaKey,shift:e.shiftKey},location.origin);
}, true);
parent.postMessage({type:'gosu:preview-loaded'},location.origin);
