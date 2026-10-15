import { tr } from '../../preferences/language.js';
const $ = selector => document.querySelector(selector);
let ready = false, saving = false, ticket = 0;
let saved = { globalMS: 0, chartMS: 0, judgeMS: 0 };
export function available() { return typeof window.go?.main?.App?.Offsets === 'function'; }
function offsetValues() { return { globalMS: $('#global-offset').valueAsNumber, chartMS: $('#chart-offset').valueAsNumber, judgeMS: $('#judge-offset').valueAsNumber }; }
function dirty() { const o = offsetValues(); return o.globalMS !== saved.globalMS || o.chartMS !== saved.chartMS || o.judgeMS !== (saved.judgeMS ?? 0); }
export function offsetText(o = {}) { return tr`전체 ${o.globalMS ?? 0} ms + 채보 ${o.chartMS ?? 0} ms = ${(o.globalMS ?? 0) + (o.chartMS ?? 0)} ms`; }
export function needsSave() { return available() && (!ready || dirty() || saving); }
export function editing() { return available() && (ready && dirty() || saving); }
export function setBusy(busy, hasChart) {
    const disabled = busy || saving || !ready;
    $('#global-offset').disabled = $('#offset-reset-global').disabled = disabled;
    $('#judge-offset').disabled = $('#offset-reset-judge').disabled = disabled;
    $('#chart-offset').disabled = $('#offset-reset-chart').disabled = disabled || !hasChart;
}
export function mount({ selection, busy, update, setPending }) {
    async function load() {
        if (!available()) return;
        $('#offsets').hidden = false;
        const request = ++ticket;
        ready = false; update();
        $('#offset-status').textContent = '';
        try {
            const o = await window.go.main.App.Offsets(selection() ?? {});
            if (request !== ticket) return;
            saved = o;
            $('#global-offset').value = o.globalMS; $('#chart-offset').value = o.chartMS; $('#judge-offset').value = o.judgeMS ?? 0;
            $('#offset-total').textContent = tr`적용 합계: ${offsetText(o)}`;
            $('#offset-status').textContent = '';
            ready = true;
        } catch (e) { if (request === ticket) $('#offset-status').textContent = tr`설정 읽기 실패: ${e?.message ?? String(e)}`; }
        finally { if (request === ticket) update(); }
    }
    async function save() {
        if (busy() || saving || !ready || !dirty() || !$('#offsets').reportValidity()) return;
        saving = true; setPending(true);
        try {
            const o = await window.go.main.App.SaveOffsets(selection() ?? {}, offsetValues());
            saved = o;
            $('#offset-total').textContent = tr`적용 합계: ${offsetText(o)}`;
            $('#offset-status').textContent = tr('설정을 저장했습니다. 다음 플레이부터 적용합니다.');
        } catch (e) { $('#offset-status').textContent = tr`설정 저장 실패: ${e?.message ?? String(e)}`; }
        finally { saving = false; setPending(false); }
    }
    $('#offsets').addEventListener('submit', e => { e.preventDefault(); save(); });
    // Auto offset is one switch for every chart and saves as soon as it changes.
    const auto = $('#auto-offset');
    if (typeof window.go?.main?.App?.AutoOffset === 'function') window.go.main.App.AutoOffset().then(on => { auto.checked = on; }).catch(e => { $('#offset-status').textContent = tr`설정 읽기 실패: ${e?.message ?? e}`; });
    else $('#auto-offset-row').hidden = true;
    auto.addEventListener('change', async () => {
        if (busy()) { auto.checked = !auto.checked; return; }
        try { auto.checked = await window.go.main.App.SaveAutoOffset(auto.checked); $('#offset-status').textContent = auto.checked ? tr('자동 보정을 켰습니다.') : tr('자동 보정을 껐습니다.'); }
        catch (e) { auto.checked = !auto.checked; $('#offset-status').textContent = tr`설정 저장 실패: ${e?.message ?? e}`; }
    });
    for (const id of ['global', 'chart', 'judge']) {
        $(`#${id}-offset`).addEventListener('input', () => {
            $('#offset-total').textContent = tr`적용 합계: ${offsetText(offsetValues())}`;
            update();
        });
        // A number field saves when committed (Enter or leaving the field).
        $(`#${id}-offset`).addEventListener('change', () => save());
        $(`#${id}-offset`).addEventListener('blur', () => save());
        $(`#${id}-offset`).addEventListener('keydown', e => { if (e.key === 'Enter') { e.preventDefault(); save(); } });
        $(`#offset-reset-${id}`).addEventListener('click', () => { $(`#${id}-offset`).value = 0; save(); });
    }

    // The title screen opens settings before any chart is selected; load the global value right away.
    load();
    return { load };
}
