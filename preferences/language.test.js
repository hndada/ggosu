import test from 'node:test';
import assert from 'node:assert/strict';
import { english } from './english.js';
import { LANGUAGE_KEY, language, setLanguage, storedLanguage, saveLanguage, tr } from './language.js';

test('language defaults to Korean, persists, and rejects unsupported values and failed saves', t => {
    t.after(() => setLanguage('ko'));
    const values = new Map(), storage = { getItem: k => values.get(k), setItem: (k, v) => values.set(k, v) };
    assert.equal(storedLanguage(storage), 'ko');
    assert.equal(saveLanguage('en', storage), 'en');
    assert.equal(storedLanguage(storage), 'en');
    assert.throws(() => saveLanguage('other', storage));
    assert.equal(storedLanguage(storage), 'en');
    values.set(LANGUAGE_KEY, 'other');
    assert.equal(storedLanguage(storage), 'ko');
    const blocked = { getItem() { throw new Error('denied'); }, setItem() { throw new Error('denied'); } };
    assert.equal(storedLanguage(blocked), 'ko');
    assert.throws(() => saveLanguage('en', blocked), /denied/);
    assert.throws(() => setLanguage('other'));
    assert.equal(language(), 'ko');
});

test('messages switch both ways while preserving interpolation values verbatim', t => {
    t.after(() => setLanguage('ko'));
    setLanguage('en');
    assert.equal(tr('환경 설정'), 'Settings');
    assert.equal(tr`${'음악'} 입장`, 'Join 음악');
    const selected = tr`선택 ${2} / ${5} · 표시 ${1}–${5}`;
    assert.equal(selected, 'Selected 2 / 5 · Showing 1–5');
    setLanguage('ko');
    assert.equal(tr(selected), '선택 2 / 5 · 표시 1–5');
    assert.equal(tr('a chart title'), 'a chart title');
});

test('authored markup translates text and accessibility labels without changing controls or content values', t => {
    t.after(() => setLanguage('ko'));
    setLanguage('en');
    const title = '음악';
    assert.equal(tr`<label>검색 <input id="query" placeholder="제목·아티스트·제작자"></label><b>${title}</b>`,
        '<label>Search <input id="query" placeholder="Title, artist, creator"></label><b>음악</b>');
});

test('translated templates retain exactly the source interpolation slots', () => {
    const slots = value => [...value.matchAll(/\{\d+\}/g)].map(m => m[0]).sort();
    for (const [source, translation] of Object.entries(english)) assert.deepEqual(slots(translation), slots(source), source);
});
