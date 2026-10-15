//go:build !windows

package play

// Ebitengine and browsers do not expose the monitor refresh rate here.
func monitorHz() float64 { return 0 }
