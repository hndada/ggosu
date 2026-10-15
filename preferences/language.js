import { english } from './english.js';

export const LANGUAGE_KEY = 'ggosu:language';
let current = 'ko';
const originals = new Map(Object.entries(english).map(([ko, en]) => [en, ko]));
// Keep rendered messages so changing language can update their interpolations too.
const rendered = new Map();
const bindings = new WeakMap();
const format = (text, values) => text.replace(/\{(\d+)\}/g, (all, i) => i < values.length ? String(values[i]) : all);
export const language = () => current;
export const validLanguage = value => value === 'ko' || value === 'en';

export function storedLanguage(storage = globalThis.localStorage) {
    try { const value = storage?.getItem(LANGUAGE_KEY); return validLanguage(value) ? value : 'ko'; }
    catch { return 'ko'; }
}

export function saveLanguage(value, storage = globalThis.localStorage) {
    if (!validLanguage(value)) throw new Error(`Unsupported language: ${value}`);
    storage.setItem(LANGUAGE_KEY, value);
    return value;
}

export function setLanguage(value) {
    if (!validLanguage(value)) throw new Error(`Unsupported language: ${value}`);
    current = value;
}

function message(source, values) {
    const text = format(current === 'en' ? english[source] ?? source : source, values);
    for (const pattern of [source, english[source] ?? source]) {
        const key = format(pattern, values);
        // Unknown text is content, rather than an interface message.
        if (!Object.hasOwn(english, source)) continue;
        if (rendered.size >= 4096 && !rendered.has(key)) rendered.delete(rendered.keys().next().value);
        rendered.set(key, { source, values });
    }
    return text;
}

// Accept source messages, previously translated labels, and tagged templates.
// Interpolated titles, nicknames and chat messages are kept verbatim.
export function tr(text, ...values) {
    if (Array.isArray(text)) {
        const source = text.map((part, i) => part + (i < values.length ? `{${i}}` : '')).join('');
        if (Object.hasOwn(english, source)) return message(source, values);
        return text.map((part, i) => markup(part) + (i < values.length ? String(values[i]) : '')).join('');
    }
    if (typeof text !== 'string') return text;
    if (Object.hasOwn(english, text)) return message(text, values);
    const previous = rendered.get(text);
    if (previous) return message(previous.source, previous.values);
    const source = originals.get(text) ?? text;
    return message(source, values);
}

function fragment(text) {
    if (Object.hasOwn(english, text) || originals.has(text) || rendered.has(text)) return tr(text);
    const trimmed = text.trim();
    if (!trimmed || !Object.hasOwn(english, trimmed) && !originals.has(trimmed) && !rendered.has(trimmed)) return text;
    return text.slice(0, text.indexOf(trimmed)) + tr(trimmed) + text.slice(text.indexOf(trimmed) + trimmed.length);
}

// Only authored markup passes here; values inserted into a template never do.
function markup(text) {
    return text.split(/(<[^>]*>)/g).map(part => part.startsWith('<')
        ? part.replace(/\b(title|aria-label|placeholder)="([^"]*)"/g, (_, name, value) => `${name}="${fragment(value)}"`)
        : fragment(part)).join('');
}

// Called for the initial authored page, and when the user changes language.
// Content supplied by charts and players is explicitly excluded.
export function translate(root) {
    const skip = '[data-language-skip],script,style,input,textarea,#language-choice,#title-song,#song-name,#difficulty,#player-name,#loader-title,#loader-artist,#loader-chart,#playing-chart,#chart-menu-name,#result-song,#result-difficulty,.chart-name,#download-list strong,#download-list li > span,#download-transfer,#multi-title,#multi-song,#multi-members td:first-child,#multi-rooms strong,#multi-proposals,#multi-select select[name="chart"],#medley-candidates span,#medley-songs strong,#edit-title,#edit-chart-list,#edit-name,#edit-creator,.room-chat-text,.room-chat-name,.room-chat-context,.room-chat-ticker,.room-chat-popup';
    const doc = root.ownerDocument ?? root;
    function localized(node, key, text) {
        let entries = bindings.get(node);
        if (!entries) { entries = new Map(); bindings.set(node, entries); }
        let binding = entries.get(key);
        if (!binding || binding.last !== text) {
            const trimmed = text.trim();
            const source = Object.hasOwn(english, text) ? text : Object.hasOwn(english, trimmed) ? trimmed : null;
            const prior = rendered.get(text) ?? rendered.get(trimmed);
            const original = source ?? prior?.source ?? originals.get(text) ?? originals.get(trimmed);
            if (!original) return text;
            const match = source === text || rendered.has(text) || originals.has(text) ? text : trimmed;
            const at = text.indexOf(match);
            binding = { source: original, values: source ? [] : prior?.values ?? [], before: text.slice(0, at), after: text.slice(at + match.length) };
            entries.set(key, binding);
        }
        binding.last = binding.before + message(binding.source, binding.values) + binding.after;
        return binding.last;
    }
    const walker = doc.createTreeWalker(root, 4);
    while (walker.nextNode()) {
        const node = walker.currentNode;
        if (!node.parentElement?.closest(skip)) node.nodeValue = localized(node, 'text', node.nodeValue);
    }
    for (const el of root.querySelectorAll('[title],[aria-label],[placeholder]')) {
        if (el.closest(skip)) continue;
        for (const attr of ['title', 'aria-label', 'placeholder']) {
            if (el.hasAttribute(attr)) el.setAttribute(attr, localized(el, attr, el.getAttribute(attr)));
        }
    }
    doc.documentElement.lang = current;
}
