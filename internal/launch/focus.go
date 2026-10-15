//go:build !windows

package launch

// Other native platforms use Ebitengine's usual focus-on-show behavior.
func AllowFocus(int) {}
func Focus()         {}
