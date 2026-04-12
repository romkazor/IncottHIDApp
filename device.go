package main

import (
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/getlantern/systray"
	"github.com/karalabe/hid"
)

const (
	vendorID          = 0x093A
	productID         = 0x522C
	productIDCharging = 0x622C
)

// Known mouse identifiers in HID Product string (lowercase).
var mouseKeywords = []string{"mouse", "g23", "g24", "ghero", "zero 29", "zero 39"}

// parseLODByte decodes the LOD value (in tenths of mm) from byte[7] of a 0x84 response.
// Upper nibble: 0=1mm, 1=2mm, 2=0.7mm. Returns 0 if unknown.
func parseLODByte(b byte) int {
	switch b >> 4 {
	case 0x00:
		return 10
	case 0x01:
		return 20
	case 0x02:
		return 7
	}
	return 0
}

// parseMotionSyncByte decodes motion sync (0=off, 1=on) from the lower nibble of byte[7].
func parseMotionSyncByte(b byte) int {
	if b&0x0F == 0x01 {
		return 1
	}
	return 0
}

// parseSleepBytes decodes the sleep timer (seconds) from a LE uint16 in bytes[3:5] of a 0x85/0x03 response.
// Returns 0 if out of valid range.
func parseSleepBytes(lo, hi byte) int {
	v := int(lo) | int(hi)<<8
	if v <= 0 || v > 900 {
		return 0
	}
	return v
}

// parseStatus decodes battery level, DPI and polling rate from a status interrupt report.
// buf[1] = battery (high bit may indicate charging — subtract 128), buf[2] upper nibble = DPI preset, lower nibble = Hz preset.
func parseStatus(buf []byte) (battery int16, dpi int, hz int) {
	if len(buf) < 3 {
		return 0, 0, 0
	}
	battery = int16(buf[1])
	if battery > 100 {
		battery -= 128
	}
	hzPreset := int(buf[2] & 0x0F)
	dpiPreset := int((buf[2] & 0xF0) >> 4)
	dpi, hz = presets(dpiPreset, hzPreset)
	return
}

// parseDebounceByte decodes the debounce value (ms) from byte[3] of a 0x85/0x01 response.
// Returns -1 if out of valid range (0-30).
func parseDebounceByte(b byte) int {
	v := int(b)
	if v < 0 || v > 30 {
		return -1
	}
	return v
}

// parseToggleByte decodes a boolean toggle (motion sync, angle snap, ripple ctrl) from a single byte.
// Returns 1 if byte is 0x01, otherwise 0.
func parseToggleByte(b byte) int {
	if b == 0x01 {
		return 1
	}
	return 0
}

// parseReceiverLEDByte decodes the receiver LED mode from byte[2] of a 0x88 response.
// Returns -1 if value is out of valid range (0-2).
func parseReceiverLEDByte(b byte) int {
	v := int(b)
	if v < 0 || v > 2 {
		return -1
	}
	return v
}

// isMouseDevice checks if the HID product name matches a known Incott mouse.
// Returns true if product is empty (firmware may not report it) or contains a known keyword.
func isMouseDevice(product string) bool {
	if product == "" {
		return true
	}
	p := strings.ToLower(product)
	for _, kw := range mouseKeywords {
		if strings.Contains(p, kw) {
			return true
		}
	}
	return false
}

// Pre-allocated HID report buffers. WARNING: only access from mouseWorker goroutine (no lock).
var (
	readBuf [64]byte
	reqBuf  [9]byte
)

// Shared send buffer for apply* functions (guarded by mu).
var sendBuf [9]byte

var (
	activeDevice hid.Device
	mu           sync.Mutex

	// Device status
	currentHz       int = 1000
	currentLOD      int   // tenths of mm (7=0.7mm, 10=1mm, 20=2mm), 0 = unknown
	currentSleep    int   // seconds, 0 = unknown
	currentDebounce  int = -1 // milliseconds (0-30), -1 = unknown
	currentMotionSync   int = -1 // -1 = unknown, 0 = off, 1 = on
	currentAngleSnap    int = -1
	currentRippleCtrl   int = -1
	currentReceiverLED  int = -1 // -1 = unknown, 0 = battery status, 1 = connect & polling rate, 2 = battery warning
	lastBattery     int16 = -1
	lastDPI         int
	lastHz          int

	// Connected device info
	deviceName string
)

