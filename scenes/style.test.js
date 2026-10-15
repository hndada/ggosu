import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

// CSS는 화면 배치에 쓰고, 색·글자 크기·짧은 전환 효과는 theme.css에서만 정한다.
// 요청된 IIDX 램프의 유리 질감·발광(F/P 글자 발광 포함)·점멸은 해당 램프에서만 허용한다.
const allowed = new Set([
    'display', 'position', 'inset', 'top', 'right', 'bottom', 'left', 'z-index',
    'width', 'height', 'min-width', 'min-height', 'max-width', 'max-height', 'box-sizing',
    'margin', 'margin-top', 'margin-right', 'margin-bottom', 'margin-left', 'margin-inline', 'margin-block',
    'padding', 'padding-top', 'padding-right', 'padding-bottom', 'padding-left', 'padding-inline', 'padding-block',
    'overflow', 'overflow-x', 'overflow-y', 'overflow-wrap', 'scrollbar-width', 'resize',
    'flex', 'flex-direction', 'flex-wrap', 'flex-flow', 'flex-grow', 'flex-shrink', 'flex-basis', 'order',
    'grid', 'grid-template', 'grid-template-columns', 'grid-template-rows', 'grid-template-areas',
    'grid-area', 'grid-column', 'grid-row', 'grid-column-start', 'grid-column-end', 'grid-row-start', 'grid-row-end',
    'grid-auto-flow', 'grid-auto-columns', 'grid-auto-rows',
    'gap', 'row-gap', 'column-gap',
    'align-items', 'align-content', 'align-self', 'justify-items', 'justify-content', 'justify-self',
    'place-items', 'place-content', 'place-self',
    'text-align',
    'aspect-ratio', 'object-fit', 'object-position',
]);
const allowedAtRules = new Set(['media']);
// theme.css에서만 쓰는 색 속성. 색 값은 :root의 변수로만 정의하고 규칙에서는 var(--...)로 쓴다.
const THEME = 'theme.css';
const colorProperties = new Set([
    'color', 'color-scheme', 'background', 'background-color',
    'border', 'border-color', 'border-width', 'border-style', 'border-top', 'border-right', 'border-bottom', 'border-left',
    'outline',
    // Type scale and motion that make the screen read as a game: big level numbers, fades, quick slides.
    'font-size', 'font-weight', 'font-style', 'font-family', 'letter-spacing', 'line-height', 'font-variant-numeric',
    'opacity', 'transform', 'transition',
    // Hides the system cursor for the in-window one (cursor.js).
    'cursor',
    // Removes link underlines on the website (approved 2026-10-14).
    'text-decoration',
]);

function declarations(css) {
    const source = css.replace(/\/\*[\s\S]*?\*\//g, '');
    const found = [];
    for (const m of source.matchAll(/@([-\w]+)/g)) found.push({ atRule: m[1].toLowerCase() });
    for (const m of source.matchAll(/([-\w]+)\s*:[^;{}]*(?=[;}])/g)) found.push({ property: m[1].toLowerCase() });
    return found;
}

function violations(css, where) {
    const out = [];
    for (const d of declarations(css)) {
        if (d.atRule && !allowedAtRules.has(d.atRule)) out.push(`${where}: @${d.atRule}`);
        if (d.property && !allowed.has(d.property)) out.push(`${where}: ${d.property}`);
    }
    return out;
}

