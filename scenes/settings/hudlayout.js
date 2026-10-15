import { tr } from '../../preferences/language.js';
import { validateDisplay } from '/preferences/display.js';
const $ = s => document.querySelector(s);
// [hudLayout key, game name, chip label]
const items = [
    ['score','SCORE',tr('점수')],['counts','COUNTS',tr('판정 집계')],['time','TIME',tr('시간')],['combo','COMBO',tr('콤보')],['judgement','JUDGEMENT',tr('판정')],
    ['meter','TIMING',tr('오차 미터')],['health','HEALTH',tr('체력')],['song','SONG',tr('곡 정보')],['ghost','GHOST','Ghost'],['hitPosition','HIT POSITION',tr('판정선')],
];
// osu!lazer's skin editor keeps an undo state per change; input within this window (a drag, a held arrow) is one step.
const STEP_MS = 400;
const NUDGE = {ArrowLeft:[-1,0],ArrowRight:[1,0],ArrowUp:[0,-1],ArrowDown:[0,1]};
const clamp = (v,lo,hi) => Math.min(hi,Math.max(lo,v));
export function mount({current,change,save}) {
    let frame=null, geometry={}, ready=false, timer=0, lanes=7, last='',revision=0,error='', selected='score';
    const undo=[], redo=[]; let before=null, stepTimer=0;
    const selection=()=>items.find(i=>i[0]===selected);
    const geo=id=>geometry[items.find(i=>i[0]===id)[1].toLowerCase()];
    const layout=()=>JSON.stringify(current().hudLayout ?? {});
    const chips=new Map();
    for(const [id,,name] of items) {
        const b=document.createElement('button');b.type='button';b.textContent=tr(name);
        b.addEventListener('click',()=>{if(id!==selected){selected=id;send();paint();}});
        $('#hud-items').append(b);chips.set(id,b);
    }
    for(let i=1;i<=18;i++) $('#hud-preview-keys').add(new Option(`${i}K`,String(i)));
    $('#hud-preview-keys').value=String(lanes);
    function send() {
        if (!frame || !ready) return;
        const d={...current(),letterbox:false};
        const text=JSON.stringify(d); last=text;
        frame.contentWindow.postMessage({type:'gosu:display-settings',data:text,selection:selection()[1],revision:++revision},location.origin);
    }
    // Remember the layout before the first change of a step; the step closes STEP_MS after the last change.
    function remember() {
        before ??= layout();
        clearTimeout(stepTimer);stepTimer=setTimeout(settle,STEP_MS);
    }
    function settle() {
        clearTimeout(stepTimer);stepTimer=0;
        if(before!==null && before!==layout()) {undo.push(before);if(undo.length>100)undo.shift();redo.length=0;}
        before=null;paint();
    }
    function apply(d) {remember();change(validateDisplay(d));send();commitSoon();error='';paint();}
    function restore(from,to) {
        settle();
        const text=from.pop();
        if(text===undefined)return;
        to.push(layout());
        change(validateDisplay({...current(),hudLayout:JSON.parse(text)}));send();commitSoon();paint();
    }
    function nudge(dx,dy) {
        const g=geo(selected);
        if(!ready || !g?.movable) return;
        const d=structuredClone(current()); d.hudLayout ??= {};
        if(selected==='hitPosition') {
            if(!dy) return;
            g.y=clamp(g.y+12+dy,240,640)-12;
            d.hudLayout.hitPosition=g.y+12;
        } else {
            g.x=clamp(g.x+dx,0,1280-g.width);g.y=clamp(g.y+dy,0,720-g.height);
            d.hudLayout[selected]={...(d.hudLayout[selected]??{}),x:g.x-g.baseX,y:g.y-g.baseY};
        }
        apply(d);
    }
    // Same keys whether the preview or the panel has focus; fields and the tab list keep their own arrows.
    function key({key,ctrl,shift}) {
        if(ctrl) {
            const k=key.toLowerCase();
            if(k==='z' && !shift) restore(undo,redo);
            else if(k==='y' || k==='z') restore(redo,undo);
            else return false;
            return true;
        }
        if(!NUDGE[key]) return false;
        nudge(...NUDGE[key]);
        return true;
    }
    document.addEventListener('keydown',e=>{
        if(!$('#settings').open || $('#panel-hud-layout').hidden || e.isComposing || e.altKey) return;
        if(e.target.closest?.('#settings-tabs, input, select, textarea')) return;
        if(key({key:e.key,ctrl:e.ctrlKey||e.metaKey,shift:e.shiftKey})) e.preventDefault();
    });
    function paint() {
        for(const [id,b] of chips) {b.setAttribute('aria-pressed',String(id===selected));b.disabled=!ready || !geo(id)?.movable;}
        const g=geo(selected), hit=selected==='hitPosition', saved=current().hudLayout ?? {};
        for (const key of ['x','y','scale']) $('#hud-'+key+'-number').disabled=!ready || !g?.movable || hit && key!=='y';
        $('#hud-reset-item').disabled=!ready || saved[selected]===undefined;
        $('#hud-reset-all').disabled=!ready || !Object.keys(saved).length;
        $('#hud-undo').disabled=!ready || !undo.length && (before===null || before===layout());
        if (!g) return;
        const p=saved[selected] ?? {}, scale=p.scale||100;
        const values={x:g.x,y:hit?g.y+12:g.y,scale};
        for(const key of ['x','y','scale']) {
            const n=$('#hud-'+key+'-number');
            const min=key==='scale'?25:hit && key==='y'?240:0;
            const max=key==='scale'?200:hit && key==='y'?640:key==='x'?1280-g.width:720-g.height;
            n.min=String(min);n.max=String(max);
            if (document.activeElement!==n) n.value=String(values[key]);
        }
    }
    function commitSoon() {
        if(timer)return;
        timer=setTimeout(async()=>{
            timer=0;
            const ok=await save();
            if(ok===undefined)commitSoon();
            if(ok===false)error=tr('배치 저장 실패. 조절하면 다시 저장합니다.');
        },100);
    }
    function input(key,node) {
        const g=geo(selected);
        if(!g || !node.validity.valid || !Number.isFinite(node.valueAsNumber)) return;
        const d=structuredClone(current()); d.hudLayout ??= {};
        if(selected==='hitPosition') d.hudLayout.hitPosition=node.valueAsNumber;
        else {
            const p={...(d.hudLayout[selected]??{x:0,y:0})};
            if(key==='scale') p.scale=node.valueAsNumber;
            else p[key]=node.valueAsNumber-(key==='x'?g.baseX:g.baseY);
            d.hudLayout[selected]=p;
        }
        apply(d);
    }
    for(const key of ['x','y','scale']) {const node=$('#hud-'+key+'-number');node.addEventListener('input',()=>input(key,node));}
    $('#hud-reset-item').addEventListener('click',()=>{
        const d=structuredClone(current());
        if(d.hudLayout) delete d.hudLayout[selected];
        apply(d);
    });
    $('#hud-reset-all').addEventListener('click',()=>apply({...current(),hudLayout:{}}));
    $('#hud-undo').addEventListener('click',()=>restore(undo,redo));
    $('#hud-layout-form').addEventListener('submit',e=>e.preventDefault());
    $('#hud-preview-keys').addEventListener('change',()=>{lanes=Number($('#hud-preview-keys').value);stop();sync();});
    function stop() {frame?.remove();frame=null;ready=false;geometry={};paint();}
    function sync() {
        if(!$('#settings').open || $('#panel-hud-layout').hidden) {stop();return;}
        if(frame) return;
        $('#hud-layout-status').textContent=tr('미리보기 준비 중…');
        frame=document.createElement('iframe');frame.title=tr('HUD 실시간 미리보기');frame.src='/settings/hud.html';
        $('#hud-preview').append(frame);
    }
    const observer=new MutationObserver(sync);
    observer.observe($('#settings'),{attributes:true,attributeFilter:['open']});
    observer.observe($('#panel-hud-layout'),{attributes:true,attributeFilter:['hidden']});
    window.addEventListener('message',e=>{
        if(!frame || e.source!==frame.contentWindow || e.origin!==location.origin) return;
        if(e.data?.type==='gosu:preview-loaded') {
            frame.contentWindow.postMessage({type:'gosu:preview-start',display:current(),lanes},location.origin);
        }
        if(e.data?.type==='gosu:preview-error') $('#hud-layout-status').textContent=e.data.data;
        if(e.data?.type==='gosu:ready') {ready=true;send();}
        if(e.data?.type==='gosu:preview-key') {
            if(e.data.key==='Escape') $('#settings form[method="dialog"] button')?.click();
            else key(e.data);
        }
        if(e.data?.type==='gosu:hud-preview') {
            const data=JSON.parse(e.data.data);
            if(data.revision<revision)return;
            geometry=data.geometry;
            const picked=items.find(i=>i[1].toLowerCase()===data.selection);
            if(picked) selected=picked[0];
            const text=JSON.stringify(data.display);
            if(text!==last) {
                const d=validateDisplay({...current(),hudLayout:data.display.hudLayout});
                if(JSON.stringify(d.hudLayout)!==JSON.stringify(current().hudLayout)) {remember();change(d);commitSoon();}
                last=text;
            }
            $('#hud-layout-status').textContent=error;paint();
        }
    });
    paint();
    return {render(){send();paint();},setLanes(n){if(n===lanes)return;lanes=n;$('#hud-preview-keys').value=String(n);stop();sync();},sync};
}
