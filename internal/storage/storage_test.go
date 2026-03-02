package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Gamezar/difftypp/internal/models"
)

func TestJSONStorage(t *testing.T) {
	// Create a temporary directory for the test
	tempDir := t.TempDir()

	// Create a test .difftypp directory
	difftyDir := filepath.Join(tempDir, ".difftypp")
	if err := os.MkdirAll(difftyDir, 0755); err != nil {
		t.Fatalf("Failed to create .difftypp directory: %v", err)
	}

	// Create a test storage instance with a custom path
	storage := &JSONStorage{
		baseStoragePath: difftyDir,
		reposPath:       filepath.Join(difftyDir, "repositories.json"),
	}

	// Test SaveReviewState and LoadReviewState
	t.Run("ReviewState", func(t *testing.T) {
		// Create a test review state
		testState := &models.ReviewState{
			ReviewedFiles: []models.FileReview{
				{
					Repo: "/path/to/repo",
					Path: "test/file.go",
					Lines: map[string]string{
						"1":   models.StateApproved,
						"2":   models.StateRejected,
						"3-5": models.StateSkipped,
					},
				},
			},
			SourceBranch: "feature",
			TargetBranch: "main",
			SourceCommit: "abc123",
			TargetCommit: "def456",
		}

		// Save the test state
		if err := storage.SaveReviewState(testState, "/path/to/repo"); err != nil {
			t.Fatalf("Failed to save review state: %v", err)
		}

		// Load the test state
		loadedState, err := storage.LoadReviewState("/path/to/repo", "feature", "main", "abc123", "def456")
		if err != nil {
			t.Fatalf("Failed to load review state: %v", err)
		}

		// Verify the loaded state
		if len(loadedState.ReviewedFiles) != 1 {
			t.Fatalf("Expected 1 reviewed file, got %d", len(loadedState.ReviewedFiles))
		}

		if loadedState.ReviewedFiles[0].Repo != "/path/to/repo" {
			t.Errorf("Expected repository path to be '/path/to/repo', got '%s'", loadedState.ReviewedFiles[0].Repo)
		}

		if loadedState.ReviewedFiles[0].Path != "test/file.go" {
			t.Errorf("Expected file path to be 'test/file.go', got '%s'", loadedState.ReviewedFiles[0].Path)
		}

		if len(loadedState.ReviewedFiles[0].Lines) != 3 {
			t.Errorf("Expected 3 lines, got %d", len(loadedState.ReviewedFiles[0].Lines))
		}

		if loadedState.ReviewedFiles[0].Lines["1"] != models.StateApproved {
			t.Errorf("Expected line 1 to be '%s', got '%s'", models.StateApproved, loadedState.ReviewedFiles[0].Lines["1"])
		}

		if loadedState.ReviewedFiles[0].Lines["2"] != models.StateRejected {
			t.Errorf("Expected line 2 to be '%s', got '%s'", models.StateRejected, loadedState.ReviewedFiles[0].Lines["2"])
		}

		if loadedState.ReviewedFiles[0].Lines["3-5"] != models.StateSkipped {
			t.Errorf("Expected lines 3-5 to be '%s', got '%s'", models.StateSkipped, loadedState.ReviewedFiles[0].Lines["3-5"])
		}

		if loadedState.SourceBranch != "feature" {
			t.Errorf("Expected source branch to be 'feature', got '%s'", loadedState.SourceBranch)
		}

		if loadedState.TargetBranch != "main" {
			t.Errorf("Expected target branch to be 'main', got '%s'", loadedState.TargetBranch)
		}

		if loadedState.SourceCommit != "abc123" {
			t.Errorf("Expected source commit to be 'abc123', got '%s'", loadedState.SourceCommit)
		}

		if loadedState.TargetCommit != "def456" {
			t.Errorf("Expected target commit to be 'def456', got '%s'", loadedState.TargetCommit)
		}
	})

	// Test LoadReviewState with missing file
	t.Run("LoadMissingReviewState", func(t *testing.T) {
		// Load a non-existent review state
		loadedState, err := storage.LoadReviewState("/nonexistent/repo", "feature", "main", "abc123", "def456")
		if err != nil {
			t.Fatalf("Failed to load non-existent review state: %v", err)
		}

		// Verify we get an empty review state
		if len(loadedState.ReviewedFiles) != 0 {
			t.Errorf("Expected 0 reviewed files, got %d", len(loadedState.ReviewedFiles))
		}

		if loadedState.SourceBranch != "feature" {
			t.Errorf("Expected source branch to be 'feature', got '%s'", loadedState.SourceBranch)
		}

		if loadedState.TargetBranch != "main" {
			t.Errorf("Expected target branch to be 'main', got '%s'", loadedState.TargetBranch)
		}

		if loadedState.SourceCommit != "abc123" {
			t.Errorf("Expected source commit to be 'abc123', got '%s'", loadedState.SourceCommit)
		}

		if loadedState.TargetCommit != "def456" {
			t.Errorf("Expected target commit to be 'def456', got '%s'", loadedState.TargetCommit)
		}
	})

	// Test SaveReviewState with missing commit hashes
	t.Run("MissingCommitHashes", func(t *testing.T) {
		testState := &models.ReviewState{
			ReviewedFiles: []models.FileReview{
				{
					Repo: "/path/to/repo",
					Path: "test/file.go",
					Lines: map[string]string{
						"1": models.StateApproved,
					},
				},
			},
			SourceBranch: "feature",
			TargetBranch: "main",
			// Missing commit hashes
		}

		err := storage.SaveReviewState(testState, "/path/to/repo")
		if err == nil {
			t.Errorf("Expected error for missing commit hashes, got nil")
		}
	})

	// Test SaveRepositories and LoadRepositories
	t.Run("Repositories", func(t *testing.T) {
		// Save repositories
		testRepos := []string{"/path/to/repo1", "/path/to/repo2"}
		if err := storage.SaveRepositories(testRepos); err != nil {
			t.Fatalf("Failed to save repositories: %v", err)
		}

		// Load repositories
		loadedRepos, err := storage.LoadRepositories()
		if err != nil {
			t.Fatalf("Failed to load repositories: %v", err)
		}

		// Verify loaded repositories
		if len(loadedRepos) != 2 {
			t.Fatalf("Expected 2 repositories, got %d", len(loadedRepos))
		}

		if loadedRepos[0] != "/path/to/repo1" {
			t.Errorf("Expected repository 1 to be '/path/to/repo1', got '%s'", loadedRepos[0])
		}

		if loadedRepos[1] != "/path/to/repo2" {
			t.Errorf("Expected repository 2 to be '/path/to/repo2', got '%s'", loadedRepos[1])
		}
	})

	// Test LoadRepositories with no file
	t.Run("LoadEmptyRepositories", func(t *testing.T) {
		// Create a new storage instance with a different path
		emptyStorage := &JSONStorage{
			baseStoragePath: difftyDir,
			reposPath:       filepath.Join(difftyDir, "nonexistent.json"),
		}

		// Load repositories
		loadedRepos, err := emptyStorage.LoadRepositories()
		if err != nil {
			t.Fatalf("Failed to load repositories: %v", err)
		}

		// Verify we get an empty slice
		if len(loadedRepos) != 0 {
			t.Errorf("Expected 0 repositories, got %d", len(loadedRepos))
		}
	})
}

