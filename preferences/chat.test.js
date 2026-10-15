import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultChat, validateChat, mentioned, alertFor} from './chat.js';

test('nickname mentions use word boundaries and literal punctuation', () => {
  for (const [text,name,want] of [['@mint 준비됐어?','mint',true],['MINT!','mint',true],['chocomint','mint',false],['민수야','민수',false],['@민수 같이 해요','민수',true],['@mint+ hi','mint+',true],['anything','',false]]) {
    assert.equal(mentioned(text,name),want,`${name} in ${text}`);
  }
});

test('mention and general alerts are independent and gameplay defers sound with popups', () => {
  const prefs={...defaultChat(),ticker:true,messagePopup:false,mentionSound:true};
  const input={text:'@mint 안녕',name:'mint',playing:true,own:false,visible:false};
  assert.deepEqual(alertFor(prefs,input),{ticker:true,popup:true,sound:true,defer:true});
  assert.equal(alertFor({...prefs,instant:true},input).defer,false);
  assert.equal(alertFor(prefs,{...input,text:'다음 곡은?'}).popup,false);
  assert.deepEqual(alertFor(prefs,{...input,own:true}),{ticker:false,popup:false,sound:false,defer:false});
  assert.deepEqual(alertFor(prefs,{...input,visible:true}),{ticker:false,popup:false,sound:false,defer:false});
  assert.equal(validateChat({ticker:true}).autoHide,true);
  assert.throws(()=>validateChat({instant:'true'}));
});
