import test from 'node:test';
import assert from 'node:assert/strict';
import { Preview, clips, segment } from './preview.js';

const ns = s => s * 1e9;
const buffer = (numberOfChannels, length, sampleRate = 48000) => {
    const data = Array.from({length:numberOfChannels}, () => new Float32Array(length));
    return {numberOfChannels,length,sampleRate,duration:length/sampleRate,getChannelData:ch=>data[ch]};
};
test('BGM preview seeks into an earlier voice and plays to the end of the song', () => {
    const buffer = { duration: 90 }, buffers = new Map([['music', buffer]]);
    const events = [{ asset: 'music', at: 0, volume: 1 }, { asset: 'music', at: 35, volume: .5 }];
    const p = segment(events, buffers, 30);
    assert.equal(p.from, 30); assert.equal(p.duration, 95);
    assert.deepEqual(p.voices.map(v => [v.offset, v.delay, v.length]), [[30, 0, 60], [0, 5, 90]]);
    assert.deepEqual(segment(events.slice(0, 1), new Map([['music', { duration: 8 }]]), 100).voices.map(v => [v.offset, v.length]), [[0, 8]]);
});

test('keysound previews include BGM and note starts but not tails, excluding fake/mine and silent sounds', () => {
    const c = { source: { format: 'ojn' }, assets: { music: { kind: 'audio' }, key: { kind: 'bank' } }, audio: [{ at_ns: 0, sound: { asset: 'music', volume: 1 } }], notes: [
        { kind: 'hold', at_ns: ns(1), end_ns: ns(2), sounds: [{ asset: 'key', volume: .8 }], tail_sounds: [{ asset: 'key', volume: .5 }] },
        { kind: 'fake', at_ns: 0, sounds: [{ asset: 'key', volume: 1 }] },
        { kind: 'mine', at_ns: 0, sounds: [{ asset: 'key', volume: 1 }] },
        { kind: 'tap', at_ns: 0, sounds: [{ asset: 'key', volume: 0 }] },
    ] };
    assert.deepEqual(clips(c).map(e => [e.at, e.volume]), [[0, 1], [1, .8]]);
    c.source.format = 'osu'; assert.equal(clips(c).length, 1);
});

test('edited keysound charts preview notes while osu and SM copies keep BGM only', () => {
    const c = { source: { format: 'ggc' }, assets: {
        music: { kind: 'audio' }, key: { kind: 'bank', sample: 1 }, WAV01: { kind: 'audio' }, skin: { kind: 'audio', path: 'normal-hitnormal.wav' },
    }, audio: [{ sound: { asset: 'music', volume: 1 } }], notes: [
        { kind: 'tap', at_ns: ns(1), sounds: [{ asset: 'key', volume: 1 }, { asset: 'WAV01', volume: .5 }, { asset: 'skin', volume: 1 }] },
        { kind: 'fake', at_ns: ns(2), sounds: [{ asset: 'key', volume: 1 }] },
        { kind: 'mine', at_ns: ns(3), sounds: [{ asset: 'key', volume: 1 }] },
    ] };
    // Older saved OJN/BMS copies still work without the new provenance field.
    assert.deepEqual(clips(c).map(e => e.asset), ['music', 'key', 'WAV01']);
    for (const format of ['ojn', 'bms', 'bme', 'bml']) {
        c.source.originalFormat = format;
        const edited = clips(c);
        c.source = { format };
        assert.deepEqual(edited, clips(c));
        c.source = { format: 'ggc' };
    }
    for (const format of ['osu', 'sm']) {
        c.source.originalFormat = format;
        assert.deepEqual(clips(c).map(e => e.asset), ['music']);
    }
});

