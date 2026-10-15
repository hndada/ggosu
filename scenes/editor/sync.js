import { tr } from '../../preferences/language.js';
import { calibration, eventTime } from '/chartedit/beatfit.js';
import { deviceSettings, click } from '/chartedit/sync.js';

const key = 'ggosu-editor-device-v1';

// Device values belong to this browser/WebView, never to a Draft or chart file.
export function mountSync(root, { context, pause, apply, listen, seek }) {
    const dialog = document.createElement('dialog'); dialog.id = 'edit-sync-dialog';
    dialog.innerHTML = tr`<h2>음원 타이밍과 제작 환경</h2>
<p>음원 분석값은 곡 타이밍입니다. 장치 보정값은 이 브라우저·앱에만 저장합니다.</p>
<section><h3>음원 분석</h3><label>전체 음원 <select id="sync-audio"></select></label>
<button id="sync-analyze" type="button">BPM·시작 위치 분석</button><button id="sync-cancel" type="button" disabled>분석 취소</button>
<p id="sync-status" role="status" aria-live="polite">전체 음원을 고르고 분석해 주세요.</p>
<label>분석 범위 <select id="sync-section"><option value="-1">곡 전체</option></select></label>
<label>타이밍 후보 <select id="sync-candidate"></select></label>
<p id="sync-detail"></p><label>첫 박 위치 (ms) <input id="sync-offset" type="number" step="any" min="-86400000" max="86400000"></label>
<button id="sync-half" type="button">반 박 앞으로</button><button id="sync-later" type="button">반 박 뒤로</button>
<button id="sync-listen" type="button">후보 메트로놈 미리듣기</button><button id="sync-apply" type="button">선택한 타이밍 이벤트에 적용</button>
<p>구간 후보는 변속·박자 누락을 확인하기 위한 값입니다. 변속 경계와 마디 첫 박은 직접 확인해 주세요. 적용은 선택한 이벤트의 BPM·시작 시각만 바꾸며 Undo로 복구할 수 있습니다.</p>
<ol id="sync-regions"></ol></section>
<section><h3>제작 환경 보정</h3>
<label>출력 보정 (ms) <input id="sync-output" type="number" min="-1000" max="1000" step="1" value="0"></label>
<p>양수면 화면을 소리에 맞춰 늦춥니다. 음원과 메트로놈의 상대 위치는 바뀌지 않습니다.</p>
<label>입력 보정 (ms) <input id="sync-input" type="number" min="-1000" max="1000" step="1" value="0"></label>
<p>양수면 실시간 노트 입력 시각을 그만큼 앞당깁니다. 아래 보정은 출력·입력 지연과 사람의 반응 편차를 함께 포함합니다.</p>
<button id="sync-calibrate" type="button">메트로놈 입력 보정</button><button id="sync-cal-stop" type="button" disabled>보정 중단</button>
<p id="sync-cal-status" role="status" aria-live="polite">소리에 맞춰 F 키를 누릅니다. 첫 4박은 연습, 이후 24박 중 12회 이상 입력해 주세요.</p>
<button id="sync-save" type="button">장치 보정 저장</button><button id="sync-reset" type="button">장치 보정 초기화</button><p id="sync-device-status" role="status"></p></section>
<button id="sync-close" type="button">닫기</button>`;
    root.append(dialog);
    const $ = name => dialog.querySelector('#sync-' + name);
    let device = { audioMS: 0, inputMS: 0 }, tracks = [], result, worker, selected, cal, timer, clicks = [], serial = 0, calSerial = 0;
    try { device = deviceSettings(JSON.parse(localStorage.getItem(key))); }
    catch { $('device-status').textContent = tr('저장된 장치 설정을 읽지 못했습니다. 기본값을 사용합니다.'); }
    const refreshDevice = () => { $('output').value = device.audioMS; $('input').value = device.inputMS; };
    refreshDevice();
    function cancel() { serial++; worker?.terminate(); worker = undefined; $('analyze').disabled = !tracks.length; $('cancel').disabled = true; }
    function stopCalibration() {
        calSerial++;
        clearTimeout(timer); cal = undefined;
        for (const source of clicks) { try { source.stop(); } catch { /* Already ended. */ } }
        clicks = []; $('calibrate').disabled = false; $('cal-stop').disabled = true;
    }
    function selection() {
        const section = Number($('section').value), choices = section < 0 ? result?.candidates : result?.sections[section].candidates;
        selected = choices?.[Number($('candidate').value)];
        for (const name of ['offset', 'half', 'later', 'listen', 'apply']) $(name).disabled = !selected;
        if (!selected) { $('detail').textContent = tr('이 구간에서 박자 후보를 찾지 못했습니다.'); return; }
        $('offset').value = (selected.offset * 1000).toFixed(3);
        $('detail').textContent = tr`${selected.bpm.toFixed(6)} BPM · 검출 ${selected.matched}박 · 박 일치율 ${(selected.coverage * 100).toFixed(0)}% · 어택 오차 중앙값 ${(selected.median * 1000).toFixed(1)} / 95백분위 ${(selected.p95 * 1000).toFixed(1)} ms. 이는 음악적 정답률이 아닙니다.`;
    }
    function candidates() {
        const section = Number($('section').value), choices = section < 0 ? result?.candidates : result?.sections[section].candidates;
        $('candidate').replaceChildren(...(choices || []).map((p, i) => Object.assign(document.createElement('option'), { value: i, textContent: `${p.bpm.toFixed(4)} BPM · ${(p.offset * 1000).toFixed(1)} ms` })));
        selection();
    }
    selection();
    $('section').onchange = candidates; $('candidate').onchange = selection;
    $('analyze').onclick = () => {
        stopCalibration(); cancel(); result = undefined; candidates();
        const track = tracks[Number($('audio').value)]; if (!track) return;
        const token = serial;
        try {
            worker = new Worker(new URL('/chartedit/beatfitworker.js', import.meta.url), { type: 'module' });
            const channels = Array.from({ length: Math.min(2, track.buffer.numberOfChannels) }, (_, i) => track.buffer.getChannelData(i).slice());
            worker.onmessage = ({ data }) => {
                if (token !== serial) return; cancel();
                if (data.error) { $('status').textContent = data.error; return; }
                result = data.result;
                for (const p of result.candidates) p.offset += track.at;
                for (const s of result.sections) { s.from += track.at; s.to += track.at; for (const p of s.candidates) p.offset += track.at; }
                $('section').replaceChildren(Object.assign(document.createElement('option'), { value: -1, textContent: tr('곡 전체') }),
                    ...result.sections.map((s, i) => Object.assign(document.createElement('option'), { value: i, textContent: `${s.from.toFixed(1)}–${s.to.toFixed(1)} s` })));
                const changing = result.sections.some(s => s.changed);
                $('status').textContent = tr`${result.duration.toFixed(1)}초 분석 · ${result.onsets}개 어택. ${changing ? tr('구간별 BPM 차이가 있습니다. 변속 또는 오검출 구간을 확인해 주세요.') : tr('후보를 소리로 확인한 뒤 적용해 주세요.')}`;
                $('regions').replaceChildren(...result.sections.map((s, i) => {
                    const li = document.createElement('li'), button = document.createElement('button');
                    button.type = 'button'; button.textContent = `${s.from.toFixed(1)}–${s.to.toFixed(1)} s: ${s.bpm?.toFixed(3) ?? tr('검출 실패')} BPM${s.changed ? tr(' · 전체 후보와 BPM 차이') : ''}${s.uncertain ? tr(' · 확인 필요') : ''}`;
                    button.onclick = () => { $('section').value = i; candidates(); seek(s.from * 1000); }; li.append(button); return li;
                })); candidates();
            };
            worker.onerror = () => { if (token === serial) { cancel(); $('status').textContent = tr('분석 작업을 시작하지 못했습니다. 다시 시도해 주세요.'); } };
            worker.postMessage({ channels, sampleRate: track.buffer.sampleRate }, channels.map(c => c.buffer));
            $('analyze').disabled = true; $('cancel').disabled = false; $('status').textContent = tr('음원 전체를 분석하고 있습니다…');
        } catch (e) { cancel(); $('status').textContent = tr`분석 실패: ${e.message}`; }
    };
    $('cancel').onclick = () => { cancel(); $('status').textContent = tr('분석을 취소했습니다.'); };
    $('audio').onchange = () => { cancel(); result = undefined; candidates(); $('regions').replaceChildren(); $('status').textContent = tr('선택한 음원을 분석해 주세요.'); };
    for (const [name, sign] of [['half', -1], ['later', 1]]) $(name).onclick = () => { if (selected) $('offset').value = ($('offset').valueAsNumber + sign * selected.period * 500).toFixed(3); };
    function proposal() {
        if (!selected || !$('offset').reportValidity() || !Number.isFinite($('offset').valueAsNumber)) return;
        return { bpm: selected.bpm, at: $('offset').valueAsNumber };
    }
    $('apply').onclick = () => { const p = proposal(); if (p) { stopCalibration(); apply(p); dialog.close(); } };
    $('listen').onclick = () => { const p = proposal(); if (p) { stopCalibration(); dialog.close(); listen(p); } };
    $('save').onclick = () => {
        if (!$('output').reportValidity() || !$('input').reportValidity()) return;
        try {
            const values = { audioMS: $('output').valueAsNumber, inputMS: $('input').valueAsNumber };
            if (!Object.values(values).every(Number.isFinite)) throw new Error(tr('보정값을 입력해 주세요.'));
            localStorage.setItem(key, JSON.stringify(values)); device = values; $('device-status').textContent = tr('장치 보정을 저장했습니다. 채보 파일에는 포함되지 않습니다.');
        } catch (e) { $('device-status').textContent = tr`저장 실패: ${e.message}`; }
    };
    $('reset').onclick = () => { $('output').value = $('input').value = 0; $('save').click(); };
    $('calibrate').onclick = async () => {
        stopCalibration(); pause();
        const token = calSerial; $('calibrate').disabled = true; $('cal-stop').disabled = false;
        try {
            const ctx = context(); await ctx.resume(); if (!dialog.open || token !== calSerial) return;
            const start = ctx.currentTime + .5; cal = { start, errors: [], beats: new Set() };
            for (let i = 0; i < 28; i++) clicks.push(click(ctx, start + i * .5));
            $('calibrate').disabled = true; $('cal-stop').disabled = false;
            $('cal-status').textContent = tr('F 키로 소리에 맞춰 입력하세요. 첫 4박은 연습입니다. 0회 입력');
            timer = setTimeout(() => {
                if (!cal) return;
                try { const p = calibration(cal.errors); $('input').value = p.inputMS; $('cal-status').textContent = tr`제안 ${p.inputMS} ms · ${p.count}회 · 중앙 절대 편차 ${p.spread} ms. 장치 보정 저장을 눌러 적용하세요.`; }
                catch (e) { $('cal-status').textContent = e.message; }
                stopCalibration();
            }, (start - ctx.currentTime + 14) * 1000);
        } catch (e) { if (token === calSerial) { stopCalibration(); $('cal-status').textContent = tr`보정 실패: ${e.message}`; } }
    };
    const onKey = e => {
        if (!cal || e.code !== 'KeyF' || e.repeat || e.isComposing || e.target.matches('input,select,textarea')) return;
        e.preventDefault(); const at = eventTime(context(), e.timeStamp), beat = Math.round((at - cal.start) / .5);
        if (beat < 4 || beat >= 28 || cal.beats.has(beat)) return;
        const error = (at - cal.start - beat * .5) * 1000;
        if (Math.abs(error) > 200) return;
        cal.beats.add(beat); cal.errors.push(error); $('cal-status').textContent = tr`F 키로 소리에 맞춰 입력하세요. ${cal.errors.length}회 입력`;
    };
    document.addEventListener('keydown', onKey);
    $('cal-stop').onclick = () => { stopCalibration(); $('cal-status').textContent = tr('보정을 중단했습니다.'); };
    $('close').onclick = () => dialog.close(); dialog.addEventListener('close', () => { cancel(); stopCalibration(); });
    window.addEventListener('blur', stopCalibration);
    return {
        get device() { return device; },
        open() { pause(); dialog.showModal(); },
        audio(buffers, chart) {
            tracks = (chart.audio || []).filter(a => buffers.has(a.sound.asset)).map(a => ({ id: a.sound.asset, at: a.at_ns / 1e9, buffer: buffers.get(a.sound.asset) })).filter(a => a.buffer.duration >= 8);
            $('audio').replaceChildren(...tracks.map((t, i) => Object.assign(document.createElement('option'), { value: i, textContent: tr`${chart.assets[t.id]?.path || t.id} · ${t.buffer.duration.toFixed(1)} s · 시작 ${t.at.toFixed(3)} s` })));
            $('analyze').disabled = !tracks.length;
            if (!tracks.length) $('status').textContent = tr('분석 가능한 8초 이상 전체 음원이 없습니다. 키음형 채보는 전체 믹스 음원이 필요합니다.');
        },
        reset() { cancel(); stopCalibration(); tracks = []; result = undefined; candidates(); $('audio').replaceChildren(); $('section').replaceChildren(Object.assign(document.createElement('option'), { value: -1, textContent: tr('곡 전체') })); $('regions').replaceChildren(); $('status').textContent = tr('전체 음원을 고르고 분석해 주세요.'); },
    };
}
