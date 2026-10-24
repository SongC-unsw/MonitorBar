package main

import (
	"time"

	"github.com/egoist/mygo"

	"github.com/binaryify/MonitorBar/internal/mediakeys"
)

// mediaKeyTap is the system's media-key access, replaced in tests without
// asking for permissions or taking real keys.
type mediaKeyTap struct {
	trusted func() bool
	request func()
	running func() bool
	start   func(mediakeys.Handler) error
	stop    func()
}

func systemMediaKeys() *mediaKeyTap {
	if !mediakeys.Supported {
		return nil
	}
	return &mediaKeyTap{
		trusted: mediakeys.Trusted,
		request: mediakeys.RequestAccess,
		running: mediakeys.Running,
		start:   mediakeys.Start,
		stop:    mediakeys.Stop,
	}
}

// mediaKey is what a brightness or volume key does while MonitorBar takes
// the keys, or nil to leave it to macOS. Fine adjustments move one point
// instead of the usual five, on the same combined scale as the shortcuts.
func (s *store) mediaKey(k mediakeys.Key, fine bool) func() {
	delta := float64(shortcutStep)
	if fine {
		delta = 1
	}
	switch k {
	case mediakeys.BrightnessUp, mediakeys.BrightnessDown:
		if s.monitor.Builtin || s.pointerOnBuiltin() || s.prefs.FollowBuiltin && s.canFollow() {
			return nil // macOS owns the built-in display and supplies the brightness to follow
		}
		if k == mediakeys.BrightnessDown {
			delta = -delta
		}
		return func() { s.stepBrightness(delta); s.adjusted(hudBrightness) }
	case mediakeys.VolumeUp, mediakeys.VolumeDown:
		if !s.hasVolume || !s.soundToDisplay() {
			return nil // the sound plays elsewhere, at macOS's volume
		}
		if k == mediakeys.VolumeDown {
			delta = -delta
		}
		return func() { s.stepVolume(delta); s.adjusted(hudVolume) }
	case mediakeys.Mute:
		if !s.hasVolume || !s.soundToDisplay() {
			return nil
		}
		return func() { s.toggleMute(); s.adjusted(hudVolume) }
	}
	return nil
}

// onMediaKey is the event tap's handler. Defer changes (and especially HUD
// creation) until the tap returns: macOS holds key events until it does.
func (s *store) onMediaKey(k mediakeys.Key, fine bool) bool {
	step := s.mediaKey(k, fine)
	if step == nil {
		return false
	}
	go s.post(step)
	return true
}

// syncMediaKeys takes the keys or gives them back, as asked and allowed.
// Check at launch, on wake, when the panel opens, and on the existing
// screen-time timer, not on a separate one-second idle loop.
func (s *store) syncMediaKeys() {
	keys := s.mediaKeys
	if keys == nil {
		return
	}
	want := s.prefs.MediaKeys && s.bindShortcuts
	switch {
	case !want:
		keys.stop()
		s.mediaKeysDenied = false
	case !keys.trusted():
		keys.stop()
		s.mediaKeysDenied = true
	case !keys.running():
		s.mediaKeysDenied = keys.start(s.onMediaKey) != nil
	default:
		s.mediaKeysDenied = false
	}
}

const trustWait = 2 * time.Minute

// lookForTrust briefly checks more often after an explicit permission
// request. When it expires, normal lifecycle checks still notice grants
// and revocations without another idle timer.
func (s *store) lookForTrust() {
	s.trustUntil = s.now().Add(trustWait)
	if s.lookingForTrust || s.after == nil {
		return
	}
	s.lookingForTrust = true
	var check func()
	check = func() {
		if s.prefs.MediaKeys && s.mediaKeysDenied {
			denied := s.mediaKeysDenied
			s.syncMediaKeys()
			if denied != s.mediaKeysDenied {
				s.redraw()
			}
			if s.mediaKeysDenied && s.now().Before(s.trustUntil) {
				s.after(2*time.Second, check)
				return
			}
		}
		s.lookingForTrust = false
	}
	s.after(2*time.Second, check)
}

// setMediaKeys turns taking the keys on or off, asking macOS for the
// permission the first time.
func (s *store) setMediaKeys(on bool) {
	s.prefs.MediaKeys, s.dirty = on, true
	if on && s.bindShortcuts && s.mediaKeys != nil && !s.mediaKeys.trusted() {
		s.mediaKeys.request()
	}
	s.syncMediaKeys()
	if on && s.mediaKeysDenied {
		s.lookForTrust()
	}
}

// pointerOnBuiltin tells whether brightness keys should stay with macOS.
func pointerOnBuiltin() bool {
	return mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint()).Internal
}

// openAccessibility opens the Accessibility list in System Settings.
func openAccessibility() { _ = mygo.Shell.OpenExternal(mediakeys.SettingsURL) }
