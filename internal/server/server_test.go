package server

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Gamezar/difftypp/internal/models"
)

// MockStorage is a mock implementation of the Storage interface for testing
type MockStorage struct {
	repositories []string
	reviewState  *models.ReviewState
	review       *models.Review
	saveCalled   bool
	loadCalled   bool
}

func (m *MockStorage) SaveReviewState(state *models.ReviewState, repoPath string) error {
	m.reviewState = state
	m.saveCalled = true
	return nil
}

func (m *MockStorage) LoadReviewState(repoPath, sourceBranch, targetBranch, sourceCommit, targetCommit string) (*models.ReviewState, error) {
	m.loadCalled = true
	if m.reviewState != nil {
		return m.reviewState, nil
	}
	return &models.ReviewState{
		ReviewedFiles: []models.FileReview{},
		SourceBranch:  sourceBranch,
		TargetBranch:  targetBranch,
		SourceCommit:  sourceCommit,
		TargetCommit:  targetCommit,
	}, nil
}

func (m *MockStorage) SaveReview(review *models.Review, repoPath string) error {
	m.review = review
	m.saveCalled = true
	return nil
}

func (m *MockStorage) LoadReview(repoPath, sourceBranch, targetBranch, sourceCommit, targetCommit string) (*models.Review, error) {
	if m.review != nil {
		return m.review, nil
	}
	return &models.Review{
		RepoPath:     repoPath,
		SourceBranch: sourceBranch,
		TargetBranch: targetBranch,
		SourceCommit: sourceCommit,
		TargetCommit: targetCommit,
		Comments:     []models.ReviewComment{},
		Status:       models.ReviewStatusDraft,
	}, nil
}

func (m *MockStorage) SaveRepositories(repos []string) error {
	m.repositories = repos
	return nil
}

func (m *MockStorage) LoadRepositories() ([]string, error) {
	return m.repositories, nil
}

// baseTestTemplates returns an fstest.MapFS with minimal template stubs shared
// across all test setup functions. Callers may add or override entries before use.
func baseTestTemplates() fstest.MapFS {
	return fstest.MapFS{
		"templates/layout.html": &fstest.MapFile{
			Data: []byte(`{{define "layout.html"}}<!DOCTYPE html><html><body>{{.RenderedContent}}</body></html>{{end}}`),
			Mode: 0644,
		},
		"templates/index.html": &fstest.MapFile{
			Data: []byte(`{{define "index.html"}}Index Page{{end}}`),
			Mode: 0644,
		},
		"templates/compare.html": &fstest.MapFile{
			Data: []byte(`{{define "compare.html"}}Compare Page{{end}}`),
			Mode: 0644,
		},
		"templates/diff.html": &fstest.MapFile{
			Data: []byte(`{{define "diff.html"}}Diff Page{{end}}`),
			Mode: 0644,
		},
		"templates/error.html": &fstest.MapFile{
			Data: []byte(`{{define "error.html"}}Error: {{.Title}} - {{.Message}}{{end}}`),
			Mode: 0644,
		},
		"templates/review_submitted.html": &fstest.MapFile{
			Data: []byte(`{{define "review_submitted.html"}}Review Submitted{{end}}`),
			Mode: 0644,
		},
	}
}

// Helper function to create a test server with mocked dependencies
func setupTestServer(t *testing.T) (*Server, *MockStorage) {
	t.Helper()

	mockStorage := &MockStorage{
		repositories: []string{"/test/repo"},
		reviewState: &models.ReviewState{
			ReviewedFiles: []models.FileReview{},
			SourceBranch:  "feature",
			TargetBranch:  "main",
			SourceCommit:  "feature-commit-hash",
			TargetCommit:  "main-commit-hash",
		},
	}

	// Temporarily replace getTemplateDir with a mocked one.
	origFS := getTemplateDir
	getTemplateDir = func() fs.FS {
		return baseTestTemplates()
	}
	t.Cleanup(func() {
		getTemplateDir = origFS
	})

	server, err := New(mockStorage)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	return server, mockStorage
}

// TestServerInit tests that the server initializes correctly
func TestServerInit(t *testing.T) {
	server, _ := setupTestServer(t)
	if server == nil {
		t.Fatal("Server should not be nil")
	}
}

// TestHandleIndex tests the index handler
func TestHandleIndex(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	server.handleIndex(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	if !strings.Contains(string(body), "Index Page") {
		t.Errorf("Expected body to contain 'Index Page', got %s", string(body))
	}
}

// TestHandleReviewState tests the review state handler
func TestHandleReviewState(t *testing.T) {
	server, mockStorage := setupTestServer(t)

	formData := url.Values{}
	formData.Set("repo", "/test/repo")
	formData.Set("source", "feature")
	formData.Set("target", "main")
	formData.Set("source_commit", "feature-commit-hash")
	formData.Set("target_commit", "main-commit-hash")
	formData.Set("file", "file.txt")
	formData.Set("status", "approved")

	req := httptest.NewRequest("POST", "/api/review-state?"+formData.Encode(), nil)
	w := httptest.NewRecorder()

	server.handleReviewState(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("Expected status code %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}

	if !mockStorage.saveCalled {
		t.Error("SaveReviewState should have been called")
	}

	if !mockStorage.loadCalled {
		t.Error("LoadReviewState should have been called")
	}

	if mockStorage.reviewState == nil || len(mockStorage.reviewState.ReviewedFiles) == 0 {
		t.Error("ReviewState should have been updated with a file review")
	}
}

// TestExtractFilesFromDiff tests the extractFilesFromDiff function
func TestExtractFilesFromDiff(t *testing.T) {
	// Provide pre-parsed DiffFile structs (as the function now expects)
	parsedFiles := []models.DiffFile{
		{Path: "file1.txt", Additions: 1, Deletions: 0},
		{Path: "file2.txt", Additions: 1, Deletions: 1},
	}

	reviewState := &models.ReviewState{
		ReviewedFiles: []models.FileReview{
			{
				Repo:  "/test/repo",
				Path:  "file1.txt",
				Lines: map[string]string{"all": models.StateApproved},
			},
		},
	}

	files := extractFilesFromDiff(parsedFiles, reviewState, "/test/repo")

	if len(files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(files))
	}

	if files[0]["Path"] != "file2.txt" {
		t.Errorf("Expected first file to be file2.txt (unreviewed), got %s", files[0]["Path"])
	}

	if files[1]["Path"] != "file1.txt" {
		t.Errorf("Expected second file to be file1.txt (approved), got %s", files[1]["Path"])
	}

	if files[0]["Status"] != "unreviewed" {
		t.Errorf("Expected file2.txt status to be unreviewed, got %s", files[0]["Status"])
	}

	if files[1]["Status"] != models.StateApproved {
		t.Errorf("Expected file1.txt status to be approved, got %s", files[1]["Status"])
	}
}

// TestGetDiffMode tests the getDiffMode helper function
func TestGetDiffMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		expected string
	}{
		{"empty defaults to branches", "", models.ModeBranches},
		{"explicit branches", "branches", models.ModeBranches},
		{"commits mode", "commits", models.ModeCommits},
		{"staged mode", "staged", models.ModeStaged},
		{"unstaged mode", "unstaged", models.ModeUnstaged},
		{"invalid defaults to branches", "bogus", models.ModeBranches},
		{"case sensitive", "Staged", models.ModeBranches},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reqURL := "/compare?repo=/test/repo"
			if tc.mode != "" {
				reqURL += "&mode=" + tc.mode
			}
			req := httptest.NewRequest("GET", reqURL, nil)
			got := getDiffMode(req)
			if got != tc.expected {
				t.Errorf("getDiffMode(%q) = %q, want %q", tc.mode, got, tc.expected)
			}
		})
	}
}

// TestHandleCompareStagedRendersPage tests that staged mode on compare GET renders the compare page
func TestHandleCompareStagedRendersPage(t *testing.T) {
	server, _, tempDir := setupRealTestServer(t)

	req := httptest.NewRequest("GET", "/compare?repo="+url.QueryEscape(tempDir)+"&mode=staged", nil)
	w := httptest.NewRecorder()

	server.handleCompare(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "DiffMode=staged") {
		t.Errorf("Expected DiffMode=staged in response body, got %s", body)
	}
}

// TestHandleCompareUnstagedRendersPage tests that unstaged mode on compare GET renders the compare page
func TestHandleCompareUnstagedRendersPage(t *testing.T) {
	server, _, tempDir := setupRealTestServer(t)

	req := httptest.NewRequest("GET", "/compare?repo="+url.QueryEscape(tempDir)+"&mode=unstaged", nil)
	w := httptest.NewRecorder()

	server.handleCompare(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "DiffMode=unstaged") {
		t.Errorf("Expected DiffMode=unstaged in response body, got %s", body)
	}
}

// TestHandleCompareNoRepoRedirect tests that missing repo redirects to index
func TestHandleCompareNoRepoRedirect(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/compare", nil)
	w := httptest.NewRecorder()

	server.handleCompare(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("Expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location != "/" {
		t.Errorf("Expected redirect to /, got %s", location)
	}
}

// TestHandleReviewStateByMode tests review state handler across staged and unstaged modes.
func TestHandleReviewStateByMode(t *testing.T) {
	tests := []struct {
		mode         string
		targetCommit string
		status       string
		expectedMode string
	}{
		{mode: "staged", targetCommit: "staged", status: "approved", expectedMode: models.ModeStaged},
		{mode: "unstaged", targetCommit: "unstaged", status: "rejected", expectedMode: models.ModeUnstaged},
	}

	for _, tc := range tests {
		t.Run(tc.mode, func(t *testing.T) {
			server, mockStorage := setupTestServer(t)

			params := url.Values{}
			params.Set("repo", "/test/repo")
			params.Set("source_commit", "abc123")
			params.Set("target_commit", tc.targetCommit)
			params.Set("file", "file.txt")
			params.Set("status", tc.status)
			params.Set("mode", tc.mode)

			req := httptest.NewRequest("POST", "/api/review-state?"+params.Encode(), nil)
			w := httptest.NewRecorder()

			server.handleReviewState(w, req)

			resp := w.Result()
			if resp.StatusCode != http.StatusSeeOther {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("Expected status %d, got %d; body: %s", http.StatusSeeOther, resp.StatusCode, body)
			}

			if !mockStorage.saveCalled {
				t.Error("SaveReviewState should have been called")
			}

			location := resp.Header.Get("Location")
			if !strings.Contains(location, "mode="+tc.mode) {
				t.Errorf("Expected mode=%s in redirect URL, got %s", tc.mode, location)
			}

			if mockStorage.reviewState.DiffMode != tc.expectedMode {
				t.Errorf("Expected DiffMode to be %q, got %q", tc.expectedMode, mockStorage.reviewState.DiffMode)
			}
		})
	}
}

// TestHandleReviewStateModeInRedirect tests that mode param propagates through redirect for branches mode
func TestHandleReviewStateModeInRedirect(t *testing.T) {
	server, _ := setupTestServer(t)

	params := url.Values{}
	params.Set("repo", "/test/repo")
	params.Set("source", "feature")
	params.Set("target", "main")
	params.Set("source_commit", "feature-commit-hash")
	params.Set("target_commit", "main-commit-hash")
	params.Set("file", "file.txt")
	params.Set("status", "approved")
	params.Set("mode", "commits")

	req := httptest.NewRequest("POST", "/api/review-state?"+params.Encode(), nil)
	w := httptest.NewRecorder()

	server.handleReviewState(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status %d, got %d; body: %s", http.StatusSeeOther, resp.StatusCode, body)
	}

	location := resp.Header.Get("Location")
	if !strings.Contains(location, "mode=commits") {
		t.Errorf("Expected mode=commits in redirect URL, got %s", location)
	}
}

// TestHandleReviewStateStagedMissingParams tests that staged mode still requires essential params
func TestHandleReviewStateStagedMissingParams(t *testing.T) {
	server, _ := setupTestServer(t)

	// Missing source_commit - should fail even in staged mode
	params := url.Values{}
	params.Set("repo", "/test/repo")
	params.Set("target_commit", "staged")
	params.Set("file", "file.txt")
	params.Set("status", "approved")
	params.Set("mode", "staged")

	req := httptest.NewRequest("POST", "/api/review-state?"+params.Encode(), nil)
	w := httptest.NewRecorder()

	server.handleReviewState(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status %d for missing source_commit, got %d", http.StatusBadRequest, resp.StatusCode)
	}
}

// TestHandleReviewStateNextFileWithMode tests that next file redirect includes mode param
func TestHandleReviewStateNextFileWithMode(t *testing.T) {
	server, _ := setupTestServer(t)

	params := url.Values{}
	params.Set("repo", "/test/repo")
	params.Set("source_commit", "abc123")
	params.Set("target_commit", "staged")
	params.Set("file", "file1.txt")
	params.Set("status", "approved")
	params.Set("next", "file2.txt")
	params.Set("mode", "staged")

	req := httptest.NewRequest("POST", "/api/review-state?"+params.Encode(), nil)
	w := httptest.NewRecorder()

	server.handleReviewState(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status %d, got %d; body: %s", http.StatusSeeOther, resp.StatusCode, body)
	}

	location := resp.Header.Get("Location")
	if !strings.Contains(location, "mode=staged") {
		t.Errorf("Expected mode=staged in redirect URL, got %s", location)
	}
	if !strings.Contains(location, "file=file2.txt") {
		t.Errorf("Expected file=file2.txt in redirect URL, got %s", location)
	}
}

// TestAddRepository tests the AddRepository method
func TestAddRepository(t *testing.T) {
	server, mockStorage := setupTestServer(t)

	// Create a temporary directory that will be our mock git repo
	tempDir, err := os.MkdirTemp("", "diffty-test-repo")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a .git directory to make it look like a git repo
	if err := os.Mkdir(filepath.Join(tempDir, ".git"), 0755); err != nil {
		t.Fatalf("Failed to create .git directory: %v", err)
	}

	// Add the repository
	success, err := server.AddRepository(tempDir)

	if !success || err != nil {
		t.Errorf("AddRepository failed: %v", err)
	}

	// Check that the repository was added to the storage
	if len(mockStorage.repositories) != 2 || mockStorage.repositories[1] != tempDir {
		t.Errorf("Repository not added to storage correctly: %v", mockStorage.repositories)
	}
}

// TestRenderError tests the renderError method
func TestRenderError(t *testing.T) {
	server, _ := setupTestServer(t)

	w := httptest.NewRecorder()

	server.renderError(w, "Test Error", "This is a test error message", http.StatusBadRequest)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, resp.StatusCode)
	}

	expectedContent := "Error: Test Error - This is a test error message"
	if !strings.Contains(string(body), expectedContent) {
		t.Errorf("Expected body to contain '%s', got '%s'", expectedContent, string(body))
	}
}

