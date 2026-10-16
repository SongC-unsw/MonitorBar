//go:build darwin

package mediakeys

import (
	"testing"

	"github.com/ebitengine/purego/objc"
)

// mediaKeyEvent makes the CGEvent macOS sends for a media key: an
// NX_SYSDEFINED event of the aux control buttons.
func mediaKeyEvent(t *testing.T, button, state int, flags uint64) uintptr {
	t.Helper()
	type nsPoint struct{ X, Y float64 }
	ns := objc.ID(objc.GetClass("NSEvent")).Send(
		objc.RegisterName("otherEventWithType:location:modifierFlags:timestamp:windowNumber:context:subtype:data1:data2:"),
		uint64(systemDefined), nsPoint{}, flags, float64(0), int64(0), objc.ID(0), int16(auxControlButtons), int64(data1(button, state)), int64(-1))
	if ns == 0 {
		t.Fatal("no NSEvent")
	}
	return objc.Send[uintptr](ns, objc.RegisterName("CGEvent"))
}

// The tap's callback reads a key from macOS's event and keeps it from
// macOS while handle takes it; other events pass. Nothing is posted.
func TestTapped(t *testing.T) {
	if err := load(); err != nil {
		t.Fatal(err)
	}
	var pressed []Press
	current = &tap{keys: keys{held: map[Key]bool{}, handle: func(p Press) bool {
		pressed = append(pressed, p)
		return p.Key != Mute
	}}}
	defer func() { current = nil }()

	up := mediaKeyEvent(t, nxBrightnessUp, 0x0A, 0)
	if tapped(0, systemDefined, up, 0) != 0 {
		t.Error("a brightness key handle took reached macOS")
	}
	if release := mediaKeyEvent(t, nxBrightnessUp, 0x0B, 0); tapped(0, systemDefined, release, 0) != 0 {
		t.Error("the release of a key handle took reached macOS")
	}
	fine := mediaKeyEvent(t, nxSoundDown, 0x0A, flagShift|flagOption)
	if tapped(0, systemDefined, fine, 0) != 0 {
		t.Error("a fine volume key handle took reached macOS")
	}
	if mute := mediaKeyEvent(t, nxMute, 0x0A, 0); tapped(0, systemDefined, mute, 0) != mute {
		t.Error("a key handle left was kept from macOS")
	}
	if play := mediaKeyEvent(t, 16, 0x0A, 0); tapped(0, systemDefined, play, 0) != play {
		t.Error("the play key was kept from macOS")
	}
	want := []Press{{Key: BrightnessUp}, {Key: VolumeDown, Fine: true}, {Key: Mute}}
	if len(pressed) != len(want) {
		t.Fatalf("handle got %v, want %v", pressed, want)
	}
	for i := range want {
		if pressed[i] != want[i] {
			t.Errorf("press %d = %+v, want %+v", i, pressed[i], want[i])
		}
	}
}
