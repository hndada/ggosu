// Short choices as a row of ShearedButtons instead of a browser checkbox or dropdown: every option
// stays in sight and the picked one is lit (aria-pressed). The real control stays in the page,
// hidden, so the code reading .value / .checked and listening for input / change is unchanged.
const MAX_OPTIONS = 4;

export function wrap(control) {
    const box = document.createElement('span');
    box.className = 'choice';
    box.setAttribute('role', 'group');
    const label = control.id && document.querySelector(`label[for="${control.id}"]`);
    if (label) box.setAttribute('aria-label', label.textContent.trim());
    else if (control.getAttribute('aria-label')) box.setAttribute('aria-label', control.getAttribute('aria-label'));
    const options = control.type === 'checkbox' ? [['', 'OFF'], ['on', 'ON']] : [...control.options].map(o => [o.value, o.textContent]);
    for (const [value, text] of options) {
        const b = document.createElement('button');
        b.type = 'button'; b.value = value; b.textContent = text;
        b.addEventListener('click', () => pick(control, value));
        box.append(b);
    }
    control.hidden = true;
    control.after(box);
    // Code sets .value / .checked / .disabled directly and that fires no event, so this control's
    // own setters repaint the buttons.
    const proto = Object.getPrototypeOf(control);
    for (const key of ['value', 'checked', 'disabled']) {
        const d = Object.getOwnPropertyDescriptor(proto, key);
        if (!d?.set) continue;
        Object.defineProperty(control, key, { configurable: true, get() { return d.get.call(this); }, set(v) { d.set.call(this, v); paint(control); } });
    }
    paint(control);
}

function pick(control, value) {
    if (control.disabled) return;
    if (control.type === 'checkbox') {
        if (control.checked === (value === 'on')) return;
        control.checked = value === 'on';
    } else {
        if (control.value === value) return;
        control.value = value;
    }
    control.dispatchEvent(new Event('input', { bubbles: true }));
    control.dispatchEvent(new Event('change', { bubbles: true }));
}

export function paint(control) {
    const box = control.nextElementSibling;
    if (!box?.classList.contains('choice')) return;
    const current = control.type === 'checkbox' ? (control.checked ? 'on' : '') : control.value;
    for (const b of box.querySelectorAll(':scope > button')) {
        b.setAttribute('aria-pressed', String(b.value === current));
        b.disabled = control.disabled;
    }
}

// Every visible checkbox and short dropdown in a settings row.
export function wrapAll(root) {
    for (const c of root.querySelectorAll('.settings-row input[type="checkbox"], .settings-row select')) {
        if (c.hidden || c.tagName === 'SELECT' && c.options.length > MAX_OPTIONS) continue;
        wrap(c);
    }
}
