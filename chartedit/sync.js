export function deviceSettings(value) {
    const valid = x => Number.isFinite(x) && Math.abs(x) <= 1000;
    return { audioMS: valid(value?.audioMS) ? value.audioMS : 0, inputMS: valid(value?.inputMS) ? value.inputMS : 0 };
}

export function click(context, at, major = false) {
    const source = context.createOscillator(), gain = context.createGain(); source.frequency.value = major ? 1600 : 1000;
    gain.gain.setValueAtTime(.12, at); gain.gain.exponentialRampToValueAtTime(.001, at + .025);
    source.connect(gain); gain.connect(context.destination); source.start(at); source.stop(at + .03);
    source.onended = () => { source.disconnect(); gain.disconnect(); }; return source;
}
