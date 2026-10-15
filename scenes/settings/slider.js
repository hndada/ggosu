// osu!lazer's ShearedSliderBar: a sheared bar filled up to a 50px nub.
// The real range input lies on top, invisible, so drag, keys and wheel stay native.
const NUB = 50;

export function wrap(input) {
    const box = document.createElement('span');
    box.className = 'slider';
    input.replaceWith(box);
    for (const name of ['slider-fill', 'slider-rest', 'slider-nub']) {
        const part = document.createElement('span');
        part.className = name;
        box.append(part);
    }
    box.append(input);
    input.addEventListener('input', () => paint(input));
    paint(input);
}

// Call after setting input.value in code; that fires no input event.
export function paint(input) {
    const box = input.parentElement;
    if (!box?.classList.contains('slider')) return;
    const min = Number(input.min || 0), max = Number(input.max || 100);
    const v = max > min ? (input.valueAsNumber - min) / (max - min) : 0;
    const [fill, rest, nub] = box.children;
    fill.style.width = `calc((100% - ${NUB}px) * ${v})`;
    rest.style.width = `calc((100% - ${NUB}px) * ${1 - v})`;
    nub.style.left = `calc((100% - ${NUB}px) * ${v})`;
}