// setupRealTestServer creates a real temporary git repo and a Server wired to it,
// so that handler integration tests exercise the real code paths end-to-end.
func setupRealTestServer(t *testing.T) (*Server, *MockStorage, string) {
	t.Helper()

	// 1. Create temp directory
	tempDir, err := os.MkdirTemp("", "diffty-integration-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	// Helper to run git commands — fatals on error
	runGit := func(args ...string) string {
		t.Helper()
		fullArgs := append([]string{"-C", tempDir}, args...)
		cmd := exec.Command("git", fullArgs...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\noutput: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	// 2. git init
	runGit("init")

	// 3. Disable GPG signing
	runGit("config", "--local", "commit.gpgsign", "false")

	// 4. Create test file
	testFilePath := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFilePath, []byte("initial content"), 0644); err != nil {
		t.Fatalf("Failed to write test.txt: %v", err)
	}

	// 5. Add and commit
	runGit("add", "test.txt")
	runGit("commit", "-m", "Initial commit")

	// 6. Ensure branch is named main
	runGit("branch", "-M", "main")

	// 7. Create feature branch
	runGit("checkout", "-b", "feature")

	// 8. Modify test.txt on feature branch
	if err := os.WriteFile(testFilePath, []byte("initial content\nnew line"), 0644); err != nil {
		t.Fatalf("Failed to modify test.txt: %v", err)
	}

	// 9. Add and commit on feature
	runGit("add", "test.txt")
	runGit("commit", "-m", "Add new line")

	// 10. Switch back to main
	runGit("checkout", "main")

	// 11. Set up getTemplateDir override with richer templates
	origFS := getTemplateDir
	getTemplateDir = func() fs.FS {
		tpl := baseTestTemplates()
		// Override compare.html and diff.html with richer versions for integration tests
		tpl["templates/compare.html"] = &fstest.MapFile{
			Data: []byte(`{{define "compare.html"}}DiffMode={{.DiffMode}} RepoName={{.RepoName}}{{if .Branches}} Branches={{range .Branches}}{{.}},{{end}}{{end}}{{if .RecentCommits}} RecentCommits=yes{{end}}{{end}}`),
			Mode: 0644,
		}
		tpl["templates/diff.html"] = &fstest.MapFile{
			Data: []byte(`{{define "diff.html"}}DiffMode={{.DiffMode}} SourceLabel={{.SourceLabel}} TargetLabel={{.TargetLabel}} SourceCommit={{.SourceCommit}} TargetCommit={{.TargetCommit}} NoDiff={{.NoDiff}} RepoName={{.RepoName}}{{if .Files}} Files={{range .Files}}{{.Path}},{{end}}{{end}}{{if .SelectedFile}} SelectedFile={{.SelectedFile}}{{end}}{{if .Error}} Error={{.Error}}{{end}}{{end}}`),
			Mode: 0644,
		}
		return tpl
	}

	// 12. Create MockStorage with the real temp dir
	mockStorage := &MockStorage{
		repositories: []string{tempDir},
	}

	// 13. Create Server
	server, err := New(mockStorage)
	if err != nil {
		os.RemoveAll(tempDir)
		getTemplateDir = origFS
		t.Fatalf("Failed to create server: %v", err)
	}

	// 14. Register cleanup
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
		getTemplateDir = origFS
	})

	return server, mockStorage, tempDir
}

