import test from 'node:test';
import assert from 'node:assert/strict';
import { mount } from './volume.js';

test('Alt arrows change volume on the first press, select channels and appear over dialogs', () => {
    class Element {
        hidden = false;
        dataset = {};
        attributes = new Map();
        children = [];
        append(...children) { for (const child of children) { child.parentElement = this; this.children.push(child); } }
        setAttribute(key, value) { this.attributes.set(key, value); }
        removeAttribute(key) { this.attributes.delete(key); }
        hasAttribute(key) { return this.attributes.has(key); }
        toggleAttribute(key, value) { if (value) this.setAttribute(key, ''); else this.removeAttribute(key); }
        addEventListener() {}
    }
    const parent = new Element(), dialog = new Element();
    const document = globalThis.document, raf = globalThis.requestAnimationFrame;
    let modal = null;
    globalThis.document = { createElement: () => new Element(), createElementNS: () => new Element(), querySelector: () => modal };
    const values = { master: 50, music: 70, keys: 80, effects: 30 };
    // Drawing animations are outside this shortcut test; enqueue their frames.
    const frames = [];
    globalThis.requestAnimationFrame = fn => { frames.push(fn); return frames.length; };
    const overlay = mount(parent, { get: () => values, set: (key, value) => { values[key] = value; return true; } });
    try {
        const key = name => overlay.key({ altKey: true, key: name });
        assert.equal(key('ArrowDown'), true);
        assert.equal(values.master, 45);
        frames.shift()(); frames.shift()(); // Overlay enters its visible state.
        key('ArrowRight'); key('ArrowDown'); key('ArrowDown');
        assert.equal(values.music, 60); // Keyboard steps remain 5%, including repeats.
        assert.equal(values.master, 45);
        assert.equal(overlay.key({ altKey: false, key: 'ArrowDown' }), false);
        overlay.hide();
        modal = dialog;
        key('ArrowUp');
        assert.equal(dialog.children.at(-1), parent.children[0]);
        modal = null;
        key('ArrowDown');
        assert.equal(parent.children.at(-1).parentElement, parent);
    } finally {
        overlay.hide();
        globalThis.document = document;
        globalThis.requestAnimationFrame = raf;
    }
});
