package mediakeys

import "testing"

// data1 builds the data1 of a media key's event: the button, then its
// state, 0x0A down or 0x0B up.
func data1(button, state int) int { return button<<16 | state<<8 }

func TestDecode(t *testing.T) {
	for _, c := range []struct {
		data1 int
		flags uint64
		want  event
	}{
		{data1(nxBrightnessUp, 0x0A), 0, event{key: BrightnessUp, down: true}},
		{data1(nxBrightnessDown, 0x0B), 0, event{key: BrightnessDown}},
		{data1(nxSoundUp, 0x0A) | 1, 0, event{key: VolumeUp, down: true}}, // a repeat
		{data1(nxSoundDown, 0x0A), flagShift | flagOption, event{key: VolumeDown, down: true, fine: true}},
		{data1(nxMute, 0x0A), flagOption, event{key: Mute, down: true, shortcut: true}}, // opens Sound settings
		{data1(nxSoundUp, 0x0A), flagCommand, event{key: VolumeUp, down: true, shortcut: true}},
		{data1(16, 0x0A), 0, event{}}, // play
	} {
		if got := decode(c.data1, c.flags); got != c.want {
			t.Errorf("decode(%#x, %#x) = %+v, want %+v", c.data1, c.flags, got, c.want)
		}
	}
}

// A key taken on its press is kept from macOS until it is released; a key
// left on its press, or with a shortcut's modifiers, reaches macOS.
func TestTake(t *testing.T) {
	var pressed []Press
	taking := true
	k := keys{held: map[Key]bool{}, handle: func(p Press) bool {
		pressed = append(pressed, p)
		return taking
	}}
	down, up := event{key: BrightnessUp, down: true}, event{key: BrightnessUp}
	if !k.take(down) || !k.take(down) || !k.take(up) {
		t.Error("a taken key, its repeat or its release reached macOS")
	}
	taking = false
	if k.take(down) || k.take(up) {
		t.Error("a key left by handle was kept from macOS")
	}
	if k.take(event{key: Mute, down: true, shortcut: true}) || k.take(event{}) {
		t.Error("a shortcut or another button was kept from macOS")
	}
	if len(pressed) != 3 || pressed[0] != (Press{Key: BrightnessUp}) {
		t.Errorf("handle got %v, want the 3 presses without modifiers", pressed)
	}
}
