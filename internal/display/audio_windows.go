//go:build windows

package display

// SoundToDisplay has no Windows counterpart yet: the keyboard's volume
// keys are taken on macOS only.
func SoundToDisplay() bool { return false }
