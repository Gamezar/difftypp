package updater

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		latest   string
		expected int
	}{
		{name: "current older", current: "v1.0.0", latest: "v1.1.0", expected: -1},
		{name: "current newer", current: "v1.2.0", latest: "v1.1.0", expected: 1},
		{name: "same version", current: "v1.0.0", latest: "v1.0.0", expected: 0},
		{name: "dev is older than release", current: "dev", latest: "v1.0.0", expected: -1},
		{name: "prerelease upgrades to stable", current: "v1.0.0-beta.1", latest: "v1.0.0", expected: -1},
		{name: "stable is newer than prerelease", current: "v1.0.0", latest: "v1.0.0-beta.1", expected: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := compareVersions(tc.current, tc.latest)
			if result != tc.expected {
				t.Fatalf("compareVersions(%q, %q) = %d, want %d", tc.current, tc.latest, result, tc.expected)
			}
		})
	}
}

func TestSelfUpdateReplacesExecutable(t *testing.T) {
	oldClient := httpClient
	oldExecPath := getExecPath
	oldAPIURL := apiURL
	oldOS := currentOS
	oldArch := currentArch

	t.Cleanup(func() {
		httpClient = oldClient
		getExecPath = oldExecPath
		apiURL = oldAPIURL
		currentOS = oldOS
		currentArch = oldArch
	})

	dir := t.TempDir()
	execPath := filepath.Join(dir, binaryName)
	if err := os.WriteFile(execPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("write exec file: %v", err)
	}

	getExecPath = func() (string, error) { return execPath, nil }
	apiURL = "https://example.test/releases/latest"
	currentOS = "linux"
	currentArch = "amd64"
	httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case apiURL:
				body := `{"tag_name":"v1.0.0","assets":[{"name":"diffty-linux-amd64","browser_download_url":"https://example.test/assets/diffty-linux-amd64"}]}`
				return jsonResponse(http.StatusOK, body), nil
			case "https://example.test/assets/diffty-linux-amd64":
				return binaryResponse(http.StatusOK, "new-binary"), nil
			default:
				return binaryResponse(http.StatusNotFound, "not found"), nil
			}
		}),
	}

	tag, updated, err := SelfUpdate("v0.9.0")
	if err != nil {
		t.Fatalf("SelfUpdate returned error: %v", err)
	}
	if !updated {
		t.Fatalf("SelfUpdate updated = false, want true")
	}
	if tag != "v1.0.0" {
		t.Fatalf("SelfUpdate tag = %q, want v1.0.0", tag)
	}

	content, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatalf("read updated exec: %v", err)
	}
	if string(content) != "new-binary" {
		t.Fatalf("updated executable = %q, want new-binary", string(content))
	}
}

func TestSelfUpdateNoopWhenCurrentIsLatest(t *testing.T) {
	oldClient := httpClient
	oldAPIURL := apiURL
	t.Cleanup(func() {
		httpClient = oldClient
		apiURL = oldAPIURL
	})

	apiURL = "https://example.test/releases/latest"
	httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != apiURL {
				return binaryResponse(http.StatusNotFound, "not found"), nil
			}
			return jsonResponse(http.StatusOK, `{"tag_name":"v1.0.0","assets":[]}`), nil
		}),
	}

	tag, updated, err := SelfUpdate("v1.0.0")
	if err != nil {
		t.Fatalf("SelfUpdate returned error: %v", err)
	}
	if updated {
		t.Fatalf("SelfUpdate updated = true, want false")
	}
	if tag != "v1.0.0" {
		t.Fatalf("SelfUpdate tag = %q, want v1.0.0", tag)
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func binaryResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
