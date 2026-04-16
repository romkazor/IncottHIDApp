package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const (
	appName           = "IncottHIDApp"          // Registry key name
	legacyAppName     = "IncottDriver"          // Previous registry key name — removed on every setAutoStart call to clean up after rename
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

	// exeDir is the directory containing the running executable, cached once at startup.
	// All persistence files (settings.json, incott.log) live here so autostart (CWD=System32) works correctly.
	exeDir string
)

// initPaths determines the executable directory. Called once at program start.
func initPaths() {
	exe, err := os.Executable()
	if err != nil {
		exeDir = "." // fallback to CWD
		return
	}
	exeDir = filepath.Dir(exe)
}

// resolvePath returns an absolute path for a file stored next to the executable.
func resolvePath(name string) string {
	if exeDir == "" {
		return name
	}
	return filepath.Join(exeDir, name)
}

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

	data, err := os.ReadFile(resolvePath("settings.json"))
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

// validRepoRe matches GitHub's owner/repo slug format (alphanumeric, dash, underscore, dot).
var validRepoRe = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)

// isValidRepo checks that the repo string is a safe "owner/repo" identifier
// that won't introduce path traversal or URL-manipulation characters.
func isValidRepo(repo string) bool {
	return validRepoRe.MatchString(repo)
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
	if err := os.WriteFile(resolvePath("settings.json"), data, 0644); err != nil {
		logDebug("failed to write settings.json: %v", err)
	}
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

	// Remove any leftover entry written by older builds under the legacy name.
	if err := k.DeleteValue(legacyAppName); err == nil {
		logDebug("removed legacy autostart entry: %s", legacyAppName)
	}

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

// escapePowerShellSingleQuoted escapes a string for use inside a single-quoted PowerShell literal.
// In PS single-quoted strings, the only special character is the single quote itself, which is escaped by doubling.
func escapePowerShellSingleQuoted(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func promptForExe(current string) string {
	safe := escapePowerShellSingleQuoted(current)
	script := fmt.Sprintf(`Add-Type -AssemblyName Microsoft.VisualBasic; [Microsoft.VisualBasic.Interaction]::InputBox('Enter process names, comma-separated (e.g., cs2.exe, dota2.exe):', 'Auto-boost Settings', '%s')`, safe)
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