// TestHandleDiffViewStagedMode tests the real handleDiffView with staged changes.
func TestHandleDiffViewStagedMode(t *testing.T) {
	server, _, tempDir := setupRealTestServer(t)

	// Create a new file and stage it
	stagedPath := filepath.Join(tempDir, "staged.txt")
	if err := os.WriteFile(stagedPath, []byte("staged content"), 0644); err != nil {
		t.Fatalf("Failed to write staged.txt: %v", err)
	}
	cmd := exec.Command("git", "-C", tempDir, "add", "staged.txt")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to git add staged.txt: %v\n%s", err, out)
	}

	reqURL := fmt.Sprintf("/diff?repo=%s&mode=staged", url.QueryEscape(tempDir))
	req := httptest.NewRequest("GET", reqURL, nil)
	w := httptest.NewRecorder()

	server.handleDiffView(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d; body: %s", resp.StatusCode, bodyStr)
	}
	if !strings.Contains(bodyStr, "DiffMode=staged") {
		t.Errorf("Expected body to contain 'DiffMode=staged', got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "SourceLabel=HEAD") {
		t.Errorf("Expected body to contain 'SourceLabel=HEAD', got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "TargetLabel=Staged Changes") {
		t.Errorf("Expected body to contain 'TargetLabel=Staged Changes', got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "Files=staged.txt,") {
		t.Errorf("Expected body to contain 'Files=staged.txt,', got: %s", bodyStr)
	}
}

// TestHandleDiffViewUnstagedMode tests the real handleDiffView with unstaged working-tree changes.
func TestHandleDiffViewUnstagedMode(t *testing.T) {
	server, _, tempDir := setupRealTestServer(t)

	// Modify test.txt without staging
	testFilePath := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFilePath, []byte("initial content\nunstaged change"), 0644); err != nil {
		t.Fatalf("Failed to modify test.txt: %v", err)
	}

	reqURL := fmt.Sprintf("/diff?repo=%s&mode=unstaged", url.QueryEscape(tempDir))
	req := httptest.NewRequest("GET", reqURL, nil)
	w := httptest.NewRecorder()

	server.handleDiffView(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d; body: %s", resp.StatusCode, bodyStr)
	}
	if !strings.Contains(bodyStr, "DiffMode=unstaged") {
		t.Errorf("Expected body to contain 'DiffMode=unstaged', got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "SourceLabel=HEAD") {
		t.Errorf("Expected body to contain 'SourceLabel=HEAD', got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "TargetLabel=Working Tree") {
		t.Errorf("Expected body to contain 'TargetLabel=Working Tree', got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "Files=test.txt,") {
		t.Errorf("Expected body to contain 'Files=test.txt,', got: %s", bodyStr)
	}
}

// TestHandleComparePostCommitsMode tests the real handleCompare POST in commits mode.
func TestHandleComparePostCommitsMode(t *testing.T) {
	server, _, tempDir := setupRealTestServer(t)

	// Resolve commit hashes for assertion
	featureHash := strings.TrimSpace(func() string {
		cmd := exec.Command("git", "-C", tempDir, "rev-parse", "feature")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to rev-parse feature: %v\n%s", err, out)
		}
		return string(out)
	}())
	mainHash := strings.TrimSpace(func() string {
		cmd := exec.Command("git", "-C", tempDir, "rev-parse", "main")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to rev-parse main: %v\n%s", err, out)
		}
		return string(out)
	}())

	formData := url.Values{}
	formData.Set("repo", tempDir)
	formData.Set("source", "feature")
	formData.Set("target", "main")
	formData.Set("mode", "commits")

	reqURL := fmt.Sprintf("/compare?repo=%s&mode=commits", url.QueryEscape(tempDir))
	req := httptest.NewRequest("POST", reqURL, strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	server.handleCompare(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 303, got %d; body: %s", resp.StatusCode, string(body))
	}

	location := resp.Header.Get("Location")
	if !strings.Contains(location, "/diff") {
		t.Errorf("Expected redirect to /diff, got: %s", location)
	}
	if !strings.Contains(location, "mode=commits") {
		t.Errorf("Expected mode=commits in location, got: %s", location)
	}
	if !strings.Contains(location, featureHash) {
		t.Errorf("Expected feature commit hash %s in location, got: %s", featureHash, location)
	}
	if !strings.Contains(location, mainHash) {
		t.Errorf("Expected main commit hash %s in location, got: %s", mainHash, location)
	}
	// source and target params now use resolved full hashes (not original form values)
	if !strings.Contains(location, "source="+featureHash) {
		t.Errorf("Expected source=%s in location, got: %s", featureHash, location)
	}
	if !strings.Contains(location, "target="+mainHash) {
		t.Errorf("Expected target=%s in location, got: %s", mainHash, location)
	}
}

// TestHandleDiffViewCommitsMode tests the real handleDiffView in commits mode.
func TestHandleDiffViewCommitsMode(t *testing.T) {
	server, _, tempDir := setupRealTestServer(t)

	reqURL := fmt.Sprintf("/diff?repo=%s&source=feature&target=main&mode=commits", url.QueryEscape(tempDir))
	req := httptest.NewRequest("GET", reqURL, nil)
	w := httptest.NewRecorder()

	server.handleDiffView(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d; body: %s", resp.StatusCode, bodyStr)
	}
	if !strings.Contains(bodyStr, "DiffMode=commits") {
		t.Errorf("Expected body to contain 'DiffMode=commits', got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "SourceLabel=feature") {
		t.Errorf("Expected body to contain 'SourceLabel=feature', got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "TargetLabel=main") {
		t.Errorf("Expected body to contain 'TargetLabel=main', got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "Files=test.txt,") {
		t.Errorf("Expected body to contain 'Files=test.txt,', got: %s", bodyStr)
	}
}

// TestHandleDiffViewBranchesModeReal tests the real handleDiffView in default branches mode.
func TestHandleDiffViewBranchesModeReal(t *testing.T) {
	server, _, tempDir := setupRealTestServer(t)

	// No mode param — defaults to branches
	reqURL := fmt.Sprintf("/diff?repo=%s&source=feature&target=main", url.QueryEscape(tempDir))
	req := httptest.NewRequest("GET", reqURL, nil)
	w := httptest.NewRecorder()

	server.handleDiffView(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d; body: %s", resp.StatusCode, bodyStr)
	}
	if !strings.Contains(bodyStr, "DiffMode=branches") {
		t.Errorf("Expected body to contain 'DiffMode=branches', got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "SourceLabel=feature") {
		t.Errorf("Expected body to contain 'SourceLabel=feature', got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "TargetLabel=main") {
		t.Errorf("Expected body to contain 'TargetLabel=main', got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "Files=test.txt,") {
		t.Errorf("Expected body to contain 'Files=test.txt,', got: %s", bodyStr)
	}
}

// TestHandleDiffViewStagedModeWithFileParam tests the real handleDiffView with staged mode
// and a specific file selected.
func TestHandleDiffViewStagedModeWithFileParam(t *testing.T) {
	server, _, tempDir := setupRealTestServer(t)

	// Create a new file and stage it
	stagedPath := filepath.Join(tempDir, "staged-file.txt")
	if err := os.WriteFile(stagedPath, []byte("staged file content"), 0644); err != nil {
		t.Fatalf("Failed to write staged-file.txt: %v", err)
	}
	cmd := exec.Command("git", "-C", tempDir, "add", "staged-file.txt")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to git add staged-file.txt: %v\n%s", err, out)
	}

	reqURL := fmt.Sprintf("/diff?repo=%s&mode=staged&file=staged-file.txt", url.QueryEscape(tempDir))
	req := httptest.NewRequest("GET", reqURL, nil)
	w := httptest.NewRecorder()

	server.handleDiffView(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d; body: %s", resp.StatusCode, bodyStr)
	}
	if !strings.Contains(bodyStr, "SelectedFile=staged-file.txt") {
		t.Errorf("Expected body to contain 'SelectedFile=staged-file.txt', got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "DiffMode=staged") {
		t.Errorf("Expected body to contain 'DiffMode=staged', got: %s", bodyStr)
	}
}

// TestHandleComparePostBranchesModeReal tests the real handleCompare POST in branches mode.
func TestHandleComparePostBranchesModeReal(t *testing.T) {
	server, _, tempDir := setupRealTestServer(t)

	formData := url.Values{}
	formData.Set("repo", tempDir)
	formData.Set("source", "feature")
	formData.Set("target", "main")
	formData.Set("mode", "branches")

	reqURL := fmt.Sprintf("/compare?repo=%s&mode=branches", url.QueryEscape(tempDir))
	req := httptest.NewRequest("POST", reqURL, strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	server.handleCompare(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 303, got %d; body: %s", resp.StatusCode, string(body))
	}

	location := resp.Header.Get("Location")
	if !strings.Contains(location, "/diff") {
		t.Errorf("Expected redirect to /diff, got: %s", location)
	}
	if !strings.Contains(location, "mode=branches") {
		t.Errorf("Expected mode=branches in location, got: %s", location)
	}
	if !strings.Contains(location, "source=feature") {
		t.Errorf("Expected source=feature in location, got: %s", location)
	}
	if !strings.Contains(location, "target=main") {
		t.Errorf("Expected target=main in location, got: %s", location)
	}
}

// TestHandleCompareGetCommitsMode tests the compare GET handler in commits mode,
// verifying that the response renders DiffMode, RecentCommits, and RepoName.
func TestHandleCompareGetCommitsMode(t *testing.T) {
	srv, _, tempDir := setupRealTestServer(t)

	reqURL := fmt.Sprintf("/compare?repo=%s&mode=commits", url.QueryEscape(tempDir))
	req := httptest.NewRequest("GET", reqURL, nil)
	w := httptest.NewRecorder()

	srv.handleCompare(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d; body: %s", resp.StatusCode, bodyStr)
	}

	if !strings.Contains(bodyStr, "DiffMode=commits") {
		t.Errorf("Expected body to contain 'DiffMode=commits', got: %s", bodyStr)
	}

	if !strings.Contains(bodyStr, "RecentCommits=yes") {
		t.Errorf("Expected body to contain 'RecentCommits=yes', got: %s", bodyStr)
	}

	expectedRepoName := filepath.Base(tempDir)
	if !strings.Contains(bodyStr, "RepoName="+expectedRepoName) {
		t.Errorf("Expected body to contain 'RepoName=%s', got: %s", expectedRepoName, bodyStr)
	}
}

// setupRealTemplateTestServer creates a real git repo with multiple commits and
// a Server that uses the actual embedded templates (no mock overrides). This is
// needed to verify the real HTML structure rendered by templates.
func setupRealTemplateTestServer(t *testing.T) (*Server, *MockStorage, string) {
	t.Helper()

	// 1. Create temp directory
	tempDir, err := os.MkdirTemp("", "diffty-template-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	// Helper to run git commands — fatals on error
	runGit := func(args ...string) string {
		t.Helper()
		fullArgs := append([]string{"-C", tempDir}, args...)
		cmd := exec.Command("git", fullArgs...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\noutput: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	// 2. git init + configure
	runGit("init")
	runGit("config", "--local", "commit.gpgsign", "false")
	runGit("config", "--local", "user.email", "test@diffty.test")
	runGit("config", "--local", "user.name", "Test User")

	// 3. First commit
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("first"), 0644); err != nil {
		t.Fatalf("Failed to write test.txt: %v", err)
	}
	runGit("add", "test.txt")
	runGit("commit", "-m", "First commit")
	runGit("branch", "-M", "main")

	// 4. Second commit (so we have at least two for selection testing)
	if err := os.WriteFile(testFile, []byte("first\nsecond"), 0644); err != nil {
		t.Fatalf("Failed to modify test.txt: %v", err)
	}
	runGit("add", "test.txt")
	runGit("commit", "-m", "Second commit")

	// 5. Create feature branch (so branches mode has > 1 branch)
	runGit("checkout", "-b", "feature")
	if err := os.WriteFile(testFile, []byte("first\nsecond\nthird"), 0644); err != nil {
		t.Fatalf("Failed to modify test.txt: %v", err)
	}
	runGit("add", "test.txt")
	runGit("commit", "-m", "Feature work")

	// 6. Switch back to main
	runGit("checkout", "main")

	// 7. Do NOT override getTemplateDir — use real embedded templates
	mockStorage := &MockStorage{
		repositories: []string{tempDir},
	}

	server, err := New(mockStorage)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create server: %v", err)
	}

	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	return server, mockStorage, tempDir
}

// TestCompareCommitsRendersInteractiveList verifies that the real compare.html
// template in commits mode renders the interactive commit selection list with
// all required data attributes, IDs, and the click-to-select script block.
func TestCompareCommitsRendersInteractiveList(t *testing.T) {
	srv, _, tempDir := setupRealTemplateTestServer(t)

	reqURL := fmt.Sprintf("/compare?repo=%s&mode=commits", url.QueryEscape(tempDir))
	req := httptest.NewRequest("GET", reqURL, nil)
	w := httptest.NewRecorder()

	srv.handleCompare(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d; body: %s", resp.StatusCode, bodyStr)
	}

	// Verify commits-list container with correct ID and data-testid
	checks := []struct {
		name    string
		content string
	}{
		{"commits-list ID", `id="commits-list"`},
		{"commits-list data-testid", `data-testid="commits-list"`},
		{"commit-row data-testid", `data-testid="commit-row"`},
		{"data-hash attribute", `data-hash="`},
		{"instruction text", "Click commits to select target and source refs"},
		{"DOMContentLoaded script", "DOMContentLoaded"},
		{"selection state object", "target: null, source: null"},
		{"target input ID", `id="target"`},
		{"source input ID", `id="source"`},
		{"pointer-events-none on children", "pointer-events-none"},
		{"commit-badge class in script", "commit-badge"},
		{"commits-hint ID", `id="commits-hint"`},
		{"compare-form-commits testid", `data-testid="compare-form-commits"`},
		{"First commit subject", "First commit"},
		{"Second commit subject", "Second commit"},
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(bodyStr, c.content) {
				t.Errorf("Expected body to contain %q for %s", c.content, c.name)
			}
		})
	}

	// Verify data-hash values are full 40-character SHA hashes
	// Find all data-hash="..." occurrences
	hashPrefix := `data-hash="`
	idx := 0
	hashCount := 0
	for {
		pos := strings.Index(bodyStr[idx:], hashPrefix)
		if pos == -1 {
			break
		}
		start := idx + pos + len(hashPrefix)
		end := strings.Index(bodyStr[start:], `"`)
		if end == -1 {
			break
		}
		hash := bodyStr[start : start+end]
		hashCount++
		if len(hash) != 40 {
			t.Errorf("Expected data-hash to be 40 chars (full SHA), got %d chars: %q", len(hash), hash)
		}
		// Verify it looks like a hex hash
		for _, c := range hash {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("data-hash contains non-hex character %q in %q", string(c), hash)
				break
			}
		}
		idx = start + end
	}

	// We made 2 commits on main, so we expect at least 2 data-hash attributes
	if hashCount < 2 {
		t.Errorf("Expected at least 2 data-hash attributes, found %d", hashCount)
	}

	// Verify the script block contains the expected selection logic patterns
	scriptChecks := []string{
		"selection.target",
		"selection.source",
		"updateAll",
		"updateRow",
		"targetInput.value",
		"sourceInput.value",
	}
	for _, sc := range scriptChecks {
		if !strings.Contains(bodyStr, sc) {
			t.Errorf("Expected script to contain %q", sc)
		}
	}
}

// TestCompareBranchesModeNoCommitsList verifies that branches mode does NOT
// render the interactive commit list, data-hash attributes, or script block.
func TestCompareBranchesModeNoCommitsList(t *testing.T) {
	srv, _, tempDir := setupRealTemplateTestServer(t)

	reqURL := fmt.Sprintf("/compare?repo=%s&mode=branches", url.QueryEscape(tempDir))
	req := httptest.NewRequest("GET", reqURL, nil)
	w := httptest.NewRecorder()

	srv.handleCompare(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d; body: %s", resp.StatusCode, bodyStr)
	}

	// Verify branches mode renders its own form
	if !strings.Contains(bodyStr, `data-testid="compare-form-branches"`) {
		t.Errorf("Expected branches form to be present")
	}

	// Verify commits-mode elements are NOT present
	absent := []struct {
		name    string
		content string
	}{
		{"commits-list ID", `id="commits-list"`},
		{"commits-list data-testid", `data-testid="commits-list"`},
		{"commit-row data-testid", `data-testid="commit-row"`},
		{"data-hash attribute", `data-hash="`},
		{"DOMContentLoaded script", "DOMContentLoaded"},
		{"selection state", "target: null, source: null"},
		{"compare-form-commits testid", `data-testid="compare-form-commits"`},
	}

	for _, a := range absent {
		t.Run(a.name, func(t *testing.T) {
			if strings.Contains(bodyStr, a.content) {
				t.Errorf("Expected body NOT to contain %q in branches mode (%s)", a.content, a.name)
			}
		})
	}
}

// TestHandleDiffViewStagedNoDiff tests the diff view handler in staged mode when
// the working tree is clean and nothing is staged, expecting NoDiff=true.
func TestHandleDiffViewStagedNoDiff(t *testing.T) {
	srv, _, tempDir := setupRealTestServer(t)

	reqURL := fmt.Sprintf("/diff?repo=%s&mode=staged", url.QueryEscape(tempDir))
	req := httptest.NewRequest("GET", reqURL, nil)
	w := httptest.NewRecorder()

	srv.handleDiffView(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d; body: %s", resp.StatusCode, bodyStr)
	}

	if !strings.Contains(bodyStr, "NoDiff=true") {
		t.Errorf("Expected body to contain 'NoDiff=true', got: %s", bodyStr)
	}

	if !strings.Contains(bodyStr, "DiffMode=staged") {
		t.Errorf("Expected body to contain 'DiffMode=staged', got: %s", bodyStr)
	}
}

// TestHandleComparePostCommitsMissingRefs tests the compare POST handler in
// commits mode when source and target refs are omitted, expecting a 400 error
// with "Missing Refs" in the response body.
func TestHandleComparePostCommitsMissingRefs(t *testing.T) {
	srv, _, tempDir := setupRealTestServer(t)

	formData := url.Values{}
	formData.Set("repo", tempDir)
	formData.Set("mode", "commits")

	reqURL := fmt.Sprintf("/compare?repo=%s&mode=commits", url.QueryEscape(tempDir))
	req := httptest.NewRequest("POST", reqURL, strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.handleCompare(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d; body: %s", resp.StatusCode, bodyStr)
	}

	if !strings.Contains(bodyStr, "Missing Refs") {
		t.Errorf("Expected body to contain 'Missing Refs', got: %s", bodyStr)
	}
}

// ---------- Review Comment Handler Tests (Fix 5.9) ----------

// TestHandleAddComment tests adding a new inline comment
func TestHandleAddComment(t *testing.T) {
	server, mockStorage := setupTestServer(t)

	t.Run("happy path adds comment and redirects", func(t *testing.T) {
		mockStorage.review = nil // force fresh review from LoadReview
		mockStorage.saveCalled = false

		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source", "feature")
		params.Set("target", "main")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "def456")
		params.Set("mode", "branches")

		formData := url.Values{}
		formData.Set("file_path", "internal/server/server.go")
		formData.Set("start_line", "10")
		formData.Set("end_line", "15")
		formData.Set("side", "right")
		formData.Set("body", "This function needs error handling")

		reqURL := "/api/review/comment?" + params.Encode()
		req := httptest.NewRequest("POST", reqURL, strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.handleAddComment(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusSeeOther {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 303, got %d; body: %s", resp.StatusCode, body)
		}

		if !mockStorage.saveCalled {
			t.Error("SaveReview should have been called")
		}

		if mockStorage.review == nil {
			t.Fatal("Review should have been saved")
		}

		if len(mockStorage.review.Comments) != 1 {
			t.Fatalf("Expected 1 comment, got %d", len(mockStorage.review.Comments))
		}

		c := mockStorage.review.Comments[0]
		if c.FilePath != "internal/server/server.go" {
			t.Errorf("Expected FilePath 'internal/server/server.go', got %q", c.FilePath)
		}
		if c.StartLine != 10 || c.EndLine != 15 {
			t.Errorf("Expected lines 10-15, got %d-%d", c.StartLine, c.EndLine)
		}
		if c.Side != "right" {
			t.Errorf("Expected side 'right', got %q", c.Side)
		}
		if c.Body != "This function needs error handling" {
			t.Errorf("Expected body 'This function needs error handling', got %q", c.Body)
		}
		if c.Status != "open" {
			t.Errorf("Expected status 'open', got %q", c.Status)
		}
		if c.ID == "" {
			t.Error("Comment ID should be generated")
		}

		// Verify redirect URL
		location := resp.Header.Get("Location")
		if !strings.Contains(location, "/diff") {
			t.Errorf("Expected redirect to /diff, got %s", location)
		}
		if !strings.Contains(location, "file=internal") {
			t.Errorf("Expected file param in redirect, got %s", location)
		}
	})

	t.Run("single line defaults end_line to start_line", func(t *testing.T) {
		mockStorage.review = nil
		mockStorage.saveCalled = false

		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "def456")

		formData := url.Values{}
		formData.Set("file_path", "main.go")
		formData.Set("start_line", "42")
		// end_line omitted
		formData.Set("body", "Fix this")

		reqURL := "/api/review/comment?" + params.Encode()
		req := httptest.NewRequest("POST", reqURL, strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.handleAddComment(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusSeeOther {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected 303, got %d; body: %s", resp.StatusCode, body)
		}

		c := mockStorage.review.Comments[0]
		if c.StartLine != 42 || c.EndLine != 42 {
			t.Errorf("Expected single line 42-42, got %d-%d", c.StartLine, c.EndLine)
		}
	})

	t.Run("defaults side to right when omitted", func(t *testing.T) {
		mockStorage.review = nil
		mockStorage.saveCalled = false

		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "def456")

		formData := url.Values{}
		formData.Set("file_path", "main.go")
		formData.Set("start_line", "1")
		formData.Set("body", "Comment")
		// side omitted

		reqURL := "/api/review/comment?" + params.Encode()
		req := httptest.NewRequest("POST", reqURL, strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.handleAddComment(w, req)

		if mockStorage.review.Comments[0].Side != "right" {
			t.Errorf("Expected default side 'right', got %q", mockStorage.review.Comments[0].Side)
		}
	})

	t.Run("missing required params returns 400", func(t *testing.T) {
		// Missing repo
		formData := url.Values{}
		formData.Set("file_path", "main.go")
		formData.Set("start_line", "1")
		formData.Set("body", "Comment")

		req := httptest.NewRequest("POST", "/api/review/comment?source_commit=a&target_commit=b", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.handleAddComment(w, req)

		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400 for missing repo, got %d", w.Result().StatusCode)
		}
	})

	t.Run("missing body returns 400", func(t *testing.T) {
		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "def456")

		formData := url.Values{}
		formData.Set("file_path", "main.go")
		formData.Set("start_line", "1")
		// body omitted

		reqURL := "/api/review/comment?" + params.Encode()
		req := httptest.NewRequest("POST", reqURL, strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.handleAddComment(w, req)

		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400 for missing body, got %d", w.Result().StatusCode)
		}
	})

	t.Run("invalid start_line returns 400", func(t *testing.T) {
		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "def456")

		formData := url.Values{}
		formData.Set("file_path", "main.go")
		formData.Set("start_line", "not-a-number")
		formData.Set("body", "Comment")

		reqURL := "/api/review/comment?" + params.Encode()
		req := httptest.NewRequest("POST", reqURL, strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.handleAddComment(w, req)

		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400 for invalid start_line, got %d", w.Result().StatusCode)
		}
	})

	t.Run("invalid end_line returns 400", func(t *testing.T) {
		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "def456")

		formData := url.Values{}
		formData.Set("file_path", "main.go")
		formData.Set("start_line", "10")
		formData.Set("end_line", "xyz")
		formData.Set("body", "Comment")

		reqURL := "/api/review/comment?" + params.Encode()
		req := httptest.NewRequest("POST", reqURL, strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.handleAddComment(w, req)

		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400 for invalid end_line, got %d", w.Result().StatusCode)
		}
	})
}

// TestHandleDeleteComment tests deleting a comment
func TestHandleDeleteComment(t *testing.T) {
	server, mockStorage := setupTestServer(t)

	t.Run("happy path deletes comment and redirects", func(t *testing.T) {
		mockStorage.review = &models.Review{
			RepoPath:     "/test/repo",
			SourceCommit: "abc123",
			TargetCommit: "def456",
			Comments: []models.ReviewComment{
				{ID: "comment-1", FilePath: "main.go", StartLine: 10, EndLine: 10, Body: "Fix this", Status: "open"},
				{ID: "comment-2", FilePath: "main.go", StartLine: 20, EndLine: 25, Body: "Refactor", Status: "open"},
			},
			Status: "draft",
		}
		mockStorage.saveCalled = false

		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "def456")
		params.Set("comment_id", "comment-1")
		params.Set("file", "main.go")

		req := httptest.NewRequest("DELETE", "/api/review/comment?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleDeleteComment(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusSeeOther {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected 303, got %d; body: %s", resp.StatusCode, body)
		}

		if !mockStorage.saveCalled {
			t.Error("SaveReview should have been called")
		}

		if len(mockStorage.review.Comments) != 1 {
			t.Fatalf("Expected 1 remaining comment, got %d", len(mockStorage.review.Comments))
		}

		if mockStorage.review.Comments[0].ID != "comment-2" {
			t.Errorf("Expected remaining comment to be 'comment-2', got %q", mockStorage.review.Comments[0].ID)
		}
	})

	t.Run("comment not found returns 404", func(t *testing.T) {
		mockStorage.review = &models.Review{
			Comments: []models.ReviewComment{
				{ID: "comment-1", Body: "Exists"},
			},
		}

		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "def456")
		params.Set("comment_id", "nonexistent-id")

		req := httptest.NewRequest("DELETE", "/api/review/comment?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleDeleteComment(w, req)

		if w.Result().StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404, got %d", w.Result().StatusCode)
		}
	})

	t.Run("missing params returns 400", func(t *testing.T) {
		// Missing comment_id
		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "def456")

		req := httptest.NewRequest("DELETE", "/api/review/comment?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleDeleteComment(w, req)

		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400 for missing comment_id, got %d", w.Result().StatusCode)
		}
	})
}

// TestHandleResolveComment tests resolving/reopening a comment
func TestHandleResolveComment(t *testing.T) {
	server, mockStorage := setupTestServer(t)

	t.Run("resolves open comment", func(t *testing.T) {
		mockStorage.review = &models.Review{
			Comments: []models.ReviewComment{
				{ID: "c1", Body: "Fix", Status: "open"},
			},
		}
		mockStorage.saveCalled = false

		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "def456")
		params.Set("comment_id", "c1")

		req := httptest.NewRequest("POST", "/api/review/comment/resolve?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleResolveComment(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusSeeOther {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected 303, got %d; body: %s", resp.StatusCode, body)
		}

		if mockStorage.review.Comments[0].Status != "resolved" {
			t.Errorf("Expected status 'resolved', got %q", mockStorage.review.Comments[0].Status)
		}
		if mockStorage.review.Comments[0].ResolvedAt == "" {
			t.Error("ResolvedAt should be set")
		}
	})

	t.Run("reopens resolved comment", func(t *testing.T) {
		mockStorage.review = &models.Review{
			Comments: []models.ReviewComment{
				{ID: "c1", Body: "Fix", Status: "resolved", ResolvedAt: "2025-01-01T00:00:00Z"},
			},
		}

		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "def456")
		params.Set("comment_id", "c1")

		req := httptest.NewRequest("POST", "/api/review/comment/resolve?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleResolveComment(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusSeeOther {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected 303, got %d; body: %s", resp.StatusCode, body)
		}

		if mockStorage.review.Comments[0].Status != "open" {
			t.Errorf("Expected status 'open', got %q", mockStorage.review.Comments[0].Status)
		}
		if mockStorage.review.Comments[0].ResolvedAt != "" {
			t.Errorf("ResolvedAt should be cleared, got %q", mockStorage.review.Comments[0].ResolvedAt)
		}
	})

	t.Run("comment not found returns 404", func(t *testing.T) {
		mockStorage.review = &models.Review{
			Comments: []models.ReviewComment{},
		}

		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "def456")
		params.Set("comment_id", "nonexistent")

		req := httptest.NewRequest("POST", "/api/review/comment/resolve?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleResolveComment(w, req)

		if w.Result().StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404, got %d", w.Result().StatusCode)
		}
	})

	t.Run("missing params returns 400", func(t *testing.T) {
		params := url.Values{}
		params.Set("repo", "/test/repo")
		// missing source_commit, target_commit, comment_id

		req := httptest.NewRequest("POST", "/api/review/comment/resolve?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleResolveComment(w, req)

		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400, got %d", w.Result().StatusCode)
		}
	})
}

