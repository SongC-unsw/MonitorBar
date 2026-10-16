package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

// settings are the choices kept between launches, in a JSON file.
type settings struct {
	SoftBrightness float64 `json:"softBrightness"` // gamma %, 20...100
	Warmth         float64 `json:"warmth"`         // 0...100
	Tab            string  `json:"tab"`
	NetworkHost    string  `json:"networkHost"` // the display found last time
	LaunchedBefore bool    `json:"launchedBefore"`

	// Switching to the day and night scenes at set times.
	AutoSwitch bool   `json:"autoSwitch"`
	DayStart   string `json:"dayStart"` // "07:00"
	NightStart string `json:"nightStart"`
	DayScene   string `json:"dayScene"`
	NightScene string `json:"nightScene"`
	AutoPeriod string `json:"autoPeriod"` // the period whose scene was applied last

	// The keyboard's brightness and volume keys adjust the monitor (macOS).
	MediaKeys bool `json:"mediaKeys"`

	// Following the built-in display's brightness over DDC/CI (MacBooks).
	FollowBuiltin bool    `json:"followBuiltin"`
	FollowMin     float64 `json:"followMin"` // the external backlight at the built-in's darkest, %
	FollowMax     float64 `json:"followMax"` // and at its brightest
	path          string
	saved         []byte // what the file holds, as save writes it
}

// dataDir is where MonitorBar keeps its files.
func dataDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "MonitorBar")
}

func loadSettings() *settings {
	s := &settings{SoftBrightness: 100, DayStart: "07:00", NightStart: "19:00", DayScene: "日间", NightScene: "夜间",
		FollowMin: 10, FollowMax: 100,
		path: filepath.Join(dataDir(), "settings.json")}
	if data, err := os.ReadFile(s.path); err == nil && json.Unmarshal(data, s) == nil {
		s.saved, _ = json.MarshalIndent(s, "", "  ")
	}
	return s
}

// save writes the settings when they differ from the file's: most changes
// marked for saving, such as the dimming reapplied after a display change,
// leave them as they were.
func (s *settings) save() {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil || bytes.Equal(data, s.saved) {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.path), 0o755)
	if os.WriteFile(s.path, data, 0o644) == nil {
		s.saved = data
	}
}
