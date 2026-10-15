import { tr } from '../../preferences/language.js';
// Song folders tab: extra local folders the song select scans beside the songs folder, such as an
// O2Mania Music\o2jam folder. A change rescans the song list (main.js listens for ggosu:songs-changed).
const $ = s => document.querySelector(s);
let busy = false;

function render(list) {
    $('#folder-list').replaceChildren(...list.map(path => {
        const row = document.createElement('div'), name = document.createElement('span'), remove = document.createElement('button');
        name.textContent = path; remove.type = 'button'; remove.textContent = tr('삭제');
        remove.addEventListener('click', () => run(() => window.go.main.App.RemoveSongFolder(path)));
        row.append(name, remove);
        return row;
    }));
}

async function run(change) {
    if (busy) return;
    busy = true; $('#folder-status').textContent = '';
    try {
        render(await change());
        window.dispatchEvent(new Event('ggosu:songs-changed'));
        return true;
    } catch (e) { $('#folder-status').textContent = e?.message ?? String(e); return false; }
    finally { busy = false; }
}

export function mount() {
    const app = window.go?.main?.App;
    // The browser build serves the server's own songs folder only.
    if (typeof app?.SongFolders !== 'function') return;
    $('#folder-form').hidden = false;
    $('#folder-pick').hidden = typeof app.PickSongFolder !== 'function';
    app.SongFolders().then(render).catch(e => { $('#folder-status').textContent = e?.message ?? String(e); });
    $('#folder-form').addEventListener('submit', async e => {
        e.preventDefault();
        if (await run(() => app.AddSongFolder($('#folder-path').value))) $('#folder-path').value = '';
    });
    $('#folder-pick').addEventListener('click', async () => {
        try {
            const path = await app.PickSongFolder();
            if (path) run(() => app.AddSongFolder(path));
        } catch (e) { $('#folder-status').textContent = e?.message ?? String(e); }
    });
}
