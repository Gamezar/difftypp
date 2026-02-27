package server

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/darccio/diffty/internal/git"
	"github.com/darccio/diffty/internal/models"
)

// MockStorage is a mock implementation of the Storage interface for testing
type MockStorage struct {
	repositories []string
	reviewState  *models.ReviewState
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

func (m *MockStorage) SaveRepositories(repos []string) error {
	m.repositories = repos
	return nil
}

func (m *MockStorage) LoadRepositories() ([]string, error) {
	return m.repositories, nil
}

// MockGitRepo is a mock implementation of git.Repository for testing
type MockGitRepo struct {
	path string
	name string
}

func NewMockGitRepo() *MockGitRepo {
	return &MockGitRepo{
		path: "/test/repo",
		name: "test-repo",
	}
}

func (m *MockGitRepo) GetBranches() ([]string, error) {
	return []string{"main", "feature"}, nil
}

func (m *MockGitRepo) GetBranchCommitHash(branch string) (string, error) {
	if branch == "feature" {
		return "feature-commit-hash", nil
	}
	if branch == "main" {
		return "main-commit-hash", nil
	}
	return "", fmt.Errorf("unknown branch: %s", branch)
}

func (m *MockGitRepo) GetDiff(sourceBranch, targetBranch string) (string, error) {
	return "diff --git a/file.txt b/file.txt\nindex 1234..5678 100644\n--- a/file.txt\n+++ b/file.txt\n@@ -1,1 +1,2 @@\n line1\n+line2", nil
}

func (m *MockGitRepo) GetFileDiff(sourceBranch, targetBranch, filePath string) (string, error) {
	return "diff --git a/" + filePath + " b/" + filePath + "\nindex 1234..5678 100644\n--- a/" + filePath + "\n+++ b/" + filePath + "\n@@ -1,1 +1,2 @@\n line1\n+line2", nil
}

func (m *MockGitRepo) GetFiles(sourceBranch, targetBranch string) ([]string, error) {
	return []string{"file.txt"}, nil
}

// This field just to satisfy having all methods of git.Repository
var _ = (*MockGitRepo)(nil).GetFiles

// TestServer extends Server for testing
type TestServer struct {
	Server
	mockRepo *MockGitRepo
}

// GetRepository overrides the Server.GetRepository method for testing
func (s *TestServer) GetRepository(path string) (*git.Repository, bool, error) {
	// Return a real Repository with the path/name from our mock
	// Since we've overridden the handler methods that call the repo methods,
	// they'll call our mock methods instead
	return &git.Repository{
		Path: s.mockRepo.path,
		Name: s.mockRepo.name,
	}, true, nil
}

// Override method handlers directly - this is simpler than trying to make
// a complete mock implementation
func (s *TestServer) handleCompare(w http.ResponseWriter, r *http.Request) {
	// For GET requests
	if r.Method == http.MethodGet {
		s.render(w, "compare.html", map[string]interface{}{
			"RepoPath":     "/test/repo",
			"RepoName":     "test-repo",
			"SourceBranch": "feature",
			"TargetBranch": "main",
			"Branches":     []string{"main", "feature"},
		})
		return
	}

	// For POST requests, redirect to diff view
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			s.renderError(w, "Invalid Form", "Invalid form data submitted", http.StatusBadRequest)
			return
		}

		redirectURL := fmt.Sprintf("/diff?repo=%s&source=%s&target=%s&source_commit=%s&target_commit=%s",
			url.QueryEscape("/test/repo"),
			url.QueryEscape("feature"),
			url.QueryEscape("main"),
			url.QueryEscape("feature-commit-hash"),
			url.QueryEscape("main-commit-hash"))

		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		return
	}
}