const context = () => {
    const param = { value: 1, setValueAtTime() {}, linearRampToValueAtTime() {}, setTargetAtTime() {}, cancelScheduledValues() {}, setValueCurveAtTime() {} };
    const node = () => ({ connect() {}, disconnect() {}, gain: param, pan: {} });
    const ctx = { state: 'running', currentTime: 0, createGain: node, createStereoPanner: node, decodeAudioData: async () => ({ duration: 8, length: 8000, numberOfChannels: 1 }), started: [], stopped: 0 };
    ctx.createBufferSource = () => ({ ...node(), playbackRate: { value: 1 }, start(...args) { ctx.started.push(args); }, stop() { ctx.stopped++; } });
    return ctx;
};
const c = { metadata: { preview_ns: ns(2) }, assets: { music: { kind: 'audio' } }, audio: [{ sound: { asset: 'music', volume: 1 } }] };

test('same referenced music keeps voices, cursor, gain and loop across difficulties and preview points', async t => {
    t.mock.timers.enable({ apis: ['setTimeout'] });
    let fetches = 0;
    t.mock.method(globalThis, 'fetch', async () => { fetches++; return { ok: true, arrayBuffer: async () => new ArrayBuffer(1) }; });
    const charts = {
        easy: { ...c, assets: { music: { kind: 'audio', path: 'music.mp3' } } },
        hard: { ...c, metadata: { preview_ns: ns(5) }, assets: { bgm: { kind: 'audio', path: '../music.mp3' } }, audio: [{ sound: { asset: 'bgm', volume: 1 } }] },
    };
    const ctx = context(), p = new Preview({ chart: async s => charts[s.chartID], url: () => '/audio', notify() {}, context: () => ctx });
    p.select({ path: 'easy', folder: 'songs/track', index: 0 });
    t.mock.timers.tick(200); await new Promise(setImmediate);
    p.setVolume(25); ctx.currentTime = 1.03;
    const ready = p.ready, gain = p.gain, sources = p.sources, loop = p.loop, started = p.started;
    p.select({ path: 'hard', folder: 'songs\\track\\charts', index: 1, hash: 'different' });
    assert.equal(ctx.stopped, 0);
    t.mock.timers.tick(200); await new Promise(setImmediate);
    assert.equal(p.ready, ready); assert.equal(p.gain, gain); assert.equal(p.sources, sources);
    assert.equal(p.loop, loop); assert.equal(p.started, started); assert.equal(p.volume, .25);
    assert.equal(ctx.started.length, 1); assert.equal(ctx.stopped, 0); assert.equal(fetches, 1);
    // The original loop must still run after a selection request changes.
    t.mock.timers.tick(5830);
    assert.equal(ctx.started.length, 2); assert.equal(ctx.stopped, 1); assert.equal(fetches, 1);
    p.stop();
});

test('matching titles or filenames do not keep playback for different referenced files', async t => {
    t.mock.timers.enable({ apis: ['setTimeout'] });
    let fetches = 0;
    t.mock.method(globalThis, 'fetch', async () => { fetches++; return { ok: true, arrayBuffer: async () => new ArrayBuffer(1) }; });
    const ctx = context(), p = new Preview({
        chart: async s => ({ ...c, metadata: { title: 'Same title' }, assets: { music: { kind: 'audio', path: s.chartID === 'alternate' ? 'alternate.mp3' : 'music.mp3' } } }),
        url: () => '/audio', notify() {}, context: () => ctx,
    });
    for (const entry of [{ path: 'one', folder: 'first' }, { path: 'two', folder: 'second' }, { path: 'alternate', folder: 'second' }]) {
        p.select(entry); t.mock.timers.tick(200); await new Promise(setImmediate);
    }
    t.mock.timers.tick(170);
    assert.equal(fetches, 3); assert.equal(ctx.started.length, 3); assert.equal(ctx.stopped, 2);
    p.stop();
});

