import { tr } from '../../preferences/language.js';
const $ = s => document.querySelector(s);
const active = s => ['download', 'extract', 'verify'].includes(s?.phase);
const bytes = n => n >= 1024 ** 3 ? `${(n / 1024 ** 3).toFixed(2)} GiB` : n >= 1024 ** 2 ? `${(n / 1024 ** 2).toFixed(1)} MiB` : `${Math.round(n / 1024)} KiB`;

export function mount({ busy, installed, text }) {
    const app = () => window.go?.main?.App, dialog = $('#downloads');
    let info = { canInstall: false, canConfigure: false }, page = null, offset = 0, searching = false, installing = false, ticket = 0, stateTicket = 0, timer = 0, failures = 0, handled = '';
    const note = value => { $('#download-status').textContent = value; };
    const update = () => {
        $('#download-search').disabled = searching;
        $('#download-prev').disabled = searching || offset === 0;
        $('#download-next').disabled = searching || !page?.more;
        $('#download-cancel').hidden = !installing;
        $('#download-archive-row').hidden = $('#download-catalog').value !== 'archive';
        $('#download-server').readOnly = !info.canConfigure;
        $('#download-save-server').hidden = !info.canConfigure;
        $('#download-limit').hidden = info.canInstall !== false;
        for (const b of $('#download-list').querySelectorAll('button')) b.disabled = installing || busy() || b.dataset.unavailable === 'true';
    };
    function render() {
        $('#download-list').replaceChildren(...(page?.items ?? []).map(p => {
            const li = document.createElement('li'), title = document.createElement('strong'), details = document.createElement('span'), button = document.createElement('button');
            title.textContent = `${p.artist ? p.artist + ' — ' : ''}${p.title}`;
            details.textContent = [p.source, p.creator, p.keys?.map(n => n + 'K').join(' · '), p.status, p.size > 0 ? bytes(p.size) : '', p.reason].filter(Boolean).join(' · ');
            button.type = 'button'; button.textContent = p.installed ? tr('설치됨') : tr('다운로드·설치');
            button.dataset.unavailable = String(!p.available || p.installed || info.canInstall === false);
            button.addEventListener('click', () => start(p)); li.append(title, details, button); return li;
        }));
        $('#download-summary').textContent = page ? tr`${page.total >= 0 ? page.total.toLocaleString() + tr('개 중 ') : ''}${page.items.length}개 표시 · ${Math.floor(offset / 20) + 1}페이지` : '';
        update();
    }
    async function search(reset = true) {
        if (reset) offset = 0;
        searching = true; const mine = ++ticket; update(); note(tr('검색 중…'));
        try {
            const p = await app().SearchSongs({ catalog: $('#download-catalog').value, text: $('#download-query').value, keys: Number($('#download-keys').value), offset });
            if (mine !== ticket) return;
            page = p; render(); note(p.items.length ? '' : p.more ? tr('이 페이지에는 조건에 맞는 곡이 없습니다. 다음 페이지를 확인해 주세요.') : tr('검색 결과가 없습니다.'));
        } catch (e) { if (mine === ticket) { page = null; render(); note(tr`곡 검색 실패: ${e?.message ?? e}`); } }
        finally { if (mine === ticket) { searching = false; update(); } }
    }
    function showState(s) {
        installing = active(s);
        const progress = $('#download-progress'); progress.hidden = s.phase === 'idle';
        if (s.phase === 'download' && s.total > 0) { progress.max = s.total; progress.value = s.received; }
        else progress.removeAttribute('value');
        $('#download-transfer').textContent = s.phase === 'download' ? `${s.title} · ${bytes(s.received)}${s.total > 0 ? ' / ' + bytes(s.total) : ''}` : s.phase === 'extract' ? tr`압축 해제 · ${s.files}개 파일` : s.phase === 'verify' ? tr('채보·음원 확인 중…') : '';
        if (s.phase === 'failed' || s.phase === 'cancelled') note(s.error);
        update();
    }
    async function poll() {
        clearTimeout(timer); timer = 0;
        const mine = ++stateTicket;
        try {
            const s = await app().InstallState(); if (mine !== stateTicket) return; failures = 0; showState(s);
            if (s.phase === 'done' && s.result) {
                const key = `${s.result.source}/${s.result.id}`;
                if (handled !== key) {
                    handled = key;
                    for (const p of page?.items ?? []) if (p.source === s.result.source && p.id === s.result.id) p.installed = true;
                    render(); note(tr`${s.result.already ? tr('이미 설치된 곡입니다') : tr('설치 완료')} · ${s.result.charts}개 채보`);
                    await installed(s.result);
                }
            }
            if (installing) timer = setTimeout(() => { void poll(); }, 250);
        } catch (e) {
            if (mine !== stateTicket) return;
            note(tr`설치 상태 확인 실패: ${e?.message ?? e}`);
            if (++failures < 5 && installing) timer = setTimeout(() => { void poll(); }, 1000);
            else { installing = false; update(); }
        }
    }
    async function start(p) {
        if (installing || busy()) return;
        installing = true; ++stateTicket; update();
        try { handled = ''; const s = await app().StartInstall(p.key); showState(s); note(''); void poll(); }
        catch (e) { installing = false; update(); note(tr`설치 시작 실패: ${e?.message ?? e}`); }
    }
    $('#download-form').addEventListener('submit', e => { e.preventDefault(); void search(); });
    $('#download-catalog').addEventListener('change', () => { update(); void search(); });
    $('#download-keys').addEventListener('change', () => { void search(); });
    $('#download-prev').addEventListener('click', () => { offset = Math.max(0, offset - 20); void search(false); });
    $('#download-next').addEventListener('click', () => { offset += 20; void search(false); });
    $('#download-close').addEventListener('click', () => dialog.close());
    $('#download-cancel').addEventListener('click', async () => { try { await app().CancelInstall(); void poll(); } catch (e) { note(e?.message ?? String(e)); } });
    $('#download-save-server').addEventListener('click', async () => {
        try { info.archive = await app().SaveArchiveServer($('#download-server').value.trim()); $('#download-server').value = info.archive; void search(); }
        catch (e) { note(e?.message ?? String(e)); }
    });
    dialog.addEventListener('keydown', e => e.stopPropagation());
    $('#download-list').addEventListener('keydown', e => {
        if (!['ArrowUp', 'ArrowDown', 'Home', 'End'].includes(e.key)) return;
        const buttons = [...$('#download-list').querySelectorAll('button')].filter(b => !b.disabled), at = buttons.indexOf(document.activeElement);
        if (!buttons.length) return;
        e.preventDefault(); const next = e.key === 'Home' ? 0 : e.key === 'End' ? buttons.length - 1 : Math.max(0, Math.min(buttons.length - 1, at + (e.key === 'ArrowDown' ? 1 : -1)));
        buttons[next].focus();
    });
    window.addEventListener('pagehide', () => { clearTimeout(timer); ++ticket; ++stateTicket; }, { once: true });
    return { async open() {
        if (busy()) return;
        if (typeof app()?.SearchSongs !== 'function') { note(tr('현재 실행 환경에서는 곡 받기를 사용할 수 없습니다.')); return; }
        if (!dialog.open) dialog.showModal();
        if (!$('#download-query').value) $('#download-query').value = text();
        try { info = await app().DownloadInfo(); $('#download-server').value = info.archive; update(); if (info.canInstall) void poll(); }
        catch (e) { note(e?.message ?? String(e)); }
        $('#download-query').focus(); if (!page) void search();
    } };
}
