package main

import (
	"testing"
	"time"
)

// An idle app doesn't schedule one-second samples. Enabling following
// starts exactly one chain; unchanged samples don't redraw; disabling it
// or losing the backlight lets the chain stop.
func TestFollowTicksOnlyWhileFollowing(t *testing.T) {
	p, gamma := testPanel(t)
	s := p.s
	var ticks []func()
	s.after = func(d time.Duration, fn func()) {
		if d != time.Second {
			t.Errorf("a tick in %v, want a second", d)
		}
		ticks = append(ticks, fn)
	}
	s.builtin = func() (float64, bool) { return 0.6, true }
	redraws := 0
	s.redraw = func() { redraws++ }
	tick := func() {
		t.Helper()
		if len(ticks) != 1 {
			t.Fatalf("%d ticks scheduled, want 1", len(ticks))
		}
		fn := ticks[0]
		ticks = nil
		fn()
	}

	s.hasBuiltin, s.hasBrightness, s.brightness = true, true, 30
	s.follow()
	if len(ticks) != 0 {
		t.Fatal("a tick while following is off")
	}
	s.prefs.SoftBrightness = 60 // following must lift the combined software dimming
	s.prefs.FollowBuiltin = true
	s.followChanged()
	s.follow() // another display or DDC callback must not start a second chain
	for steps := 0; steps == 0 || s.easing; steps++ {
		if steps == 30 {
			t.Fatalf("still easing after %d ticks, at %v", steps, s.brightness)
		}
		tick()
	}
	if want := followTarget(0.6, s.prefs.FollowMin, s.prefs.FollowMax); s.brightness != want || redraws == 0 {
		t.Fatalf("eased to %v with %d redraws, want %v", s.brightness, redraws, want)
	}
	if s.prefs.SoftBrightness != 100 || gamma[0] != 1 {
		t.Fatal("following left the combined software dimming in place")
	}
	drawn := redraws
	tick()
	if redraws != drawn {
		t.Error("an unchanged sample redrew")
	}

	s.prefs.FollowBuiltin = false
	s.followChanged()
	tick() // the one scheduled before disabling
	if len(ticks) != 0 || s.followDue {
		t.Fatal("ticks continued after following was disabled")
	}
	s.prefs.FollowBuiltin = true
	s.follow()
	s.applyDDC(ddcReading{}) // no DDC channel: the backlight went away
	tick()
	if len(ticks) != 0 || s.followDue {
		t.Fatal("ticks continued after the backlight went away")
	}
}

func TestFollowDoesNotScheduleWithoutDisplays(t *testing.T) {
	for _, c := range []struct {
		name                      string
		hasBuiltin, hasBrightness bool
	}{
		{"no displays", false, false},
		{"no built-in", false, true},
		{"no backlight", true, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, _ := testPanel(t)
			s := p.s
			s.prefs.FollowBuiltin = true
			s.hasBuiltin, s.hasBrightness = c.hasBuiltin, c.hasBrightness
			s.builtin = func() (float64, bool) { t.Fatal("read an unavailable display"); return 0, false }
			s.after = func(time.Duration, func()) { t.Fatal("scheduled an idle tick") }
			s.follow()
			if s.followTick(s.builtin) {
				t.Error("a missing display changed the panel")
			}
		})
	}
}