test('changed keysound timing or bank sample switches the preview even with the same bank', async t => {
    t.mock.timers.enable({ apis: ['setTimeout'] });
    t.mock.method(globalThis, 'fetch', async () => ({ ok: true, arrayBuffer: async () => new ArrayBuffer(1) }));
    const ctx = context(), p = new Preview({
        chart: async s => ({ source: { format: 'ojn' }, assets: { key: { kind: 'bank', path: 'song.ojm', sample: s.index === 2 ? 2 : 1 } },
            notes: [{ kind: 'tap', at_ns: ns(s.index === 1 ? 1 : 0), sounds: [{ asset: 'key', volume: 1 }] }] }),
        url: () => '/audio', notify() {}, context: () => ctx,
    });
    for (const index of [0, 1, 2]) {
        p.select({ path: 'song', folder: 'track', index });
        t.mock.timers.tick(200); await new Promise(setImmediate);
    }
    assert.equal(ctx.started.length, 3);
    p.stop();
});

test('a stale chart response cannot interrupt the retained song after a newer selection', async t => {
    t.mock.timers.enable({ apis: ['setTimeout'] });
    t.mock.method(globalThis, 'fetch', async () => ({ ok: true, arrayBuffer: async () => new ArrayBuffer(1) }));
    const music = { ...c, assets: { music: { kind: 'audio', path: 'music.mp3' } } };
    let resolve;
    const ctx = context(), p = new Preview({ chart: s => s.chartID === 'slow' ? new Promise(r => resolve = r) : Promise.resolve(music), url: () => '/audio', notify() {}, context: () => ctx });
    p.select({ path: 'one', folder: 'track' });
    t.mock.timers.tick(200); await new Promise(setImmediate);
    const ready = p.ready;
    p.select({ path: 'slow', folder: 'other' }); t.mock.timers.tick(200);
    p.select({ path: 'hard', folder: 'track' }); t.mock.timers.tick(200); await new Promise(setImmediate);
    resolve(music); await new Promise(setImmediate);
    assert.equal(p.ready, ready); assert.equal(ctx.started.length, 1); assert.equal(ctx.stopped, 0);
    p.select(null); t.mock.timers.tick(170);
    assert.equal(ctx.stopped, 1); assert.equal(p.ready, null);
});

test('switching or leaving while an old chart loads cannot resurrect its audio', async t => {
    t.mock.timers.enable({ apis: ['setTimeout'] });
    const ctx = context(); let resolve, calls = 0;
    const p = new Preview({ chart: () => { calls++; return new Promise(r => resolve = r); }, url: () => '/audio', notify() {}, context: () => ctx });
    p.select({ path: 'old', index: 0 }); t.mock.timers.tick(200);
    p.select(null); resolve(c); await Promise.resolve();
    assert.equal(ctx.started.length, 0); assert.equal(p.ready, null);
    p.select({ path: 'new', index: 0 }); t.mock.timers.tick(199);
    assert.equal(calls, 1); p.stop();
});

test('preview stops every scheduled voice and loop on play/edit; unchanged selection keeps playing', async t => {
    t.mock.timers.enable({ apis: ['setTimeout'] });
    t.mock.method(globalThis, 'fetch', async () => ({ ok: true, arrayBuffer: async () => new ArrayBuffer(1) }));
    const ctx = context(), notices = [];
    const p = new Preview({ chart: async () => c, url: () => '/audio', notify: s => notices.push(s), context: () => ctx });
    p.key = JSON.stringify(['one', 0, undefined]);
    await p.load({ path: 'one', index: 0 }, p.ticket);
    assert.deepEqual(ctx.started[0], [.03, 2, 6]);
    p.select({ path: 'one', index: 0 }); assert.equal(ctx.stopped, 0);
    p.setVolume(25); assert.equal(p.volume, .25);
    // Leaving fades out over 150 ms before the voices stop.
    p.select(null); assert.equal(ctx.stopped, 0);
    t.mock.timers.tick(170); assert.equal(ctx.stopped, 1);
    t.mock.timers.tick(20000); assert.equal(ctx.started.length, 1);
    assert.match(notices.at(-1), /2.0–8.0/);
});

