// Package chartedit embeds the chart editing model, onset analysis and metronome modules for the menu screens.
package chartedit

import "embed"

//go:embed edit.js field.js beatfit.js beatfitworker.js sync.js
var Files embed.FS
