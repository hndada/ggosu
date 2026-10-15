import { tr } from '../../preferences/language.js';
const $ = s => document.querySelector(s);
export function mount() {
    const app = window.go?.main?.App;
    if (typeof app?.ScoreServer !== 'function') return;
    $('#score-server-form').hidden = false;
    app.ScoreServer().then(server => { $('#score-server-url').value = server; }).catch(e => { $('#score-server-status').textContent = e.message ?? String(e); });
    $('#score-server-form').addEventListener('submit', async e => {
        e.preventDefault();
        const button = $('#score-server-save');
        button.disabled = true;
        try {
            $('#score-server-url').value = await app.SaveScoreServer($('#score-server-url').value.trim());
            $('#score-server-status').textContent = tr('저장했습니다.');
        } catch (e) { $('#score-server-status').textContent = e.message ?? String(e); }
        finally { button.disabled = false; }
    });
}
