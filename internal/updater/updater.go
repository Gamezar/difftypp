package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	binaryName = "diffty"
	latestURL  = "https://api.github.com/repos/Gamezar/difftypp/releases/latest"
)

var (
	httpClient  = &http.Client{Timeout: 30 * time.Second}
	getExecPath = os.Executable
	currentOS   = runtime.GOOS
	currentArch = runtime.GOARCH
	apiURL      = latestURL
)

type release struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// SelfUpdate replaces the current executable with the latest GitHub release.
func SelfUpdate(currentVersion string) (string, bool, error) {
	rel, err := fetchLatestRelease()
	if err != nil {
		return "", false, err
	}

	if compareVersions(currentVersion, rel.TagName) >= 0 {
		return rel.TagName, false, nil
	}

	assetName := assetNameForPlatform(currentOS, currentArch)
	asset, err := findAsset(rel.Assets, assetName)
	if err != nil {
		return rel.TagName, false, err
	}

	execPath, err := getExecPath()
	if err != nil {
		return rel.TagName, false, fmt.Errorf("get executable path: %w", err)
	}

	if err := downloadAndReplace(execPath, asset.BrowserDownloadURL); err != nil {
		return rel.TagName, false, err
	}

	return rel.TagName, true, nil
}

func fetchLatestRelease() (*release, error) {
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", binaryName)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("fetch latest release: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode latest release: %w", err)
	}

	if rel.TagName == "" {
		return nil, fmt.Errorf("latest release is missing a tag name")
	}

	return &rel, nil
}

func assetNameForPlatform(goos string, goarch string) string {
	return fmt.Sprintf("%s-%s-%s", binaryName, goos, goarch)
}

func findAsset(assets []releaseAsset, name string) (*releaseAsset, error) {
	for _, asset := range assets {
		if asset.Name == name {
			return &asset, nil
		}
	}

	return nil, fmt.Errorf("release asset %q not found", name)
}

func downloadAndReplace(execPath string, downloadURL string) error {
	req, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("build asset request: %w", err)
	}
	req.Header.Set("User-Agent", binaryName)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download release asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("download release asset: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	info, err := os.Stat(execPath)
	if err != nil {
		return fmt.Errorf("stat executable: %w", err)
	}

	dir := filepath.Dir(execPath)
	tmpFile, err := os.CreateTemp(dir, ".diffty-update-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write updated executable: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Chmod(tmpPath, info.Mode()); err != nil {
		return fmt.Errorf("preserve executable permissions: %w", err)
	}

	if err := os.Rename(tmpPath, execPath); err != nil {
		return fmt.Errorf("replace executable: %w", err)
	}

	return nil
}

func compareVersions(current string, latest string) int {
	currentStable := isStableVersion(current)
	latestStable := isStableVersion(latest)

	currentParts, currentOK := parseVersion(current)
	latestParts, latestOK := parseVersion(latest)

	switch {
	case !currentOK && !latestOK:
		return strings.Compare(current, latest)
	case !currentOK:
		return -1
	case !latestOK:
		return 1
	}

	maxLen := len(currentParts)
	if len(latestParts) > maxLen {
		maxLen = len(latestParts)
	}

	for i := 0; i < maxLen; i++ {
		currentPart := 0
		latestPart := 0
		if i < len(currentParts) {
			currentPart = currentParts[i]
		}
		if i < len(latestParts) {
			latestPart = latestParts[i]
		}

		switch {
		case currentPart < latestPart:
			return -1
		case currentPart > latestPart:
			return 1
		}
	}

	if currentStable != latestStable {
		if currentStable {
			return 1
		}
		return -1
	}

	return 0
}

func isStableVersion(version string) bool {
	trimmed := strings.TrimSpace(version)
	return !strings.Contains(trimmed, "-")
}

func parseVersion(version string) ([]int, bool) {
	trimmed := strings.TrimSpace(version)
	trimmed = strings.TrimPrefix(trimmed, "v")
	if trimmed == "" || trimmed == "dev" {
		return nil, false
	}

	trimmed = strings.SplitN(trimmed, "-", 2)[0]
	trimmed = strings.SplitN(trimmed, "+", 2)[0]

	parts := strings.Split(trimmed, ".")
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return nil, false
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		values = append(values, value)
	}

	return values, true
}