// TestHandleSubmitReview tests submitting a review
func TestHandleSubmitReview(t *testing.T) {
	t.Run("happy path marks review submitted and renders page", func(t *testing.T) {
		server, mockStorage, tempDir := setupRealTestServer(t)

		mockStorage.review = &models.Review{
			ID:           "review-1",
			RepoPath:     tempDir,
			SourceBranch: "feature",
			TargetBranch: "main",
			SourceCommit: "abc123",
			TargetCommit: "def456",
			Comments: []models.ReviewComment{
				{ID: "c1", FilePath: "test.txt", StartLine: 1, EndLine: 1, Body: "Looks good", Status: "open"},
			},
			Status: "draft",
		}
		mockStorage.saveCalled = false

		params := url.Values{}
		params.Set("repo", tempDir)
		params.Set("source", "feature")
		params.Set("target", "main")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "def456")

		req := httptest.NewRequest("POST", "/api/review/submit?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleSubmitReview(w, req)

		resp := w.Result()
		body, _ := io.ReadAll(resp.Body)

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d; body: %s", resp.StatusCode, body)
		}

		if !mockStorage.saveCalled {
			t.Error("SaveReview should have been called")
		}

		if mockStorage.review.Status != "submitted" {
			t.Errorf("Expected review status 'submitted', got %q", mockStorage.review.Status)
		}

		if mockStorage.review.SubmittedAt == "" {
			t.Error("SubmittedAt should be set")
		}

		if !strings.Contains(string(body), "Review Submitted") {
			t.Errorf("Expected rendered page to contain 'Review Submitted', got: %s", body)
		}
	})

	t.Run("missing params returns 400", func(t *testing.T) {
		server, _ := setupTestServer(t)

		// Missing source_commit and target_commit
		params := url.Values{}
		params.Set("repo", "/test/repo")

		req := httptest.NewRequest("POST", "/api/review/submit?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleSubmitReview(w, req)

		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400, got %d", w.Result().StatusCode)
		}
	})

	t.Run("repo not found returns 404", func(t *testing.T) {
		server, mockStorage := setupTestServer(t)
		mockStorage.repositories = []string{} // empty repo list

		params := url.Values{}
		params.Set("repo", "/nonexistent/repo")
		params.Set("source_commit", "abc")
		params.Set("target_commit", "def")

		req := httptest.NewRequest("POST", "/api/review/submit?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleSubmitReview(w, req)

		if w.Result().StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404 for unknown repo, got %d", w.Result().StatusCode)
		}
	})
}

