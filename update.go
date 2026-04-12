package main

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// buildReleasesAPIURL builds the GitHub Releases API URL for the given "owner/repo" string.
// Returns empty string if repo format is invalid.
func buildReleasesAPIURL(repo string) string {
	if !isValidRepo(repo) {
		return ""
	}
	return "https://api.github.com/repos/" + repo + "/releases/latest"
}

// Update state (guarded by updateMu).
var (
	updateMu        sync.Mutex
	latestVersion   string
	latestURL       string
	updateAvailable bool
)

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// compareSemver returns -1 if a < b, 0 if a == b, 1 if a > b.
// Accepts versions in the form "vMAJOR.MINOR.PATCH" (leading "v" optional).
// Returns 0 for unparseable versions.
func compareSemver(a, b string) int {
	pa, okA := parseSemver(a)
	pb, okB := parseSemver(b)
	if !okA || !okB {
		return 0
	}
	for i := 0; i < 3; i++ {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}
	return 0
}

func parseSemver(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(v, "v")
	// Strip any pre-release/build metadata (e.g. "1.0.0-rc1")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// checkForUpdate queries the GitHub Releases API and compares with the current version.
// Returns the latest tag, release URL, and whether an update is available.
func checkForUpdate() (latest string, url string, hasUpdate bool, err error) {
	apiURL := buildReleasesAPIURL(updateRepo)
	if apiURL == "" {
		apiURL = buildReleasesAPIURL(defaultUpdateRepo)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", "", false, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "IncottDriver")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", false, &httpError{Code: resp.StatusCode}
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", "", false, err
	}

	hasUpdate = compareSemver(version, rel.TagName) < 0
	return rel.TagName, rel.HTMLURL, hasUpdate, nil
}

type httpError struct{ Code int }

func (e *httpError) Error() string { return "github api returned status " + strconv.Itoa(e.Code) }

// openBrowser opens the given URL in the default browser (Windows-only).
// Uses rundll32 url.dll to avoid any shell metacharacter interpretation.
// Only https URLs are accepted to prevent launching arbitrary protocols.
func openBrowser(url string) error {
	if !strings.HasPrefix(url, "https://") {
		return &httpError{Code: 0}
	}
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

// updateCheckWorker runs a background check for updates after a short startup delay.
// Skips check if version is "dev" (local unversioned build).
func updateCheckWorker() {
	if version == "dev" {
		logDebug("update check skipped: running dev build")
		return
	}
	time.Sleep(10 * time.Second)
	runUpdateCheck()
}

// runUpdateCheck performs a single update check and updates global state + UI.
func runUpdateCheck() {
	latest, url, hasUpdate, err := checkForUpdate()
	if err != nil {
		logDebug("update check failed: %v", err)
		return
	}
	logInfo("update check: current=%s, latest=%s, hasUpdate=%v", version, latest, hasUpdate)

	updateMu.Lock()
	latestVersion = latest
	latestURL = url
	updateAvailable = hasUpdate
	updateMu.Unlock()

	if hasUpdate {
		refreshUpdateMenuItem()
	}
}
