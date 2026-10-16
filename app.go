package main

import (
	"fmt"
	"log"
	"math"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/internal/usage"
)

// chrome is the panel's height besides the scrolling content: arrow,
// header, tabs and footer, with a margin from the screen's edge.
const chrome = 230

// The level a key of the keyboard set shows under the menu bar icon for a
// moment, as macOS shows its own: in a pill of levelWidth by levelHeight,
// for levelFor after the last press.
const (
	levelWidth  = 236
	levelHeight = 46
	levelFor    = 1500 * time.Millisecond
)

// app is the menu bar (notification area) icon and the panel it opens.
type app struct {
	s        *store
	panel    *panel
	tray     *mygo.Tray
	flyout   *mygo.Window // the open panel, or nil
	levelWin *mygo.Window // the level a key set, or nil
	level    keyLevel
}

// keyLevel is what the level window shows.
type keyLevel struct {
	icon  string
	value float64 // 0...100
	shown int     // counts the showings: only the last one's timer closes it
}

func (a *app) ready(show bool) {
	a.s = newStore()
	a.s.redraw = func() {
		if a.flyout != nil {
			a.flyout.Invalidate()
		}
	}
	a.s.level = func(icon string, value float64) {
		// After the key's event is answered: a window takes a moment to make.
		go mygo.RunOnMain(func() { a.showLevel(icon, value) })
	}
	a.panel = &panel{s: a.s, height: 600, maxContent: 640, onHeight: a.resize}

	var err error
	a.tray, err = mygo.NewTray(mygo.TrayOptions{Icon: trayIcon(), IconIsTemplate: true, ToolTip: "MonitorBar"})
	if err != nil {
		log.Fatal("tray: ", err)
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "打开 MonitorBar", Click: func(*mygo.MenuItem, *mygo.Window) { a.open() }},
		mygo.Separator(),
		{Label: "退出 MonitorBar", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
	if runtime.GOOS == "linux" { // AppIndicator reports no clicks
		a.tray.SetMenu(menu)
	} else {
		a.tray.OnClick(a.toggle)
		a.tray.OnRightClick(func() {
			a.close()
			a.tray.SetMenu(menu) // for this popup only, so clicks keep toggling the panel
			defer a.tray.SetMenu(nil)
			a.tray.PopUpMenu()
		})
	}

	changed := func() {
		a.s.displaysChanged()
		a.s.redraw()
	}
	// Launching the app again (Start menu, Finder) opens its panel.
	mygo.App.OnSecondInstance(func([]string, string) { a.open() })
	mygo.Screen.OnDisplaysChanged(changed)
	mygo.Power.OnResume(changed)
	go func() {
		for range time.Tick(usage.Interval) {
			mygo.RunOnMain(func() {
				a.s.recordUsage()
				a.s.redraw()
			})
		}
	}()
	// Screen time is written every few minutes: write the rest before the
	// computer sleeps (it may not wake) and as the app quits.
	mygo.Power.OnSuspend(a.s.tracker.Flush)
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		a.s.save()
		a.s.tracker.Flush()
		display.RestoreGamma()
		if a.s.ddc != nil {
			a.s.ddc.Close()
		}
		a.tray.Destroy()
	})

	a.s.detect()
	a.s.autoCheck()
	a.s.tapKeys(false)
	go func() { // a newer version: soon after launch, then every few hours
		time.Sleep(10 * time.Second)
		for {
			mygo.RunOnMain(func() { a.s.checkUpdate(false) })
			time.Sleep(6 * time.Hour)
		}
	}()
	// Open the panel on the first launch (or with --show) so new users find the icon.
	if show || !a.s.prefs.LaunchedBefore {
		a.s.prefs.LaunchedBefore, a.s.dirty = true, true
		time.AfterFunc(300*time.Millisecond, func() { mygo.RunOnMain(a.open) })
	}
}

func (a *app) toggle() {
	if a.flyout != nil {
		a.close()
	} else {
		a.open()
	}
}

