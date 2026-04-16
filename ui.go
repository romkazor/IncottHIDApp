package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/getlantern/systray"
)

// Menu item groups with fixed arrays instead of maps.

var dpiValues = [6]int{400, 800, 1600, 2400, 3200, 6400}

type dpiMenu struct {
	items [6]*systray.MenuItem
}

func (m *dpiMenu) checkValue(dpi int) {
	for i, v := range dpiValues {
		if m.items[i] == nil { continue }
		if v == dpi { m.items[i].Check() } else { m.items[i].Uncheck() }
	}
}

func (m *dpiMenu) forwardClicks(apply func(int)) {
	for i, v := range dpiValues {
		val := v
		item := m.items[i]
		go func() {
			for range item.ClickedCh {
				apply(val)
			}
		}()
	}
}

var hzValues = [7]int{125, 250, 500, 1000, 2000, 4000, 8000}

type hzMenu struct {
	items [7]*systray.MenuItem
}

func (m *hzMenu) checkValue(hz int) {
	for i, v := range hzValues {
		if m.items[i] == nil { continue }
		if v == hz { m.items[i].Check() } else { m.items[i].Uncheck() }
	}
}

func (m *hzMenu) forwardClicks(apply func(int)) {
	for i, v := range hzValues {
		val := v
		item := m.items[i]
		go func() {
			for range item.ClickedCh {
				apply(val)
			}
		}()
	}
}

var lodValues = [3]int{7, 10, 20}

type lodMenu struct {
	items [3]*systray.MenuItem
}

func (m *lodMenu) checkValue(lod int) {
	for i, v := range lodValues {
		if m.items[i] == nil { continue }
		if v == lod { m.items[i].Check() } else { m.items[i].Uncheck() }
	}
}

func (m *lodMenu) forwardClicks(apply func(int)) {
	for i, v := range lodValues {
		val := v
		item := m.items[i]
		go func() {
			for range item.ClickedCh {
				apply(val)
			}
		}()
	}
}

var sleepValues = [18]int{10, 20, 30, 60, 120, 180, 240, 300, 360, 420, 480, 540, 600, 660, 720, 780, 840, 900}

type sleepMenu struct {
	items [18]*systray.MenuItem
}

func (m *sleepMenu) checkValue(seconds int) {
	for i, v := range sleepValues {
		if m.items[i] == nil { continue }
		if v == seconds { m.items[i].Check() } else { m.items[i].Uncheck() }
	}
}

func (m *sleepMenu) forwardClicks(apply func(int)) {
	for i, v := range sleepValues {
		val := v
		item := m.items[i]
		go func() {
			for range item.ClickedCh {
				apply(val)
			}
		}()
	}
}

type debounceMenu struct {
	items [31]*systray.MenuItem // index = ms value (0-30)
}

func (m *debounceMenu) checkValue(ms int) {
	for i := range m.items {
		if m.items[i] == nil { continue }
		if i == ms { m.items[i].Check() } else { m.items[i].Uncheck() }
	}
}

func (m *debounceMenu) forwardClicks(apply func(int)) {
	for i := range m.items {
		val := i
		item := m.items[i]
		go func() {
			for range item.ClickedCh {
				apply(val)
			}
		}()
	}
}

var receiverLEDValues = [3]int{1, 0, 2} // Battery status=1, Connect & polling rate=0, Battery warning=2

type receiverLEDMenu struct {
	items [3]*systray.MenuItem
}

func (m *receiverLEDMenu) checkValue(mode int) {
	for i, v := range receiverLEDValues {
		if m.items[i] == nil { continue }
		if v == mode { m.items[i].Check() } else { m.items[i].Uncheck() }
	}
}

func (m *receiverLEDMenu) forwardClicks(apply func(int)) {
	for i, v := range receiverLEDValues {
		val := v
		item := m.items[i]
		go func() {
			for range item.ClickedCh {
				apply(val)
			}
		}()
	}
}

// Global menu item groups
var (
	statusItem       *systray.MenuItem
	mMotionSync      *systray.MenuItem
	mAngleSnap       *systray.MenuItem
	mRippleCtrl      *systray.MenuItem
	mUpdate          *systray.MenuItem
	dpiItems         dpiMenu
	hzItems          hzMenu
	lodItems         lodMenu
	sleepItems       sleepMenu
	debounceItems    debounceMenu
	receiverLEDItems receiverLEDMenu
)

