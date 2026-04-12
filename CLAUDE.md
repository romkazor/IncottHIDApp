# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Windows system tray driver for **Incott** wireless mice (Ghero, G23, G24, G23V2, Zero 29, Zero 39). Communicates with the mouse over HID to control DPI, polling rate, LOD, debounce, sleep timer, motion sync, angle snapping, ripple control, and receiver LED mode. Provides an "auto-boost" feature that switches to 8000 Hz when any of the configured target processes is detected.

- **Language**: Go
- **Platform**: Windows only (uses `syscall`, `windows/registry`, PowerShell dialogs)
- **Device IDs**: Vendor `0x093A`, Product `0x522C` (wireless) / `0x622C` (charging) — shared across all models
- **Model detection**: via HID `Product` string from device firmware

## Build & Run

```bash
go build -o IncottDriver.exe -ldflags="-H windowsgui" .
./IncottDriver.exe
```

The `-H windowsgui` linker flag hides the console window. Omit it during development to see stdout output. Requires CGO — the `CC` environment variable should point to a modern MinGW-w64 GCC (TDM-GCC 10.x is incompatible with Go 1.26+).

### Icons

- **Tray icon**: `tray_icon.ico` — embedded via `go:embed`, generated from `mouse.png` (resized to 16/32/48/64px ICO)
- **Exe icon**: `app.ico` — compiled into `app_windows.syso` via `windres app.rc`. The `.syso` file is auto-linked by Go.
- **Source image**: `mouse.png` — original high-res image of the mouse

To regenerate icons after changing `mouse.png`:
```bash
python -c "
from PIL import Image
img = Image.open('mouse.png').convert('RGBA')
# Tray
sizes = [16,32,48,64]
icons = [img.resize((s,s), Image.LANCZOS) for s in sizes]
icons[0].save('tray_icon.ico', format='ICO', sizes=[(s,s) for s in sizes], append_images=icons[1:])
# Exe
sizes = [16,32,48,64,128,256]
icons = [img.resize((s,s), Image.LANCZOS) for s in sizes]
icons[0].save('app.ico', format='ICO', sizes=[(s,s) for s in sizes], append_images=icons[1:])
"
windres app.rc -o app_windows.syso
```

## Project Structure

| File | Responsibility |
|---|---|
| `main.go` | Entry point, tray icon via `go:embed tray_icon.ico`, `onExit` |
| `config.go` | `AppConfig` struct, `loadConfig`/`saveConfig`, `setAutoStart` (registry), `promptForExe` (PowerShell dialog), `parseTargetApps`/`setTargetApps` (comma-separated app list) |
| `logging.go` | `logInfo` (always writes), `logDebug` (only when debug enabled via `atomic.Bool`). Log file: `incott.log` |
| `device.go` | HID constants, pre-allocated report buffers, all `apply*` functions, `mouseWorker`, `gameMonitorWorker`, `findRunningApp`, `isMouseDevice` (product name filter) |
| `ui.go` | Menu structs (fixed arrays replacing maps), `onReady`, `refreshStatusText`, `updateCheckmarks`, click forwarding via goroutines |

## Architecture

Three concurrent components:

1. **systray UI** (`onReady` in `ui.go`) — system tray menu with DPI, Hz, LOD, Debounce, Sleep presets, toggle checkboxes (Motion Sync, Angle Snapping, Ripple Control), Receiver LED submenu, auto-boost toggle, autostart, debug logging toggle. Each submenu group uses `forwardClicks()` which spawns one goroutine per menu item, reducing the main select to ~8 cases.
2. **`mouseWorker`** (`device.go`) — reconnection loop. Enumerates HID devices by vendor/product ID, filters by `isMouseDevice(info.Product)` to avoid connecting to Incott keyboards sharing the same vendor ID. Opens UsagePage `0xFF05`. On connect, reads current settings (status via `0x89`, debounce via `0x85/0x01`, LOD + motion sync via `0x84`, angle snapping via `0x84/0x03`, ripple control via `0x84/0x02`, receiver LED via `0x88`, sleep via `0x85/0x03`). Device model name is read from HID `Product` field and shown in tray tooltip. Then enters a read loop for live status updates.
3. **`gameMonitorWorker`** (`device.go`) — polls every 3s via a single `CreateToolhelp32Snapshot` call. Checks all target apps (`targetAppsLower`, comma-separated in config) in one pass over the process list. Auto-boosts to 8000 Hz when any target is found, restores on exit.

