// Package build provides assets shared by desktop builds and native windows.
package build

import _ "embed"

// Icon is the Gosu application icon in PNG format.
//
//go:embed appicon.png
var Icon []byte