test('suspended autoplay waits for a gesture and missing audio is reported without starting', async t => {
    t.mock.method(globalThis, 'fetch', async () => ({ ok: false, status: 404, text: async () => 'file unavailable' }));
    const ctx = context(), notices = [];
    const p = new Preview({ chart: async () => c, url: () => '/audio', notify: s => notices.push(s), context: () => ctx });
    await p.load({ path: 'one', index: 0 }, 0);
    assert.match(notices.at(-1), /HTTP 404/); assert.equal(ctx.started.length, 0);
    ctx.state = 'suspended'; p.ready = segment(clips(c), new Map([['music', { duration: 8 }]]), 2);
    p.play(0); assert.match(notices.at(-1), /목록을 클릭/); assert.equal(ctx.started.length, 0);
    p.stop();
});

test('silent bank slots do not reach the decoder or report an audio failure', async t => {
    t.mock.timers.enable({ apis: ['setTimeout'] });
    t.mock.method(globalThis, 'fetch', async url => url === 'empty'
        ? { ok: true, status: 204, arrayBuffer() { throw new Error('silent slot was decoded'); } }
        : { ok: true, arrayBuffer: async () => new ArrayBuffer(1) });
    const ctx = context(), notices = [];
    const p = new Preview({ chart: async () => ({ source: { format: 'ojn' }, assets: {
        empty: { kind: 'bank', path: 'music.ojm', sample: 1000 }, music: { kind: 'bank', path: 'music.ojm', sample: 1 },
    }, audio: [{ sound: { asset: 'empty', volume: 1 } }, { sound: { asset: 'music', volume: 1 } }] }),
    url: (s, id) => id, notify: s => notices.push(s), context: () => ctx });
    await p.load({ path: 'one' }, 0);
    assert.equal(ctx.started.length, 1);
    assert.doesNotMatch(notices.at(-1), /실패/);
    p.stop();
});

test('partial audio failures retain working voices and identify the file, sample and server reason', async t => {
    t.mock.timers.enable({ apis: ['setTimeout'] });
    t.mock.method(globalThis, 'fetch', async url => url === 'broken'
        ? { ok: false, status: 422, text: async () => 'truncated M30 sample' }
        : { ok: true, arrayBuffer: async () => new ArrayBuffer(1) });
    const ctx = context(), notices = [];
    const p = new Preview({ chart: async () => ({ source: { format: 'ojn' }, assets: {
        broken: { kind: 'bank', path: 'track.ojm', sample: 42 }, music: { kind: 'bank', path: 'track.ojm', sample: 1 },
    }, audio: [{ sound: { asset: 'broken', volume: 1 } }, { sound: { asset: 'music', volume: 1 } }] }),
    url: (s, id) => id, notify: s => notices.push(s), context: () => ctx });
    await p.load({ path: 'one' }, 0);
    assert.equal(ctx.started.length, 1);
    assert.match(notices.at(-1), /오디오 1개 실패: track\.ojm \(샘플 42\): HTTP 422: truncated M30 sample/);
    p.stop();
});

test('playback rate scales voice delays and loop duration, leaving song offsets unchanged', t => {
    t.mock.timers.enable({ apis: ['setTimeout'] });
    for (const rate of [.5, .75, 1, 1.5, 2]) {
        const ctx = context();
        const p = new Preview({ notify() {}, context: () => ctx });
        p.context = ctx;
        p.ready = segment([{ at: 0, asset: 'music', volume: 1 }, { at: 4, asset: 'key', volume: 1 }], new Map([['music', { duration: 8 }], ['key', { duration: 1 }]]), 2);
        p.setRate(rate); p.play(0);
        assert.deepEqual(ctx.started, [[.03, 2, 6], [.03 + 2 / rate, 0, 1]]);
        assert.ok(p.sources.every(s => s.playbackRate.value === rate));
        t.mock.timers.tick(6 / rate * 1000 + 29); assert.equal(ctx.started.length, 2);
        t.mock.timers.tick(1); assert.equal(ctx.started.length, 4);
        p.stop();
    }
});