// TestHandleExportReview tests the markdown export endpoint
func TestHandleExportReview(t *testing.T) {
	t.Run("happy path returns markdown", func(t *testing.T) {
		server, mockStorage, tempDir := setupRealTestServer(t)

		mockStorage.review = &models.Review{
			ID:           "review-1",
			RepoPath:     tempDir,
			SourceBranch: "feature",
			TargetBranch: "main",
			SourceCommit: "abc12345678",
			TargetCommit: "def45678901",
			Comments: []models.ReviewComment{
				{ID: "c1", FilePath: "test.txt", StartLine: 1, EndLine: 1, Body: "Needs work", Status: "open"},
				{ID: "c2", FilePath: "test.txt", StartLine: 5, EndLine: 5, Body: "Already fixed", Status: "resolved"},
			},
			Status: "submitted",
		}

		params := url.Values{}
		params.Set("repo", tempDir)
		params.Set("source", "feature")
		params.Set("target", "main")
		params.Set("source_commit", "abc12345678")
		params.Set("target_commit", "def45678901")

		req := httptest.NewRequest("GET", "/api/review/export?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleExportReview(w, req)

		resp := w.Result()
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d; body: %s", resp.StatusCode, bodyStr)
		}

		contentType := resp.Header.Get("Content-Type")
		if !strings.Contains(contentType, "text/markdown") {
			t.Errorf("Expected Content-Type text/markdown, got %q", contentType)
		}

		// Should contain header
		if !strings.Contains(bodyStr, "# Code Review") {
			t.Errorf("Expected markdown to contain '# Code Review'")
		}

		// Should contain only open comments (1 of 2)
		if !strings.Contains(bodyStr, "Needs work") {
			t.Errorf("Expected markdown to contain open comment 'Needs work'")
		}

		// Should NOT contain resolved comment
		if strings.Contains(bodyStr, "Already fixed") {
			t.Errorf("Expected markdown to NOT contain resolved comment 'Already fixed'")
		}

		// Should contain branch info
		if !strings.Contains(bodyStr, "feature") {
			t.Errorf("Expected markdown to contain source branch 'feature'")
		}
	})

	t.Run("missing params returns 400", func(t *testing.T) {
		server, _ := setupTestServer(t)

		req := httptest.NewRequest("GET", "/api/review/export?repo=/test/repo", nil)
		w := httptest.NewRecorder()

		server.handleExportReview(w, req)

		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400, got %d", w.Result().StatusCode)
		}
	})
}

// TestGenerateMarkdownExport tests the markdown export generation function
func TestGenerateMarkdownExport(t *testing.T) {
	t.Run("generates header with repo and branch info", func(t *testing.T) {
		review := &models.Review{
			RepoPath:     "/home/user/my-project",
			SourceBranch: "feature-xyz",
			TargetBranch: "main",
			SourceCommit: "abc12345deadbeef",
			TargetCommit: "def67890cafebabe",
			Comments:     []models.ReviewComment{},
		}

		md := generateMarkdownExport(review, "")

		if !strings.Contains(md, "# Code Review") {
			t.Error("Expected '# Code Review' header")
		}
		if !strings.Contains(md, "**Repository**: my-project") {
			t.Errorf("Expected repo name 'my-project', got: %s", md)
		}
		if !strings.Contains(md, "**Comparing**: feature-xyz -> main") {
			t.Errorf("Expected branch comparison, got: %s", md)
		}
		if !strings.Contains(md, "**Source commit**: abc12345") {
			t.Errorf("Expected truncated source commit, got: %s", md)
		}
		if !strings.Contains(md, "**Target commit**: def67890") {
			t.Errorf("Expected truncated target commit, got: %s", md)
		}
		if !strings.Contains(md, "**Comments**: 0") {
			t.Errorf("Expected 0 open comments, got: %s", md)
		}
	})

	t.Run("only counts open comments", func(t *testing.T) {
		review := &models.Review{
			RepoPath:     "/repo",
			SourceBranch: "src",
			TargetBranch: "tgt",
			SourceCommit: "abcdefgh",
			TargetCommit: "12345678",
			Comments: []models.ReviewComment{
				{ID: "c1", FilePath: "a.go", StartLine: 1, EndLine: 1, Body: "Open1", Status: "open"},
				{ID: "c2", FilePath: "a.go", StartLine: 2, EndLine: 2, Body: "Resolved1", Status: "resolved"},
				{ID: "c3", FilePath: "b.go", StartLine: 1, EndLine: 1, Body: "Open2", Status: "open"},
			},
		}

		md := generateMarkdownExport(review, "")

		if !strings.Contains(md, "**Comments**: 2") {
			t.Errorf("Expected 2 open comments counted, got: %s", md)
		}

		// Only open comments should be in the body
		if !strings.Contains(md, "Open1") {
			t.Error("Expected open comment 'Open1' in export")
		}
		if !strings.Contains(md, "Open2") {
			t.Error("Expected open comment 'Open2' in export")
		}
		if strings.Contains(md, "Resolved1") {
			t.Error("Resolved comment should NOT appear in export body")
		}
	})

	t.Run("groups comments by file and sorts by line", func(t *testing.T) {
		review := &models.Review{
			RepoPath:     "/repo",
			SourceBranch: "src",
			TargetBranch: "tgt",
			SourceCommit: "abcdefgh",
			TargetCommit: "12345678",
			Comments: []models.ReviewComment{
				{ID: "c1", FilePath: "z.go", StartLine: 20, EndLine: 20, Body: "Z line 20", Status: "open"},
				{ID: "c2", FilePath: "a.go", StartLine: 50, EndLine: 50, Body: "A line 50", Status: "open"},
				{ID: "c3", FilePath: "a.go", StartLine: 10, EndLine: 10, Body: "A line 10", Status: "open"},
				{ID: "c4", FilePath: "z.go", StartLine: 5, EndLine: 5, Body: "Z line 5", Status: "open"},
			},
		}

		md := generateMarkdownExport(review, "")

		// a.go should appear before z.go (alphabetical)
		aIdx := strings.Index(md, "## a.go")
		zIdx := strings.Index(md, "## z.go")
		if aIdx < 0 || zIdx < 0 {
			t.Fatalf("Expected file headers for a.go and z.go, got: %s", md)
		}
		if aIdx > zIdx {
			t.Errorf("Expected a.go before z.go, but a.go at %d, z.go at %d", aIdx, zIdx)
		}

		// Within a.go, line 10 should appear before line 50
		a10Idx := strings.Index(md, "A line 10")
		a50Idx := strings.Index(md, "A line 50")
		if a10Idx > a50Idx {
			t.Errorf("Expected 'A line 10' before 'A line 50'")
		}

		// Within z.go, line 5 should appear before line 20
		z5Idx := strings.Index(md, "Z line 5")
		z20Idx := strings.Index(md, "Z line 20")
		if z5Idx > z20Idx {
			t.Errorf("Expected 'Z line 5' before 'Z line 20'")
		}
	})

	t.Run("renders single line vs range headers", func(t *testing.T) {
		review := &models.Review{
			RepoPath:     "/repo",
			SourceBranch: "src",
			TargetBranch: "tgt",
			SourceCommit: "abcdefgh",
			TargetCommit: "12345678",
			Comments: []models.ReviewComment{
				{ID: "c1", FilePath: "main.go", StartLine: 42, EndLine: 42, Body: "Single", Status: "open"},
				{ID: "c2", FilePath: "main.go", StartLine: 10, EndLine: 15, Body: "Range", Status: "open"},
			},
		}

		md := generateMarkdownExport(review, "")

		if !strings.Contains(md, "### Line 42") {
			t.Errorf("Expected '### Line 42' for single-line comment, got: %s", md)
		}
		if !strings.Contains(md, "### Lines 10-15") {
			t.Errorf("Expected '### Lines 10-15' for range comment, got: %s", md)
		}
	})

	t.Run("renders comment body as blockquote", func(t *testing.T) {
		review := &models.Review{
			RepoPath:     "/repo",
			SourceBranch: "src",
			TargetBranch: "tgt",
			SourceCommit: "abcdefgh",
			TargetCommit: "12345678",
			Comments: []models.ReviewComment{
				{ID: "c1", FilePath: "main.go", StartLine: 1, EndLine: 1, Body: "Line one\nLine two", Status: "open"},
			},
		}

		md := generateMarkdownExport(review, "")

		if !strings.Contains(md, "> Line one\n> Line two") {
			t.Errorf("Expected blockquoted comment body, got: %s", md)
		}
	})

	t.Run("uses commit hashes as labels when branches empty", func(t *testing.T) {
		review := &models.Review{
			RepoPath:     "/repo",
			SourceBranch: "",
			TargetBranch: "",
			SourceCommit: "abc12345deadbeef",
			TargetCommit: "def67890cafebabe",
			Comments:     []models.ReviewComment{},
		}

		md := generateMarkdownExport(review, "")

		if !strings.Contains(md, "**Comparing**: abc12345deadbeef -> def67890cafebabe") {
			t.Errorf("Expected commit hashes as labels, got: %s", md)
		}
	})

	t.Run("includes code context from parsed diff", func(t *testing.T) {
		review := &models.Review{
			RepoPath:     "/repo",
			SourceBranch: "src",
			TargetBranch: "tgt",
			SourceCommit: "abcdefgh",
			TargetCommit: "12345678",
			Comments: []models.ReviewComment{
				{ID: "c1", FilePath: "file.txt", StartLine: 2, EndLine: 2, Body: "Check this line", Status: "open"},
			},
		}

		rawDiff := `diff --git a/file.txt b/file.txt
index 1234..5678 100644
--- a/file.txt
+++ b/file.txt
@@ -1,3 +1,4 @@
 line1
 line2
+line3
 line4`

		md := generateMarkdownExport(review, rawDiff)

		// Should contain a code block with exactly the selected line (line 2)
		if !strings.Contains(md, "```") {
			t.Errorf("Expected code block in markdown, got: %s", md)
		}
		if !strings.Contains(md, "line2") {
			t.Errorf("Expected code context to include selected line 2, got: %s", md)
		}
		// Must NOT include neighboring lines the user did not select
		if strings.Contains(md, "line1\n") || strings.Contains(md, "line4") {
			t.Errorf("Code context should only contain selected lines, not neighbors, got: %s", md)
		}
	})

	t.Run("detects language for code block", func(t *testing.T) {
		review := &models.Review{
			RepoPath:     "/repo",
			SourceBranch: "src",
			TargetBranch: "tgt",
			SourceCommit: "abcdefgh",
			TargetCommit: "12345678",
			Comments: []models.ReviewComment{
				{ID: "c1", FilePath: "main.go", StartLine: 2, EndLine: 2, Body: "Check", Status: "open"},
			},
		}

		rawDiff := `diff --git a/main.go b/main.go
index 1234..5678 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
 func main() {}
+// new comment
 var x = 1`

		md := generateMarkdownExport(review, rawDiff)

		if !strings.Contains(md, "```go") {
			t.Errorf("Expected Go language detection in code block, got: %s", md)
		}
	})
}