// Preset lookup tables
var (
	dpiPresetValues     = [6]int{400, 800, 1600, 2400, 3200, 6400}
	pollingPresetValues = [7]int{1000, 500, 250, 125, 8000, 4000, 2000}
)

// Reused by isProcessRunning (single-goroutine access from gameMonitorWorker).
var procEntry syscall.ProcessEntry32

func presets(dpi int, pollingRate int) (int, int) {
	d, p := 800, 1000
	if dpi >= 0 && dpi < len(dpiPresetValues) {
		d = dpiPresetValues[dpi]
	}
	if pollingRate >= 0 && pollingRate < len(pollingPresetValues) {
		p = pollingPresetValues[pollingRate]
	}
	return d, p
}

func applyDPI(dpi int) {
	mu.Lock()
	defer mu.Unlock()
	logInfo("set DPI: %d", dpi)

	if activeDevice == nil {
		return
	}

	var idx byte
	switch dpi {
	case 400:  idx = 0x00
	case 800:  idx = 0x01
	case 1600: idx = 0x02
	case 2400: idx = 0x03
	case 3200: idx = 0x04
	case 6400: idx = 0x05
	default:
		return
	}

	sendBuf = [9]byte{0x09, 0x03, 0x06, idx}
	_, err := activeDevice.SendFeatureReport(sendBuf[:])
	if err != nil {
		logDebug("send DPI report failed: %v", err)
	} else {
		logDebug("DPI report sent: %d (0x%02X)", dpi, idx)
	}
}

func applyHz(hz int) {
	mu.Lock()
	defer mu.Unlock()
	logInfo("set polling rate: %d Hz", hz)

	if activeDevice == nil {
		return
	}

	var b byte
	switch hz {
	case 1000: b = 0x00
	case 500:  b = 0x01
	case 250:  b = 0x02
	case 125:  b = 0x03
	case 8000: b = 0x04
	case 4000: b = 0x05
	case 2000: b = 0x06
	default:
		return
	}

	sendBuf = [9]byte{0x09, 0x01, b}
	_, err := activeDevice.SendFeatureReport(sendBuf[:])
	if err != nil {
		logDebug("send Hz report failed: %v", err)
	} else {
		logDebug("Hz report sent: %d (0x%02X)", hz, b)
	}
}

func applyLOD(lod int) {
	mu.Lock()
	defer mu.Unlock()
	logInfo("set LOD: %s", lodLabel(lod))

	if activeDevice == nil {
		return
	}

	var b byte
	switch lod {
	case 7:  b = 0x02 // 0.7 mm
	case 10: b = 0x00 // 1 mm
	case 20: b = 0x01 // 2 mm
	default:
		return
	}

	sendBuf = [9]byte{0x09, 0x04, 0x01, b}
	_, err := activeDevice.SendFeatureReport(sendBuf[:])
	if err != nil {
		logDebug("send LOD report failed: %v", err)
	} else {
		currentLOD = lod
		logDebug("LOD report sent: %s (0x%02X)", lodLabel(lod), b)
		lodItems.checkValue(lod)
		refreshStatusText()
	}
}

func applyMotionSync(on bool) {
	mu.Lock()
	defer mu.Unlock()

	var val byte
	if on {
		val = 0x01
		logInfo("set motion sync: on")
	} else {
		logInfo("set motion sync: off")
	}

	if activeDevice == nil {
		return
	}

	sendBuf = [9]byte{0x09, 0x04, 0x04, val}
	_, err := activeDevice.SendFeatureReport(sendBuf[:])
	if err != nil {
		logDebug("send motion sync report failed: %v", err)
	} else {
		if on {
			currentMotionSync = 1
		} else {
			currentMotionSync = 0
		}
		logDebug("motion sync report sent: %d", val)
	}
}

