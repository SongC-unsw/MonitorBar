package main

import (
	"errors"
	"testing"
	"time"

	"github.com/binaryify/MonitorBar/internal/mediakeys"
)

func TestFineMediaKeys(t *testing.T) {
	for _, c := range []struct {
		name       string
		key        mediakeys.Key
		fine       bool
		combined   bool
		brightness float64
		volume     float64
	}{
		{"software up", mediakeys.BrightnessUp, false, false, 55, 80},
		{"software fine up", mediakeys.BrightnessUp, true, false, 51, 80},
		{"software fine down", mediakeys.BrightnessDown, true, false, 49, 80},
		{"combined fine up", mediakeys.BrightnessUp, true, true, 76, 80},
		{"combined fine down", mediakeys.BrightnessDown, true, true, 74, 80},
		{"volume up", mediakeys.VolumeUp, false, false, 50, 85},
		{"volume fine up", mediakeys.VolumeUp, true, false, 50, 81},
		{"volume fine down", mediakeys.VolumeDown, true, false, 50, 79},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, _ := testPanel(t)
			s := p.s
			s.link = nil
			s.prefs.SoftBrightness = 50
			if c.combined {
				s.hasBrightness, s.brightness, s.prefs.SoftBrightness = true, 50, 100
			}
			step := s.mediaKey(c.key, c.fine)
			if step == nil {
				t.Fatal("the display's key was left to macOS")
			}
			step()
			brightness := s.prefs.SoftBrightness
			if c.combined {
				brightness = s.combinedBrightness()
			}
			if brightness != c.brightness || s.volume != c.volume {
				t.Errorf("brightness %v, volume %v; want %v, %v", brightness, s.volume, c.brightness, c.volume)
			}
		})
	}
}

func TestMediaKeyBoundsAndUnavailableVolume(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	s.link = nil
	s.prefs.SoftBrightness = softMin
	s.mediaKey(mediakeys.BrightnessDown, true)()
	if s.prefs.SoftBrightness != softMin {
		t.Fatal("a fine press dimmed below the software minimum")
	}
	s.prefs.SoftBrightness, s.volume = 100, 100
	s.mediaKey(mediakeys.BrightnessUp, true)()
	s.mediaKey(mediakeys.VolumeUp, true)()
	if s.prefs.SoftBrightness != 100 || s.volume != 100 {
		t.Fatal("a fine press went above 100")
	}
	s.hasVolume = false
	for _, key := range []mediakeys.Key{mediakeys.VolumeUp, mediakeys.VolumeDown, mediakeys.Mute} {
		if s.mediaKey(key, true) != nil {
			t.Errorf("took key %v without a channel to the volume", key)
		}
	}
}

type fakeMediaKeys struct {
	trusted, running        bool
	requests, starts, stops int
	err                     error
	handler                 mediakeys.Handler
}

func testMediaKeys(s *store) *fakeMediaKeys {
	f := &fakeMediaKeys{}
	s.bindShortcuts = true
	s.mediaKeys = &mediaKeyTap{
		trusted: func() bool { return f.trusted },
		request: func() { f.requests++ },
		running: func() bool { return f.running },
		start: func(h mediakeys.Handler) error {
			f.starts++
			if f.err != nil {
				return f.err
			}
			f.handler, f.running = h, true
			return nil
		},
		stop: func() {
			if f.running {
				f.stops++
			}
			f.running = false
		},
	}
	return f
}

func TestMediaKeysPermissionLifecycle(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	f := testMediaKeys(s)
	var checks []func()
	s.after = func(d time.Duration, fn func()) {
		if d != 2*time.Second {
			t.Errorf("permission check in %v, want 2 seconds", d)
		}
		checks = append(checks, fn)
	}
	redraws := 0
	s.redraw = func() { redraws++ }
	s.syncMediaKeys()
	if len(checks) != 0 || f.starts != 0 {
		t.Fatal("permission polling while the setting is off")
	}
	s.setMediaKeys(true)
	s.lookForTrust() // a repeated request extends the window, not the timer chain
	if f.requests != 1 || !s.mediaKeysDenied || len(checks) != 1 {
		t.Fatalf("requests %d, denied %v, scheduled %d", f.requests, s.mediaKeysDenied, len(checks))
	}
	f.trusted = true
	check := checks[0]
	checks = nil
	check()
	if !f.running || f.handler == nil || s.mediaKeysDenied || s.lookingForTrust || len(checks) != 0 || redraws != 1 {
		t.Fatal("permission was granted but polling or the denial notice remained")
	}
	s.syncMediaKeys()
	if f.starts != 1 {
		t.Error("a running tap was started again")
	}
	s.setMediaKeys(false)
	if f.running || f.stops != 1 || s.mediaKeysDenied {
		t.Fatal("disabling the setting didn't release the keys")
	}
}

func TestMediaKeysPermissionPollingStops(t *testing.T) {
	for _, disable := range []bool{false, true} {
		name := "timeout"
		if disable {
			name = "disabled"
		}
		t.Run(name, func(t *testing.T) {
			p, _ := testPanel(t)
			s := p.s
			f := testMediaKeys(s)
			now := s.now()
			s.now = func() time.Time { return now }
			var checks []func()
			s.after = func(_ time.Duration, fn func()) { checks = append(checks, fn) }
			s.setMediaKeys(true)
			if len(checks) != 1 {
				t.Fatal("the explicit request didn't start permission polling")
			}
			// An unsuccessful check continues the same bounded chain.
			fn := checks[0]
			checks = nil
			fn()
			if len(checks) != 1 {
				t.Fatal("the permission request wasn't checked again")
			}
			if disable {
				s.setMediaKeys(false)
			} else {
				now = now.Add(trustWait)
			}
			fn = checks[0]
			checks = nil
			fn()
			if len(checks) != 0 || s.lookingForTrust {
				t.Fatal("permission checks continued after disabling or timing out")
			}
			if !disable {
				f.trusted = true // later grants are still noticed by ordinary lifecycle checks
				s.syncMediaKeys()
				if !f.running || s.mediaKeysDenied {
					t.Fatal("a grant after the polling window wasn't noticed")
				}
			}
		})
	}
}

func TestMediaKeysPermissionRevocationAndRetry(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	f := testMediaKeys(s)
	s.prefs.MediaKeys, f.trusted = true, true
	s.after = func(time.Duration, func()) { t.Fatal("an idle permission timer was scheduled") }
	s.syncMediaKeys()
	f.trusted = false
	s.syncMediaKeys()
	if !s.mediaKeysDenied || f.running || f.stops != 1 {
		t.Fatal("revoking permission didn't release the keys and report denial")
	}
	f.trusted, f.err = true, errors.New("tap refused")
	s.syncMediaKeys()
	if !s.mediaKeysDenied || f.running {
		t.Fatal("a failed tap wasn't reported")
	}
	f.err = nil
	s.syncMediaKeys()
	if s.mediaKeysDenied || !f.running {
		t.Fatal("a failed tap wasn't retried on the next lifecycle check")
	}
}