// TestGetCodeContext tests the code context extraction helper
func TestGetCodeContext(t *testing.T) {
	t.Run("returns exact lines matching target range", func(t *testing.T) {
		fileMap := map[string]models.DiffFile{
			"test.go": {
				Path: "test.go",
				Sections: []models.DiffHunk{
					{
						Lines: []string{" line1", " line2", "+line3", " line4", " line5", " line6"},
						LineNumbers: struct {
							Left  []int `json:"left"`
							Right []int `json:"right"`
						}{
							Left:  []int{1, 2, 0, 3, 4, 5},
							Right: []int{1, 2, 3, 4, 5, 6},
						},
					},
				},
			},
		}

		lines := getCodeContext(fileMap, "test.go", 3, 3, "right")
		// Should include only the exact line at right=3, not surrounding lines
		if len(lines) != 1 {
			t.Fatalf("Expected exactly 1 line for single-line selection, got %d: %v", len(lines), lines)
		}
		if lines[0] != "line3" {
			t.Errorf("Expected 'line3' (stripped prefix), got: %q", lines[0])
		}
	})

	t.Run("returns nil for unknown file", func(t *testing.T) {
		fileMap := map[string]models.DiffFile{}
		lines := getCodeContext(fileMap, "nonexistent.go", 1, 1, "right")
		if lines != nil {
			t.Errorf("Expected nil for unknown file, got %v", lines)
		}
	})

	t.Run("strips diff prefixes", func(t *testing.T) {
		fileMap := map[string]models.DiffFile{
			"test.go": {
				Path: "test.go",
				Sections: []models.DiffHunk{
					{
						Lines: []string{"+added", "-removed", " context"},
						LineNumbers: struct {
							Left  []int `json:"left"`
							Right []int `json:"right"`
						}{
							Left:  []int{0, 1, 2},
							Right: []int{1, 0, 2},
						},
					},
				},
			},
		}

		lines := getCodeContext(fileMap, "test.go", 1, 2, "right")
		if len(lines) == 0 {
			t.Fatal("Expected some context lines")
		}
		for _, l := range lines {
			if len(l) > 0 && (l[0] == '+' || l[0] == '-') {
				t.Errorf("Expected diff prefix to be stripped, but line still starts with prefix: %q", l)
			}
		}
	})

	t.Run("left side matches deleted lines", func(t *testing.T) {
		fileMap := map[string]models.DiffFile{
			"test.go": {
				Path: "test.go",
				Sections: []models.DiffHunk{
					{
						Lines: []string{" ctx", "-deleted1", "-deleted2", "+added1", " ctx2"},
						LineNumbers: struct {
							Left  []int `json:"left"`
							Right []int `json:"right"`
						}{
							Left:  []int{10, 11, 12, 0, 13},
							Right: []int{10, 0, 0, 11, 12},
						},
					},
				},
			},
		}

		// Right side should NOT find line 11 on left (it's a deletion, right=0)
		rightLines := getCodeContext(fileMap, "test.go", 11, 12, "right")
		// Left side SHOULD find lines 11 and 12 (deleted lines)
		leftLines := getCodeContext(fileMap, "test.go", 11, 12, "left")

		if len(leftLines) == 0 {
			t.Fatal("Expected left-side context to find deleted lines")
		}

		foundDeleted := false
		for _, l := range leftLines {
			if l == "deleted1" || l == "deleted2" {
				foundDeleted = true
			}
		}
		if !foundDeleted {
			t.Errorf("Expected deleted lines in left-side context, got: %v", leftLines)
		}

		// Right side at line 11 matches the added line — verify it's different content
		_ = rightLines // right side matches right=11 which is "+added1"
	})

	t.Run("both side checks either side", func(t *testing.T) {
		fileMap := map[string]models.DiffFile{
			"test.go": {
				Path: "test.go",
				Sections: []models.DiffHunk{
					{
						Lines: []string{"-old", "+new"},
						LineNumbers: struct {
							Left  []int `json:"left"`
							Right []int `json:"right"`
						}{
							Left:  []int{5, 0},
							Right: []int{0, 5},
						},
					},
				},
			},
		}

		bothLines := getCodeContext(fileMap, "test.go", 5, 5, "both")
		if len(bothLines) != 2 {
			t.Errorf("Expected both deleted and added lines, got %d lines: %v", len(bothLines), bothLines)
		}
	})
}

// TestBuildDiffRedirectURL tests the URL builder helper
func TestBuildDiffRedirectURL(t *testing.T) {
	t.Run("includes all params", func(t *testing.T) {
		url := buildDiffRedirectURL("/test/repo", "feature", "main", "abc", "def", "branches", "file.go")
		if !strings.Contains(url, "/diff?") {
			t.Errorf("Expected /diff? prefix, got %s", url)
		}
		if !strings.Contains(url, "repo=") {
			t.Errorf("Expected repo param, got %s", url)
		}
		if !strings.Contains(url, "file=file.go") {
			t.Errorf("Expected file param, got %s", url)
		}
	})

	t.Run("omits file param when empty", func(t *testing.T) {
		url := buildDiffRedirectURL("/test/repo", "feature", "main", "abc", "def", "branches", "")
		if strings.Contains(url, "file=") {
			t.Errorf("Expected no file param when empty, got %s", url)
		}
	})
}

// TestHandleDiffViewUnstagedWithFileParam tests the diff view handler in
// unstaged mode with a file parameter after modifying test.txt without staging.
func TestHandleDiffViewUnstagedWithFileParam(t *testing.T) {
	srv, _, tempDir := setupRealTestServer(t)

	// Modify test.txt without staging the change
	testFilePath := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFilePath, []byte("initial content\nmodified"), 0644); err != nil {
		t.Fatalf("Failed to modify test.txt: %v", err)
	}

	reqURL := fmt.Sprintf("/diff?repo=%s&mode=unstaged&file=test.txt", url.QueryEscape(tempDir))
	req := httptest.NewRequest("GET", reqURL, nil)
	w := httptest.NewRecorder()

	srv.handleDiffView(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d; body: %s", resp.StatusCode, bodyStr)
	}

	if !strings.Contains(bodyStr, "SelectedFile=test.txt") {
		t.Errorf("Expected body to contain 'SelectedFile=test.txt', got: %s", bodyStr)
	}

	if !strings.Contains(bodyStr, "DiffMode=unstaged") {
		t.Errorf("Expected body to contain 'DiffMode=unstaged', got: %s", bodyStr)
	}
}

// ---------- Low-Coverage Handler Tests ----------

// ErrorMockStorage wraps MockStorage but allows injecting errors for specific operations.
type ErrorMockStorage struct {
	MockStorage
	saveRepoErr        error
	loadRepoErr        error
	saveReviewStateErr error
	loadReviewStateErr error
	saveReviewErr      error
	loadReviewErr      error
}

func (m *ErrorMockStorage) SaveRepositories(repos []string) error {
	if m.saveRepoErr != nil {
		return m.saveRepoErr
	}
	return m.MockStorage.SaveRepositories(repos)
}

func (m *ErrorMockStorage) LoadRepositories() ([]string, error) {
	if m.loadRepoErr != nil {
		return nil, m.loadRepoErr
	}
	return m.MockStorage.LoadRepositories()
}

func (m *ErrorMockStorage) SaveReviewState(state *models.ReviewState, repoPath string) error {
	if m.saveReviewStateErr != nil {
		return m.saveReviewStateErr
	}
	return m.MockStorage.SaveReviewState(state, repoPath)
}

func (m *ErrorMockStorage) LoadReviewState(repoPath, sourceBranch, targetBranch, sourceCommit, targetCommit string) (*models.ReviewState, error) {
	if m.loadReviewStateErr != nil {
		return nil, m.loadReviewStateErr
	}
	return m.MockStorage.LoadReviewState(repoPath, sourceBranch, targetBranch, sourceCommit, targetCommit)
}

func (m *ErrorMockStorage) SaveReview(review *models.Review, repoPath string) error {
	if m.saveReviewErr != nil {
		return m.saveReviewErr
	}
	return m.MockStorage.SaveReview(review, repoPath)
}

func (m *ErrorMockStorage) LoadReview(repoPath, sourceBranch, targetBranch, sourceCommit, targetCommit string) (*models.Review, error) {
	if m.loadReviewErr != nil {
		return nil, m.loadReviewErr
	}
	return m.MockStorage.LoadReview(repoPath, sourceBranch, targetBranch, sourceCommit, targetCommit)
}

// setupTestServerWithErrorStorage creates a server backed by an ErrorMockStorage so
// that individual storage operations can be made to fail.
func setupTestServerWithErrorStorage(t *testing.T, ems *ErrorMockStorage) *Server {
	t.Helper()

	origFS := getTemplateDir
	getTemplateDir = func() fs.FS {
		return baseTestTemplates()
	}
	t.Cleanup(func() {
		getTemplateDir = origFS
	})

	server, err := New(ems)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	return server
}

