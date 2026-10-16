package main

import (
	"os"
	"strings"
	"testing"
)

// Settings are written when they change, not each time they are saved:
// launching, display changes and reconnecting mark them for saving.
func TestSettingsWrittenOnlyWhenChanged(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)    // where os.UserConfigDir is on macOS
	t.Setenv("AppData", dir) // and on Windows
	path := loadSettings().path
	file := func() string {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	// mark puts a marker in the file, which a write would replace.
	mark := func() {
		if err := os.WriteFile(path, []byte("marker"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	s := loadSettings() // no file yet
	s.Warmth = 20
	s.save()
	want := file()
	if !strings.Contains(want, `"warmth": 20`) {
		t.Fatalf("the change is not in the file: %s", want)
	}
	mark()
	s.save()
	if got := file(); got != "marker" {
		t.Errorf("unchanged settings were written again: %s", got)
	}

	// The next launch reads them, and saves them unchanged.
	if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded := loadSettings()
	mark()
	loaded.save()
	if got := file(); got != "marker" {
		t.Errorf("settings loaded and saved unchanged were written: %s", got)
	}
	loaded.Warmth = 30
	loaded.save()
	if got := file(); !strings.Contains(got, `"warmth": 30`) {
		t.Errorf("the change is not in the file: %s", got)
	}
}