func lodLabel(lod int) string {
	switch lod {
	case 7:
		return "0.7mm"
	case 10:
		return "1mm"
	case 20:
		return "2mm"
	default:
		return strconv.Itoa(lod/10) + "mm"
	}
}

func sleepLabel(seconds int) string {
	if seconds < 60 {
		return strconv.Itoa(seconds) + "s"
	}
	return strconv.Itoa(seconds/60) + "m"
}

func refreshUpdateMenuItem() {
	if mUpdate == nil {
		return
	}
	updateMu.Lock()
	defer updateMu.Unlock()
	if updateAvailable {
		mUpdate.SetTitle("Update available: " + latestVersion)
		mUpdate.SetTooltip("Click to open the release page in browser")
	}
}

func refreshStatusText() {
	if statusItem == nil || lastBattery < 0 {
		return
	}
	var b strings.Builder
	b.Grow(80)
	b.WriteString("Bat: ")
	b.WriteString(strconv.Itoa(int(lastBattery)))
	b.WriteString("% | ")
	b.WriteString(strconv.Itoa(lastDPI))
	b.WriteString(" DPI | ")
	b.WriteString(strconv.Itoa(lastHz))
	b.WriteString(" Hz")
	if currentLOD > 0 {
		b.WriteString(" | ")
		b.WriteString(lodLabel(currentLOD))
		b.WriteString(" LOD")
	}
	if currentDebounce >= 0 {
		b.WriteString(" | ")
		b.WriteString(strconv.Itoa(currentDebounce))
		b.WriteString("ms")
	}
	if currentSleep > 0 {
		b.WriteString(" | Sleep ")
		b.WriteString(sleepLabel(currentSleep))
	}
	statusItem.SetTitle(b.String())
}

func updateCheckmarks() {
	dpiItems.checkValue(lastDPI)
	hzItems.checkValue(lastHz)
	if currentLOD > 0 {
		lodItems.checkValue(currentLOD)
	}
	if currentSleep > 0 {
		sleepItems.checkValue(currentSleep)
	}
	if currentDebounce >= 0 {
		debounceItems.checkValue(currentDebounce)
	}
}

