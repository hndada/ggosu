// Package songlist embeds song list search, layout, range and record statistics modules for the menu screens.
package songlist

import "embed"

//go:embed search.js list.js range.js analysis.js level.js
var Files embed.FS
