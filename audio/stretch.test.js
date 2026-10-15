import test from 'node:test';
import assert from 'node:assert/strict';
import { stretch } from './stretch.js';

export function buffer(channels, length, sampleRate = 48000) {
    const data = Array.from({length:channels}, () => new Float32Array(length));
    return {length, numberOfChannels:channels, sampleRate, duration:length/sampleRate, getChannelData:ch=>data[ch]};
}
test('pitch-preserving stretch keeps frequency, stereo phase and exact requested duration', () => {
    const b = buffer(2,48000), ctx = {createBuffer:buffer};
    for(let i=0;i<b.length;i++) { const v=Math.sin(2*Math.PI*220*i/b.sampleRate)*.5;b.getChannelData(0)[i]=v;b.getChannelData(1)[i]=-v; }
    for(const rate of [.5,.75,1,1.5,2]) {
        const out=stretch(b,rate,ctx), left=out.getChannelData(0),right=out.getChannelData(1);
        assert.equal(out.length,Math.round(b.length/rate));
        const from=Math.floor(out.length/10),to=Math.floor(out.length*.9);let crossings=0;
        for(let i=from+1;i<to;i++) {if(left[i-1]<=0&&left[i]>0)crossings++;assert.ok(Math.abs(left[i]+right[i])<1e-6);}
        assert.ok(Math.abs(crossings*out.sampleRate/(to-from)-220)<6, `rate ${rate}`);
    }
    assert.equal(stretch(b,1,ctx),b);
});

test('stretch rejects rates outside 0.5 to 2.0', () => {
    const b = buffer(2, 480), ctx = { createBuffer: buffer };
    for (const rate of [0, .25, .49, 2.01, 4, NaN, Infinity]) {
        assert.throws(() => stretch(b, rate, ctx), /0\.5–2\.0/);
    }
});
