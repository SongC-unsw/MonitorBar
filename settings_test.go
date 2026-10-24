package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testSettingsPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("AppData", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	return loadSettings().path
}

// Launching, waking, and reconnecting mark preferences for saving even
// when their values haven't changed. Neither those saves nor an unchanged
// save on the next launch should rewrite the file.
func TestSettingsWrittenOnlyWhenChanged(t *testing.T) {
	path := testSettingsPath(t)
	file := func() string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	write := func(data string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	s := loadSettings()
	s.Warmth, s.MediaKeys, s.Language = 20, true, "en"
	s.Shortcuts = map[string]string{"brightnessUp": "F2", "input-24": "Ctrl+Alt+2"}
	s.save()
	want := file()
	if !strings.Contains(want, `"warmth": 20`) {
		t.Fatalf("the change is not in the file: %s", want)
	}
	write("marker") // a write would replace this marker
	s.save()
	if got := file(); got != "marker" {
		t.Errorf("unchanged settings were written again: %s", got)
	}

	write(want)
	loaded := loadSettings()
	if !loaded.HUD || !loaded.MediaKeys || loaded.Language != "en" || loaded.Shortcuts["input-24"] != "Ctrl+Alt+2" {
		t.Fatalf("main's newer settings weren't preserved: %+v", loaded)
	}
	write("marker")
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

func TestSettingsSavesLegacySceneMigration(t *testing.T) {
	path := testSettingsPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"dayScene":"日间","nightScene":"夜间"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := loadSettings()
	s.save()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.DayScene != "day" || saved.NightScene != "night" {
		t.Fatalf("legacy scene names weren't migrated on disk: %s", data)
	}
}

func TestSettingsRetriesFailedWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	s := &settings{path: path, Warmth: 20}
	s.save()
	if s.saved != nil {
		t.Fatal("a failed write was remembered as saved")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	s.save()
	if _, err := os.Stat(path); err != nil || s.saved == nil {
		t.Fatalf("the failed save wasn't retried: %v", err)
	}
}