func TestSaveAndLoadReview(t *testing.T) {
	tempDir := t.TempDir()

	difftyDir := filepath.Join(tempDir, ".diffty")
	if err := os.MkdirAll(difftyDir, 0755); err != nil {
		t.Fatalf("Failed to create .diffty directory: %v", err)
	}

	storage := &JSONStorage{
		baseStoragePath: difftyDir,
		reposPath:       filepath.Join(difftyDir, "repositories.json"),
	}

	t.Run("SaveAndReloadReviewWithComments", func(t *testing.T) {
		review := &models.Review{
			ID:           "review-001",
			RepoPath:     "/home/user/myrepo",
			SourceBranch: "feature-xyz",
			TargetBranch: "main",
			SourceCommit: "aaa111",
			TargetCommit: "bbb222",
			DiffMode:     models.ModeBranches,
			Status:       models.ReviewStatusDraft,
			CreatedAt:    "2025-06-15T10:30:00Z",
			Comments: []models.ReviewComment{
				{
					ID:         "comment-1",
					FilePath:   "internal/server/server.go",
					StartLine:  10,
					EndLine:    15,
					Side:       "right",
					Body:       "This needs refactoring",
					Status:     models.CommentStatusOpen,
					CreatedAt:  "2025-06-15T10:31:00Z",
					ResolvedAt: "",
				},
				{
					ID:         "comment-2",
					FilePath:   "internal/git/git.go",
					StartLine:  42,
					EndLine:    42,
					Side:       "left",
					Body:       "Resolved: looks good now",
					Status:     models.CommentStatusResolved,
					CreatedAt:  "2025-06-15T10:32:00Z",
					ResolvedAt: "2025-06-15T11:00:00Z",
				},
			},
		}

		if err := storage.SaveReview(review, "/home/user/myrepo"); err != nil {
			t.Fatalf("Failed to save review: %v", err)
		}

		loaded, err := storage.LoadReview("/home/user/myrepo", "feature-xyz", "main", "aaa111", "bbb222")
		if err != nil {
			t.Fatalf("Failed to load review: %v", err)
		}

		// Verify top-level fields
		if loaded.ID != "review-001" {
			t.Errorf("Expected ID 'review-001', got '%s'", loaded.ID)
		}
		if loaded.RepoPath != "/home/user/myrepo" {
			t.Errorf("Expected RepoPath '/home/user/myrepo', got '%s'", loaded.RepoPath)
		}
		if loaded.SourceBranch != "feature-xyz" {
			t.Errorf("Expected SourceBranch 'feature-xyz', got '%s'", loaded.SourceBranch)
		}
		if loaded.TargetBranch != "main" {
			t.Errorf("Expected TargetBranch 'main', got '%s'", loaded.TargetBranch)
		}
		if loaded.SourceCommit != "aaa111" {
			t.Errorf("Expected SourceCommit 'aaa111', got '%s'", loaded.SourceCommit)
		}
		if loaded.TargetCommit != "bbb222" {
			t.Errorf("Expected TargetCommit 'bbb222', got '%s'", loaded.TargetCommit)
		}
		if loaded.DiffMode != models.ModeBranches {
			t.Errorf("Expected DiffMode '%s', got '%s'", models.ModeBranches, loaded.DiffMode)
		}
		if loaded.Status != models.ReviewStatusDraft {
			t.Errorf("Expected Status '%s', got '%s'", models.ReviewStatusDraft, loaded.Status)
		}
		if loaded.CreatedAt != "2025-06-15T10:30:00Z" {
			t.Errorf("Expected CreatedAt '2025-06-15T10:30:00Z', got '%s'", loaded.CreatedAt)
		}

		// Verify comments
		if len(loaded.Comments) != 2 {
			t.Fatalf("Expected 2 comments, got %d", len(loaded.Comments))
		}

		c1 := loaded.Comments[0]
		if c1.ID != "comment-1" {
			t.Errorf("Comment 1: expected ID 'comment-1', got '%s'", c1.ID)
		}
		if c1.FilePath != "internal/server/server.go" {
			t.Errorf("Comment 1: expected FilePath 'internal/server/server.go', got '%s'", c1.FilePath)
		}
		if c1.StartLine != 10 {
			t.Errorf("Comment 1: expected StartLine 10, got %d", c1.StartLine)
		}
		if c1.EndLine != 15 {
			t.Errorf("Comment 1: expected EndLine 15, got %d", c1.EndLine)
		}
		if c1.Side != "right" {
			t.Errorf("Comment 1: expected Side 'right', got '%s'", c1.Side)
		}
		if c1.Body != "This needs refactoring" {
			t.Errorf("Comment 1: expected Body 'This needs refactoring', got '%s'", c1.Body)
		}
		if c1.Status != models.CommentStatusOpen {
			t.Errorf("Comment 1: expected Status '%s', got '%s'", models.CommentStatusOpen, c1.Status)
		}
		if c1.CreatedAt != "2025-06-15T10:31:00Z" {
			t.Errorf("Comment 1: expected CreatedAt '2025-06-15T10:31:00Z', got '%s'", c1.CreatedAt)
		}

		c2 := loaded.Comments[1]
		if c2.ID != "comment-2" {
			t.Errorf("Comment 2: expected ID 'comment-2', got '%s'", c2.ID)
		}
		if c2.Status != models.CommentStatusResolved {
			t.Errorf("Comment 2: expected Status '%s', got '%s'", models.CommentStatusResolved, c2.Status)
		}
		if c2.ResolvedAt != "2025-06-15T11:00:00Z" {
			t.Errorf("Comment 2: expected ResolvedAt '2025-06-15T11:00:00Z', got '%s'", c2.ResolvedAt)
		}
		if c2.Side != "left" {
			t.Errorf("Comment 2: expected Side 'left', got '%s'", c2.Side)
		}
	})

	t.Run("LoadNonExistentReviewReturnsEmpty", func(t *testing.T) {
		loaded, err := storage.LoadReview("/nonexistent/repo", "feat", "main", "ccc333", "ddd444")
		if err != nil {
			t.Fatalf("Expected no error for non-existent review, got: %v", err)
		}

		if loaded.RepoPath != "/nonexistent/repo" {
			t.Errorf("Expected RepoPath '/nonexistent/repo', got '%s'", loaded.RepoPath)
		}
		if loaded.SourceBranch != "feat" {
			t.Errorf("Expected SourceBranch 'feat', got '%s'", loaded.SourceBranch)
		}
		if loaded.TargetBranch != "main" {
			t.Errorf("Expected TargetBranch 'main', got '%s'", loaded.TargetBranch)
		}
		if loaded.SourceCommit != "ccc333" {
			t.Errorf("Expected SourceCommit 'ccc333', got '%s'", loaded.SourceCommit)
		}
		if loaded.TargetCommit != "ddd444" {
			t.Errorf("Expected TargetCommit 'ddd444', got '%s'", loaded.TargetCommit)
		}
		if loaded.Status != models.ReviewStatusDraft {
			t.Errorf("Expected Status '%s', got '%s'", models.ReviewStatusDraft, loaded.Status)
		}
		if loaded.Comments == nil {
			t.Errorf("Expected non-nil Comments slice, got nil")
		}
		if len(loaded.Comments) != 0 {
			t.Errorf("Expected 0 comments, got %d", len(loaded.Comments))
		}
	})

	t.Run("LoadReviewWithEmptyCommitsReturnsEmpty", func(t *testing.T) {
		loaded, err := storage.LoadReview("/some/repo", "feat", "main", "", "")
		if err != nil {
			t.Fatalf("Expected no error for empty commits, got: %v", err)
		}

		if loaded.SourceCommit != "" {
			t.Errorf("Expected empty SourceCommit, got '%s'", loaded.SourceCommit)
		}
		if loaded.TargetCommit != "" {
			t.Errorf("Expected empty TargetCommit, got '%s'", loaded.TargetCommit)
		}
		if loaded.Status != models.ReviewStatusDraft {
			t.Errorf("Expected Status '%s', got '%s'", models.ReviewStatusDraft, loaded.Status)
		}
	})

	t.Run("SaveOverwritesExistingReview", func(t *testing.T) {
		original := &models.Review{
			RepoPath:     "/home/user/overwrite-repo",
			SourceBranch: "feature",
			TargetBranch: "main",
			SourceCommit: "eee555",
			TargetCommit: "fff666",
			Status:       models.ReviewStatusDraft,
			Comments: []models.ReviewComment{
				{
					ID:        "old-comment",
					FilePath:  "old.go",
					StartLine: 1,
					EndLine:   1,
					Side:      "right",
					Body:      "Old comment",
					Status:    models.CommentStatusOpen,
					CreatedAt: "2025-01-01T00:00:00Z",
				},
			},
		}

		if err := storage.SaveReview(original, "/home/user/overwrite-repo"); err != nil {
			t.Fatalf("Failed to save original review: %v", err)
		}

		updated := &models.Review{
			RepoPath:     "/home/user/overwrite-repo",
			SourceBranch: "feature",
			TargetBranch: "main",
			SourceCommit: "eee555",
			TargetCommit: "fff666",
			Status:       models.ReviewStatusSubmitted,
			SubmittedAt:  "2025-06-16T12:00:00Z",
			Comments: []models.ReviewComment{
				{
					ID:        "new-comment",
					FilePath:  "new.go",
					StartLine: 5,
					EndLine:   10,
					Side:      "both",
					Body:      "Updated comment",
					Status:    models.CommentStatusOpen,
					CreatedAt: "2025-06-16T11:00:00Z",
				},
			},
		}

		if err := storage.SaveReview(updated, "/home/user/overwrite-repo"); err != nil {
			t.Fatalf("Failed to save updated review: %v", err)
		}

		loaded, err := storage.LoadReview("/home/user/overwrite-repo", "feature", "main", "eee555", "fff666")
		if err != nil {
			t.Fatalf("Failed to load updated review: %v", err)
		}

		if loaded.Status != models.ReviewStatusSubmitted {
			t.Errorf("Expected Status '%s', got '%s'", models.ReviewStatusSubmitted, loaded.Status)
		}
		if loaded.SubmittedAt != "2025-06-16T12:00:00Z" {
			t.Errorf("Expected SubmittedAt '2025-06-16T12:00:00Z', got '%s'", loaded.SubmittedAt)
		}
		if len(loaded.Comments) != 1 {
			t.Fatalf("Expected 1 comment after overwrite, got %d", len(loaded.Comments))
		}
		if loaded.Comments[0].ID != "new-comment" {
			t.Errorf("Expected comment ID 'new-comment', got '%s'", loaded.Comments[0].ID)
		}
		if loaded.Comments[0].Body != "Updated comment" {
			t.Errorf("Expected comment Body 'Updated comment', got '%s'", loaded.Comments[0].Body)
		}
	})

	t.Run("SaveReviewMissingCommitHashes", func(t *testing.T) {
		review := &models.Review{
			RepoPath:     "/some/repo",
			SourceBranch: "feature",
			TargetBranch: "main",
			// Missing SourceCommit and TargetCommit
			Status:   models.ReviewStatusDraft,
			Comments: []models.ReviewComment{},
		}

		err := storage.SaveReview(review, "/some/repo")
		if err == nil {
			t.Errorf("Expected error for missing commit hashes, got nil")
		}
	})
}