func (a *app) open() {
	if a.flyout != nil {
		return
	}
	if a.levelWin != nil { // the panel shows the level itself
		a.levelWin.Close()
	}
	a.s.panelOpened()
	icon := a.tray.Bounds()
	area := mygo.Screen.DisplayNearestPoint(mygo.Point{X: icon.X + icon.Width/2, Y: icon.Y + icon.Height/2}).WorkArea
	a.panel.maxContent = float32(max(area.Height-chrome, 200))
	a.flyout = mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:      a.tray,
		Placement: mygo.PlacementBottom,
		Width:     panelWidth,
		Height:    min(a.panel.height, area.Height),
		Focusable: true,
		Shadow:    true,
		Content:   ui.View(a.panel.view),
	})
	a.flyout.OnClosed(func() {
		a.flyout = nil
		a.s.save()
	})
	a.pointArrow()
}

func (a *app) close() {
	if a.flyout != nil {
		a.flyout.Close()
	}
}

// showLevel shows the level a key set under the menu bar icon, for a
// moment; the panel, while open, shows it itself.
func (a *app) showLevel(icon string, value float64) {
	if a.flyout != nil {
		return
	}
	a.level.icon, a.level.value = icon, value
	a.level.shown++
	if a.levelWin == nil {
		a.levelWin = mygo.NewFlyout(mygo.FlyoutOptions{
			Tray:      a.tray,
			Placement: mygo.PlacementBottom,
			Width:     levelWidth,
			Height:    levelHeight,
			Shadow:    true, // not Focusable: it leaves the keyboard where it is
			Content:   ui.View(a.levelView),
		})
		a.levelWin.OnClosed(func() { a.levelWin = nil })
	} else {
		a.levelWin.Invalidate()
	}
	shown := a.level.shown
	afterOnMain(levelFor, func() {
		if a.level.shown == shown && a.levelWin != nil {
			a.levelWin.Close()
		}
	})
}

// levelView is the level window: the key's icon, a meter and the value.
func (a *app) levelView(c *ui.Context) {
	c.SetTheme(levelTheme)
	c.Root().Background(ui.Transparent)
	label := "显示器音量"
	if a.level.icon == "sun" {
		label = "亮度"
	}
	ui.Row(c).Fill().PaddingX(16).Gap(10).AlignItems(ui.Center).Radius(levelHeight/2).
		Gradient(panelTop, panelBottom, 135).Border(0.7, ui.RGBA(255, 255, 255, 0.12)).TextColor(ink).Children(func() {
		icon(c, a.level.icon).FontSize(15).TextColor(accent).Width(18)
		ui.Meter(c, a.level.value, 0, 100, nil).Label(label).Grow(1)
		ui.Text(c, fmt.Sprint(int(math.Round(a.level.value)))).FontSize(13).FontWeight(600).FontFeatures("tnum").Width(26)
	})
}

// resize fits the panel to its content, which changes with the tab.
func (a *app) resize(height int) {
	if a.flyout == nil {
		return
	}
	if corner, ok := a.corner(height); ok {
		// SetSize would place the flyout again against an icon whose place
		// is unknown, in the middle of the screen, before pointArrow moved
		// it: set the size and the place at once, so it doesn't flash.
		a.flyout.SetBounds(corner)
		return
	}
	a.flyout.SetSize(panelWidth, height)
	a.pointArrow()
}

// pointArrow aims the arrow at the icon's middle, wherever the panel was
// placed against the screen's edges.
func (a *app) pointArrow() {
	if corner, ok := a.corner(a.flyout.Bounds().Height); ok {
		a.flyout.SetBounds(corner)
		return
	}
	icon, win := a.tray.Bounds(), a.flyout.Bounds()
	a.panel.arrowX = float32(icon.X + icon.Width/2 - win.X)
	a.flyout.Invalidate()
}

// corner is where the panel goes when the icon's place is unknown (Windows
// keeps it in the notification area's overflow): the bottom right corner,
// where Windows puts the panels of its own icons.
func (a *app) corner(height int) (mygo.Rectangle, bool) {
	if runtime.GOOS != "windows" || a.tray.Bounds().Width != 0 {
		return mygo.Rectangle{}, false
	}
	area := mygo.Screen.PrimaryDisplay().WorkArea
	height = min(height, area.Height-24)
	return mygo.Rectangle{X: area.X + area.Width - panelWidth - 12, Y: area.Y + area.Height - height - 12,
		Width: panelWidth, Height: height}, true
}
