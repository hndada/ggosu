// Package scenes embeds the menu screens: song select, settings, editor and lobby.
// Assets serves them from the root and each non-screen module folder under its own name.
package scenes

import (
	"embed"
	"io/fs"
	"strings"

	"ggosu/audio"
	"ggosu/chartedit"
	"ggosu/internal/typeface"
	"ggosu/motion"
	"ggosu/preferences"
	"ggosu/songlist"
)

//go:embed index.html main.js controlsounds.js cursor.js notification.js browser.js layout.css theme.css
//go:embed songselect/catalog.js songselect/shortcuts.js songselect/records.js songselect/range.js songselect/analysis.js songselect/info.js songselect/confirm.js
//go:embed songselect/download.js songselect/pulse.js
//go:embed settings/preferences.js settings/playback.js settings/offsets.js settings/keytest.js settings/tabs.js settings/slider.js settings/choice.js settings/panel.js settings/folders.js settings/skins/*.png
//go:embed settings/mods.js
//go:embed settings/language.js
//go:embed settings/submission.js
//go:embed settings/chat.js
//go:embed settings/hud.html settings/hudpreview.js settings/hudlayout.js
//go:embed editor/editor.js editor/sync.js
//go:embed lobby/lobby.js lobby/lobby.css lobby/chat.js lobby/chat.css
//go:embed medley/medley.js
var files embed.FS

var modules = map[string]fs.FS{
	"songlist":    songlist.Files,
	"chartedit":   chartedit.Files,
	"audio":       audio.Files,
	"preferences": preferences.Files,
	"motion":      motion.Files,
	"ui":          typeface.Files,
}

var Assets fs.FS = mount{files, modules}

// Paths lists every file Assets serves, for servers that register routes one by one.
func Paths() ([]string, error) {
	var out []string
	add := func(fsys fs.FS, prefix string) error {
		return fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				out = append(out, prefix+p)
			}
			return err
		})
	}
	if err := add(files, ""); err != nil {
		return nil, err
	}
	for dir, fsys := range modules {
		if err := add(fsys, dir+"/"); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// mount resolves "<folder>/<file>" in that folder's FS and everything else in root.
type mount struct {
	root fs.FS
	dirs map[string]fs.FS
}

func (m mount) Open(name string) (fs.File, error) {
	if dir, rest, ok := strings.Cut(name, "/"); ok {
		if f, ok := m.dirs[dir]; ok {
			return f.Open(rest)
		}
	}
	return m.root.Open(name)
}