func TestReviewPath(t *testing.T) {
	storage := &JSONStorage{
		baseStoragePath: "/base/path",
		reposPath:       "/base/path/repositories.json",
	}

	t.Run("ConsistentPathForSameInputs", func(t *testing.T) {
		path1 := storage.reviewPath("/home/user/repo", "abc123", "def456")
		path2 := storage.reviewPath("/home/user/repo", "abc123", "def456")

		if path1 != path2 {
			t.Errorf("Expected identical paths for same inputs, got '%s' and '%s'", path1, path2)
		}
	})

	t.Run("DifferentInputsProduceDifferentPaths", func(t *testing.T) {
		pathA := storage.reviewPath("/home/user/repoA", "abc123", "def456")
		pathB := storage.reviewPath("/home/user/repoB", "abc123", "def456")

		if pathA == pathB {
			t.Errorf("Expected different paths for different repos, both got '%s'", pathA)
		}

		pathC := storage.reviewPath("/home/user/repo", "abc123", "def456")
		pathD := storage.reviewPath("/home/user/repo", "xyz789", "def456")

		if pathC == pathD {
			t.Errorf("Expected different paths for different source commits, both got '%s'", pathC)
		}

		pathE := storage.reviewPath("/home/user/repo", "abc123", "def456")
		pathF := storage.reviewPath("/home/user/repo", "abc123", "xyz789")

		if pathE == pathF {
			t.Errorf("Expected different paths for different target commits, both got '%s'", pathE)
		}
	})

	t.Run("PathContainsReviewsSubdir", func(t *testing.T) {
		path := storage.reviewPath("/home/user/repo", "abc123", "def456")

		// reviewPath uses filepath.Join(s.baseStoragePath, "reviews", safeRepoPath, sourceCommit, targetCommit, "review.json")
		if !filepath.IsAbs(path) {
			t.Errorf("Expected absolute path, got '%s'", path)
		}

		// Verify the path ends with review.json
		if filepath.Base(path) != "review.json" {
			t.Errorf("Expected path to end with 'review.json', got '%s'", filepath.Base(path))
		}
	})

	t.Run("PathIncludesCommitHashes", func(t *testing.T) {
		path := storage.reviewPath("/home/user/repo", "sourceabc", "targetdef")

		// The path should contain both commit hashes as directory components
		dir := filepath.Dir(path)                     // .../sourceabc/targetdef
		targetDir := filepath.Base(dir)               // targetdef
		sourceDir := filepath.Base(filepath.Dir(dir)) // sourceabc

		if sourceDir != "sourceabc" {
			t.Errorf("Expected source commit 'sourceabc' in path, got '%s'", sourceDir)
		}
		if targetDir != "targetdef" {
			t.Errorf("Expected target commit 'targetdef' in path, got '%s'", targetDir)
		}
	})
}