// TestHandleAddRepository tests the handleAddRepository handler across success and error paths.
func TestHandleAddRepository(t *testing.T) {
	t.Run("valid repo path redirects to index", func(t *testing.T) {
		server, mockStorage := setupTestServer(t)

		// Create a temporary directory with a .git folder to simulate a valid repo
		tempDir, err := os.MkdirTemp("", "diffty-add-repo-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(tempDir) })

		if err := os.Mkdir(filepath.Join(tempDir, ".git"), 0755); err != nil {
			t.Fatalf("Failed to create .git dir: %v", err)
		}

		formData := url.Values{}
		formData.Set("path", tempDir)

		req := httptest.NewRequest("POST", "/api/add-repo", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.handleAddRepository(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusSeeOther {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status %d, got %d; body: %s", http.StatusSeeOther, resp.StatusCode, body)
		}

		location := resp.Header.Get("Location")
		if location != "/" {
			t.Errorf("Expected redirect to '/', got %q", location)
		}

		// Verify the repo was saved
		absPath, _ := filepath.Abs(tempDir)
		found := false
		for _, r := range mockStorage.repositories {
			if r == absPath {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected repository %q to be saved, repos: %v", absPath, mockStorage.repositories)
		}
	})

	t.Run("empty path returns 400", func(t *testing.T) {
		server, _ := setupTestServer(t)

		formData := url.Values{}
		formData.Set("path", "")

		req := httptest.NewRequest("POST", "/api/add-repo", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.handleAddRepository(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "Missing Path") {
			t.Errorf("Expected error about missing path, got: %s", body)
		}
	})

	t.Run("invalid repo path (no .git) returns 500", func(t *testing.T) {
		server, _ := setupTestServer(t)

		// Create a temporary directory WITHOUT a .git folder
		tempDir, err := os.MkdirTemp("", "diffty-invalid-repo-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(tempDir) })

		formData := url.Values{}
		formData.Set("path", tempDir)

		req := httptest.NewRequest("POST", "/api/add-repo", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.handleAddRepository(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "Repository Error") {
			t.Errorf("Expected 'Repository Error', got: %s", body)
		}
	})

	t.Run("storage save failure returns 500", func(t *testing.T) {
		ems := &ErrorMockStorage{
			MockStorage: MockStorage{
				repositories: []string{},
			},
			saveRepoErr: fmt.Errorf("disk full"),
		}
		server := setupTestServerWithErrorStorage(t, ems)

		// Create a valid repo directory
		tempDir, err := os.MkdirTemp("", "diffty-save-fail-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(tempDir) })

		if err := os.Mkdir(filepath.Join(tempDir, ".git"), 0755); err != nil {
			t.Fatalf("Failed to create .git dir: %v", err)
		}

		formData := url.Values{}
		formData.Set("path", tempDir)

		req := httptest.NewRequest("POST", "/api/add-repo", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.handleAddRepository(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "Repository Error") {
			t.Errorf("Expected 'Repository Error', got: %s", body)
		}
	})

	t.Run("missing path field returns 400", func(t *testing.T) {
		server, _ := setupTestServer(t)

		// POST with no form data at all
		req := httptest.NewRequest("POST", "/api/add-repo", strings.NewReader(""))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.handleAddRepository(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
		}
	})
}

// TestRenderWithStatus tests the renderWithStatus method with various status codes and error conditions.
func TestRenderWithStatus(t *testing.T) {
	t.Run("renders with 200 status", func(t *testing.T) {
		server, _ := setupTestServer(t)
		w := httptest.NewRecorder()

		server.renderWithStatus(w, "index.html", nil, http.StatusOK)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "Index Page") {
			t.Errorf("Expected body to contain 'Index Page', got: %s", body)
		}

		contentType := resp.Header.Get("Content-Type")
		if !strings.Contains(contentType, "text/html") {
			t.Errorf("Expected Content-Type text/html, got %q", contentType)
		}
	})

	t.Run("renders with 404 status", func(t *testing.T) {
		server, _ := setupTestServer(t)
		w := httptest.NewRecorder()

		errorData := map[string]interface{}{
			"Title":   "Not Found",
			"Message": "Page not found",
		}
		server.renderWithStatus(w, "error.html", errorData, http.StatusNotFound)

		resp := w.Result()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "Not Found") {
			t.Errorf("Expected body to contain 'Not Found', got: %s", body)
		}
	})

	t.Run("renders with 500 status for custom error", func(t *testing.T) {
		server, _ := setupTestServer(t)
		w := httptest.NewRecorder()

		errorData := map[string]interface{}{
			"Title":   "Server Error",
			"Message": "Something went wrong",
		}
		server.renderWithStatus(w, "error.html", errorData, http.StatusInternalServerError)

		resp := w.Result()
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, resp.StatusCode)
		}
	})

	t.Run("invalid template name returns 500", func(t *testing.T) {
		server, _ := setupTestServer(t)
		w := httptest.NewRecorder()

		server.renderWithStatus(w, "nonexistent.html", nil, http.StatusOK)

		resp := w.Result()
		// renderWithStatus should fall back to 500 when template execution fails
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("Expected status %d for invalid template, got %d", http.StatusInternalServerError, resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "Internal Server Error") {
			t.Errorf("Expected fallback error page, got: %s", body)
		}
	})

	t.Run("preserves status code in response", func(t *testing.T) {
		server, _ := setupTestServer(t)

		// Test with an unusual but valid status code
		w := httptest.NewRecorder()
		server.renderWithStatus(w, "index.html", nil, http.StatusAccepted)

		resp := w.Result()
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("Expected status %d, got %d", http.StatusAccepted, resp.StatusCode)
		}
	})
}

// TestHandleCompareEdgeCases tests additional edge cases for handleCompare.
func TestHandleCompareEdgeCases(t *testing.T) {
	t.Run("GET with missing repo redirects to index", func(t *testing.T) {
		server, _ := setupTestServer(t)

		req := httptest.NewRequest("GET", "/compare", nil)
		w := httptest.NewRecorder()

		server.handleCompare(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("Expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
		}

		location := resp.Header.Get("Location")
		if location != "/" {
			t.Errorf("Expected redirect to '/', got %q", location)
		}
	})

	t.Run("GET with repo not in storage returns 404", func(t *testing.T) {
		server, mockStorage := setupTestServer(t)
		mockStorage.repositories = []string{} // no repos registered

		req := httptest.NewRequest("GET", "/compare?repo=/nonexistent/repo", nil)
		w := httptest.NewRecorder()

		server.handleCompare(w, req)

		resp := w.Result()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d; body: %s", http.StatusNotFound, resp.StatusCode, body)
		}

		if !strings.Contains(string(body), "Not Found") {
			t.Errorf("Expected 'Not Found' in body, got: %s", body)
		}
	})

	t.Run("POST with missing repo returns 400", func(t *testing.T) {
		server, _ := setupTestServer(t)

		formData := url.Values{}
		formData.Set("source", "feature")
		formData.Set("target", "main")
		// no repo

		req := httptest.NewRequest("POST", "/compare", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.handleCompare(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusBadRequest {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status %d, got %d; body: %s", http.StatusBadRequest, resp.StatusCode, body)
		}
	})

	t.Run("POST branches mode with missing source and target returns 400", func(t *testing.T) {
		server, _, tempDir := setupRealTestServer(t)

		formData := url.Values{}
		formData.Set("repo", tempDir)
		formData.Set("mode", "branches")
		// no source or target

		req := httptest.NewRequest("POST", "/compare", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.handleCompare(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusBadRequest {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status %d, got %d; body: %s", http.StatusBadRequest, resp.StatusCode, body)
		}

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "Missing Branches") {
			t.Errorf("Expected 'Missing Branches' error, got: %s", body)
		}
	})

	t.Run("POST with repo not in storage returns 404", func(t *testing.T) {
		server, mockStorage := setupTestServer(t)
		mockStorage.repositories = []string{} // empty

		formData := url.Values{}
		formData.Set("repo", "/nonexistent/repo")
		formData.Set("source", "feature")
		formData.Set("target", "main")

		req := httptest.NewRequest("POST", "/compare", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.handleCompare(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusNotFound {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status %d, got %d; body: %s", http.StatusNotFound, resp.StatusCode, body)
		}
	})

	t.Run("POST staged mode redirects to diff view", func(t *testing.T) {
		server, _, tempDir := setupRealTestServer(t)

		formData := url.Values{}
		formData.Set("repo", tempDir)
		formData.Set("mode", "staged")

		req := httptest.NewRequest("POST", "/compare", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.handleCompare(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusSeeOther {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status %d, got %d; body: %s", http.StatusSeeOther, resp.StatusCode, body)
		}

		location := resp.Header.Get("Location")
		if !strings.Contains(location, "/diff") {
			t.Errorf("Expected redirect to /diff, got: %s", location)
		}
		if !strings.Contains(location, "mode=staged") {
			t.Errorf("Expected mode=staged in location, got: %s", location)
		}
	})

	t.Run("POST with invalid mode defaults to branches", func(t *testing.T) {
		server, _, tempDir := setupRealTestServer(t)

		formData := url.Values{}
		formData.Set("repo", tempDir)
		formData.Set("source", "feature")
		formData.Set("target", "main")
		formData.Set("mode", "invalid-mode")

		req := httptest.NewRequest("POST", "/compare", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.handleCompare(w, req)

		resp := w.Result()
		// Should treat invalid mode as branches and redirect to diff view
		if resp.StatusCode != http.StatusSeeOther {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status %d, got %d; body: %s", http.StatusSeeOther, resp.StatusCode, body)
		}

		location := resp.Header.Get("Location")
		if !strings.Contains(location, "mode=branches") {
			t.Errorf("Expected mode=branches in location, got: %s", location)
		}
	})
}

// TestHandleReviewStateEdgeCases tests additional edge cases for handleReviewState.
func TestHandleReviewStateEdgeCases(t *testing.T) {
	t.Run("branches mode missing all required fields returns 400", func(t *testing.T) {
		server, _ := setupTestServer(t)

		// Only provide repo — missing source, target, commits, file, status
		params := url.Values{}
		params.Set("repo", "/test/repo")

		req := httptest.NewRequest("POST", "/api/review-state?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleReviewState(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusBadRequest {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status %d, got %d; body: %s", http.StatusBadRequest, resp.StatusCode, body)
		}

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "Missing Parameters") {
			t.Errorf("Expected 'Missing Parameters' error, got: %s", body)
		}
	})

	t.Run("invalid status value returns 400", func(t *testing.T) {
		server, _ := setupTestServer(t)

		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source", "feature")
		params.Set("target", "main")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "def456")
		params.Set("file", "file.txt")
		params.Set("status", "invalid-status")

		req := httptest.NewRequest("POST", "/api/review-state?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleReviewState(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusBadRequest {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status %d, got %d; body: %s", http.StatusBadRequest, resp.StatusCode, body)
		}

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "Invalid Status") {
			t.Errorf("Expected 'Invalid Status' error, got: %s", body)
		}
	})

	t.Run("storage load failure returns 500", func(t *testing.T) {
		ems := &ErrorMockStorage{
			MockStorage: MockStorage{
				repositories: []string{"/test/repo"},
			},
			loadReviewStateErr: fmt.Errorf("database connection lost"),
		}
		server := setupTestServerWithErrorStorage(t, ems)

		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source", "feature")
		params.Set("target", "main")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "def456")
		params.Set("file", "file.txt")
		params.Set("status", "approved")

		req := httptest.NewRequest("POST", "/api/review-state?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleReviewState(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusInternalServerError {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status %d, got %d; body: %s", http.StatusInternalServerError, resp.StatusCode, body)
		}

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "Review State Error") {
			t.Errorf("Expected 'Review State Error', got: %s", body)
		}
	})

	t.Run("storage save failure returns 500", func(t *testing.T) {
		ems := &ErrorMockStorage{
			MockStorage: MockStorage{
				repositories: []string{"/test/repo"},
				reviewState: &models.ReviewState{
					ReviewedFiles: []models.FileReview{},
					SourceBranch:  "feature",
					TargetBranch:  "main",
					SourceCommit:  "abc123",
					TargetCommit:  "def456",
				},
			},
			saveReviewStateErr: fmt.Errorf("disk full"),
		}
		server := setupTestServerWithErrorStorage(t, ems)

		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source", "feature")
		params.Set("target", "main")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "def456")
		params.Set("file", "file.txt")
		params.Set("status", "approved")

		req := httptest.NewRequest("POST", "/api/review-state?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleReviewState(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusInternalServerError {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status %d, got %d; body: %s", http.StatusInternalServerError, resp.StatusCode, body)
		}

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "Review State Error") {
			t.Errorf("Expected 'Review State Error', got: %s", body)
		}
	})

	t.Run("successful state change with redirect to next file", func(t *testing.T) {
		server, mockStorage := setupTestServer(t)

		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source", "feature")
		params.Set("target", "main")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "def456")
		params.Set("file", "file1.txt")
		params.Set("status", "approved")
		params.Set("next", "file2.txt")

		req := httptest.NewRequest("POST", "/api/review-state?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleReviewState(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusSeeOther {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status %d, got %d; body: %s", http.StatusSeeOther, resp.StatusCode, body)
		}

		// Verify redirect goes to next file
		location := resp.Header.Get("Location")
		if !strings.Contains(location, "file=file2.txt") {
			t.Errorf("Expected redirect to file2.txt, got: %s", location)
		}

		// Verify state was saved
		if !mockStorage.saveCalled {
			t.Error("SaveReviewState should have been called")
		}

		// Verify the reviewed file was added
		if mockStorage.reviewState == nil || len(mockStorage.reviewState.ReviewedFiles) == 0 {
			t.Fatal("Expected at least one reviewed file")
		}

		found := false
		for _, fr := range mockStorage.reviewState.ReviewedFiles {
			if fr.Path == "file1.txt" && fr.Lines["all"] == models.StateApproved {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected file1.txt to be approved, state: %+v", mockStorage.reviewState.ReviewedFiles)
		}
	})

	t.Run("updates existing file review instead of duplicating", func(t *testing.T) {
		server, mockStorage := setupTestServer(t)

		// Pre-populate review state with an existing file review
		mockStorage.reviewState = &models.ReviewState{
			ReviewedFiles: []models.FileReview{
				{
					Repo:  "/test/repo",
					Path:  "file.txt",
					Lines: map[string]string{"all": models.StateApproved},
				},
			},
			SourceBranch: "feature",
			TargetBranch: "main",
			SourceCommit: "abc123",
			TargetCommit: "def456",
		}

		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source", "feature")
		params.Set("target", "main")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "def456")
		params.Set("file", "file.txt")
		params.Set("status", "rejected") // change from approved to rejected

		req := httptest.NewRequest("POST", "/api/review-state?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleReviewState(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusSeeOther {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status %d, got %d; body: %s", http.StatusSeeOther, resp.StatusCode, body)
		}

		// Should still have exactly 1 file review, not 2
		if len(mockStorage.reviewState.ReviewedFiles) != 1 {
			t.Errorf("Expected 1 reviewed file (updated), got %d", len(mockStorage.reviewState.ReviewedFiles))
		}

		// Verify status was updated
		if mockStorage.reviewState.ReviewedFiles[0].Lines["all"] != models.StateRejected {
			t.Errorf("Expected status 'rejected', got %q", mockStorage.reviewState.ReviewedFiles[0].Lines["all"])
		}
	})

	t.Run("skipped status is valid", func(t *testing.T) {
		server, mockStorage := setupTestServer(t)

		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source", "feature")
		params.Set("target", "main")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "def456")
		params.Set("file", "file.txt")
		params.Set("status", "skipped")

		req := httptest.NewRequest("POST", "/api/review-state?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleReviewState(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusSeeOther {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status %d, got %d; body: %s", http.StatusSeeOther, resp.StatusCode, body)
		}

		if !mockStorage.saveCalled {
			t.Error("SaveReviewState should have been called")
		}
	})

	t.Run("staged mode missing file returns 400", func(t *testing.T) {
		server, _ := setupTestServer(t)

		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "staged")
		params.Set("status", "approved")
		params.Set("mode", "staged")
		// Missing file

		req := httptest.NewRequest("POST", "/api/review-state?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleReviewState(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusBadRequest {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status %d, got %d; body: %s", http.StatusBadRequest, resp.StatusCode, body)
		}
	})

	t.Run("staged mode missing status returns 400", func(t *testing.T) {
		server, _ := setupTestServer(t)

		params := url.Values{}
		params.Set("repo", "/test/repo")
		params.Set("source_commit", "abc123")
		params.Set("target_commit", "staged")
		params.Set("file", "file.txt")
		params.Set("mode", "staged")
		// Missing status

		req := httptest.NewRequest("POST", "/api/review-state?"+params.Encode(), nil)
		w := httptest.NewRecorder()

		server.handleReviewState(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusBadRequest {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status %d, got %d; body: %s", http.StatusBadRequest, resp.StatusCode, body)
		}
	})
}

