// Package typeface embeds the fonts shared by the native game and menu screens.
package typeface

import "embed"

// Files includes local fonts and their redistribution notices. Web and native
// play share the same counter; no external font request is needed at runtime.
//
//go:embed fonts/*.ttf fonts/*.woff2 fonts/*license.txt
var Files embed.FS