func TestNewEmptyReview(t *testing.T) {
	t.Run("ReturnsReviewWithNonNilComments", func(t *testing.T) {
		review := newEmptyReview("/repo", "feature", "main", "src123", "tgt456")

		if review.Comments == nil {
			t.Errorf("Expected non-nil Comments slice, got nil")
		}
		if len(review.Comments) != 0 {
			t.Errorf("Expected 0 comments, got %d", len(review.Comments))
		}
	})

	t.Run("PopulatesAllFields", func(t *testing.T) {
		review := newEmptyReview("/my/repo", "feat-branch", "develop", "aaa", "bbb")

		if review.RepoPath != "/my/repo" {
			t.Errorf("Expected RepoPath '/my/repo', got '%s'", review.RepoPath)
		}
		if review.SourceBranch != "feat-branch" {
			t.Errorf("Expected SourceBranch 'feat-branch', got '%s'", review.SourceBranch)
		}
		if review.TargetBranch != "develop" {
			t.Errorf("Expected TargetBranch 'develop', got '%s'", review.TargetBranch)
		}
		if review.SourceCommit != "aaa" {
			t.Errorf("Expected SourceCommit 'aaa', got '%s'", review.SourceCommit)
		}
		if review.TargetCommit != "bbb" {
			t.Errorf("Expected TargetCommit 'bbb', got '%s'", review.TargetCommit)
		}
		if review.Status != models.ReviewStatusDraft {
			t.Errorf("Expected Status '%s', got '%s'", models.ReviewStatusDraft, review.Status)
		}
	})
}

