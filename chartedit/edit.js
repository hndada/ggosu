import { tr } from '../preferences/language.js';
// Chart times remain on the original audio timeline. Zoom and playback rate
// never rescale saved timestamps.
export const NS = 1e6;
const clone = value => structuredClone(value);
export const ms = value => value / NS;
export const ns = value => Math.round(value * NS);

export function segments(timing) {
    let beat = 0, bar = 0;
    return timing.map((p, i) => {
        if (i) {
            const prev = timing[i - 1];
            const beats = Math.max(0, ms(p.at_ns - prev.at_ns - (prev.stop_ns || 0))) * prev.bpm / 60000;
            beat += beats; bar += beats / prev.meter;
        }
        return { ...p, beat, bar };
    });
}
export function position(timing, at) {
    const ps = segments(timing);
    const p = ps.findLast(p => ms(p.at_ns) <= at) || ps[0];
    const delta = at < ms(p.at_ns) ? at - ms(p.at_ns) : Math.max(0, at - ms(p.at_ns + (p.stop_ns || 0)));
    const beat = p.beat + delta * p.bpm / 60000;
    return { beat, bar: p.bar + (beat - p.beat) / p.meter, bpm: p.bpm, meter: p.meter };
}
export function snap(timing, at, division) {
    if (!division) return at;
    const ps = segments(timing), p = ps.findLast(p => ms(p.at_ns) <= at) || ps[0];
    const origin = ms(p.at_ns + (p.stop_ns || 0));
    if (at >= ms(p.at_ns) && at < origin) return ms(p.at_ns);
    const beat = Math.round(position(timing, at).beat * division) / division;
    const target = ps.findLast(p => p.beat <= beat) || ps[0];
    const start = ms(target.at_ns + (beat > target.beat ? target.stop_ns || 0 : 0));
    return start + (beat - target.beat) * 60000 / target.bpm;
}
export function ticks(timing, from, to, division) {
    const ps = segments(timing), out = [];
    const add = (p, origin, lo, hi) => {
        for (let k = Math.ceil((p.beat + (lo - origin) * p.bpm / 60000) * division); out.length < 3000; k++) {
            const at = origin + (k / division - p.beat) * 60000 / p.bpm;
            if (at > hi) break;
            out.push({ at, beat: k / division, major: k % division === 0 });
        }
    };
    for (let i = 0; i < ps.length; i++) {
        const p = ps[i], origin = ms(p.at_ns + (p.stop_ns || 0));
        const lo = Math.max(from, origin);
        const hi = Math.min(to, i + 1 < ps.length ? ms(ps[i + 1].at_ns) : to);
        if (!i && from < ms(p.at_ns)) add(p, ms(p.at_ns), from, Math.min(to, ms(p.at_ns)));
        if (lo <= hi) add(p, origin, lo, hi);
    }
    return out;
}
export function issues(c) {
    const sorted = c.notes.filter(n => n.kind !== 'fake').toSorted((a, b) => a.at_ns - b.at_ns);
    const end = new Map(), starts = new Map(), result = [];
    for (const n of sorted) {
        if (n.at_ns < end.get(n.lane) || n.at_ns === starts.get(n.lane)) result.push({ at: ms(n.at_ns), id: n.id, text: tr`${n.lane + 1}번 레인 노트 겹침` });
        end.set(n.lane, Math.max(end.get(n.lane) ?? -Infinity, n.end_ns)); starts.set(n.lane, n.at_ns);
    }
    return result;
}
export function mirrorLanes(c) {
    const result = Array.from({ length: c.lanes }, (_, i) => i), groups = new Map();
    result.forEach(i => {
        const l = c.layout?.[i] || { role: 'key', group: 0 };
        if (l.role === 'scratch') return;
        if (!groups.has(l.group || 0)) groups.set(l.group || 0, []);
        groups.get(l.group || 0).push(i);
    });
    for (const lanes of groups.values()) lanes.forEach((lane, i) => result[lane] = lanes.at(-1 - i));
    return result;
}
export class Draft {
    constructor(c) {
        this.c = clone(c); this.nextID = 1;
        this.c.notes = (this.c.notes || []).map(n => ({ ...n, id: this.nextID++ }));
        this.selected = new Set(); this.undo = []; this.redo = [];
        this.saved = JSON.stringify(this.document()); this.clipboard = [];
    }
    document() {
        const c = clone(this.c);
        c.notes = c.notes.map(({ id, ...n }) => n).sort((a, b) => a.at_ns - b.at_ns || a.lane - b.lane);
        c.timing.sort((a, b) => a.at_ns - b.at_ns);
        return c;
    }
    get dirty() { return JSON.stringify(this.document()) !== this.saved; }
    snapshot() { return { c: clone(this.c), selected: [...this.selected] }; }
    change(fn) {
        const before = this.snapshot(); fn(this.c);
        if (JSON.stringify(before.c) === JSON.stringify(this.c)) return false;
        this.undo.push(before); if (this.undo.length > 50) this.undo.shift();
        this.redo = []; return true;
    }
    restore(from, to) {
        if (!from.length) return;
        to.push(this.snapshot()); const s = from.pop(); this.c = s.c; this.selected = new Set(s.selected);
    }
    add(lane, at, end = at) {
        const id = this.nextID++;
        this.change(c => { c.notes.push({ lane, at_ns: ns(at), end_ns: ns(end), kind: end > at ? 'hold' : 'tap', id }); this.selected = new Set([id]); });
        return id;
    }
    remove() { this.change(c => { c.notes = c.notes.filter(n => !this.selected.has(n.id)); this.selected.clear(); }); }
    // Like Quaver's PlaceNoteAtLane: a lane key removes the note covering that time, otherwise adds a tap.
    toggle(lane, at) {
        const hit = this.c.notes.find(n => n.lane === lane && n.at_ns <= ns(at) && n.end_ns >= ns(at));
        if (!hit) return this.add(lane, at);
        this.change(c => { c.notes = c.notes.filter(n => n !== hit); this.selected.delete(hit.id); });
    }
    cut() { this.copy(); this.remove(); }
    move(delta, lanes = 0) {
        const notes = this.c.notes.filter(n => this.selected.has(n.id));
        if (notes.some(n => n.lane + lanes < 0 || n.lane + lanes >= this.c.lanes || Math.abs(ms(n.at_ns) + delta) > 86400000 || Math.abs(ms(n.end_ns) + delta) > 86400000)) return;
        this.change(() => notes.forEach(n => { n.at_ns += ns(delta); n.end_ns += ns(delta); n.lane += lanes; }));
    }
    copy() {
        const notes = this.c.notes.filter(n => this.selected.has(n.id));
        if (!notes.length) return;
        const at = notes.reduce((at, n) => Math.min(at, n.at_ns), Infinity);
        this.clipboard = notes.map(n => ({ ...clone(n), at_ns: n.at_ns - at, end_ns: n.end_ns - at }));
    }
    paste(at) {
        if (!this.clipboard.length) return;
        this.change(c => {
            const notes = this.clipboard.map(n => ({ ...clone(n), id: this.nextID++, at_ns: n.at_ns + ns(at), end_ns: n.end_ns + ns(at) }));
            c.notes.push(...notes); this.selected = new Set(notes.map(n => n.id));
        });
    }
    mirror() {
        const map = mirrorLanes(this.c);
        this.change(c => c.notes.filter(n => this.selected.has(n.id)).forEach(n => n.lane = map[n.lane]));
    }
}