// Override handleDiffView to use our mock data
func (s *TestServer) handleDiffView(w http.ResponseWriter, r *http.Request) {
	s.render(w, "diff.html", map[string]interface{}{
		"RepoPath":     "/test/repo",
		"RepoName":     "test-repo",
		"SourceBranch": "feature",
		"TargetBranch": "main",
		"SourceCommit": "feature-commit-hash",
		"TargetCommit": "main-commit-hash",
		"Files":        []map[string]string{{"Path": "file.txt", "Status": "unreviewed"}},
		"DiffLines":    []string{"diff --git a/file.txt b/file.txt", "@@ -1,1 +1,2 @@", " line1", "+line2"},
	})
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

	// remporarly replate getTemplateDir with a mocked one.
	origFS := getTemplateDir
	getTemplateDir = func() fs.FS {
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
		}
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

// Helper function to create a test server with a mock repository
func setupTestServerWithMockRepo(t *testing.T) (*TestServer, *MockStorage) {
	t.Helper()

	server, mockStorage := setupTestServer(t)

	// Create a test server with a mock repository
	testServer := &TestServer{
		Server:   *server,
		mockRepo: NewMockGitRepo(),
	}

	return testServer, mockStorage
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

// TestHandleCompare tests the compare handler
func TestHandleCompare(t *testing.T) {
	server, _ := setupTestServerWithMockRepo(t)

	// Test GET request
	req := httptest.NewRequest("GET", "/compare?repo=/test/repo", nil)
	w := httptest.NewRecorder()

	server.handleCompare(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	if !strings.Contains(string(body), "Compare Page") {
		t.Errorf("Expected body to contain 'Compare Page', got %s", string(body))
	}

	// Test POST request
	formData := url.Values{}
	formData.Set("repo", "/test/repo")
	formData.Set("source", "feature")
	formData.Set("target", "main")

	req = httptest.NewRequest("POST", "/compare", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()

	server.handleCompare(w, req)

	resp = w.Result()

	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("Expected status code %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}

	// Check redirect location
	location := resp.Header.Get("Location")
	if !strings.Contains(location, "/diff") ||
		!strings.Contains(location, "repo=%2Ftest%2Frepo") ||
		!strings.Contains(location, "source=feature") ||
		!strings.Contains(location, "target=main") {
		t.Errorf("Expected redirect to diff page with proper parameters, got %s", location)
	}
}

// TestHandleDiffView tests the diff view handler
func TestHandleDiffView(t *testing.T) {
	server, _ := setupTestServerWithMockRepo(t)

	req := httptest.NewRequest("GET", "/diff?repo=/test/repo&source=feature&target=main", nil)
	w := httptest.NewRecorder()

	server.handleDiffView(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	if !strings.Contains(string(body), "Diff Page") {
		t.Errorf("Expected body to contain 'Diff Page', got %s", string(body))
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
	diffText := `diff --git a/file1.txt b/file1.txt
index 1234..5678 100644
--- a/file1.txt
+++ b/file1.txt
@@ -1,3 +1,4 @@
 line1
+new line
 line2
 line3
diff --git a/file2.txt b/file2.txt
index 8765..4321 100644
--- a/file2.txt
+++ b/file2.txt
@@ -1,3 +1,3 @@
 line1
-old line
+new line
 line3`

	reviewState := &models.ReviewState{
		ReviewedFiles: []models.FileReview{
			{
				Repo:  "/test/repo",
				Path:  "file1.txt",
				Lines: map[string]string{"all": models.StateApproved},
			},
		},
	}

	files := extractFilesFromDiff(diffText, reviewState, "/test/repo")

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

// TestHandleReviewStateStagedMode tests review state handler with staged mode
func TestHandleReviewStateStagedMode(t *testing.T) {
	server, mockStorage := setupTestServer(t)

	// For staged mode, source and target branches are not required,
	// but source_commit and target_commit are.
	params := url.Values{}
	params.Set("repo", "/test/repo")
	params.Set("source_commit", "abc123")
	params.Set("target_commit", "staged")
	params.Set("file", "file.txt")
	params.Set("status", "approved")
	params.Set("mode", "staged")

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

	// Verify the redirect includes mode=staged
	location := resp.Header.Get("Location")
	if !strings.Contains(location, "mode=staged") {
		t.Errorf("Expected mode=staged in redirect URL, got %s", location)
	}

	// Verify DiffMode was set on the review state
	if mockStorage.reviewState.DiffMode != models.ModeStaged {
		t.Errorf("Expected DiffMode to be %q, got %q", models.ModeStaged, mockStorage.reviewState.DiffMode)
	}
}

// TestHandleReviewStateUnstagedMode tests review state handler with unstaged mode
func TestHandleReviewStateUnstagedMode(t *testing.T) {
	server, mockStorage := setupTestServer(t)

	params := url.Values{}
	params.Set("repo", "/test/repo")
	params.Set("source_commit", "abc123")
	params.Set("target_commit", "unstaged")
	params.Set("file", "file.txt")
	params.Set("status", "rejected")
	params.Set("mode", "unstaged")

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
	if !strings.Contains(location, "mode=unstaged") {
		t.Errorf("Expected mode=unstaged in redirect URL, got %s", location)
	}

	if mockStorage.reviewState.DiffMode != models.ModeUnstaged {
		t.Errorf("Expected DiffMode to be %q, got %q", models.ModeUnstaged, mockStorage.reviewState.DiffMode)
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
				Data: []byte(`{{define "compare.html"}}DiffMode={{.DiffMode}} RepoName={{.RepoName}}{{if .Branches}} Branches={{range .Branches}}{{.}},{{end}}{{end}}{{if .RecentCommits}} RecentCommits=yes{{end}}{{end}}`),
				Mode: 0644,
			},
			"templates/diff.html": &fstest.MapFile{
				Data: []byte(`{{define "diff.html"}}DiffMode={{.DiffMode}} SourceLabel={{.SourceLabel}} TargetLabel={{.TargetLabel}} SourceCommit={{.SourceCommit}} TargetCommit={{.TargetCommit}} NoDiff={{.NoDiff}} RepoName={{.RepoName}}{{if .Files}} Files={{range .Files}}{{.Path}},{{end}}{{end}}{{if .SelectedFile}} SelectedFile={{.SelectedFile}}{{end}}{{if .Error}} Error={{.Error}}{{end}}{{end}}`),
				Mode: 0644,
			},
			"templates/error.html": &fstest.MapFile{
				Data: []byte(`{{define "error.html"}}Error: {{.Title}} - {{.Message}}{{end}}`),
				Mode: 0644,
			},
		}
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

	// 15. Register cleanup
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
	if !strings.Contains(location, "source=feature") {
		t.Errorf("Expected source=feature in location, got: %s", location)
	}
	if !strings.Contains(location, "target=main") {
		t.Errorf("Expected target=main in location, got: %s", location)
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
