"""Regenerate THIRD_PARTY_NOTICES.txt from the Go modules linked into the shipped binaries.

Run from the repository root: python scripts/notices.py
Needs the Go toolchain and a populated module cache; fetches the osu!lazer licence with gh.
"""
import json, os, subprocess

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
# Binaries in the desktop zip and the servers; game also builds for the browser.
TARGETS = [('', './cmd/desktop'), ('', './cmd/game'), ('js/wasm', './cmd/game'),
           ('', './cmd/web'), ('', './cmd/multi'), ('', './cmd/archive'), ('', './cmd/chart')]
LICENSE_NAMES = ('LICENSE', 'LICENSE.txt', 'LICENSE.md', 'LICENCE', 'COPYING')
NOTICE_NAMES = ('NOTICE', 'NOTICE.txt', 'NOTICE.md')


def go(*args, env=None):
    return subprocess.run(['go', *args], cwd=ROOT, env=env, capture_output=True, text=True, check=True).stdout


def modules():
    found = {}
    for platform, pkg in TARGETS:
        env = dict(os.environ)
        if platform:
            env['GOOS'], env['GOARCH'] = platform.split('/')
        out = go('list', '-deps', '-json=Module', pkg, env=env)
        for chunk in out.replace('}\n{', '}\x00{').split('\x00'):
            m = json.loads(chunk).get('Module')
            if m and not m.get('Main'):
                found[m['Path']] = m
    return [found[k] for k in sorted(found)]


def first(directory, names):
    for n in names:
        p = os.path.join(directory, n)
        if os.path.isfile(p):
            return open(p, encoding='utf-8', errors='replace').read().strip()
    return None


def section(title, body):
    return f"{'=' * 79}\n{title}\n{'=' * 79}\n\n{body.strip()}\n\n"


def main():
    out = ["Third-party notices for Gosu\n\n"
           "Gosu itself is MIT licensed (see LICENSE). The software, fonts and sounds below are\n"
           "distributed with it under their own licences, reproduced here as they require.\n\n"]
    goroot = go('env', 'GOROOT').strip()
    out.append(section('Go standard library and runtime (https://go.dev)', open(os.path.join(goroot, 'LICENSE'), encoding='utf-8').read()))
    for m in modules():
        d = m.get('Dir') or ''
        text = first(d, LICENSE_NAMES)
        if text is None:
            raise SystemExit(f"no licence file for {m['Path']} in {d}")
        notice = first(d, NOTICE_NAMES)
        body = text + (f"\n\n--- NOTICE ---\n\n{notice}" if notice else '')
        out.append(section(f"{m['Path']} {m['Version']}", body))
        if m['Path'] == 'github.com/wailsapp/go-webview2':
            loader = first(os.path.join(d, 'webviewloader'), LICENSE_NAMES)
            out.append(section(f"{m['Path']}/webviewloader {m['Version']}", loader))
    fonts = os.path.join(ROOT, 'internal', 'typeface', 'fonts')
    out.append(section('Gmarket Sans (fonts gmarketmedium, gmarketbold; web family "Gosu UI")',
                       open(os.path.join(fonts, 'gmarket-license.txt'), encoding='utf-8').read()))
    out.append(section('Chakra Petch (font chakrabold; derived "Gosu Counter")',
                       open(os.path.join(fonts, 'chakra-license.txt'), encoding='utf-8').read()))
    out.append(section('Kenney sound packs (menu and combo-break sounds)',
                       "Interface Sounds 1.0 and UI Audio by Kenney (www.kenney.nl), released under\n"
                       "Creative Commons Zero (CC0 1.0): https://creativecommons.org/publicdomain/zero/1.0/\n"
                       "Credit is not required; it is given here. File mapping: audio/sounds/SOURCES.txt,\n"
                       "internal/play/sounds/SOURCES.txt."))
    out.append(section('Heavenly Loop (pause-menu ambience)',
                       '"Heavenly Loop" by isaiah658, released under Creative Commons Zero (CC0 1.0):\n'
                       'https://opengameart.org/content/heavenly-loop. Softened and lowered for Gosu.'))
    lazer = subprocess.run(['gh', 'api', 'repos/ppy/osu/contents/LICENCE', '-H', 'Accept: application/vnd.github.raw'],
                           capture_output=True, text=True, check=True).stdout
    out.append(section('osu!lazer (https://github.com/ppy/osu)',
                       "Layout values, timings, easings and behaviour of several screens, skins and sounds\n"
                       "follow osu!lazer and are reimplemented from its MIT-licensed source. osu!lazer's\n"
                       "art and sound assets (osu-resources, CC BY-NC 4.0) are not included.\n\n" + lazer))
    out.append(section('Design references (no code or assets included)',
                       "Quaver (https://github.com/Quaver/Quaver, MPL-2.0) and hndada/gosu\n"
                       "(https://github.com/hndada/gosu, Apache-2.0) were consulted for behaviour and default\n"
                       "values such as key mappings, editor transport order and animation timings."))
    with open(os.path.join(ROOT, 'THIRD_PARTY_NOTICES.txt'), 'w', encoding='utf-8', newline='\n') as f:
        f.write(''.join(out).rstrip() + '\n')


if __name__ == '__main__':
    main()
