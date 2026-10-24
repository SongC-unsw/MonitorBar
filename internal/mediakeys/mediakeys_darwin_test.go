//go:build darwin

package mediakeys

import (
	"runtime"
	"testing"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// Exercise the actual event decoder without creating a tap, requesting
// permission, or posting any events to the system.
func testEvents(t *testing.T, h Handler) {
	t.Helper()
	runtime.LockOSThread()
	load()
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	old := pressed
	pressed = keys{handle: h}
	t.Cleanup(func() {
		pressed = old
		pool.Send(objc.RegisterName("drain"))
		runtime.UnlockOSThread()
	})
}

func mediaKeyEvent(t *testing.T, button, state int, flags uint64) uintptr {
	t.Helper()
	type point struct{ X, Y float64 }
	ns := objc.ID(objc.GetClass("NSEvent")).Send(
		objc.RegisterName("otherEventWithType:location:modifierFlags:timestamp:windowNumber:context:subtype:data1:data2:"),
		uint64(systemDefined), point{}, flags, float64(0), int64(0), objc.ID(0), int16(auxControlButton), int64(button<<16|state<<8), int64(-1))
	if ns == 0 {
		t.Fatal("no NSEvent")
	}
	return objc.Send[uintptr](ns, objc.RegisterName("CGEvent"))
}

func TestTappedMediaKeys(t *testing.T) {
	var got []struct {
		key  Key
		fine bool
	}
	testEvents(t, func(k Key, fine bool) bool {
		got = append(got, struct {
			key  Key
			fine bool
		}{k, fine})
		return k != Mute
	})
	for _, c := range []struct {
		button, state int
		flags         uint64
		take          bool
	}{
		{2, 0xA, 0, true}, // brightness up
		{2, 0xB, flagCommand, true},
		{1, 0xA, flagShift | flagOption, true}, // fine volume down
		{1, 0xB, 0, true},
		{0, 0xA, flagOption, false}, // system shortcut
		{0, 0xB, 0, false},
		{7, 0xA, 0, false}, // declined by the handler
		{7, 0xB, 0, false},
		{16, 0xA, 0, false}, // play is never taken
	} {
		event := mediaKeyEvent(t, c.button, c.state, c.flags)
		if result := tapped(0, systemDefined, event, 0); (result == 0) != c.take {
			t.Errorf("button %d, state %x: take %v, want %v", c.button, c.state, result == 0, c.take)
		}
	}
	if len(got) != 3 || got[0].key != BrightnessUp || got[0].fine || got[1].key != VolumeDown || !got[1].fine || got[2].key != Mute {
		t.Errorf("handler got %+v", got)
	}
}

// Preserve main's support for keyboards that send plain keycodes 144/145
// instead of NX_SYSDEFINED events, with the same modifier handling.
func TestTappedPlainBrightnessKeys(t *testing.T) {
	calls := 0
	testEvents(t, func(k Key, fine bool) bool {
		calls++
		if k != BrightnessUp || !fine {
			t.Errorf("handler got key %v, fine %v", k, fine)
		}
		return true
	})
	lib, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatal(err)
	}
	defer purego.Dlclose(lib)
	var create func(source uintptr, key uint16, down bool) uintptr
	var setFlags func(event uintptr, flags uint64)
	var release func(event uintptr)
	purego.RegisterLibFunc(&create, lib, "CGEventCreateKeyboardEvent")
	purego.RegisterLibFunc(&setFlags, lib, "CGEventSetFlags")
	purego.RegisterLibFunc(&release, lib, "CFRelease")
	for _, c := range []struct {
		key   uint16
		down  bool
		flags uint64
		take  bool
	}{
		{144, true, flagShift | flagOption, true},
		{144, false, 0, true},
		{145, true, flagCommand, false},
		{145, false, 0, false},
		{122, true, 0, false}, // a standard F1 belongs to the shortcut system
	} {
		event := create(0, c.key, c.down)
		if event == 0 {
			t.Fatal("no CGEvent")
		}
		setFlags(event, c.flags)
		kind := uint32(keyUp)
		if c.down {
			kind = keyDown
		}
		result := tapped(0, kind, event, 0)
		release(event)
		if (result == 0) != c.take {
			t.Errorf("plain key %d: take %v, want %v", c.key, result == 0, c.take)
		}
	}
	if calls != 1 {
		t.Errorf("handler called %d times, want 1", calls)
	}
}
