import { tr } from '../preferences/language.js';
import { stretch } from './stretch.js';
import { grid } from './beat.js';
const seconds = ns => (ns || 0) / 1e9;
// osu!lazer MusicController.changeTrack: the new track waits 30 ms then fades in over 220 ms;
// the old one fades out over 150 ms with Easing.Out.
const fadeInDelay = .03, fadeIn = .22, fadeOut = .15;
// O2Jam/BMS previews hold thousands of keysounds; creating every node at once stalls the song list.
// Voices are created in batches a few seconds ahead of the audio clock instead.
const lookahead = 5, pumpMS = 1000;

// BMS/O2Jam melodies include playable keysounds, including their edited copies.
export function clips(c) {
    const events = (c.audio || []).map(a => ({ at: seconds(a.at_ns), ...a.sound }));
    const format = c.source?.originalFormat || c.source?.format;
    if (['bms', 'bme', 'bml', 'ojn', 'ggc'].includes(format)) {
        for (const n of c.notes || []) {
            if (['fake', 'mine'].includes(n.kind)) continue;
            for (const s of n.sounds || []) {
                // Older .ggc copies lack provenance. Banks and BMS WAV IDs identify
                // their keysounds without including osu! skin hitsounds.
                if (format === 'ggc' && c.assets?.[s.asset]?.kind !== 'bank' && !/^WAV[A-Z0-9]{2}$/i.test(s.asset)) continue;
                events.push({ at: seconds(n.at_ns), ...s });
            }
        }
    }
    return events.filter(e => e.volume > 0 && ['audio', 'bank'].includes(c.assets?.[e.asset]?.kind)).sort((a, b) => a.at - b.at);
}

// Play from the preview point to the end of the song, including music that began earlier.
export function segment(events, buffers, requested) {
    const end = Math.max(0, ...events.map(e => e.at + (buffers.get(e.asset)?.duration || 0)));
    const from = requested >= 0 && requested < end ? requested : 0;
    const duration = end - from;
    const voices = events.flatMap(e => {
        const buffer = buffers.get(e.asset);
        if (!buffer) return [];
        const offset = Math.max(0, from - e.at), delay = Math.max(0, e.at - from);
        const length = Math.min(buffer.duration - offset, duration - delay);
        return length > 0 ? [{ ...e, buffer, offset, delay, length }] : [];
    });
    return { from, duration, voices };
}

// Compare referenced files and their audio events, independent of chart/asset IDs.
function audioKey(entry, c, events) {
    if (!events.length || events.some(e => !c.assets[e.asset].path)) return '';
    return JSON.stringify(events.map(e => {
        const a = c.assets[e.asset];
        const path = `${entry.folder || ''}/${a.path}`.replaceAll('\\', '/');
        const file = new URL('https://preview.invalid/' + path.split('/').map(encodeURIComponent).join('/')).pathname;
        return JSON.stringify([file, a.kind, a.sample || 0, e.at, e.volume, e.pan || 0]);
    }).sort());
}