test('changing rate preserves the current song cursor and reschedules future keysounds', t => {
    t.mock.timers.enable({ apis: ['setTimeout'] });
    const ctx = context(), p = new Preview({ notify() {}, context: () => ctx });
    p.context = ctx;
    p.ready = segment([{ at: 0, asset: 'music', volume: 1 }, { at: 4, asset: 'key', volume: 1 }], new Map([['music', { duration: 8 }], ['key', { duration: 1 }]]), 2);
    p.play(0); ctx.currentTime = 1.03;
    p.setRate(2);
    assert.equal(ctx.stopped, 2);
    assert.deepEqual(ctx.started.slice(2), [[1.06, 3, 5], [1.56, 0, 1]]);
    assert.equal(p.position, 1); assert.equal(p.end, 3.56);
    for (const invalid of [NaN, 0, .25, .49, 2.01, 4, Infinity]) p.setRate(invalid);
    assert.equal(p.rate, 2); assert.equal(ctx.started.length, 4);
    p.stop();
});

test('Pitch Off preserves source frequency while offsets, keysounds and loop follow the rate', t => {
    t.mock.timers.enable({apis:['setTimeout']});
    const ctx=context(), p=new Preview({notify(){},context:()=>ctx});
    ctx.createBuffer=buffer; p.context=ctx;
    const music=buffer(2,48000*8), key=buffer(2,48000);
    p.ready=segment([{at:0,asset:'music',volume:1},{at:4,asset:'key',volume:1}],new Map([['music',music],['key',key]]),2);
    p.setPlayback(2,false);p.play(0);
    assert.deepEqual(ctx.started,[[.03,1,3],[1.03,0,.5]]);
    assert.ok(p.sources.every(s=>s.playbackRate.value===1));
    assert.equal(p.sources[0].buffer.duration,4);
    ctx.currentTime=1.03; p.setPlayback(2,true);
    assert.equal(p.position,2);
    assert.deepEqual(ctx.started.slice(2),[[1.06,4,4],[1.06,0,1]]);
    assert.ok(p.sources.every(s=>s.playbackRate.value===2));
    p.stop();
});

test('progress reports the song-timeline cursor while playing and null when stopped', t => {
    t.mock.timers.enable({apis:['setTimeout']});
    const ctx=context(), p=new Preview({notify(){},context:()=>ctx});
    p.context=ctx;
    assert.equal(p.progress(), null);
    p.ready=segment([{at:0,asset:'music',volume:1}],new Map([['music',{duration:8}]]),2);
    p.play(0);
    assert.deepEqual(p.progress(), {at:2,end:8});
    ctx.currentTime=1.03; assert.deepEqual(p.progress(), {at:3,end:8});
    p.setRate(2); ctx.currentTime=2.06; assert.deepEqual(p.progress(), {at:5,end:8});
    p.stop(); assert.equal(p.progress(), null);
});

test('long keysound previews create voices a few seconds ahead instead of all at once', t => {
    t.mock.timers.enable({apis:['setTimeout']});
    const ctx=context(), p=new Preview({notify(){},context:()=>ctx});
    p.context=ctx;
    const events=Array.from({length:60},(_,i)=>({at:i,asset:'key',volume:1}));
    p.ready=segment(events,new Map([['key',{duration:.5}]]),0);
    p.play(0);
    assert.equal(ctx.started.length,5);
    ctx.currentTime=10; t.mock.timers.tick(1000);
    assert.equal(ctx.started.length,15);
    p.stop(); ctx.currentTime=100; t.mock.timers.tick(5000);
    assert.equal(ctx.started.length,15);
});