function themeViolations(css, where) {
    const source = css.replace(/\/\*[\s\S]*?\*\//g, '');
    const out = [];
    const effects = new Set(['lamp-pulse', 'lamp-combo', 'lamp-perfect']);
    for (const m of source.matchAll(/@([-\w]+)(?:\s+([\w-]+))?/g)) {
        if (!allowedAtRules.has(m[1].toLowerCase()) && m[1] !== 'font-face' && !(m[1] === 'keyframes' && effects.has(m[2]))) out.push(`${where}: @${m[1]}`);
    }
    for (const rule of source.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
        const root = rule[1].trim() === ':root';
        const fontFace = rule[1].trim() === '@font-face';
        const lamp = rule[1].trim().split(',').every(selector => /^\.(?:chart-lamp|score-lamp|combo-lamp)(?=[\s[.:]|$)/.test(selector.trim()));
        for (const d of rule[2].matchAll(/([-\w]+)\s*:([^;]*)/g)) {
            const property = d[1].toLowerCase(), value = d[2].trim();
            if (fontFace && property === 'src') {
                if (!/^url\("\/ui\/fonts\/[\w-]+\.woff2"\) format\("woff2"\)$/.test(value)) out.push(`${where}: 외부 또는 잘못된 폰트 경로`);
                continue;
            }
            if (fontFace && property === 'font-display' && value === 'swap') continue;
            if (property.startsWith('--')) { if (!root) out.push(`${where}: ${property}는 :root에서만 정의`); continue; }
            if (/gradient\(/i.test(value) && !lamp) out.push(`${where}: gradient`);
            const effect = lamp && (property === 'box-shadow' || property === 'text-shadow' || property === 'animation' && (value === 'none' || effects.has(value.split(/\s+/)[0])));
            if (!allowed.has(property) && !colorProperties.has(property) && !effect) { out.push(`${where}: ${property}`); continue; }
            if (/#[0-9a-f]{3,8}\b|rgba?\(|hsla?\(/i.test(value)) out.push(`${where}: ${property}의 색은 :root 변수로`);
        }
    }
    return out;
}

const kebab = name => name.replace(/[A-Z]/g, c => `-${c.toLowerCase()}`);

function check(name, text) {
    const out = [];
    if (name === `scenes/${THEME}` || name === THEME) out.push(...themeViolations(text, name));
    else if (name.endsWith('.css')) out.push(...violations(text, name));
    if (name.endsWith('.html')) {
        for (const m of text.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/gi)) out.push(...violations(m[1], `${name} <style>`));
        for (const m of text.matchAll(/\sstyle\s*=\s*(["'])([\s\S]*?)\1/gi)) out.push(...violations(`x{${m[2]}}`, `${name} style=""`));
        for (const m of text.matchAll(/<link\b[^>]*rel\s*=\s*["']?stylesheet[^>]*>/gi)) {
            const href = /href\s*=\s*["']([^"']+)["']/i.exec(m[0])?.[1] ?? '';
            if (!/^\/?([\w-]+\/)*[\w.-]+\.css$/.test(href)) out.push(`${name}: 외부 스타일시트 ${href}`);
        }
    }
    if (name.endsWith('.js')) {
        for (const m of text.matchAll(/\.style\.([A-Za-z]+)/g)) {
            if (m[1] !== 'setProperty' && m[1] !== 'cssText' && !allowed.has(kebab(m[1]))) out.push(`${name}: style.${m[1]}`);
        }
        for (const m of text.matchAll(/setProperty\(\s*(["'`])([-\w]+)\1/g)) {
            if (!allowed.has(m[2])) out.push(`${name}: setProperty ${m[2]}`);
        }
        for (const m of text.matchAll(/cssText|insertRule|adoptedStyleSheets|CSSStyleSheet|setAttribute\(\s*["'`]style|\.style\s*\[/g)) {
            out.push(`${name}: ${m[0]}`);
        }
    }
    return out;
}

// 홈페이지는 게임 테마를 불러오고, home.css의 색은 테마 변수로만 쓴다.
test('홈페이지는 앱 테마를 따르고 색은 테마 변수로만 쓴다', () => {
    const home = new URL('../internal/server/web/static/', import.meta.url);
    for (const name of ['home.css', 'home.html', 'home.js', 'about.html']) {
        const source = readFileSync(new URL(name, home), 'utf8');
        assert.deepEqual(name === 'home.css' ? themeViolations(source, name) : check(name, source), []);
        if (name.endsWith('.html')) {
            const links = [...source.matchAll(/<link\b[^>]*rel="stylesheet"[^>]*>/g)].map(m => m[0]);
            assert.deepEqual(links, ['<link rel="stylesheet" href="/theme.css">', '<link rel="stylesheet" href="/home.css">']);
        }
    }
});

test('검사기는 장식 속성을 찾아낸다', () => {
    assert.deepEqual(check('a.css', '.x:hover { display: grid; color: red } @media (min-width: 1px) { .y { border-radius: 4px; } }'),
        ['a.css: color', 'a.css: border-radius']);
    assert.deepEqual(check('a.css', '@keyframes k { from { left: 0 } }'), ['a.css: @keyframes']);
    assert.deepEqual(check('a.html', '<p style="grid-area: a; background: #000">'), ['a.html style="": background']);
    assert.deepEqual(check('a.html', '<link rel="stylesheet" href="https://fonts.example/x.css">'), ['a.html: 외부 스타일시트 https://fonts.example/x.css']);
    assert.deepEqual(check('a.js', 'el.style.gridArea = "a"; el.style.boxShadow = "x"; el.style.setProperty("opacity", 0);'),
        ['a.js: style.boxShadow', 'a.js: setProperty opacity']);
    assert.deepEqual(check('a.js', 'el.style.cssText = "";'), ['a.js: cssText']);
});

test('theme.css는 :root 변수로만 색을 정하고 장식은 막는다', () => {
    assert.deepEqual(check(THEME, '@font-face { font-family: "Gosu UI"; src: url("/ui/fonts/gmarketmedium.woff2") format("woff2"); font-display: swap; }'), []);
    assert.deepEqual(check(THEME, '@font-face { src: url("https://fonts.example/font.woff2") format("woff2"); }'), [`${THEME}: 외부 또는 잘못된 폰트 경로`]);
    assert.deepEqual(check(THEME, ':root { --bg: #000; color-scheme: dark; } body { background: var(--bg); border: 1px solid var(--bg); }'), []);
    assert.deepEqual(check(THEME, 'body { color: #fff; --x: red; background: linear-gradient(red, blue); box-shadow: 0 0 1px; }'),
        [`${THEME}: color의 색은 :root 변수로`, `${THEME}: --x는 :root에서만 정의`, `${THEME}: gradient`, `${THEME}: box-shadow`]);
    assert.deepEqual(check('layout.css', 'body { color: var(--text); }'), ['layout.css: color']);
    assert.deepEqual(check(THEME, '.chart-lamp:not([data-state="none"]):not([data-state="unavailable"]) { box-shadow: 0 0 7px currentColor; }'), []);
    assert.deepEqual(check(THEME, '.other { box-shadow: 0 0 7px currentColor; border-radius: 50%; }'), [`${THEME}: box-shadow`, `${THEME}: border-radius`]);
    assert.deepEqual(check(THEME, '.combo-lamp[data-state="fullcombo"] { animation: lamp-combo 1.2s steps(1, end) infinite; background: linear-gradient(var(--lamp-core), currentColor); } @keyframes lamp-combo { from { opacity: 1; } to { opacity: .7; } }'), []);
    assert.deepEqual(check(THEME, '.other { animation: lamp-combo 1s infinite; } .chart-lamp { animation: other 1s infinite; } @keyframes other { from { opacity: 1; } }'), [`${THEME}: @keyframes`, `${THEME}: animation`, `${THEME}: animation`]);
});
