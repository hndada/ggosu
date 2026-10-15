import { language, setLanguage, storedLanguage, tr, translate } from '/preferences/language.js';
import { paint } from '/settings/choice.js';

export async function mount() {
    const choice = document.querySelector('#language-choice');
    const status = document.querySelector('#language-status');
    const app = window.go?.main?.App;
    const apply = value => {
        setLanguage(value);
        translate(document);
        choice.value = value;
        paint(choice);
        window.dispatchEvent(new Event('ggosu:language-changed'));
    };
    choice.disabled = true;
    try { apply(app?.Language ? await app.Language() : storedLanguage()); }
    catch (e) { status.textContent = tr`설정 읽기 실패: ${e?.message ?? e}`; }
    finally { choice.disabled = false; }
    document.querySelector('#language-settings').addEventListener('submit', e => e.preventDefault());
    choice.addEventListener('change', async () => {
        const previous = language();
        choice.disabled = true;
        try {
            const value = app?.SaveLanguage ? await app.SaveLanguage(choice.value) : null;
            if (!value) throw new Error('Language storage is unavailable');
            apply(value);
            status.textContent = tr('언어를 변경했습니다.');
        } catch (e) {
            choice.value = previous;
            status.textContent = tr`언어 저장 실패: ${e?.message ?? e}`;
        } finally { choice.disabled = false; paint(choice); }
    });
}