func onReady() {
	systray.SetIcon(iconData)
	systray.SetTitle("Incott")
	systray.SetTooltip("Incott Mouse Driver " + version)

	statusItem = systray.AddMenuItem("Searching for mouse...", "Current status")
	statusItem.Disable()

	systray.AddSeparator()

	// Auto-boost
	mAutoBoost := systray.AddMenuItemCheckbox(
		fmt.Sprintf("Auto-boost (%s) - 8000Hz", targetApps),
		"Switch to 8000Hz when game is running",
		autoBoostEnabled,
	)
	mConfigBoost := systray.AddMenuItem("Change boost target process...", "Specify a different game exe (Dota 2, Valorant, etc.)")

	systray.AddSeparator()

	// App settings
	mAutoStart := systray.AddMenuItemCheckbox("Start with Windows", "Add to Windows autostart", autoStartEnabled)
	mDebugToggle := systray.AddMenuItemCheckbox("Debug logging (incott.log)", "Write debug events to log file", debugEnabled.Load())
	mUpdate = systray.AddMenuItem("Check for updates", "Check GitHub for new releases")

	systray.AddSeparator()

	// Motion Sync / Angle Snapping / Ripple Control
	mMotionSync = systray.AddMenuItemCheckbox("Motion Sync", "Toggle motion sync", false)
	mAngleSnap = systray.AddMenuItemCheckbox("Angle Snapping (recommended off in game)", "Toggle angle snapping", false)
	mRippleCtrl = systray.AddMenuItemCheckbox("Ripple Control (recommended off in game)", "Toggle ripple control", false)

	// Receiver LED
	mRecLED := systray.AddMenuItem("Receiver LED", "Receiver LED indicator mode")
	recLEDLabels := [3]string{"Battery status", "Connect & polling rate", "Battery warning"}
	for i, label := range recLEDLabels {
		receiverLEDItems.items[i] = mRecLED.AddSubMenuItemCheckbox(label, "", false)
	}
	receiverLEDItems.forwardClicks(applyReceiverLED)

	systray.AddSeparator()

	// LOD
	mLOD := systray.AddMenuItem("LOD (Lift-Off Distance)", "Change lift-off distance")
	lodLabels := [3]string{"0.7 mm", "1 mm", "2 mm"}
	for i, label := range lodLabels {
		lodItems.items[i] = mLOD.AddSubMenuItemCheckbox(label, "", false)
	}
	lodItems.forwardClicks(applyLOD)

	// Sleep Time
	mSleep := systray.AddMenuItem("Sleep Time", "Mouse auto-sleep timer")
	for i, s := range sleepValues {
		var label string
		if s < 60 {
			label = strconv.Itoa(s) + " sec"
		} else {
			label = strconv.Itoa(s/60) + " min"
		}
		sleepItems.items[i] = mSleep.AddSubMenuItemCheckbox(label, "", false)
	}
	sleepItems.forwardClicks(applySleep)

	// Debounce
	mDebounce := systray.AddMenuItem("Debounce", "Button debounce time (ms)")
	for i := 0; i <= 30; i++ {
		debounceItems.items[i] = mDebounce.AddSubMenuItemCheckbox(strconv.Itoa(i)+" ms", "", false)
	}
	debounceItems.forwardClicks(applyDebounce)

	// DPI
	mDPI := systray.AddMenuItem("DPI", "Change DPI")
	for i, v := range dpiValues {
		dpiItems.items[i] = mDPI.AddSubMenuItemCheckbox(strconv.Itoa(v)+" DPI", "", false)
	}
	dpiItems.forwardClicks(applyDPI)

	// Polling Rate
	mHz := systray.AddMenuItem("Polling Rate (Hz)", "Change polling rate")
	for i, v := range hzValues {
		hzItems.items[i] = mHz.AddSubMenuItemCheckbox(strconv.Itoa(v)+" Hz", "", false)
	}
	hzItems.forwardClicks(applyHz)

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Close driver")

	// Click handler for top-level items only
	go func() {
		for {
			select {
			case <-mQuit.ClickedCh:
				logInfo("shutting down")
				systray.Quit()
				return

			case <-mAutoStart.ClickedCh:
				autoStartEnabled = !autoStartEnabled
				if autoStartEnabled {
					mAutoStart.Check()
					logInfo("user enabled autostart")
				} else {
					mAutoStart.Uncheck()
					logInfo("user disabled autostart")
				}
				saveConfig()
				setAutoStart(autoStartEnabled)

			case <-mMotionSync.ClickedCh:
				if mMotionSync.Checked() {
					applyMotionSync(false)
					mMotionSync.Uncheck()
				} else {
					applyMotionSync(true)
					mMotionSync.Check()
				}

			case <-mAngleSnap.ClickedCh:
				if mAngleSnap.Checked() {
					applyAngleSnap(false)
					mAngleSnap.Uncheck()
				} else {
					applyAngleSnap(true)
					mAngleSnap.Check()
				}

			case <-mRippleCtrl.ClickedCh:
				if mRippleCtrl.Checked() {
					applyRippleCtrl(false)
					mRippleCtrl.Uncheck()
				} else {
					applyRippleCtrl(true)
					mRippleCtrl.Check()
				}

			case <-mDebugToggle.ClickedCh:
				newVal := !debugEnabled.Load()
				debugEnabled.Store(newVal)
				if newVal {
					mDebugToggle.Check()
					logInfo("user enabled debug logging")
				} else {
					logInfo("user disabled debug logging")
					mDebugToggle.Uncheck()
				}
				saveConfig()

			case <-mUpdate.ClickedCh:
				updateMu.Lock()
				hasUpdate := updateAvailable
				url := latestURL
				updateMu.Unlock()
				if hasUpdate && url != "" {
					logInfo("user opened update page: %s", url)
					openBrowser(url)
				} else {
					logInfo("user triggered manual update check")
					go runUpdateCheck()
				}

			case <-mAutoBoost.ClickedCh:
				boostMu.Lock()
				if mAutoBoost.Checked() {
					mAutoBoost.Uncheck()
					autoBoostEnabled = false
					logInfo("user disabled auto-boost")
				} else {
					mAutoBoost.Check()
					autoBoostEnabled = true
					logInfo("user enabled auto-boost, target: %s", targetApps)
				}
				boostMu.Unlock()
				saveConfig()

			case <-mConfigBoost.ClickedCh:
				boostMu.Lock()
				current := targetApps
				boostMu.Unlock()

				newVal := promptForExe(current)

				if newVal != "" && newVal != current {
					boostMu.Lock()
					setTargetApps(newVal)
					boostMu.Unlock()

					saveConfig()
					mAutoBoost.SetTitle(fmt.Sprintf("Auto-boost (%s) - 8000Hz", newVal))
					logInfo("user changed boost target: %s", newVal)
				}
			}
		}
	}()
}
