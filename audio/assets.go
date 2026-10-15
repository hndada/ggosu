// Package audio embeds the song preview, time stretch, menu sound and volume overlay modules for the menu screens.
package audio

import "embed"

//go:embed preview.js beat.js stretch.js sound.js volume.js sounds/*.ogg
var Files embed.FS
