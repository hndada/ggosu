import { tr } from '../../preferences/language.js';
// Starting a chart, after osu!lazer: the chosen row flashes (Panel.Activated), song select
// fades out (SongSelect.OnSuspending) and, while the game window loads, the background
// brightens and a chart card grows in (PlayerLoader). Timings live in theme.css.
import { levelAt } from '/songlist/level.js';

const $ = s => document.querySelector(s);
const frame = () => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r)));

async function flash(row) {
    if (!row) return;
    row.classList.add('activated');
    await frame();
    row.classList.replace('activated', 'released');
    setTimeout(() => row.classList.remove('released'), 1000);
}

// leave starts the effect for entry and returns the calls that end it.
export function leave(entry, rate = 1) {
    const select = $('#songselect'), bg = $('#backdrop'), card = $('#loader');
    flash($('#song-list li[aria-selected="true"]'));
    select.classList.add('leaving');
    bg.classList.add('loading');
    $('#loader-title').textContent = entry.title || '';
    $('#loader-artist').textContent = entry.artist || tr('아티스트 미표기');
    const level = levelAt(entry, rate).value;
    $('#loader-chart').textContent = [entry.difficulty, `${entry.lanes}K`, level != null ? `Lv ${level.toFixed(1)}` : '', rate !== 1 ? `${rate}x` : ''].filter(Boolean).join(' · ');
    card.hidden = false;
    void card.offsetWidth;
    card.classList.add('shown');
    // The card and the brighter background go at once, under the game window or on failure.
    const away = () => {
        card.classList.remove('shown'); card.hidden = true;
        bg.classList.add('instant'); bg.classList.remove('loading'); void bg.offsetWidth; bg.classList.remove('instant');
    };
    return {
        ready: away,
        cancel() { away(); select.classList.remove('leaving'); },
    };
}

// back undoes the song select fade once the game has closed.
export function back() { $('#songselect').classList.remove('leaving'); }

// enter fades the results in like lazer's ResultsScreen.OnEntering: the screen over 250 ms
// while the background dims to half.
export function enter() {
    const result = $('#result');
    result.classList.add('entering');
    $('#backdrop').classList.add('result-dim');
    void result.offsetWidth;
    result.classList.remove('entering');
}

// undim ends the results dim when the results screen closes.
export function undim() { $('#backdrop').classList.remove('result-dim'); }