export class Preview {
    constructor({ chart, url, notify, context = () => new AudioContext() }) {
        this.chart = chart; this.url = url; this.notify = notify; this.newContext = context;
        this.key = ''; this.ticket = 0; this.request = 0; this.sources = []; this.volume = 1; this.rate = 1; this.pitch = true;
        this.stretched = new Map(); this.stretchBytes = 0;
    }
    unlock() {
        try {
            this.context ||= this.newContext();
            if (this.context.state === 'running') return;
            this.context.resume().then(() => {
                if (this.context.state === 'running' && this.ready && !this.sources.length) this.play(this.ticket, 0, true);
            }).catch(e => this.notify(tr`프리뷰 재생 실패: ${e.message}`));
        } catch (e) { this.notify(tr`프리뷰 재생 실패: ${e.message}`); }
    }
    setVolume(percent) {
        const volume = Math.max(0, Math.min(1, percent / 100));
        if (volume === this.volume) return;
        this.volume = volume;
        if (this.gain) {
            const now = this.context.currentTime, gain = this.gain.gain;
            gain.cancelScheduledValues(now);
            gain.setTargetAtTime(volume, now, .03);
        }
    }
    setRate(rate) {
        this.setPlayback(rate, this.pitch);
    }
    setPlayback(rate, pitch) {
        if (!Number.isFinite(rate) || rate < .5 || rate > 2 || rate === this.rate && pitch === this.pitch) return;
        // Keep the cursor on the original song timeline when the speed changes.
        const position = this.gain ? this.cursor() : 0;
        this.rate = rate;
        this.pitch = pitch;
        this.stretched.clear(); this.stretchBytes = 0;
        if (this.gain) {
            try { this.play(this.ticket, position < this.ready.duration ? position : 0); }
            catch (e) { this.stopVoices(); this.notify(tr`프리뷰 재생 실패: ${e.message}`); }
        }
    }
    // Seconds into the preview segment while voices play.
    cursor() {
        return Math.min(this.ready.duration, this.position + Math.max(0, this.context.currentTime - this.started) * this.rate);
    }
    // Song-timeline seconds of the playing preview, or null while nothing plays.
    progress() {
        if (!this.gain || !this.ready) return null;
        return { at: this.ready.from + this.cursor(), end: this.ready.from + this.ready.duration };
    }
    // Song-timeline milliseconds, playback rate and beat grid of the playing preview, or null.
    beat() {
        const p = this.progress();
        return p && { time: p.at * 1000, rate: this.rate, points: this.ready.beats };
    }
    select(entry) {
        const key = entry ? JSON.stringify([entry.path, entry.index, entry.hash]) : '';
        if (key === this.key) return;
        if (!entry) { this.stop(); return; }
        clearTimeout(this.timer); this.abort?.abort(); this.key = key;
        const ticket = this.ticket, request = ++this.request;
        if (!this.ready) this.notify(tr('읽는 중'));
        this.timer = setTimeout(() => this.load(entry, ticket, request), 200);
    }
    stopVoices(fade = 0) {
        clearTimeout(this.loop); clearTimeout(this.pump);
        const sources = this.sources, gain = this.gain;
        this.sources = []; this.gain = null;
        if (!gain) return;
        const end = () => { for (const source of sources) source.stop(); gain.disconnect(); };
        if (!fade || this.context.state !== 'running') { end(); return; }
        // Easing.Out is quadratic: 1 - (1 - t)^2 of the way to silence.
        const now = this.context.currentTime, from = gain.gain.value;
        const curve = Float32Array.from({ length: 16 }, (_, i) => from * (1 - i / 15) ** 2);
        gain.gain.cancelScheduledValues(now);
        gain.gain.setValueCurveAtTime(curve, now, fade);
        setTimeout(end, (fade + .02) * 1000);
    }
    stop() {
        ++this.ticket; ++this.request; clearTimeout(this.timer); this.abort?.abort(); this.stopVoices(fadeOut);
        this.ready = null; this.key = '';
        this.stretched.clear(); this.stretchBytes = 0;
    }
    async load(entry, ticket, request = this.request) {
        const abort = new AbortController(); this.abort = abort;
        const current = () => ticket === this.ticket && request === this.request;
        const s = { chartID: entry.path, folder: entry.folder, index: entry.index };
        try {
            const c = await this.chart(s);
            if (!current()) return;
            const events = clips(c), key = audioKey(entry, c, events);
            if (key && key === this.ready?.audioKey) return;
            this.stopVoices(fadeOut); this.ready = null;
            this.stretched.clear(); this.stretchBytes = 0;
            this.notify(tr('읽는 중'));
            this.context ||= this.newContext();
            const from = Math.max(0, seconds(c.metadata?.preview_ns));
            const ids = [...new Set(events.map(e => e.asset))], buffers = new Map();
            let next = 0, bytes = 0;
            const failures = [];
            await Promise.all(Array.from({ length: Math.min(4, ids.length) }, async () => {
                while (next < ids.length && !abort.signal.aborted) {
                    const id = ids[next++];
                    try {
                        const response = await fetch(this.url(s, id), { signal: abort.signal });
                        // Sparse O2Jam bank slots are silent, with no encoded audio to decode.
                        if (response.status === 204) continue;
                        if (!response.ok) {
                            const reason = (await response.text()).trim();
                            throw new Error(`HTTP ${response.status}${reason ? `: ${reason}` : ''}`);
                        }
                        const buffer = await this.context.decodeAudioData(await response.arrayBuffer());
                        if (!current()) return;
                        bytes += buffer.length * buffer.numberOfChannels * 4;
                        if (bytes > 512 * 1024 * 1024) {
                            abort.abort();
                            throw new Error(tr('오디오 메모리 한도를 초과했습니다.'));
                        }
                        buffers.set(id, buffer);
                    } catch (e) {
                        if (bytes > 512 * 1024 * 1024) throw e;
                        if (abort.signal.aborted || !current()) return;
                        const asset = c.assets[id], name = asset.path || id;
                        const label = asset.kind === 'bank' ? tr`${name} (샘플 ${asset.sample})` : name;
                        failures.push(`${label}: ${e.message || tr('오디오 디코딩 실패')}`);
                    }
                }
            }));
            if (!current()) return;
            const ready = segment(events, buffers, from);
            if (!ready.voices.length) {
                this.notify(failures.length ? tr`프리뷰 읽기 실패: ${failures[0]}` : tr('프리뷰 없음'));
                return;
            }
            this.ready = { ...ready, audioKey: key, beats: grid(c.timing),
                warning: failures.length ? tr` · 오디오 ${failures.length}개 실패: ${failures[0]}` : '',
                warningDetails: failures.join('\n') };
            this.play(ticket, 0, true);
        } catch (e) {
            if (current()) {
                this.stopVoices(fadeOut); this.ready = null;
                this.notify(tr`프리뷰 읽기 실패: ${e.message}`);
            }
        }
    }
    // Only a newly selected preview fades in; loops and speed changes continue at full volume.
    play(ticket, position = 0, fade = false) {
        if (ticket !== this.ticket || !this.ready) return;
        this.stopVoices();
        if (this.context.state !== 'running') {
            this.notify(tr('목록을 클릭하면 재생'));
            return;
        }
        const { from, duration, voices, warning, warningDetails } = this.ready;
        for (const voice of voices) {
            if (!this.pitch && this.rate !== 1 && !this.stretched.has(voice.buffer)) {
                const buffer = stretch(voice.buffer, this.rate, this.context);
                this.stretchBytes += buffer.length * buffer.numberOfChannels * 4;
                if (this.stretchBytes > 512 * 1024 * 1024) throw new Error(tr('오디오 메모리 한도를 초과했습니다.'));
                this.stretched.set(voice.buffer, buffer);
            }
        }
        const length = (duration - position) / this.rate;
        const at = this.context.currentTime + .03;
        this.started = at; this.position = position; this.end = at + length;
        const gain = this.context.createGain(); this.gain = gain; gain.connect(this.context.destination);
        if (fade) {
            gain.gain.setValueAtTime(0, at);
            gain.gain.setValueAtTime(0, at + fadeInDelay);
            gain.gain.linearRampToValueAtTime(this.volume, at + fadeInDelay + fadeIn);
        } else gain.gain.setValueAtTime(this.volume, at);
        const startAt = voice => at + Math.max(0, voice.delay - position) / this.rate;
        const pending = voices.filter(voice => voice.length > Math.max(0, position - voice.delay));
        const schedule = voice => {
            const elapsed = Math.max(0, position - voice.delay);
            const source = this.context.createBufferSource(), volume = this.context.createGain(), pan = this.context.createStereoPanner();
            const divisor = this.pitch ? 1 : this.rate;
            source.buffer = this.pitch ? voice.buffer : this.stretched.get(voice.buffer) || voice.buffer;
            source.playbackRate.value = this.pitch ? this.rate : 1;
            volume.gain.value = voice.volume; pan.pan.value = voice.pan || 0;
            source.connect(volume); volume.connect(pan); pan.connect(gain);
            source.onended = () => { source.disconnect(); volume.disconnect(); pan.disconnect(); };
            source.start(startAt(voice), (voice.offset + elapsed) / divisor, (voice.length - elapsed) / divisor);
            this.sources.push(source);
        };
        // Voices are sorted by delay, so their start times only grow.
        let next = 0;
        const pump = () => {
            if (this.gain !== gain) return;
            const horizon = this.context.currentTime + lookahead;
            while (next < pending.length && startAt(pending[next]) < horizon) schedule(pending[next++]);
            if (next < pending.length) this.pump = setTimeout(pump, pumpMS);
        };
        pump();
        this.notify(`${from.toFixed(1)}–${(from + duration).toFixed(1)}s${warning || ''}`, warningDetails || '');
        this.loop = setTimeout(() => {
            try { this.play(ticket); }
            catch (e) { this.stopVoices(); this.notify(tr`프리뷰 재생 실패: ${e.message}`); }
        }, (length + .03) * 1000);
    }
}