func TestEnsureDir(t *testing.T) {
	t.Run("CreatesNestedDirectories", func(t *testing.T) {
		tempDir := t.TempDir()
		nestedFilePath := filepath.Join(tempDir, "a", "b", "c", "file.json")

		if err := ensureDir(nestedFilePath); err != nil {
			t.Fatalf("Failed to create nested directories: %v", err)
		}

		// Verify the parent directory was created (not the file itself)
		parentDir := filepath.Dir(nestedFilePath)
		info, err := os.Stat(parentDir)
		if err != nil {
			t.Fatalf("Parent directory does not exist: %v", err)
		}
		if !info.IsDir() {
			t.Errorf("Expected a directory, got a file")
		}

		// Verify the file was NOT created (ensureDir only creates the directory)
		if _, err := os.Stat(nestedFilePath); !os.IsNotExist(err) {
			t.Errorf("Expected file to not exist, but it does (or unexpected error: %v)", err)
		}
	})

	t.Run("ExistingDirectoryNoError", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "file.json")

		// tempDir already exists, so ensuring its parent should succeed
		if err := ensureDir(filePath); err != nil {
			t.Errorf("Expected no error for existing directory, got: %v", err)
		}

		// Call again to verify idempotency
		if err := ensureDir(filePath); err != nil {
			t.Errorf("Expected no error on second call, got: %v", err)
		}
	})

	t.Run("ErrorOnInvalidPath", func(t *testing.T) {
		// /dev/null is a file, not a directory — creating a subdirectory under it should fail
		invalidPath := filepath.Join("/dev/null", "subdir", "file.json")

		err := ensureDir(invalidPath)
		if err == nil {
			t.Errorf("Expected error for invalid path, got nil")
		}
	})
}