func applyAngleSnap(on bool) {
	mu.Lock()
	defer mu.Unlock()

	var val byte
	if on {
		val = 0x01
		logInfo("set angle snapping: on")
	} else {
		logInfo("set angle snapping: off")
	}

	if activeDevice == nil {
		return
	}

	sendBuf = [9]byte{0x09, 0x04, 0x03, val}
	_, err := activeDevice.SendFeatureReport(sendBuf[:])
	if err != nil {
		logDebug("send angle snapping report failed: %v", err)
	} else {
		if on {
			currentAngleSnap = 1
		} else {
			currentAngleSnap = 0
		}
		logDebug("angle snapping report sent: %d", val)
	}
}

func applyRippleCtrl(on bool) {
	mu.Lock()
	defer mu.Unlock()

	var val byte
	if on {
		val = 0x01
		logInfo("set ripple control: on")
	} else {
		logInfo("set ripple control: off")
	}

	if activeDevice == nil {
		return
	}

	sendBuf = [9]byte{0x09, 0x04, 0x02, val}
	_, err := activeDevice.SendFeatureReport(sendBuf[:])
	if err != nil {
		logDebug("send ripple control report failed: %v", err)
	} else {
		if on {
			currentRippleCtrl = 1
		} else {
			currentRippleCtrl = 0
		}
		logDebug("ripple control report sent: %d", val)
	}
}

var receiverLEDLabels = [3]string{"Connect & polling rate", "Battery status", "Battery warning"}

