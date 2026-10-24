// Package mediakeys takes the keyboard's brightness and volume keys (F1, F2,
// F10, F11, F12 on a Mac without fn) from the system, for the external display.
// On macOS this takes an event tap, which needs the Accessibility
// permission; elsewhere it is not supported.
package mediakeys

import "errors"

// Key is a brightness or volume key.
type Key int

const (
	BrightnessUp Key = iota + 1
	BrightnessDown
	VolumeUp
	VolumeDown
	Mute
)

// Handler gets a press or repeat on the main thread and says whether to
// take it from macOS. Fine means Shift+Option is held. It must return
// quickly: the system holds every key event until it does.
type Handler func(k Key, fine bool) (take bool)

// ErrNotTrusted is Start's error without the Accessibility permission.
var ErrNotTrusted = errors.New("mediakeys: no Accessibility permission")

// CGEventFlags for modifiers that change the meaning of a media key.
const (
	flagShift   = 1 << 17
	flagControl = 1 << 18
	flagOption  = 1 << 19
	flagCommand = 1 << 20
)

// keys keeps each key's ownership from its first press through its repeats
// and release. Moving the pointer or changing modifiers while holding it
// must not send macOS an orphaned repeat or release (or take one from it).
type keys struct {
	handle Handler
	held   map[Key]bool
}

func (k *keys) take(key Key, down bool, flags uint64) bool {
	if key == 0 {
		return false
	}
	if !down {
		taken := k.held[key]
		delete(k.held, key)
		return taken
	}
	mods := flags & (flagShift | flagControl | flagOption | flagCommand)
	fine := mods == flagShift|flagOption
	shortcut := mods != 0 && !fine // Option alone opens the system settings
	if taken, held := k.held[key]; held {
		if taken && !shortcut && k.handle != nil {
			k.handle(key, fine)
		}
		return taken
	}
	taken := !shortcut && k.handle != nil && k.handle(key, fine)
	if k.held == nil {
		k.held = map[Key]bool{}
	}
	k.held[key] = taken
	return taken
}