func TestSanitizeRepoPath(t *testing.T) {
	t.Run("ReplacesPathSeparators", func(t *testing.T) {
		result := sanitizeRepoPath("/home/user/repo")
		if result == "/home/user/repo" {
			t.Errorf("Expected path separators to be replaced, got '%s'", result)
		}

		// Should not contain OS path separator
		if filepath.IsAbs(result) {
			t.Errorf("Expected sanitized path to not be absolute, got '%s'", result)
		}
	})

	t.Run("ReplacesColons", func(t *testing.T) {
		result := sanitizeRepoPath("C:/Users/repo")
		for _, c := range result {
			if c == ':' {
				t.Errorf("Expected colons to be replaced, got '%s'", result)
				break
			}
		}
	})

	t.Run("ConsistentOutput", func(t *testing.T) {
		result1 := sanitizeRepoPath("/home/user/repo")
		result2 := sanitizeRepoPath("/home/user/repo")

		if result1 != result2 {
			t.Errorf("Expected consistent output, got '%s' and '%s'", result1, result2)
		}
	})
}

func TestNewJSONStorage(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Set temporary home directory
	t.Setenv("HOME", tempDir)

	// Create new storage
	storage, err := NewJSONStorage()
	if err != nil {
		t.Fatalf("Failed to create JSON storage: %v", err)
	}

	// Verify storage creation
	if storage == nil {
		t.Fatal("Storage should not be nil")
	}

	// Verify .difftypp directory was created
	difftyPath := filepath.Join(tempDir, ".difftypp")
	if _, err := os.Stat(difftyPath); os.IsNotExist(err) {
		t.Errorf(".difftypp directory was not created")
	}

	// Verify repositories path
	expectedReposPath := filepath.Join(difftyPath, "repositories.json")
	if storage.reposPath != expectedReposPath {
		t.Errorf("Expected reposPath to be '%s', got '%s'", expectedReposPath, storage.reposPath)
	}
}