func applyReceiverLED(mode int) {
	if mode < 0 || mode >= len(receiverLEDLabels) {
		logDebug("invalid receiver LED mode: %d", mode)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	logInfo("set receiver LED: %s", receiverLEDLabels[mode])

	if activeDevice == nil {
		return
	}

	sendBuf = [9]byte{0x09, 0x08, byte(mode)}
	_, err := activeDevice.SendFeatureReport(sendBuf[:])
	if err != nil {
		logDebug("send receiver LED report failed: %v", err)
	} else {
		currentReceiverLED = mode
		logDebug("receiver LED report sent: %d", mode)
		receiverLEDItems.checkValue(mode)
	}
}

func applyDebounce(ms int) {
	mu.Lock()
	defer mu.Unlock()
	logInfo("set debounce: %d ms", ms)

	if activeDevice == nil {
		return
	}

	sendBuf = [9]byte{0x09, 0x05, 0x01, byte(ms)}
	_, err := activeDevice.SendFeatureReport(sendBuf[:])
	if err != nil {
		logDebug("send debounce report failed: %v", err)
	} else {
		currentDebounce = ms
		logDebug("debounce report sent: %d ms", ms)
		debounceItems.checkValue(ms)
		refreshStatusText()
	}
}

func applySleep(seconds int) {
	mu.Lock()
	defer mu.Unlock()
	logInfo("set sleep: %s", sleepLabel(seconds))

	if activeDevice == nil {
		return
	}

	sendBuf = [9]byte{0x09, 0x05, 0x03, byte(seconds & 0xFF), byte((seconds >> 8) & 0xFF)}
	_, err := activeDevice.SendFeatureReport(sendBuf[:])
	if err != nil {
		logDebug("send sleep report failed: %v", err)
	} else {
		currentSleep = seconds
		logDebug("sleep report sent: %d sec (0x%02X 0x%02X)", seconds, sendBuf[3], sendBuf[4])
		sleepItems.checkValue(seconds)
		refreshStatusText()
	}
}

func updateStatus(buf []byte) {
	battery, dpi, hz := parseStatus(buf)

	logDebug("status: Bat=%d%%, DPI=%d, Hz=%d", battery, dpi, hz)

	lastBattery = battery
	lastDPI = dpi
	lastHz = hz

	boostMu.Lock()
	currentHz = hz
	boostMu.Unlock()

	refreshStatusText()
	updateCheckmarks()
}

// readDeviceSetting sends a feature report request and reads the response.
// Returns true if a matching response was received.
func readDeviceSetting(dev hid.Device, cmd byte, sub byte) bool {
	reqBuf = [9]byte{0x09, cmd, sub}
	dev.SendFeatureReport(reqBuf[:])

	for i := 0; i < 10; i++ {
		time.Sleep(50 * time.Millisecond)
		readBuf = [64]byte{0x09}
		_, err := dev.GetFeatureReport(readBuf[:])
		if err != nil {
			continue
		}
		if readBuf[0] == 0x09 && readBuf[1] == cmd {
			if sub != 0 && readBuf[2] != sub {
				continue
			}
			return true
		}
	}
	return false
}

func mouseWorker() {
	for {
		devs, _ := hid.Enumerate(vendorID, productID)
		chargingDevs, _ := hid.Enumerate(vendorID, productIDCharging)
		devs = append(devs, chargingDevs...)

		if len(devs) == 0 {
			if statusItem != nil {
				statusItem.SetTitle("Mouse not connected")
			}
			time.Sleep(2 * time.Second)
			continue
		}

		var targetDevice hid.Device
		var devProduct string
		for _, info := range devs {
			if info.UsagePage == 0xFF05 {
				if !isMouseDevice(info.Product) {
					logDebug("skipped non-mouse device: %q", info.Product)
					continue
				}
				dev, err := info.Open()
				if err == nil {
					targetDevice = dev
					devProduct = info.Product
					break
				}
			}
		}
		if targetDevice == nil {
			for _, info := range devs {
				if info.UsagePage >= 0xFF00 {
					if !isMouseDevice(info.Product) {
						continue
					}
					dev, err := info.Open()
					if err == nil {
						targetDevice = dev
						devProduct = info.Product
						break
					}
				}
			}
		}
		if targetDevice == nil {
			time.Sleep(3 * time.Second)
			continue
		}

		mu.Lock()
		activeDevice = targetDevice
		mu.Unlock()

		if devProduct != "" {
			deviceName = devProduct
		} else {
			deviceName = "Incott Mouse"
		}
		if statusItem != nil {
			systray.SetTooltip(deviceName)
		}
		logDebug("device opened: %s (VID: 0x%04X)", deviceName, vendorID)

		// Read main status (battery, DPI, Hz)
		reqBuf = [9]byte{0x09, 0x89}
		targetDevice.SendFeatureReport(reqBuf[:])
		for i := 0; i < 15; i++ {
			time.Sleep(50 * time.Millisecond)
			readBuf = [64]byte{0x09}
			_, err := targetDevice.GetFeatureReport(readBuf[:])
			if err == nil && readBuf[0] == 0x09 && readBuf[1] > 0 {
				if readBuf[1] == 0x89 && readBuf[2] == 0x00 && readBuf[3] == 0x00 {
					continue
				}
				updateStatus(readBuf[:])
				break
			}
		}

		// Read debounce (0x85 sub 0x01)
		if readDeviceSetting(targetDevice, 0x85, 0x01) {
			if v := parseDebounceByte(readBuf[3]); v >= 0 {
				currentDebounce = v
				logDebug("debounce: %d ms (raw: %02X %02X %02X %02X %02X %02X %02X %02X %02X)",
					v, readBuf[0], readBuf[1], readBuf[2], readBuf[3], readBuf[4],
					readBuf[5], readBuf[6], readBuf[7], readBuf[8])
			}
		}

		// Read LOD and Motion Sync (0x84)
		if readDeviceSetting(targetDevice, 0x84, 0x00) {
			currentLOD = parseLODByte(readBuf[7])
			currentMotionSync = parseMotionSyncByte(readBuf[7])
			if currentLOD > 0 {
				logDebug("LOD: %s (raw: %02X %02X %02X %02X %02X %02X %02X %02X %02X)",
					lodLabel(currentLOD), readBuf[0], readBuf[1], readBuf[2], readBuf[3], readBuf[4],
					readBuf[5], readBuf[6], readBuf[7], readBuf[8])
			}
			msLabel := "off"
			if currentMotionSync == 1 {
				msLabel = "on"
			}
			logDebug("motion sync: %s (raw: %02X %02X %02X %02X %02X %02X %02X %02X %02X)",
				msLabel, readBuf[0], readBuf[1], readBuf[2], readBuf[3], readBuf[4],
				readBuf[5], readBuf[6], readBuf[7], readBuf[8])
		}

		// Read angle snapping (0x84 sub 0x03)
		if readDeviceSetting(targetDevice, 0x84, 0x03) {
			currentAngleSnap = parseToggleByte(readBuf[3])
			asLabel := "off"
			if currentAngleSnap == 1 {
				asLabel = "on"
			}
			logDebug("angle snapping: %s (raw: %02X %02X %02X %02X %02X %02X %02X %02X %02X)",
				asLabel, readBuf[0], readBuf[1], readBuf[2], readBuf[3], readBuf[4],
				readBuf[5], readBuf[6], readBuf[7], readBuf[8])
		}

		// Read ripple control (0x84 sub 0x02)
		if readDeviceSetting(targetDevice, 0x84, 0x02) {
			currentRippleCtrl = parseToggleByte(readBuf[3])
			rcLabel := "off"
			if currentRippleCtrl == 1 {
				rcLabel = "on"
			}
			logDebug("ripple control: %s (raw: %02X %02X %02X %02X %02X %02X %02X %02X %02X)",
				rcLabel, readBuf[0], readBuf[1], readBuf[2], readBuf[3], readBuf[4],
				readBuf[5], readBuf[6], readBuf[7], readBuf[8])
		}

		// Read receiver LED (0x88)
		if readDeviceSetting(targetDevice, 0x88, 0x00) {
			if v := parseReceiverLEDByte(readBuf[2]); v >= 0 {
				currentReceiverLED = v
				logDebug("receiver LED: %s (raw: %02X %02X %02X %02X %02X %02X %02X %02X %02X)",
					receiverLEDLabels[v], readBuf[0], readBuf[1], readBuf[2], readBuf[3], readBuf[4],
					readBuf[5], readBuf[6], readBuf[7], readBuf[8])
			}
		}

		// Read sleep timer (0x85 sub 0x03)
		if readDeviceSetting(targetDevice, 0x85, 0x03) {
			if v := parseSleepBytes(readBuf[3], readBuf[4]); v > 0 {
				currentSleep = v
				logDebug("sleep: %s (raw: %02X %02X %02X %02X %02X %02X %02X %02X %02X)",
					sleepLabel(v), readBuf[0], readBuf[1], readBuf[2], readBuf[3], readBuf[4],
					readBuf[5], readBuf[6], readBuf[7], readBuf[8])
			}
		}

		// Update UI with all read values
		refreshStatusText()
		if currentLOD > 0 { lodItems.checkValue(currentLOD) }
		if currentSleep > 0 { sleepItems.checkValue(currentSleep) }
		if currentDebounce >= 0 { debounceItems.checkValue(currentDebounce) }
		if mMotionSync != nil {
			if currentMotionSync == 1 { mMotionSync.Check() } else { mMotionSync.Uncheck() }
		}
		if mAngleSnap != nil {
			if currentAngleSnap == 1 { mAngleSnap.Check() } else { mAngleSnap.Uncheck() }
		}
		if mRippleCtrl != nil {
			if currentRippleCtrl == 1 { mRippleCtrl.Check() } else { mRippleCtrl.Uncheck() }
		}
		if currentReceiverLED >= 0 {
			receiverLEDItems.checkValue(currentReceiverLED)
		}

		// Wait for first status from interrupt read (battery, DPI, Hz)
		buf := make([]byte, 64)
		statusLogged := false
		for {
			n, err := targetDevice.Read(buf)
			if err != nil {
				break
			}
			if n > 0 && buf[0] == 0x09 && buf[1] > 0 {
				updateStatus(buf)
				if !statusLogged {
					statusLogged = true
					logDebug("battery: %d%% (raw: %02X %02X %02X %02X %02X %02X %02X %02X %02X)",
						lastBattery, buf[0], buf[1], buf[2], buf[3], buf[4],
						buf[5], buf[6], buf[7], buf[8])
					logDebug("DPI: %d (raw: %02X %02X %02X %02X %02X %02X %02X %02X %02X)",
						lastDPI, buf[0], buf[1], buf[2], buf[3], buf[4],
						buf[5], buf[6], buf[7], buf[8])
					logDebug("polling rate: %d Hz (raw: %02X %02X %02X %02X %02X %02X %02X %02X %02X)",
						lastHz, buf[0], buf[1], buf[2], buf[3], buf[4],
						buf[5], buf[6], buf[7], buf[8])
					boolStr := func(v int) string { if v == 1 { return "on" }; return "off" }
					slp := "unknown"
					if currentSleep > 0 { slp = sleepLabel(currentSleep) }
					lod := "unknown"
					if currentLOD > 0 { lod = lodLabel(currentLOD) }
					recLED := "unknown"
					if currentReceiverLED >= 0 && currentReceiverLED < len(receiverLEDLabels) {
						recLED = receiverLEDLabels[currentReceiverLED]
					}
					db := "unknown"
					if currentDebounce >= 0 { db = strconv.Itoa(currentDebounce) + " ms" }
					logInfo("%s connected — Bat: %d%%, DPI: %d, Hz: %d, LOD: %s, Debounce: %s, Sleep: %s, MotionSync: %s, AngleSnap: %s, RippleCtrl: %s, ReceiverLED: %s",
						deviceName, lastBattery, lastDPI, lastHz, lod, db, slp,
						boolStr(currentMotionSync), boolStr(currentAngleSnap), boolStr(currentRippleCtrl), recLED)
				}
			}
		}

		mu.Lock()
		activeDevice.Close()
		activeDevice = nil
		mu.Unlock()

		if statusItem != nil {
			statusItem.SetTitle("Connection lost...")
		}
		logDebug("mouse connection lost")
		time.Sleep(1 * time.Second)
	}
}

// App process monitor
func gameMonitorWorker() {
	logDebug("app monitor started")
	var detectedApp string
	var gameModeActive bool
	var savedHz int
	for {
		time.Sleep(3 * time.Second)

		boostMu.Lock()
		isEnabled := autoBoostEnabled
		targets := targetAppsLower
		curHz := currentHz
		boostMu.Unlock()

		if !isEnabled || len(targets) == 0 {
			continue
		}

		mu.Lock()
		ready := activeDevice != nil
		mu.Unlock()

		if !ready {
			continue
		}

		found := findRunningApp(targets)

		if found != "" && !gameModeActive {
			detectedApp = found
			logInfo("app detected: %s (current: %d Hz)", detectedApp, curHz)
			gameModeActive = true
			savedHz = curHz
			if curHz != 8000 {
				logInfo("applying auto-boost: 8000 Hz")
				applyHz(8000)
			}
		} else if found == "" && gameModeActive {
			logInfo("app closed: %s, restoring %d Hz", detectedApp, savedHz)
			gameModeActive = false
			if savedHz != 8000 {
				applyHz(savedHz)
			}
		}
	}
}

// findRunningApp takes a single process snapshot and checks all target apps in one pass.
// Returns the first matching process name, or "" if none found.
func findRunningApp(targets []string) string {
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(snapshot)

	procEntry.Size = uint32(unsafe.Sizeof(procEntry))
	if err = syscall.Process32First(snapshot, &procEntry); err != nil {
		return ""
	}

	for {
		name := strings.ToLower(syscall.UTF16ToString(procEntry.ExeFile[:]))
		for _, t := range targets {
			if name == t {
				return t
			}
		}
		if err = syscall.Process32Next(snapshot, &procEntry); err != nil {
			break
		}
	}
	return ""
}
