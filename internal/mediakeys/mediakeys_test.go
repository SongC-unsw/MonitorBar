package mediakeys

import "testing"

func TestKeyModifiers(t *testing.T) {
	for _, c := range []struct {
		name  string
		flags uint64
		take  bool
		fine  bool
	}{
		{"plain", 0, true, false},
		{"fine", flagShift | flagOption, true, true},
		{"option opens settings", flagOption, false, false},
		{"shift", flagShift, false, false},
		{"control", flagControl, false, false},
		{"command", flagCommand, false, false},
		{"fine plus command", flagShift | flagOption | flagCommand, false, false},
		{"caps lock is ignored", 1 << 16, true, false},
		{"fn is ignored", 1 << 23, true, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			called := 0
			k := keys{handle: func(key Key, fine bool) bool {
				called++
				if key != BrightnessUp || fine != c.fine {
					t.Errorf("handler got key %v, fine %v", key, fine)
				}
				return true
			}}
			if got := k.take(BrightnessUp, true, c.flags); got != c.take {
				t.Errorf("take = %v, want %v", got, c.take)
			}
			if c.take != (called == 1) {
				t.Errorf("handler called %d times", called)
			}
			if got := k.take(BrightnessUp, false, 0); got != c.take {
				t.Error("the release didn't keep the press's ownership")
			}
		})
	}
}

func TestKeyOwnershipThroughRepeatsAndRelease(t *testing.T) {
	called, accepting := 0, true
	k := keys{handle: func(_ Key, _ bool) bool { called++; return accepting }}
	if !k.take(BrightnessDown, true, 0) || !k.take(BrightnessDown, true, flagShift|flagOption) {
		t.Fatal("a press or fine repeat reached macOS")
	}
	accepting = false // the pointer moved onto the built-in display
	if !k.take(BrightnessDown, true, 0) || !k.take(BrightnessDown, false, flagCommand) {
		t.Fatal("a captured key sent macOS an orphaned repeat or release")
	}
	if len(k.held) != 0 || called != 3 {
		t.Fatalf("held %v, called %d", k.held, called)
	}
	if k.take(BrightnessDown, true, 0) {
		t.Fatal("a new press wasn't left to macOS")
	}
	accepting = true // moving back doesn't steal macOS's held key
	if k.take(BrightnessDown, true, 0) || k.take(BrightnessDown, false, 0) {
		t.Fatal("a forwarded press lost its repeat or release")
	}
	if called != 4 {
		t.Errorf("a forwarded repeat called the handler: %d calls", called)
	}
	if !k.take(BrightnessDown, true, 0) || !k.take(BrightnessDown, false, 0) {
		t.Fatal("a later press wasn't reconsidered after release")
	}
}

func TestModifiersChangedWhileHeld(t *testing.T) {
	called := 0
	k := keys{handle: func(Key, bool) bool { called++; return true }}
	if !k.take(VolumeDown, true, 0) || !k.take(VolumeDown, true, flagOption) || !k.take(VolumeDown, false, flagOption) {
		t.Fatal("changing modifiers leaked a captured key to macOS")
	}
	if called != 1 {
		t.Error("a shortcut-modified repeat adjusted the monitor")
	}
	if k.take(VolumeDown, true, flagOption) || k.take(VolumeDown, true, 0) || k.take(VolumeDown, false, 0) {
		t.Fatal("a system shortcut lost its repeat or release after dropping Option")
	}
}

func TestKeysWithoutHandler(t *testing.T) {
	k := keys{}
	if k.take(VolumeUp, true, 0) || k.take(VolumeUp, false, 0) || k.take(VolumeUp, false, 0) || k.take(0, true, 0) {
		t.Fatal("an unhandled key was taken")
	}
}