// browseResponse mirrors the anonymous response struct in handleBrowse.
// Kept in sync manually — update if the handler response shape changes.
type browseResponse struct {
	CurrentPath string        `json:"current_path"`
	ParentPath  string        `json:"parent_path"`
	IsGitRepo   bool          `json:"is_git_repo"`
	Entries     []browseEntry `json:"entries"`
}

// browseRequest is a test helper that sends a GET /api/browse request and returns the parsed response.
func browseRequest(t *testing.T, s *Server, path string) browseResponse {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/browse?path="+url.QueryEscape(path), nil)
	rr := httptest.NewRecorder()
	s.handleBrowse(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("browseRequest(%q): expected status 200, got %d: %s", path, rr.Code, rr.Body.String())
	}
	var resp browseResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("browseRequest(%q): failed to decode JSON: %v", path, err)
	}
	return resp
}

// mkdirOrFail creates a directory tree, failing the test if it cannot.
func mkdirOrFail(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdirOrFail(%q): %v", path, err)
	}
}

// TestHandleBrowseJSONStructure tests the browse endpoint with proper JSON deserialization
func TestHandleBrowseJSONStructure(t *testing.T) {
	t.Run("response deserializes to expected structure", func(t *testing.T) {
		server, _ := setupTestServer(t)

		tmpDir := t.TempDir()
		mkdirOrFail(t, filepath.Join(tmpDir, "plain-dir"))
		repoDir := filepath.Join(tmpDir, "my-repo")
		mkdirOrFail(t, filepath.Join(repoDir, ".git"))

		parsed := browseRequest(t, server, tmpDir)

		if parsed.CurrentPath != tmpDir {
			t.Errorf("Expected current_path=%q, got %q", tmpDir, parsed.CurrentPath)
		}
		if parsed.ParentPath != filepath.Dir(tmpDir) {
			t.Errorf("Expected parent_path=%q, got %q", filepath.Dir(tmpDir), parsed.ParentPath)
		}
		if parsed.IsGitRepo {
			t.Error("Expected is_git_repo=false for tmp dir, got true")
		}
		if len(parsed.Entries) != 2 {
			t.Fatalf("Expected 2 entries, got %d", len(parsed.Entries))
		}

		// Git repo should be first (sorted first)
		if parsed.Entries[0].Name != "my-repo" {
			t.Errorf("Expected first entry to be 'my-repo', got %q", parsed.Entries[0].Name)
		}
		if !parsed.Entries[0].IsGitRepo {
			t.Error("Expected first entry is_git_repo=true")
		}
		if parsed.Entries[0].Path != repoDir {
			t.Errorf("Expected first entry path=%q, got %q", repoDir, parsed.Entries[0].Path)
		}

		if parsed.Entries[1].Name != "plain-dir" {
			t.Errorf("Expected second entry to be 'plain-dir', got %q", parsed.Entries[1].Name)
		}
		if parsed.Entries[1].IsGitRepo {
			t.Error("Expected second entry is_git_repo=false")
		}
	})

	t.Run("default path returns home directory", func(t *testing.T) {
		server, _ := setupTestServer(t)

		// No path param — handler defaults to home directory
		req := httptest.NewRequest("GET", "/api/browse", nil)
		rr := httptest.NewRecorder()
		server.handleBrowse(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d: %s", rr.Code, rr.Body.String())
		}
		if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", ct)
		}

		var parsed browseResponse
		if err := json.NewDecoder(rr.Body).Decode(&parsed); err != nil {
			t.Fatalf("Failed to decode JSON: %v", err)
		}
		if parsed.CurrentPath == "" {
			t.Error("Expected non-empty current_path for default (home) path")
		}
		if parsed.ParentPath == "" {
			t.Error("Expected non-empty parent_path for default (home) path")
		}
	})

	t.Run("current directory git detection", func(t *testing.T) {
		server, _ := setupTestServer(t)

		tmpDir := t.TempDir()
		mkdirOrFail(t, filepath.Join(tmpDir, ".git"))

		parsed := browseRequest(t, server, tmpDir)

		if !parsed.IsGitRepo {
			t.Error("Expected is_git_repo=true for directory with .git, got false")
		}
	})

	t.Run("nonexistent path returns 404", func(t *testing.T) {
		server, _ := setupTestServer(t)

		req := httptest.NewRequest("GET", "/api/browse?path=/nonexistent/path/that/should/not/exist", nil)
		rr := httptest.NewRecorder()
		server.handleBrowse(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d; body: %s", http.StatusNotFound, rr.Code, rr.Body.String())
		}
	})

	t.Run("file path returns 400", func(t *testing.T) {
		server, _ := setupTestServer(t)

		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "afile.txt")
		if err := os.WriteFile(tmpFile, []byte("hello"), 0644); err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/browse?path="+url.QueryEscape(tmpFile), nil)
		rr := httptest.NewRecorder()
		server.handleBrowse(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d; body: %s", http.StatusBadRequest, rr.Code, rr.Body.String())
		}
	})

	t.Run("hidden directories are excluded", func(t *testing.T) {
		server, _ := setupTestServer(t)

		tmpDir := t.TempDir()
		mkdirOrFail(t, filepath.Join(tmpDir, ".hidden"))
		mkdirOrFail(t, filepath.Join(tmpDir, "visible"))

		parsed := browseRequest(t, server, tmpDir)

		for _, entry := range parsed.Entries {
			if entry.Name == ".hidden" {
				t.Error("Expected hidden directory to be excluded from entries")
			}
		}
		found := false
		for _, entry := range parsed.Entries {
			if entry.Name == "visible" {
				found = true
			}
		}
		if !found {
			t.Error("Expected visible directory to be included in entries")
		}
	})

	t.Run("files are excluded from listing", func(t *testing.T) {
		server, _ := setupTestServer(t)

		tmpDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(tmpDir, "readme.md"), []byte("hi"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
		mkdirOrFail(t, filepath.Join(tmpDir, "src"))

		parsed := browseRequest(t, server, tmpDir)

		for _, entry := range parsed.Entries {
			if entry.Name == "readme.md" {
				t.Error("Expected files to be excluded from listing")
			}
		}
		if len(parsed.Entries) != 1 || parsed.Entries[0].Name != "src" {
			t.Errorf("Expected only 'src' directory in entries, got %+v", parsed.Entries)
		}
	})

	t.Run("empty directory returns empty entries array", func(t *testing.T) {
		server, _ := setupTestServer(t)

		tmpDir := t.TempDir()

		parsed := browseRequest(t, server, tmpDir)

		if parsed.CurrentPath != tmpDir {
			t.Errorf("Expected current_path=%q, got %q", tmpDir, parsed.CurrentPath)
		}
		if len(parsed.Entries) != 0 {
			t.Errorf("Expected 0 entries for empty dir, got %d: %+v", len(parsed.Entries), parsed.Entries)
		}
	})

	t.Run("parent path is correct", func(t *testing.T) {
		server, _ := setupTestServer(t)

		tmpDir := t.TempDir()
		childDir := filepath.Join(tmpDir, "child")
		mkdirOrFail(t, childDir)

		parsed := browseRequest(t, server, childDir)

		if parsed.ParentPath != tmpDir {
			t.Errorf("Expected parent_path=%q, got %q", tmpDir, parsed.ParentPath)
		}
	})

	t.Run("relative path is resolved to absolute", func(t *testing.T) {
		server, _ := setupTestServer(t)

		// Use "." as a relative path — should resolve to current working directory
		parsed := browseRequest(t, server, ".")

		if !filepath.IsAbs(parsed.CurrentPath) {
			t.Errorf("Expected current_path to be absolute, got %q", parsed.CurrentPath)
		}
	})

	t.Run("alphabetical ordering within same group", func(t *testing.T) {
		server, _ := setupTestServer(t)

		tmpDir := t.TempDir()
		for _, name := range []string{"zebra", "alpha", "mango"} {
			mkdirOrFail(t, filepath.Join(tmpDir, name))
		}

		parsed := browseRequest(t, server, tmpDir)

		if len(parsed.Entries) != 3 {
			t.Fatalf("Expected 3 entries, got %d", len(parsed.Entries))
		}
		if parsed.Entries[0].Name != "alpha" {
			t.Errorf("Expected first entry 'alpha', got %q", parsed.Entries[0].Name)
		}
		if parsed.Entries[1].Name != "mango" {
			t.Errorf("Expected second entry 'mango', got %q", parsed.Entries[1].Name)
		}
		if parsed.Entries[2].Name != "zebra" {
			t.Errorf("Expected third entry 'zebra', got %q", parsed.Entries[2].Name)
		}
	})

	t.Run("unreadable directory returns 403", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Permission tests unreliable on Windows")
		}
		if os.Getuid() == 0 {
			t.Skip("Running as root — permission checks are bypassed")
		}

		server, _ := setupTestServer(t)

		tmpDir := t.TempDir()
		noRead := filepath.Join(tmpDir, "noperm")
		mkdirOrFail(t, noRead)
		if err := os.Chmod(noRead, 0000); err != nil {
			t.Fatalf("Failed to chmod: %v", err)
		}
		t.Cleanup(func() {
			os.Chmod(noRead, 0755) // restore so TempDir cleanup works
		})

		req := httptest.NewRequest("GET", "/api/browse?path="+url.QueryEscape(noRead), nil)
		rr := httptest.NewRecorder()
		server.handleBrowse(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status %d, got %d; body: %s", http.StatusForbidden, rr.Code, rr.Body.String())
		}
	})
}

// TestHandleBrowseViaRouter tests the browse endpoint is accessible through the router
func TestHandleBrowseViaRouter(t *testing.T) {
	server, _ := setupTestServer(t)

	tmpDir := t.TempDir()
	router := server.Router()

	req := httptest.NewRequest("GET", "/api/browse?path="+url.QueryEscape(tmpDir), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d via router, got %d; body: %s", http.StatusOK, resp.StatusCode, body)
	}

	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type application/json via router, got %s", resp.Header.Get("Content-Type"))
	}

	var parsed browseResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal response from router: %v; body: %s", err, body)
	}

	if parsed.CurrentPath != tmpDir {
		t.Errorf("Expected current_path=%q, got %q", tmpDir, parsed.CurrentPath)
	}
}
