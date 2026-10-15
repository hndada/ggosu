// Package preferences embeds the display setting and key test helper modules for the menu screens.
package preferences

import "embed"

//go:embed display.js audio.js playback.js scroll.js keytest.js
//go:embed submission.js
//go:embed language.js english.js
//go:embed chat.js
//go:embed profile.js
var Files embed.FS
