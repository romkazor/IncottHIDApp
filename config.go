package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const (
	appName           = "IncottDriver"        // Registry key name
	defaultUpdateRepo = "romkazor/IncottHIDApp" // Fallback GitHub repo for update checks
)

// AppConfig holds persisted user settings.
type AppConfig struct {
	TargetGameExe string `json:"target_game_exe"`
	AutoBoost     bool   `json:"auto_boost"`
	AutoStart     bool   `json:"auto_start"`
	Debug         bool   `json:"debug"`
	UpdateRepo    string `json:"update_repo"` // "owner/repo" on github.com, empty = use default
}

var (
	// Auto-boost state
	boostMu            sync.Mutex
	autoBoostEnabled   bool
	targetApps         string   // raw value from config, comma-separated
	targetAppsLower    []string // pre-computed lowercase list

	// Autostart (UI-goroutine only, no lock needed)
	autoStartEnabled bool

	// Update repo: "owner/repo" on github.com (UI-goroutine / background worker reads only).
	updateRepo = defaultUpdateRepo
)

// parseTargetApps splits comma-separated exe names and lowercases them.
func parseTargetApps(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(p), ".exe") {
			p += ".exe"
		}
		result = append(result, strings.ToLower(p))
	}
	return result
}

func setTargetApps(raw string) {
	targetApps = raw
	targetAppsLower = parseTargetApps(raw)
}

func loadConfig() {
	setTargetApps("cs2.exe")

	data, err := os.ReadFile("settings.json")
	if err != nil {
		return
	}
	var config AppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return
	}
	if config.TargetGameExe != "" {
		setTargetApps(config.TargetGameExe)
	}
	autoBoostEnabled = config.AutoBoost
	autoStartEnabled = config.AutoStart
	debugEnabled.Store(config.Debug)
	if isValidRepo(config.UpdateRepo) {
		updateRepo = config.UpdateRepo
	}
}

// isValidRepo checks that the repo string is in "owner/repo" format (contains a single slash).
func isValidRepo(repo string) bool {
	if repo == "" {
		return false
	}
	parts := strings.Split(repo, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}

func saveConfig() {
	boostMu.Lock()
	apps := targetApps
	boostState := autoBoostEnabled
	boostMu.Unlock()

	config := AppConfig{
		TargetGameExe: apps,
		AutoBoost:     boostState,
		AutoStart:     autoStartEnabled,
		Debug:         debugEnabled.Load(),
		UpdateRepo:    updateRepo,
	}
	data, err := json.Marshal(config)
	if err != nil {
		logDebug("failed to marshal config: %v", err)
		return
	}
	os.WriteFile("settings.json", data, 0644)
}

// setAutoStart adds or removes the app from Windows autostart via registry.
func setAutoStart(enable bool) {
	exePath, err := os.Executable()
	if err != nil {
		logDebug("failed to get executable path: %v", err)
		return
	}
	exePath = filepath.Clean(exePath)

	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.ALL_ACCESS)
	if err != nil {
		logDebug("failed to open registry key: %v", err)
		return
	}
	defer k.Close()

	if enable {
		err = k.SetStringValue(appName, fmt.Sprintf(`"%s"`, exePath))
		if err != nil {
			logDebug("failed to write registry: %v", err)
		} else {
			logDebug("added to autostart: %s", exePath)
		}
	} else {
		err = k.DeleteValue(appName)
		if err != nil && err != registry.ErrNotExist {
			logDebug("failed to remove from registry: %v", err)
		} else {
			logDebug("removed from autostart")
		}
	}
}

func promptForExe(current string) string {
	script := fmt.Sprintf(`Add-Type -AssemblyName Microsoft.VisualBasic; [Microsoft.VisualBasic.Interaction]::InputBox('Enter process names, comma-separated (e.g., cs2.exe, dota2.exe):', 'Auto-boost Settings', '%s')`, current)
	cmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	out, err := cmd.Output()
	if err != nil {
		logDebug("PowerShell dialog error: %v", err)
		return current
	}

	res := strings.TrimSpace(string(out))
	if res == "" {
		return current
	}
	return res
}