### HID Protocol (feature reports)

All reports are 9 bytes, report ID `0x09`. Read commands use the set command byte OR'd with `0x80`.

| Action | Bytes |
|---|---|
| Request status | `09 89 00 00 00 00 00 00 00` |
| Set polling rate | `09 01 <rate> 00 00 00 00 00 00` |
| Set DPI | `09 03 06 <idx> 00 00 00 00 00` |
| Set LOD | `09 04 01 <lod> 00 00 00 00 00` |
| Set ripple control | `09 04 02 <0/1> 00 00 00 00 00` |
| Set angle snapping | `09 04 03 <0/1> 00 00 00 00 00` |
| Set motion sync | `09 04 04 <0/1> 00 00 00 00 00` |
| Set debounce | `09 05 01 <ms> 00 00 00 00 00` |
| Set sleep | `09 05 03 <lo> <hi> 00 00 00 00` |
| Set receiver LED | `09 08 <mode> 00 00 00 00 00 00` |
| Read LOD + motion sync | `09 84 00 ...` → LOD in upper nibble of byte[7], motion sync in lower nibble |
| Read ripple control | `09 84 02 ...` → value in byte[3] |
| Read angle snapping | `09 84 03 ...` → value in byte[3] |
| Read debounce | `09 85 01 ...` → value in byte[3] |
| Read sleep | `09 85 03 ...` → LE uint16 in bytes[3:5] |
| Read receiver LED | `09 88 00 ...` → mode in byte[2] |

Rate bytes: `0x00`=1000, `0x01`=500, `0x02`=250, `0x03`=125, `0x04`=8000, `0x05`=4000, `0x06`=2000.
DPI indices: `0x00`=400, `0x01`=800, `0x02`=1600, `0x03`=2400, `0x04`=3200, `0x05`=6400.
LOD bytes: `0x00`=1mm, `0x01`=2mm, `0x02`=0.7mm.
Receiver LED modes: `0x00`=Connect & polling rate, `0x01`=Battery status, `0x02`=Battery warning.

### Synchronization

- `mu` (`sync.Mutex`) guards `activeDevice` and `sendBuf` — used by `mouseWorker` and all `apply*` calls.
- `boostMu` (`sync.Mutex`) guards `currentHz`, `savedHz`, `autoBoostEnabled`, `targetApps`, `targetAppsLower`.

### Logging

- `logInfo(format, args...)` — always written to `incott.log`. For user actions and lifecycle events.
- `logDebug(format, args...)` — only when `debugEnabled` (`atomic.Bool`) is true. For HID bytes, device reads, status updates.
- Config field: `"debug"` in `settings.json`.

### Persistence

- **`settings.json`** — stores `target_game_exe` (comma-separated app list), `auto_boost`, `auto_start`, `debug`. Loaded on startup, saved on setting changes.
- **Windows Registry** (`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`) — autostart entry under key `IncottDriver`.

### Performance Notes

- Menu items use fixed-size arrays in structs instead of `map[int]*MenuItem`.
- HID report buffers (`[9]byte`, `[64]byte`) are pre-allocated and reused.
- `refreshStatusText` uses `strings.Builder` + `strconv.Itoa` (zero `fmt.Sprintf`).
- `targetAppsLower` is pre-computed via `parseTargetApps`, avoiding repeated `strings.ToLower`.
- `ProcessEntry32` is reused across `findRunningApp` calls.
- `findRunningApp` takes a single process snapshot and checks all target apps in one pass.

## Key Dependencies

- `github.com/getlantern/systray` — system tray menu
- `github.com/karalabe/hid` — HID access (requires CGO, uses native Windows API)
- `golang.org/x/sys/windows/registry` — registry access for autostart
