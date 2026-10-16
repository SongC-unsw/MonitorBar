// Package mediakeys takes the brightness and volume keys of the keyboard
// for the monitor, on macOS: an event tap sees them, and keeps them from
// the system, once the user lets the app control the computer (System
// Settings › Privacy & Security › Accessibility).
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

// Press is a key going down, or repeating while it is held. A fine press,
// with Shift and Option held, moves a quarter step, as it does in macOS.
type Press struct {
	Key  Key
	Fine bool
}

// ErrNotTrusted means the user hasn't let the app take the keys yet.
var ErrNotTrusted = errors.New("mediakeys: the app may not take the keys")

// event is a press or release of a key of the keyboard's auxiliary control
// buttons, as macOS tells it: in the data1 of an NX_SYSDEFINED event, with
// its modifier flags.
type event struct {
	key      Key  // 0 for the other buttons
	down     bool // pressed or repeating, else released
	fine     bool
	shortcut bool // other modifiers make it macOS's, as Option opens the settings
}

// The buttons of data1 (IOKit's ev_keymap.h) and the modifier flags
// (CGEventFlags).
const (
	nxSoundUp        = 0
	nxSoundDown      = 1
	nxBrightnessUp   = 2
	nxBrightnessDown = 3
	nxMute           = 7

	flagShift   = 1 << 17
	flagControl = 1 << 18
	flagOption  = 1 << 19
	flagCommand = 1 << 20
)

func decode(data1 int, flags uint64) event {
	var e event
	switch (data1 >> 16) & 0xFFFF {
	case nxBrightnessUp:
		e.key = BrightnessUp
	case nxBrightnessDown:
		e.key = BrightnessDown
	case nxSoundUp:
		e.key = VolumeUp
	case nxSoundDown:
		e.key = VolumeDown
	case nxMute:
		e.key = Mute
	default:
		return event{}
	}
	e.down = (data1>>8)&0xFF == 0x0A // 0x0B is a release
	mods := flags & (flagShift | flagControl | flagOption | flagCommand)
	e.fine = mods == flagShift|flagOption
	e.shortcut = mods != 0 && !e.fine
	return e
}

// keys hands the presses of a tap to handle, and keeps a key from macOS
// from a press handle took to the key's release.
type keys struct {
	handle func(Press) bool
	held   map[Key]bool
}

// take reports whether to keep the event from macOS.
func (k *keys) take(e event) bool {
	switch {
	case e.key == 0:
		return false
	case !e.down:
		held := k.held[e.key]
		delete(k.held, e.key)
		return held
	case e.shortcut || !k.handle(Press{Key: e.key, Fine: e.fine}):
		return false
	}
	k.held[e.key] = true
	return true
}
