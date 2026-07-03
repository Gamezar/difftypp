package server

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Gamezar/difftypp/internal/git"
	"github.com/Gamezar/difftypp/internal/models"
	"github.com/Gamezar/difftypp/internal/storage"
)

//go:embed templates/*
var templateDir embed.FS

// getTemplateDir can be swapped at runtime to stub out a file system
// for test purposes.
var getTemplateDir = func() fs.FS {
	return templateDir
}

//go:embed all:static
var staticDir embed.FS

// Server represents the HTTP server
type Server struct {
	storage storage.Storage
	tmpl    *template.Template
}

// New creates a new Server instance
func New(storage storage.Storage) (*Server, error) {
	// Create template functions map
	funcMap := template.FuncMap{
		"hasPrefix": strings.HasPrefix, // Used to check if a string starts with a prefix
		"add":       func(a, b int) int { return a + b },
		"sub":       func(a, b int) int { return a - b },
		"shortHash": func(hash string) string {
			if len(hash) > 8 {
				return hash[:8]
			}
			return hash
		},
		// trimLinePrefix removes the leading +/-/space character from a diff line
		"trimLinePrefix": trimDiffPrefix,
		// dict builds a map from alternating key/value arguments. It lets a
		// template pass a bundle of named values to a sub-template (Go templates
		// only accept a single data argument), e.g. {{ template "x" (dict ...) }}.
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("dict: odd number of arguments")
			}
			m := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict: key at index %d is not a string", i)
				}
				m[key] = values[i+1]
			}
			return m, nil
		},
		// lineType returns "addition", "deletion", or "context" based on diff line prefix
		"lineType": func(line string) string {
			if strings.HasPrefix(line, "+") {
				return "addition"
			}
			if strings.HasPrefix(line, "-") {
				return "deletion"
			}
			return "context"
		},
		// commentsForLine filters review comments for a specific file and line number.
		// It checks both the right (new) and left (old) line numbers so that
		// comments on deleted lines (which only have a left number) are displayed.
		"commentsForLine": func(comments []models.ReviewComment, filePath string, rightNum int, leftNum int) []models.ReviewComment {
			var result []models.ReviewComment
			for _, c := range comments {
				if c.FilePath != filePath {
					continue
				}
				rightMatch := rightNum > 0 && c.EndLine == rightNum
				leftMatch := leftNum > 0 && c.EndLine == leftNum
				switch c.Side {
				case "left":
					if leftMatch {
						result = append(result, c)
					}
				case "right":
					if rightMatch {
						result = append(result, c)
					}
				default: // "both" or ""
					if rightMatch || leftMatch {
						result = append(result, c)
					}
				}
			}
			return result
		},
		// pastCommentsForLine filters past review comments for a specific file and line number.
		// Uses the same matching logic as commentsForLine but operates on PastComment slices.
		"pastCommentsForLine": func(comments []models.PastComment, filePath string, rightNum int, leftNum int) []models.PastComment {
			var result []models.PastComment
			for _, c := range comments {
				if c.FilePath != filePath {
					continue
				}
				rightMatch := rightNum > 0 && c.EndLine == rightNum
				leftMatch := leftNum > 0 && c.EndLine == leftNum
				switch c.Side {
				case "left":
					if leftMatch {
						result = append(result, c)
					}
				case "right":
					if rightMatch {
						result = append(result, c)
					}
				default:
					if rightMatch || leftMatch {
						result = append(result, c)
					}
				}
			}
			return result
		},
		// canLinkToDiff returns true if the diff mode supports linking back to original diff view.
		// Branch and commit modes produce permanent diffs; staged/unstaged diffs are ephemeral.
		"canLinkToDiff": func(mode string) bool {
			return mode == models.ModeBranches || mode == models.ModeCommits
		},
		// formatTime formats an RFC3339 timestamp to a short human-readable string
		"formatTime": func(ts string) string {
			t, err := time.Parse(time.RFC3339, ts)
			if err != nil {
				return ts
			}
			return t.Format("Jan 2 15:04")
		},
	}

	// Parse all templates with the function map
	tmpl, err := template.New("").Funcs(funcMap).ParseFS(getTemplateDir(), "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	// Create server
	server := &Server{
		storage: storage,
		tmpl:    tmpl,
	}

	return server, nil
}

// AddRepository adds a new repository to the server and persists it
func (s *Server) AddRepository(path string) (bool, error) {
	// Validate the repository path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, fmt.Errorf("failed to get absolute path for %s: %w", path, err)
	}

	// Check if it's a valid git repository
	if !git.IsValidRepo(absPath) {
		return false, fmt.Errorf("not a valid git repository: %s", absPath)
	}

	// Get current repositories
	repos, err := s.storage.LoadRepositories()
	if err != nil {
		return false, fmt.Errorf("failed to load repositories: %w", err)
	}

	// Check if repository already exists
	for _, existingPath := range repos {
		if existingPath == absPath {
			// Repository already exists, nothing to do
			return true, nil
		}
	}

	// Add new repository path
	repos = append(repos, absPath)

	// Save updated list
	if err := s.storage.SaveRepositories(repos); err != nil {
		return false, fmt.Errorf("failed to save repositories: %w", err)
	}

	return true, nil
}

// RemoveRepository removes a repository from the server and persists the change
func (s *Server) RemoveRepository(path string) error {
	repos, err := s.storage.LoadRepositories()
	if err != nil {
		return fmt.Errorf("failed to load repositories: %w", err)
	}

	filtered := make([]string, 0, len(repos))
	found := false
	for _, repo := range repos {
		if repo == path {
			found = true
			continue
		}
		filtered = append(filtered, repo)
	}

	if !found {
		return fmt.Errorf("repository not found: %s", path)
	}

	if err := s.storage.SaveRepositories(filtered); err != nil {
		return fmt.Errorf("failed to save repositories: %w", err)
	}

	return nil
}

// GetRepository returns a repository by path. A path is valid if it is a
// registered repository or a worktree belonging to one — the latter lets the
// worktrees page hand off to the compare/diff views without the worktree being
// registered separately.
func (s *Server) GetRepository(path string) (*git.Repository, bool, error) {
	repos, err := s.storage.LoadRepositories()
	if err != nil {
		return nil, false, fmt.Errorf("failed to load repositories: %w", err)
	}

	// Check if repository exists
	for _, repo := range repos {
		if repo == path {
			return git.NewRepository(path), true, nil
		}
	}

	// Accept a linked worktree of a registered repository.
	if s.isWorktreeOfRegistered(repos, path) {
		return git.NewRepository(path), true, nil
	}

	return nil, false, nil
}

// isWorktreeOfRegistered reports whether path is the main tree or a linked
// worktree of a registered repository. Every worktree sharing a repository
// reports the same set, so a single `git worktree list` on the requested path
// yields all sibling trees — including the registered main repo — in one
// subprocess, regardless of how many repositories are registered. This matters
// because GetRepository runs on hot paths (e.g. handleFileContext fires on each
// "expand context" click). Paths are compared canonically because git reports
// symlink-resolved paths while registered paths are only made absolute.
func (s *Server) isWorktreeOfRegistered(repos []string, path string) bool {
	if path == "" || !git.IsValidRepo(path) {
		return false
	}

	worktrees, err := git.NewRepository(path).GetWorktrees()
	if err != nil {
		return false
	}

	registered := make(map[string]struct{}, len(repos))
	for _, repoPath := range repos {
		registered[canonicalRepoPath(repoPath)] = struct{}{}
	}
	for _, wt := range worktrees {
		if _, ok := registered[canonicalRepoPath(wt.Path)]; ok {
			return true
		}
	}
	return false
}

// canonicalRepoPath normalizes a filesystem path for repository comparison.
// git reports symlink-resolved paths in `worktree list`, whereas registered
// repository paths are only made absolute, so it resolves symlinks when the
// path exists and falls back to a lexical clean otherwise.
func canonicalRepoPath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return filepath.Clean(p)
}

// GetRepositories returns all repositories
func (s *Server) GetRepositories() (map[string]*git.Repository, error) {
	repos, err := s.storage.LoadRepositories()
	if err != nil {
		return nil, fmt.Errorf("failed to load repositories: %w", err)
	}

	// Create a map of repositories
	reposMap := make(map[string]*git.Repository)
	for _, path := range repos {
		reposMap[path] = git.NewRepository(path)
	}

	return reposMap, nil
}

// Router sets up and returns the HTTP router
func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()

	// Static files — serve directly from the embed.FS which contains "static/..."
	// The URL path /static/css/main.css maps directly to static/css/main.css in the FS.
	fileServer := http.FileServer(http.FS(staticDir))
	mux.Handle("GET /static/", mimeFixHandler(fileServer))

	// API routes
	mux.HandleFunc("GET /api/browse", s.handleBrowse)
	mux.HandleFunc("POST /api/repository/add", s.handleAddRepository)
	mux.HandleFunc("DELETE /api/repository/remove", s.handleRemoveRepository)
	mux.HandleFunc("POST /api/review-state", s.handleReviewState)
	mux.HandleFunc("POST /api/review/comment", s.handleAddComment)
	mux.HandleFunc("DELETE /api/review/comment", s.handleDeleteComment)
	mux.HandleFunc("POST /api/review/comment/resolve", s.handleResolveComment)
	mux.HandleFunc("POST /api/review/submit", s.handleSubmitReview)
	mux.HandleFunc("GET /api/review/export", s.handleExportReview)
	mux.HandleFunc("GET /api/file-context", s.handleFileContext)
	mux.HandleFunc("GET /api/diff-status", s.handleDiffStatus)
	mux.HandleFunc("GET /api/recent-commits", s.handleRecentCommits)
	mux.HandleFunc("DELETE /api/review/past", s.handleDeletePastReview)
	mux.HandleFunc("DELETE /api/reviews/past", s.handleDeleteAllPastReviews)

	// HTML routes
	mux.HandleFunc("GET /worktrees", s.handleWorktrees)
	mux.HandleFunc("GET /compare", s.handleCompare)
	mux.HandleFunc("POST /compare", s.handleCompare)
	mux.HandleFunc("GET /diff", s.handleDiffView)
	mux.HandleFunc("GET /", s.handleIndex)

	return mux
}

// mimeFixHandler wraps an http.Handler to fix Content-Type for embedded static
// assets. Go's embed.FS + http.FileServer may serve .css files as text/plain
// because the embedded content lacks OS-level MIME detection.
func mimeFixHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ext := filepath.Ext(r.URL.Path)
		var forceMIME string
		switch ext {
		case ".css":
			forceMIME = "text/css; charset=utf-8"
		case ".js":
			forceMIME = "application/javascript; charset=utf-8"
		}
		if forceMIME != "" {
			next.ServeHTTP(&mimeOverrideWriter{ResponseWriter: w, contentType: forceMIME}, r)
		} else {
			next.ServeHTTP(w, r)
		}
	})
}

// mimeOverrideWriter wraps http.ResponseWriter to force a specific Content-Type,
// preventing http.FileServer from overwriting it with an incorrect type.
type mimeOverrideWriter struct {
	http.ResponseWriter
	contentType string
	wroteHeader bool
}

func (m *mimeOverrideWriter) WriteHeader(code int) {
	if !m.wroteHeader {
		m.ResponseWriter.Header().Set("Content-Type", m.contentType)
		m.wroteHeader = true
	}
	m.ResponseWriter.WriteHeader(code)
}

func (m *mimeOverrideWriter) Write(b []byte) (int, error) {
	if !m.wroteHeader {
		m.ResponseWriter.Header().Set("Content-Type", m.contentType)
		m.wroteHeader = true
	}
	return m.ResponseWriter.Write(b)
}

// handleIndex renders the index page
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	repos, err := s.GetRepositories()
	if err != nil {
		s.renderError(w, "Repository Error", fmt.Sprintf("Error loading repositories: %v", err), http.StatusInternalServerError)
		return
	}

	// Check if we have any repositories
	hasRepos := len(repos) > 0

	data := map[string]interface{}{
		"Repositories": repos,
		"HasRepos":     hasRepos,
	}

	s.render(w, "index.html", data)
}

// handleWorktrees lists the worktrees attached to a repository so the user can
// pick which working tree to review before choosing a diff mode. The repo must
// be registered (or itself a worktree of a registered repo). When a repository
// has only its main working tree, there is nothing to choose between, so we
// redirect straight to the compare view.
func (s *Server) handleWorktrees(w http.ResponseWriter, r *http.Request) {
	repoPath := r.URL.Query().Get("repo")
	if repoPath == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	repo, exists, err := s.GetRepository(repoPath)
	if err != nil {
		s.renderError(w, "Repository Error", fmt.Sprintf("Error loading repository: %v", err), http.StatusInternalServerError)
		return
	}
	if !exists {
		s.renderError(w, "Not Found", "Repository not found", http.StatusNotFound)
		return
	}

	worktrees, err := repo.GetWorktrees()
	if err != nil {
		// The worktree picker is an optional convenience layered in front of the
		// compare view; if git can't enumerate worktrees, fall back to the direct
		// compare flow rather than dead-ending on an error page.
		http.Redirect(w, r, "/compare?repo="+url.QueryEscape(repoPath), http.StatusSeeOther)
		return
	}

	// Bare entries have no working tree and cannot be diffed, so they are not
	// offered as review targets. Each row links to compare keyed on the path the
	// worktree should be reviewed under: for the tree that corresponds to the
	// registered repository, that's the registered path (reviews are stored under
	// it, and git's list reports a symlink-resolved path that may differ);
	// linked worktrees use their own git-reported path.
	regCanonical := canonicalRepoPath(repoPath)
	selectable := make([]map[string]interface{}, 0, len(worktrees))
	for _, wt := range worktrees {
		if wt.Bare {
			continue
		}
		selectPath := wt.Path
		if canonicalRepoPath(wt.Path) == regCanonical {
			selectPath = repoPath
		}
		selectable = append(selectable, map[string]interface{}{
			"Branch":     wt.Branch,
			"Path":       wt.Path,
			"IsMain":     wt.IsMain,
			"Bare":       wt.Bare,
			"SelectPath": selectPath,
		})
	}

	// Only the main working tree — nothing to navigate, go straight to compare.
	if len(selectable) <= 1 {
		http.Redirect(w, r, "/compare?repo="+url.QueryEscape(repoPath), http.StatusSeeOther)
		return
	}

	data := map[string]interface{}{
		"RepoName":  filepath.Base(repoPath),
		"RepoPath":  repoPath,
		"Worktrees": selectable,
	}

	s.render(w, "worktrees.html", data)
}

// recentCommitsLimit is how many commits the commit-selection page lists, both
// on the initial server render and on each auto-refresh poll, so the two agree.
const recentCommitsLimit = 20

// getDiffMode reads and validates the mode query parameter, defaulting to branches
func getDiffMode(r *http.Request) string {
	mode := r.URL.Query().Get("mode")
	switch mode {
	case models.ModeCommits, models.ModeStaged, models.ModeUnstaged:
		return mode
	default:
		return models.ModeBranches
	}
}

// diffViewCookie is the cookie name used to persist the reviewer's preferred
// diff layout (unified vs. split) across file navigation.
const diffViewCookie = "diffty_view"

// getDiffView resolves the diff layout mode for a request. An explicit ?view=
// query parameter wins (so links and tests can force a mode); otherwise the
// persisted cookie preference is used, defaulting to the unified layout.
func getDiffView(r *http.Request) string {
	switch r.URL.Query().Get("view") {
	case "split":
		return "split"
	case "unified":
		return "unified"
	}
	if c, err := r.Cookie(diffViewCookie); err == nil {
		if c.Value == "split" || c.Value == "unified" {
			return c.Value
		}
	}
	return "unified"
}

// trimDiffPrefix removes the leading +/-/space marker from a raw diff line,
// leaving the underlying source text. Lines without one of those markers (e.g.
// the "\ No newline at end of file" marker) are returned unchanged.
func trimDiffPrefix(line string) string {
	if len(line) > 0 && (line[0] == '+' || line[0] == '-' || line[0] == ' ') {
		return line[1:]
	}
	return line
}

// diffParams holds the common query parameters used across review handlers
type diffParams struct {
	RepoPath     string
	SourceBranch string
	TargetBranch string
	SourceCommit string
	TargetCommit string
	Mode         string
	FilePath     string
}

// parseDiffParams extracts the standard diff/review parameters from a request's query string
func parseDiffParams(r *http.Request) diffParams {
	return diffParams{
		RepoPath:     r.URL.Query().Get("repo"),
		SourceBranch: r.URL.Query().Get("source"),
		TargetBranch: r.URL.Query().Get("target"),
		SourceCommit: r.URL.Query().Get("source_commit"),
		TargetCommit: r.URL.Query().Get("target_commit"),
		Mode:         getDiffMode(r),
		FilePath:     r.URL.Query().Get("file"),
	}
}

// getDiffForMode fetches the full diff text based on the diff mode
func getDiffForMode(repo *git.Repository, p diffParams) (string, error) {
	switch p.Mode {
	case models.ModeStaged:
		return repo.GetStagedDiff()
	case models.ModeUnstaged:
		return repo.GetUnstagedDiff()
	default: // branches and commits both use GetDiff with refs
		return repo.GetDiff(p.SourceBranch, p.TargetBranch)
	}
}

// getFilesForMode lists the changed file paths based on the diff mode.
// This uses git's --name-only output, which is far cheaper than fetching and
// parsing the full textual diff just to build the sidebar file list.
func getFilesForMode(repo *git.Repository, p diffParams) ([]string, error) {
	switch p.Mode {
	case models.ModeStaged:
		return repo.GetStagedFiles()
	case models.ModeUnstaged:
		return repo.GetUnstagedFiles()
	default: // branches and commits both use GetFiles with refs
		return repo.GetFiles(p.SourceBranch, p.TargetBranch)
	}
}

// getFileDiffForMode fetches the diff text for a single file based on the diff
// mode, so viewing one file never re-fetches the whole changeset.
func getFileDiffForMode(repo *git.Repository, p diffParams, filePath string) (string, error) {
	switch p.Mode {
	case models.ModeStaged:
		return repo.GetStagedFileDiff(filePath)
	case models.ModeUnstaged:
		return repo.GetUnstagedFileDiff(filePath)
	default: // branches and commits both use GetFileDiff with refs
		return repo.GetFileDiff(p.SourceBranch, p.TargetBranch, filePath)
	}
}

// handleCompare renders the comparison page
func (s *Server) handleCompare(w http.ResponseWriter, r *http.Request) {
	repoPath := r.URL.Query().Get("repo")
	sourceBranch := r.URL.Query().Get("source")
	targetBranch := r.URL.Query().Get("target")
	mode := getDiffMode(r)

	// Handle form submission
	if r.Method == http.MethodPost {
		// Parse form data
		if err := r.ParseForm(); err != nil {
			s.renderError(w, "Invalid Form", "Invalid form data submitted", http.StatusBadRequest)
			return
		}

		// Get repository path from form data (in case of POST)
		formRepoPath := r.FormValue("repo")
		formSourceBranch := r.FormValue("source")
		formTargetBranch := r.FormValue("target")
		if formMode := r.FormValue("mode"); formMode != "" {
			// Validate formMode the same way getDiffMode() does
			switch formMode {
			case models.ModeCommits, models.ModeStaged, models.ModeUnstaged, models.ModeBranches:
				mode = formMode
			default:
				mode = models.ModeBranches
			}
		}

		if formRepoPath != "" {
			repoPath = formRepoPath
		}

		// Make sure we have a repository path
		if repoPath == "" {
			s.renderError(w, "Missing Repository", "Repository path is required", http.StatusBadRequest)
			return
		}

		// Staged and unstaged modes don't need source/target branches — redirect directly to diff view
		if mode == models.ModeStaged || mode == models.ModeUnstaged {
			redirectURL := fmt.Sprintf("/diff?repo=%s&mode=%s",
				url.QueryEscape(repoPath),
				url.QueryEscape(mode))
			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
			return
		}

		// For commits mode, source and target are arbitrary refs (SHAs, tags, HEAD~N, etc.)
		if mode == models.ModeCommits {
			if formSourceBranch != "" {
				sourceBranch = formSourceBranch
			}
			if formTargetBranch != "" {
				targetBranch = formTargetBranch
			}
			if sourceBranch == "" || targetBranch == "" {
				s.renderError(w, "Missing Refs", "Source and target refs are required for commit comparison", http.StatusBadRequest)
				return
			}

			// Check if the repository exists
			repo, exists, err := s.GetRepository(repoPath)
			if err != nil {
				s.renderError(w, "Repository Error", fmt.Sprintf("Error loading repository: %v", err), http.StatusInternalServerError)
				return
			}
			if !exists {
				s.renderError(w, "Not Found", "Repository not found", http.StatusNotFound)
				return
			}

			// Resolve refs to commit hashes
			sourceCommit, err := repo.GetBranchCommitHash(sourceBranch)
			if err != nil {
				s.renderError(w, "Ref Error", fmt.Sprintf("Failed to resolve source ref '%s': %v", sourceBranch, err), http.StatusInternalServerError)
				return
			}
			targetCommit, err := repo.GetBranchCommitHash(targetBranch)
			if err != nil {
				s.renderError(w, "Ref Error", fmt.Sprintf("Failed to resolve target ref '%s': %v", targetBranch, err), http.StatusInternalServerError)
				return
			}

			redirectURL := fmt.Sprintf("/diff?repo=%s&source=%s&target=%s&source_commit=%s&target_commit=%s&mode=%s",
				url.QueryEscape(repoPath),
				url.QueryEscape(sourceCommit),
				url.QueryEscape(targetCommit),
				url.QueryEscape(sourceCommit),
				url.QueryEscape(targetCommit),
				url.QueryEscape(mode))

			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
			return
		}

		// Branches mode (default)
		// Only update if non-empty values provided
		if formSourceBranch != "" {
			sourceBranch = formSourceBranch
		}

		if formTargetBranch != "" {
			targetBranch = formTargetBranch
		}

		// Make sure we have source and target branches
		if sourceBranch == "" || targetBranch == "" {
			s.renderError(w, "Missing Branches", "Source and target branches are required", http.StatusBadRequest)
			return
		}

		// Check if the repository exists
		repo, exists, err := s.GetRepository(repoPath)
		if err != nil {
			s.renderError(w, "Repository Error", fmt.Sprintf("Error loading repository: %v", err), http.StatusInternalServerError)
			return
		}
		if !exists {
			s.renderError(w, "Not Found", "Repository not found", http.StatusNotFound)
			return
		}

		// Get commit hashes for the branches
		sourceCommit, err := repo.GetBranchCommitHash(sourceBranch)
		if err != nil {
			s.renderError(w, "Branch Error", fmt.Sprintf("Failed to get commit hash for source branch '%s': %v", sourceBranch, err), http.StatusInternalServerError)
			return
		}

		targetCommit, err := repo.GetBranchCommitHash(targetBranch)
		if err != nil {
			s.renderError(w, "Branch Error", fmt.Sprintf("Failed to get commit hash for target branch '%s': %v", targetBranch, err), http.StatusInternalServerError)
			return
		}

		// Redirect to diff view with commit hashes
		redirectURL := fmt.Sprintf("/diff?repo=%s&source=%s&target=%s&source_commit=%s&target_commit=%s&mode=%s",
			url.QueryEscape(repoPath),
			url.QueryEscape(sourceBranch),
			url.QueryEscape(targetBranch),
			url.QueryEscape(sourceCommit),
			url.QueryEscape(targetCommit),
			url.QueryEscape(mode))

		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		return
	}

	// Handle GET request
	if repoPath == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Check if the repository exists
	repo, exists, err := s.GetRepository(repoPath)
	if err != nil {
		s.renderError(w, "Repository Error", fmt.Sprintf("Error loading repository: %v", err), http.StatusInternalServerError)
		return
	}
	if !exists {
		s.renderError(w, "Not Found", "Repository not found", http.StatusNotFound)
		return
	}

	// Get repository name from path for display
	repoName := filepath.Base(repoPath)

	// Load branches from the repository
	branches, err := repo.GetBranches()
	if err != nil {
		s.renderError(w, "Branch Error", fmt.Sprintf("Failed to load branches: %v", err), http.StatusInternalServerError)
		return
	}

	// Pre-select branches if not specified
	if sourceBranch == "" && len(branches) > 0 {
		// Try to use the second branch (usually a feature branch) as source
		if len(branches) > 1 {
			sourceBranch = branches[1]
		} else {
			sourceBranch = branches[0]
		}
	}

	if targetBranch == "" && len(branches) > 0 {
		// Usually main/master is the first branch
		targetBranch = branches[0]
	}

	data := map[string]interface{}{
		"RepoPath":     repoPath,
		"RepoName":     repoName,
		"SourceBranch": sourceBranch,
		"TargetBranch": targetBranch,
		"Branches":     branches,
		"DiffMode":     mode,
	}

	// For commits mode, load recent commits for the UI
	if mode == models.ModeCommits {
		commits, err := repo.GetRecentCommits(recentCommitsLimit)
		if err != nil {
			// Non-fatal: just show empty list
			commits = []git.Commit{}
		}
		data["RecentCommits"] = commits
	}

	s.render(w, "compare.html", data)
}

// browseEntry represents a single directory entry returned by the browse API
type browseEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsGitRepo bool   `json:"is_git_repo"`
}

// writeJSONError writes a JSON error response with the given message and status code.
func writeJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	resp := struct {
		Error string `json:"error"`
	}{Error: message}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("writeJSONError: failed to encode error response: %v", err)
	}
}

// handleBrowse handles GET /api/browse — lists directories at a given path
func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	dirPath := r.URL.Query().Get("path")
	if dirPath == "" {
		// Default to the user's home directory
		home, err := os.UserHomeDir()
		if err != nil {
			writeJSONError(w, "failed to determine home directory", http.StatusInternalServerError)
			return
		}
		dirPath = home
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		writeJSONError(w, "invalid path", http.StatusBadRequest)
		return
	}

	// Verify the path exists and is a directory
	info, err := os.Stat(absPath)
	if err != nil {
		writeJSONError(w, "path not found", http.StatusNotFound)
		return
	}
	if !info.IsDir() {
		writeJSONError(w, "path is not a directory", http.StatusBadRequest)
		return
	}

	// Read directory entries
	entries, err := os.ReadDir(absPath)
	if err != nil {
		writeJSONError(w, "cannot read directory", http.StatusForbidden)
		return
	}

	// Build response — only include directories (we're selecting a repo root)
	dirs := []browseEntry{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Skip hidden directories (starting with .)
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		fullPath := filepath.Join(absPath, entry.Name())
		dirs = append(dirs, browseEntry{
			Name:      entry.Name(),
			Path:      fullPath,
			IsGitRepo: git.IsValidRepo(fullPath),
		})
	}

	// Sort: git repos first, then alphabetically
	sort.Slice(dirs, func(i, j int) bool {
		if dirs[i].IsGitRepo != dirs[j].IsGitRepo {
			return dirs[i].IsGitRepo
		}
		return dirs[i].Name < dirs[j].Name
	})

	// Build JSON response with current path for breadcrumb support
	resp := struct {
		CurrentPath string        `json:"current_path"`
		ParentPath  string        `json:"parent_path"`
		IsGitRepo   bool          `json:"is_git_repo"`
		Entries     []browseEntry `json:"entries"`
	}{
		CurrentPath: absPath,
		ParentPath:  filepath.Dir(absPath),
		IsGitRepo:   git.IsValidRepo(absPath),
		Entries:     dirs,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("handleBrowse: failed to encode JSON response: %v", err)
	}
}

// handleAddRepository adds a new repository
func (s *Server) handleAddRepository(w http.ResponseWriter, r *http.Request) {
	// Parse the form data
	if err := r.ParseForm(); err != nil {
		s.renderError(w, "Invalid Form", "Invalid form data submitted", http.StatusBadRequest)
		return
	}

	repoPath := r.Form.Get("path")
	if repoPath == "" {
		s.renderError(w, "Missing Path", "Repository path is required", http.StatusBadRequest)
		return
	}

	// Add the repository
	success, err := s.AddRepository(repoPath)
	if !success {
		if err != nil {
			s.renderError(w, "Repository Error", err.Error(), http.StatusInternalServerError)
		} else {
			s.renderError(w, "Repository Error", "Failed to add repository", http.StatusInternalServerError)
		}
		return
	}

	// Redirect to the index page
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleRemoveRepository removes a repository from the list
func (s *Server) handleRemoveRepository(w http.ResponseWriter, r *http.Request) {
	repoPath := r.URL.Query().Get("path")
	if repoPath == "" {
		http.Error(w, `{"error":"repository path is required"}`, http.StatusBadRequest)
		return
	}

	if err := s.RemoveRepository(repoPath); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"ok":true}`)
}

// handleReviewState handles saving and loading review state
func (s *Server) handleReviewState(w http.ResponseWriter, r *http.Request) {
	// Get required parameters
	p := parseDiffParams(r)
	status := r.URL.Query().Get("status")
	nextFilePath := r.URL.Query().Get("next")

	// For staged/unstaged modes, source/target branches are not required
	if p.Mode == models.ModeStaged || p.Mode == models.ModeUnstaged {
		if p.RepoPath == "" || p.SourceCommit == "" || p.TargetCommit == "" || p.FilePath == "" || status == "" {
			s.renderError(w, "Missing Parameters", "Missing required parameters for updating review state", http.StatusBadRequest)
			return
		}
	} else {
		if p.RepoPath == "" || p.SourceBranch == "" || p.TargetBranch == "" || p.SourceCommit == "" || p.TargetCommit == "" || p.FilePath == "" || status == "" {
			s.renderError(w, "Missing Parameters", "Missing required parameters for updating review state", http.StatusBadRequest)
			return
		}
	}

	// Validate status value
	if status != models.StateApproved && status != models.StateRejected && status != models.StateSkipped {
		s.renderError(w, "Invalid Status", "Invalid status value for file review", http.StatusBadRequest)
		return
	}

	// Load existing review state
	existingState, err := s.storage.LoadReviewState(p.RepoPath, p.SourceBranch, p.TargetBranch, p.SourceCommit, p.TargetCommit)
	if err != nil {
		s.renderError(w, "Review State Error", fmt.Sprintf("Failed to load review state: %v", err), http.StatusInternalServerError)
		return
	}

	// Set diff mode on the state
	existingState.DiffMode = p.Mode

	// Look for the file in the existing review state
	fileFound := false
	for i := range existingState.ReviewedFiles {
		if existingState.ReviewedFiles[i].Path == p.FilePath && existingState.ReviewedFiles[i].Repo == p.RepoPath {
			// Update existing file review
			if existingState.ReviewedFiles[i].Lines == nil {
				existingState.ReviewedFiles[i].Lines = make(map[string]string)
			}
			existingState.ReviewedFiles[i].Lines["all"] = status
			fileFound = true
			break
		}
	}

	// If file not found, add it to the review state
	if !fileFound {
		existingState.ReviewedFiles = append(existingState.ReviewedFiles, models.FileReview{
			Repo:  p.RepoPath,
			Path:  p.FilePath,
			Lines: map[string]string{"all": status},
		})
	}

	// Save updated review state
	if err := s.storage.SaveReviewState(existingState, p.RepoPath); err != nil {
		s.renderError(w, "Review State Error", fmt.Sprintf("Failed to save review state: %v", err), http.StatusInternalServerError)
		return
	}

	// Determine where to redirect — navigate to next file on status action, otherwise stay
	redirectFile := p.FilePath
	if nextFilePath != "" && (status == models.StateApproved || status == models.StateRejected || status == models.StateSkipped) {
		redirectFile = nextFilePath
	}
	redirectPath := buildDiffRedirectURL(p.RepoPath, p.SourceBranch, p.TargetBranch, p.SourceCommit, p.TargetCommit, p.Mode, redirectFile)

	// Redirect to the appropriate diff view
	http.Redirect(w, r, redirectPath, http.StatusSeeOther)
}

// handleDiffView renders the diff visualization page
func (s *Server) handleDiffView(w http.ResponseWriter, r *http.Request) {
	p := parseDiffParams(r)

	// Validate required params based on mode
	if p.RepoPath == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if p.Mode == models.ModeBranches || p.Mode == models.ModeCommits {
		if p.SourceBranch == "" || p.TargetBranch == "" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}

	// Check if the repository exists
	repo, exists, err := s.GetRepository(p.RepoPath)
	if err != nil {
		s.renderError(w, "Repository Error", fmt.Sprintf("Error loading repository: %v", err), http.StatusInternalServerError)
		return
	}
	if !exists {
		s.renderError(w, "Not Found", "Repository not found", http.StatusNotFound)
		return
	}

	// Get repository name from path for display
	repoName := filepath.Base(p.RepoPath)

	// Compute source/target commits and display labels based on mode.
	// For branches/commits modes, reuse commit hashes from query params if already
	// resolved by handleCompare — avoids redundant git rev-parse calls.
	var sourceLabel, targetLabel string

	switch p.Mode {
	case models.ModeStaged:
		headHash, err := repo.GetBranchCommitHash("HEAD")
		if err != nil {
			s.renderError(w, "Git Error", fmt.Sprintf("Failed to resolve HEAD: %v", err), http.StatusInternalServerError)
			return
		}
		p.SourceCommit = headHash
		p.TargetCommit = "staged-" + headHash
		p.SourceBranch = "HEAD"
		p.TargetBranch = "staged"
		sourceLabel = "HEAD"
		targetLabel = "Staged Changes"

	case models.ModeUnstaged:
		headHash, err := repo.GetBranchCommitHash("HEAD")
		if err != nil {
			s.renderError(w, "Git Error", fmt.Sprintf("Failed to resolve HEAD: %v", err), http.StatusInternalServerError)
			return
		}
		p.SourceCommit = headHash
		p.TargetCommit = "unstaged-" + headHash
		p.SourceBranch = "HEAD"
		p.TargetBranch = "unstaged"
		sourceLabel = "HEAD"
		targetLabel = "Working Tree"

	case models.ModeCommits:
		if p.SourceCommit == "" {
			var err error
			p.SourceCommit, err = repo.GetBranchCommitHash(p.SourceBranch)
			if err != nil {
				s.renderError(w, "Ref Error", fmt.Sprintf("Failed to resolve source ref: %v", err), http.StatusInternalServerError)
				return
			}
		}
		if p.TargetCommit == "" {
			var err error
			p.TargetCommit, err = repo.GetBranchCommitHash(p.TargetBranch)
			if err != nil {
				s.renderError(w, "Ref Error", fmt.Sprintf("Failed to resolve target ref: %v", err), http.StatusInternalServerError)
				return
			}
		}
		sourceLabel = p.SourceBranch
		targetLabel = p.TargetBranch

	default: // branches
		if p.SourceCommit == "" {
			var err error
			p.SourceCommit, err = repo.GetBranchCommitHash(p.SourceBranch)
			if err != nil {
				s.renderError(w, "Branch Error", fmt.Sprintf("Failed to get commit hash for source branch: %v", err), http.StatusInternalServerError)
				return
			}
		}
		if p.TargetCommit == "" {
			var err error
			p.TargetCommit, err = repo.GetBranchCommitHash(p.TargetBranch)
			if err != nil {
				s.renderError(w, "Branch Error", fmt.Sprintf("Failed to get commit hash for target branch: %v", err), http.StatusInternalServerError)
				return
			}
		}
		sourceLabel = p.SourceBranch
		targetLabel = p.TargetBranch
	}

	// Load review state
	var reviewState *models.ReviewState
	reviewState, err = s.storage.LoadReviewState(p.RepoPath, p.SourceBranch, p.TargetBranch, p.SourceCommit, p.TargetCommit)
	if err != nil {
		reviewState = &models.ReviewState{
			ReviewedFiles: []models.FileReview{},
			SourceBranch:  p.SourceBranch,
			TargetBranch:  p.TargetBranch,
			SourceCommit:  p.SourceCommit,
			TargetCommit:  p.TargetCommit,
			DiffMode:      p.Mode,
		}
	}

	// Data to pass to the template
	data := map[string]interface{}{
		"RepoPath":     p.RepoPath,
		"RepoName":     repoName,
		"SourceBranch": p.SourceBranch,
		"TargetBranch": p.TargetBranch,
		"SourceCommit": p.SourceCommit,
		"TargetCommit": p.TargetCommit,
		"SourceLabel":  sourceLabel,
		"TargetLabel":  targetLabel,
		"DiffMode":     p.Mode,
		"Error":        "",
		"NoDiff":       false,
		"ReviewState":  reviewState,
	}

	// Resolve the diff layout (unified vs. split). An explicit ?view= param is
	// persisted to a cookie so the choice survives file-to-file navigation, whose
	// links don't carry the parameter.
	diffView := getDiffView(r)
	if qv := r.URL.Query().Get("view"); qv == "split" || qv == "unified" {
		http.SetCookie(w, &http.Cookie{
			Name:     diffViewCookie,
			Value:    qv,
			Path:     "/",
			MaxAge:   365 * 24 * 60 * 60,
			SameSite: http.SameSiteLaxMode,
		})
	}
	data["DiffView"] = diffView

	// Build the sidebar file list from git's --name-only output. This avoids
	// fetching and parsing the entire changeset on every file view — the diff
	// for the selected file is fetched on its own below.
	var files []map[string]string
	fileList, filesErr := getFilesForMode(repo, p)

	if filesErr != nil {
		data["Error"] = fmt.Sprintf("Failed to load diff: %v", filesErr)
	} else if len(fileList) == 0 {
		data["NoDiff"] = true
	} else {
		files = extractFilesFromDiff(fileList, reviewState, p.RepoPath)
		data["Files"] = files
	}

	// Load review comments
	review, reviewErr := s.storage.LoadReview(p.RepoPath, p.SourceBranch, p.TargetBranch, p.SourceCommit, p.TargetCommit)
	if reviewErr != nil {
		review = &models.Review{
			RepoPath:     p.RepoPath,
			SourceBranch: p.SourceBranch,
			TargetBranch: p.TargetBranch,
			SourceCommit: p.SourceCommit,
			TargetCommit: p.TargetCommit,
			Comments:     []models.ReviewComment{},
			Status:       models.ReviewStatusDraft,
		}
	}
	data["Review"] = review
	data["ReviewComments"] = review.Comments

	// Count open comments for the submit button badge
	openComments := 0
	for _, c := range review.Comments {
		if c.Status == models.CommentStatusOpen {
			openComments++
		}
	}
	data["OpenCommentCount"] = openComments

	// Load past reviews for this branch pair
	var pastReviews []models.ReviewIndexEntry
	reviewIndex, indexErr := s.storage.LoadReviewIndex(p.RepoPath, p.SourceBranch, p.TargetBranch)
	if indexErr == nil {
		for _, entry := range reviewIndex.Reviews {
			// Exclude the current commit pair — that's the active review, not a past one
			if entry.SourceCommit == p.SourceCommit && entry.TargetCommit == p.TargetCommit {
				continue
			}
			pastReviews = append(pastReviews, entry)
		}
	}
	data["PastReviews"] = pastReviews

	if p.FilePath == "" {
		// Auto-redirect to the first file if there are files to show
		if len(files) > 0 {
			redirectURL := buildDiffRedirectURL(p.RepoPath, p.SourceBranch, p.TargetBranch, p.SourceCommit, p.TargetCommit, p.Mode, files[0]["Path"])
			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
			return
		}
		// For the file list view, load past review comments for the expandable sidebar (staged/unstaged)
		var pastReviewsWithComments []map[string]interface{}
		for _, entry := range pastReviews {
			if entry.DiffMode == models.ModeStaged || entry.DiffMode == models.ModeUnstaged {
				pastReview, err := s.storage.LoadReview(p.RepoPath, p.SourceBranch, p.TargetBranch, entry.SourceCommit, entry.TargetCommit)
				if err == nil && len(pastReview.Comments) > 0 {
					pastReviewsWithComments = append(pastReviewsWithComments, map[string]interface{}{
						"Entry":    entry,
						"Comments": pastReview.Comments,
					})
				}
			}
		}
		data["PastReviewsWithComments"] = pastReviewsWithComments
		if fp, fpErr := computeDiffFingerprint(repo, p); fpErr == nil {
			data["DiffFingerprint"] = fp
		}
		s.render(w, "diff.html", data)
		return
	}

	// Fetch and parse only the requested file's diff. The git pathspec restricts
	// output to this one file, so we never re-parse the whole changeset.
	fileDiffText, fileDiffErr := getFileDiffForMode(repo, p, p.FilePath)
	var selectedFile *models.DiffFile
	if fileDiffErr != nil {
		data["Error"] = fmt.Sprintf("Failed to load file diff: %v", fileDiffErr)
	} else {
		parsed := git.ParseDiff(fileDiffText)
		for i := range parsed {
			if parsed[i].Path == p.FilePath {
				selectedFile = &parsed[i]
				break
			}
		}
		// The pathspec already restricts output to the requested file, so if the
		// header path differs (e.g. a rename), fall back to the sole parsed entry.
		if selectedFile == nil && len(parsed) > 0 {
			selectedFile = &parsed[0]
		}
	}

	if selectedFile == nil {
		if data["Error"] == "" {
			data["Error"] = fmt.Sprintf("File %q not found in diff", p.FilePath)
		}
	} else {
		data["SelectedFile"] = p.FilePath
		data["SelectedFileParsed"] = *selectedFile
		data["SelectedFileLanguage"] = git.DetectLanguage(p.FilePath)

		// Collapsed-context regions for GitHub-style expand controls
		hunkGaps, bottomGap := computeHunkGaps(*selectedFile)
		data["HunkGaps"] = hunkGaps
		data["BottomGap"] = bottomGap

		// Precompute aligned side-by-side rows for the split layout only when it
		// is the active view — the unified layout renders straight from Sections.
		if diffView == "split" {
			data["SplitSections"] = computeSplitSections(*selectedFile)
		}

		// Reconstruct raw diff lines from parsed hunks for the fallback raw view
		var diffLines []string
		for _, section := range selectedFile.Sections {
			diffLines = append(diffLines, section.Lines...)
		}
		data["DiffLines"] = diffLines

		// Filter comments for selected file
		var fileComments []models.ReviewComment
		for _, c := range review.Comments {
			if c.FilePath == p.FilePath {
				fileComments = append(fileComments, c)
			}
		}
		data["FileComments"] = fileComments

		// Load past comments for inline rendering on current diff (best-effort by file+line)
		var pastFileComments []models.PastComment
		for _, entry := range pastReviews {
			pastReview, err := s.storage.LoadReview(p.RepoPath, p.SourceBranch, p.TargetBranch, entry.SourceCommit, entry.TargetCommit)
			if err != nil {
				continue
			}
			for _, c := range pastReview.Comments {
				if c.FilePath == p.FilePath {
					pastFileComments = append(pastFileComments, models.PastComment{
						ReviewComment:     c,
						ReviewSubmittedAt: entry.SubmittedAt,
						ReviewID:          entry.ReviewID,
					})
				}
			}
		}
		data["PastFileComments"] = pastFileComments

		// Determine the file status for display in the UI
		fileStatus := "unreviewed"
		for _, review := range reviewState.ReviewedFiles {
			if review.Path == p.FilePath && review.Repo == p.RepoPath {
				// Check if all lines have the same status
				statuses := make(map[string]bool)
				for _, status := range review.Lines {
					statuses[status] = true
				}

				if len(statuses) == 1 {
					for status := range statuses {
						fileStatus = status
					}
				} else if len(statuses) > 1 {
					fileStatus = "mixed"
				}
				break
			}
		}
		data["FileStatus"] = fileStatus

		// Find next file for navigation
		if len(files) > 0 {
			currentIndex := -1
			for i, file := range files {
				if file["Path"] == p.FilePath {
					currentIndex = i
					break
				}
			}

			if currentIndex != -1 && currentIndex < len(files)-1 {
				data["NextFilePath"] = files[currentIndex+1]["Path"]
			}
		}
	}

	// Seed the freshness poller with the fingerprint of the exact diff state this
	// page is rendered against, so it can detect a change that lands between
	// render and its first poll (rather than baselining from that first poll).
	if fp, fpErr := computeDiffFingerprint(repo, p); fpErr == nil {
		data["DiffFingerprint"] = fp
	}

	s.render(w, "diff.html", data)
}

// fileContextSource picks the git ref (or working tree) to read full file
// content from when expanding diff context. The "new"/right side of each diff
// mode is what the displayed right-side line numbers map to:
//   - branches/commits: the source commit
//   - staged:           the index (empty ref → git show :path)
//   - unstaged:         the on-disk working tree file
func fileContextSource(p diffParams) (ref string, useWorkingTree bool) {
	switch p.Mode {
	case models.ModeStaged:
		return "", false
	case models.ModeUnstaged:
		return "", true
	default:
		return p.SourceCommit, false
	}
}

// handleFileContext handles GET /api/file-context — returns the raw content of a
// contiguous range of lines from a file, used to expand collapsed diff context
// the way GitHub's "expand" controls do. start/end are 1-based inclusive line
// numbers in the file's new ("right") version. Fewer lines than requested means
// end-of-file was reached.
func (s *Server) handleFileContext(w http.ResponseWriter, r *http.Request) {
	p := parseDiffParams(r)
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	if p.RepoPath == "" || p.FilePath == "" || startStr == "" || endStr == "" {
		writeJSONError(w, "missing required parameters", http.StatusBadRequest)
		return
	}

	start, err := strconv.Atoi(startStr)
	if err != nil || start < 1 {
		writeJSONError(w, "start must be a positive integer", http.StatusBadRequest)
		return
	}
	end, err := strconv.Atoi(endStr)
	if err != nil || end < start {
		writeJSONError(w, "end must be an integer >= start", http.StatusBadRequest)
		return
	}

	// Branch/commit modes read content from the source commit; require it so we
	// never silently fall back to the staged index (the empty-ref form).
	if p.Mode != models.ModeStaged && p.Mode != models.ModeUnstaged && p.SourceCommit == "" {
		writeJSONError(w, "source_commit is required for this diff mode", http.StatusBadRequest)
		return
	}

	repo, exists, err := s.GetRepository(p.RepoPath)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("failed to load repository: %v", err), http.StatusInternalServerError)
		return
	}
	if !exists {
		writeJSONError(w, "repository not found", http.StatusNotFound)
		return
	}

	ref, useWorkingTree := fileContextSource(p)
	var content string
	if useWorkingTree {
		content, err = repo.GetWorkingTreeFileContent(p.FilePath)
	} else {
		content, err = repo.GetFileContentAtRef(ref, p.FilePath)
	}
	if err != nil {
		writeJSONError(w, fmt.Sprintf("failed to read file content: %v", err), http.StatusInternalServerError)
		return
	}

	lines := splitFileLines(content)

	// Slice the requested 1-based inclusive range, clamped to the file length.
	var selected []string
	if start <= len(lines) {
		hi := end
		if hi > len(lines) {
			hi = len(lines)
		}
		selected = lines[start-1 : hi]
	} else {
		selected = []string{}
	}

	resp := struct {
		Start int      `json:"start"`
		Lines []string `json:"lines"`
		EOF   bool     `json:"eof"`
	}{
		Start: start,
		Lines: selected,
		EOF:   end >= len(lines),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("handleFileContext: failed to encode JSON response: %v", err)
	}
}

// handleDiffStatus handles GET /api/diff-status — it returns an opaque
// fingerprint of the diff the client is currently viewing. A diff page pins the
// branch tips it resolved (branches mode) or is computed against the HEAD /
// working-tree snapshot that was current when it loaded (staged/unstaged). When
// the branch advances or the working tree changes underneath the reviewer, the
// on-screen diff silently goes stale. The page polls this endpoint and, when
// the fingerprint changes, prompts the reviewer to reload rather than showing a
// stale view. Commits mode compares two pinned hashes and can never go stale.
func (s *Server) handleDiffStatus(w http.ResponseWriter, r *http.Request) {
	p := parseDiffParams(r)
	if p.RepoPath == "" {
		writeJSONError(w, "repo is required", http.StatusBadRequest)
		return
	}
	// Branches mode resolves the diff from branch names; without them there is
	// nothing to re-resolve against.
	if p.Mode == models.ModeBranches && (p.SourceBranch == "" || p.TargetBranch == "") {
		writeJSONError(w, "source and target are required for branch diffs", http.StatusBadRequest)
		return
	}

	repo, exists, err := s.GetRepository(p.RepoPath)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("failed to load repository: %v", err), http.StatusInternalServerError)
		return
	}
	if !exists {
		writeJSONError(w, "repository not found", http.StatusNotFound)
		return
	}

	fingerprint, err := computeDiffFingerprint(repo, p)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("failed to compute diff status: %v", err), http.StatusInternalServerError)
		return
	}

	resp := struct {
		Mode        string `json:"mode"`
		Fingerprint string `json:"fingerprint"`
	}{
		Mode:        p.Mode,
		Fingerprint: fingerprint,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("handleDiffStatus: failed to encode JSON response: %v", err)
	}
}

// computeDiffFingerprint returns a token that changes whenever the diff the
// reviewer is looking at would change, so the client can detect staleness by
// comparing successive values. The inputs differ per mode:
//   - branches:  the current commit hashes the two branch names resolve to
//   - commits:   the two pinned commit hashes (constant — never stale)
//   - staged:    HEAD plus the selected file's staged diff (whole diff if none)
//   - unstaged:  HEAD plus the selected file's working-tree diff (whole if none)
//
// The staged/unstaged fingerprint covers only the file being viewed (when one is
// selected) so an unrelated change elsewhere in the repo doesn't false-alarm the
// reviewer with a "stale" banner. HEAD is folded in so committing the very
// changes under review (which empties or reshapes the diff) is also detected.
func computeDiffFingerprint(repo *git.Repository, p diffParams) (string, error) {
	switch p.Mode {
	case models.ModeStaged, models.ModeUnstaged:
		head, err := repo.GetBranchCommitHash("HEAD")
		if err != nil {
			return "", err
		}
		var diff string
		if p.FilePath != "" {
			diff, err = getFileDiffForMode(repo, p, p.FilePath)
		} else {
			diff, err = getDiffForMode(repo, p)
		}
		if err != nil {
			return "", err
		}
		return fingerprintParts(head, diff), nil

	case models.ModeCommits:
		return fingerprintParts(p.SourceCommit, p.TargetCommit), nil

	default: // branches
		source, err := repo.GetBranchCommitHash(p.SourceBranch)
		if err != nil {
			return "", err
		}
		target, err := repo.GetBranchCommitHash(p.TargetBranch)
		if err != nil {
			return "", err
		}
		return fingerprintParts(source, target), nil
	}
}

// handleRecentCommits handles GET /api/recent-commits — it returns the
// repository's most recent commits as JSON. The commit-selection page polls
// this so newly created commits appear in the list without a full page reload.
func (s *Server) handleRecentCommits(w http.ResponseWriter, r *http.Request) {
	repoPath := r.URL.Query().Get("repo")
	if repoPath == "" {
		writeJSONError(w, "repo is required", http.StatusBadRequest)
		return
	}

	repo, exists, err := s.GetRepository(repoPath)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("failed to load repository: %v", err), http.StatusInternalServerError)
		return
	}
	if !exists {
		writeJSONError(w, "repository not found", http.StatusNotFound)
		return
	}

	commits, err := repo.GetRecentCommits(recentCommitsLimit)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("failed to load recent commits: %v", err), http.StatusInternalServerError)
		return
	}

	type commitJSON struct {
		Hash    string `json:"hash"`
		Subject string `json:"subject"`
	}
	out := make([]commitJSON, 0, len(commits))
	for _, c := range commits {
		out = append(out, commitJSON{Hash: c.Hash, Subject: c.Subject})
	}

	resp := struct {
		Commits []commitJSON `json:"commits"`
	}{Commits: out}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("handleRecentCommits: failed to encode JSON response: %v", err)
	}
}

// fingerprintParts hashes its parts into a hex digest, separating them with a
// NUL byte so distinct part boundaries can never collide.
func fingerprintParts(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// splitFileLines splits raw file content into lines, dropping the single
// trailing empty element produced when the content ends with a newline.
func splitFileLines(content string) []string {
	if content == "" {
		return []string{}
	}
	lines := strings.Split(content, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

// hunkGap describes the collapsed region of unchanged lines immediately above a
// hunk (or, for the first hunk, above the start of the file). RightStart/RightEnd
// and LeftStart/LeftEnd are 1-based inclusive line ranges on each side; they have
// equal length because the hidden region is unchanged context.
type hunkGap struct {
	HasGap     bool
	RightStart int
	RightEnd   int
	LeftStart  int
	LeftEnd    int
	Size       int
}

// bottomGap describes the collapsed region after the last hunk. Its extent is
// unknown until the file is fetched, so only the starting line numbers are known.
type bottomGap struct {
	HasGap     bool
	RightStart int
	LeftStart  int
}

// firstLastLineNums returns the first and last non-zero values in nums.
func firstLastLineNums(nums []int) (first, last int) {
	for _, n := range nums {
		if n > 0 {
			if first == 0 {
				first = n
			}
			last = n
		}
	}
	return first, last
}

// computeHunkGaps derives, for a parsed file, the collapsed context region above
// each hunk plus the trailing region after the last hunk. These drive the
// GitHub-style "expand context" controls in the diff view.
func computeHunkGaps(file models.DiffFile) ([]hunkGap, bottomGap) {
	gaps := make([]hunkGap, len(file.Sections))
	prevRightEnd, prevLeftEnd := 0, 0

	for i, h := range file.Sections {
		firstRight, lastRight := firstLastLineNums(h.LineNumbers.Right)
		firstLeft, lastLeft := firstLastLineNums(h.LineNumbers.Left)

		rs, re := prevRightEnd+1, firstRight-1
		ls, le := prevLeftEnd+1, firstLeft-1

		g := hunkGap{}
		// Only surface a gap when both sides agree on a non-empty, equal-length
		// region — this keeps the left/right line-number offset consistent and
		// skips odd hunks (e.g. pure additions/deletions at a file boundary).
		if firstRight > 0 && firstLeft > 0 && re >= rs && le >= ls && (re-rs) == (le-ls) {
			g.HasGap = true
			g.RightStart, g.RightEnd = rs, re
			g.LeftStart, g.LeftEnd = ls, le
			g.Size = re - rs + 1
		}
		gaps[i] = g

		if lastRight > 0 {
			prevRightEnd = lastRight
		}
		if lastLeft > 0 {
			prevLeftEnd = lastLeft
		}
	}

	bg := bottomGap{}
	if len(file.Sections) > 0 {
		bg.HasGap = true
		bg.RightStart = prevRightEnd + 1
		bg.LeftStart = prevLeftEnd + 1
	}
	return gaps, bg
}

// splitLine is one visual row of the side-by-side ("split") diff view. Each row
// pairs an old-side line with a new-side line: deletions sit on the left,
// additions on the right, unchanged context on both sides, and a modified block
// pairs its deletions with its additions row-for-row. Leftover lines get an
// empty (filler) cell on the opposite side.
type splitLine struct {
	LeftNum      int
	RightNum     int
	LeftContent  string
	RightContent string
	LeftType     string // "context", "deletion", or "" when the left cell is a filler
	RightType    string // "context", "addition", or "" when the right cell is a filler
	RowType      string // "context", "addition", "deletion", or "replace"
}

// computeSplitSections converts each parsed hunk into aligned side-by-side rows
// for the split diff view. The returned slice is index-aligned with
// file.Sections so the template can pair every hunk with its expand controls.
func computeSplitSections(file models.DiffFile) [][]splitLine {
	sections := make([][]splitLine, len(file.Sections))
	for si, h := range file.Sections {
		var rows []splitLine

		// A change block is a run of deletions followed by additions (git emits
		// them in that order). It is flushed on the next context line or at the
		// end of the hunk, pairing deletions with additions row-for-row.
		var dels, adds []splitLine
		flush := func() {
			n := len(dels)
			if len(adds) > n {
				n = len(adds)
			}
			for i := 0; i < n; i++ {
				row := splitLine{RowType: "replace"}
				if i < len(dels) {
					row.LeftNum = dels[i].LeftNum
					row.LeftContent = dels[i].LeftContent
					row.LeftType = "deletion"
				}
				if i < len(adds) {
					row.RightNum = adds[i].RightNum
					row.RightContent = adds[i].RightContent
					row.RightType = "addition"
				}
				switch {
				case row.LeftType == "":
					row.RowType = "addition"
				case row.RightType == "":
					row.RowType = "deletion"
				}
				rows = append(rows, row)
			}
			dels = dels[:0]
			adds = adds[:0]
		}

		for li, raw := range h.Lines {
			leftNum, rightNum := 0, 0
			if li < len(h.LineNumbers.Left) {
				leftNum = h.LineNumbers.Left[li]
			}
			if li < len(h.LineNumbers.Right) {
				rightNum = h.LineNumbers.Right[li]
			}
			content := trimDiffPrefix(raw)
			switch {
			case strings.HasPrefix(raw, "\\"):
				// "\ No newline at end of file" is a meta-marker attached to the
				// adjacent line, not a source line. Skip it entirely so it neither
				// renders as content nor splits an otherwise-pairable change block.
				continue
			case strings.HasPrefix(raw, "+"):
				adds = append(adds, splitLine{RightNum: rightNum, RightContent: content})
			case strings.HasPrefix(raw, "-"):
				dels = append(dels, splitLine{LeftNum: leftNum, LeftContent: content})
			default: // context: space-prefixed or blank
				flush()
				rows = append(rows, splitLine{
					LeftNum:      leftNum,
					RightNum:     rightNum,
					LeftContent:  content,
					RightContent: content,
					LeftType:     "context",
					RightType:    "context",
					RowType:      "context",
				})
			}
		}
		flush()
		sections[si] = rows
	}
	return sections
}

// extractFilesFromDiff extracts file paths from a diff output
func extractFilesFromDiff(filePaths []string, reviewState *models.ReviewState, repoPath string) []map[string]string {
	var files []map[string]string

	// Map to store file status
	fileStatusMap := make(map[string]string)

	// Process review state to determine file status
	for _, review := range reviewState.ReviewedFiles {
		if review.Repo != repoPath {
			continue
		}

		// Determine file status based on line statuses
		var approved, rejected, skipped bool
		for _, status := range review.Lines {
			switch status {
			case models.StateApproved:
				approved = true
			case models.StateRejected:
				rejected = true
			case models.StateSkipped:
				skipped = true
			}
		}

		// Prioritize rejection, then approval, then skipped
		status := "unreviewed"
		if rejected {
			status = models.StateRejected
		} else if approved {
			status = models.StateApproved
		} else if skipped {
			status = models.StateSkipped
		}

		fileStatusMap[review.Path] = status
	}

	// Build file list from the changed-file paths
	for _, path := range filePaths {
		status, exists := fileStatusMap[path]
		if !exists {
			status = "unreviewed"
		}

		files = append(files, map[string]string{
			"Path":   path,
			"Status": status,
		})
	}

	// Sort files by status and then alphabetically
	sort.Slice(files, func(i, j int) bool {
		// First sort by status
		iStatus := files[i]["Status"]
		jStatus := files[j]["Status"]

		// Priority order: unreviewed > skipped > rejected > approved
		statusPriority := map[string]int{
			"unreviewed":         0,
			models.StateSkipped:  1,
			models.StateRejected: 2,
			models.StateApproved: 3,
		}

		iPriority := statusPriority[iStatus]
		jPriority := statusPriority[jStatus]

		if iPriority != jPriority {
			return iPriority < jPriority
		}

		// Then sort alphabetically
		return files[i]["Path"] < files[j]["Path"]
	})

	return files
}

// generateCommentID generates a random hex ID for a review comment
func generateCommentID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// buildDiffRedirectURL constructs the redirect URL for returning to the diff view
func buildDiffRedirectURL(repoPath, sourceBranch, targetBranch, sourceCommit, targetCommit, mode, filePath string) string {
	redirectURL := fmt.Sprintf("/diff?repo=%s&source=%s&target=%s&source_commit=%s&target_commit=%s&mode=%s",
		url.QueryEscape(repoPath),
		url.QueryEscape(sourceBranch),
		url.QueryEscape(targetBranch),
		url.QueryEscape(sourceCommit),
		url.QueryEscape(targetCommit),
		url.QueryEscape(mode))
	if filePath != "" {
		redirectURL += "&file=" + url.QueryEscape(filePath)
	}
	return redirectURL
}

// loadOrCreateReview loads an existing review or creates a new draft
func (s *Server) loadOrCreateReview(repoPath, sourceBranch, targetBranch, sourceCommit, targetCommit, mode string) (*models.Review, error) {
	review, err := s.storage.LoadReview(repoPath, sourceBranch, targetBranch, sourceCommit, targetCommit)
	if err != nil {
		return nil, err
	}
	if review.ID == "" {
		id, err := generateCommentID()
		if err != nil {
			return nil, err
		}
		review.ID = id
		review.RepoPath = repoPath
		review.SourceBranch = sourceBranch
		review.TargetBranch = targetBranch
		review.SourceCommit = sourceCommit
		review.TargetCommit = targetCommit
		review.DiffMode = mode
		review.Status = models.ReviewStatusDraft
		review.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return review, nil
}

// handleAddComment handles POST /api/review/comment — adds a new inline comment
func (s *Server) handleAddComment(w http.ResponseWriter, r *http.Request) {
	// Parse form data
	if err := r.ParseForm(); err != nil {
		s.renderError(w, "Invalid Form", "Invalid form data submitted", http.StatusBadRequest)
		return
	}

	// Read context params from query string
	p := parseDiffParams(r)

	// Read comment data from form body
	filePath := r.FormValue("file_path")
	startLineStr := r.FormValue("start_line")
	endLineStr := r.FormValue("end_line")
	side := r.FormValue("side")
	body := r.FormValue("body")

	if p.RepoPath == "" || p.SourceCommit == "" || p.TargetCommit == "" || filePath == "" || body == "" || startLineStr == "" {
		s.renderError(w, "Missing Parameters", "Missing required parameters for adding a comment", http.StatusBadRequest)
		return
	}

	startLine, err := strconv.Atoi(startLineStr)
	if err != nil {
		s.renderError(w, "Invalid Parameter", "start_line must be a number", http.StatusBadRequest)
		return
	}

	endLine := startLine
	if endLineStr != "" {
		endLine, err = strconv.Atoi(endLineStr)
		if err != nil {
			s.renderError(w, "Invalid Parameter", "end_line must be a number", http.StatusBadRequest)
			return
		}
	}

	if side == "" {
		side = "right"
	}

	// Load or create review
	review, err := s.loadOrCreateReview(p.RepoPath, p.SourceBranch, p.TargetBranch, p.SourceCommit, p.TargetCommit, p.Mode)
	if err != nil {
		s.renderError(w, "Review Error", fmt.Sprintf("Failed to load review: %v", err), http.StatusInternalServerError)
		return
	}

	// Generate comment ID
	commentID, err := generateCommentID()
	if err != nil {
		s.renderError(w, "Internal Error", "Failed to generate comment ID", http.StatusInternalServerError)
		return
	}

	// Create comment
	comment := models.ReviewComment{
		ID:        commentID,
		FilePath:  filePath,
		StartLine: startLine,
		EndLine:   endLine,
		Side:      side,
		Body:      body,
		Status:    models.CommentStatusOpen,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	review.Comments = append(review.Comments, comment)

	// Save review
	if err := s.storage.SaveReview(review, p.RepoPath); err != nil {
		s.renderError(w, "Storage Error", fmt.Sprintf("Failed to save comment: %v", err), http.StatusInternalServerError)
		return
	}

	// Redirect back to diff view
	redirectURL := buildDiffRedirectURL(p.RepoPath, p.SourceBranch, p.TargetBranch, p.SourceCommit, p.TargetCommit, p.Mode, filePath)
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// handleDeleteComment handles DELETE /api/review/comment — removes a comment
func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	p := parseDiffParams(r)
	commentID := r.URL.Query().Get("comment_id")

	if p.RepoPath == "" || p.SourceCommit == "" || p.TargetCommit == "" || commentID == "" {
		s.renderError(w, "Missing Parameters", "Missing required parameters for deleting a comment", http.StatusBadRequest)
		return
	}

	review, err := s.storage.LoadReview(p.RepoPath, p.SourceBranch, p.TargetBranch, p.SourceCommit, p.TargetCommit)
	if err != nil {
		s.renderError(w, "Review Error", fmt.Sprintf("Failed to load review: %v", err), http.StatusInternalServerError)
		return
	}

	// Remove the comment
	found := false
	newComments := make([]models.ReviewComment, 0, len(review.Comments))
	for _, c := range review.Comments {
		if c.ID == commentID {
			found = true
			continue
		}
		newComments = append(newComments, c)
	}
	if !found {
		s.renderError(w, "Not Found", "Comment not found", http.StatusNotFound)
		return
	}
	review.Comments = newComments

	if err := s.storage.SaveReview(review, p.RepoPath); err != nil {
		s.renderError(w, "Storage Error", fmt.Sprintf("Failed to save review: %v", err), http.StatusInternalServerError)
		return
	}

	redirectURL := buildDiffRedirectURL(p.RepoPath, p.SourceBranch, p.TargetBranch, p.SourceCommit, p.TargetCommit, p.Mode, p.FilePath)
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// handleResolveComment handles POST /api/review/comment/resolve — toggles resolve/reopen
func (s *Server) handleResolveComment(w http.ResponseWriter, r *http.Request) {
	p := parseDiffParams(r)
	commentID := r.URL.Query().Get("comment_id")

	if p.RepoPath == "" || p.SourceCommit == "" || p.TargetCommit == "" || commentID == "" {
		s.renderError(w, "Missing Parameters", "Missing required parameters for resolving a comment", http.StatusBadRequest)
		return
	}

	review, err := s.storage.LoadReview(p.RepoPath, p.SourceBranch, p.TargetBranch, p.SourceCommit, p.TargetCommit)
	if err != nil {
		s.renderError(w, "Review Error", fmt.Sprintf("Failed to load review: %v", err), http.StatusInternalServerError)
		return
	}

	// Toggle comment status
	found := false
	for i := range review.Comments {
		if review.Comments[i].ID == commentID {
			found = true
			if review.Comments[i].Status == models.CommentStatusOpen {
				review.Comments[i].Status = models.CommentStatusResolved
				review.Comments[i].ResolvedAt = time.Now().UTC().Format(time.RFC3339)
			} else {
				review.Comments[i].Status = models.CommentStatusOpen
				review.Comments[i].ResolvedAt = ""
			}
			break
		}
	}
	if !found {
		s.renderError(w, "Not Found", "Comment not found", http.StatusNotFound)
		return
	}

	if err := s.storage.SaveReview(review, p.RepoPath); err != nil {
		s.renderError(w, "Storage Error", fmt.Sprintf("Failed to save review: %v", err), http.StatusInternalServerError)
		return
	}

	redirectURL := buildDiffRedirectURL(p.RepoPath, p.SourceBranch, p.TargetBranch, p.SourceCommit, p.TargetCommit, p.Mode, p.FilePath)
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// handleSubmitReview handles POST /api/review/submit — submits the review and generates markdown
func (s *Server) handleSubmitReview(w http.ResponseWriter, r *http.Request) {
	p := parseDiffParams(r)

	if p.RepoPath == "" || p.SourceCommit == "" || p.TargetCommit == "" {
		s.renderError(w, "Missing Parameters", "Missing required parameters for submitting review", http.StatusBadRequest)
		return
	}

	review, err := s.storage.LoadReview(p.RepoPath, p.SourceBranch, p.TargetBranch, p.SourceCommit, p.TargetCommit)
	if err != nil {
		s.renderError(w, "Review Error", fmt.Sprintf("Failed to load review: %v", err), http.StatusInternalServerError)
		return
	}

	// Mark review as submitted
	review.Status = models.ReviewStatusSubmitted
	review.SubmittedAt = time.Now().UTC().Format(time.RFC3339)

	// Save updated review
	if err := s.storage.SaveReview(review, p.RepoPath); err != nil {
		s.renderError(w, "Storage Error", fmt.Sprintf("Failed to save review: %v", err), http.StatusInternalServerError)
		return
	}

	// Add to review index for this branch pair so past reviews are discoverable
	reviewIndex, indexErr := s.storage.LoadReviewIndex(p.RepoPath, p.SourceBranch, p.TargetBranch)
	if indexErr != nil {
		reviewIndex = &models.ReviewIndex{
			RepoPath:     p.RepoPath,
			SourceBranch: p.SourceBranch,
			TargetBranch: p.TargetBranch,
			Reviews:      []models.ReviewIndexEntry{},
		}
	}

	// Count open comments for the index entry
	openCount := 0
	for _, c := range review.Comments {
		if c.Status == models.CommentStatusOpen {
			openCount++
		}
	}

	// Avoid duplicate entries for the same commit pair
	alreadyExists := false
	for _, entry := range reviewIndex.Reviews {
		if entry.SourceCommit == p.SourceCommit && entry.TargetCommit == p.TargetCommit {
			alreadyExists = true
			break
		}
	}
	if !alreadyExists {
		reviewIndex.Reviews = append(reviewIndex.Reviews, models.ReviewIndexEntry{
			ReviewID:     review.ID,
			SourceCommit: p.SourceCommit,
			TargetCommit: p.TargetCommit,
			DiffMode:     p.Mode,
			SubmittedAt:  review.SubmittedAt,
			CommentCount: openCount,
		})
		// Best-effort save — don't block the submit flow on index failure
		_ = s.storage.SaveReviewIndex(reviewIndex, p.RepoPath)
	}

	// Get the diff to generate markdown export with code context
	repo, exists, err := s.GetRepository(p.RepoPath)
	if err != nil || !exists {
		s.renderError(w, "Repository Error", "Repository not found", http.StatusNotFound)
		return
	}

	fullDiffText, _ := getDiffForMode(repo, p)

	// Generate markdown export
	markdown := generateMarkdownExport(review, fullDiffText)

	// Render confirmation page
	data := map[string]interface{}{
		"RepoPath":     p.RepoPath,
		"RepoName":     filepath.Base(p.RepoPath),
		"SourceBranch": p.SourceBranch,
		"TargetBranch": p.TargetBranch,
		"SourceCommit": p.SourceCommit,
		"TargetCommit": p.TargetCommit,
		"DiffMode":     p.Mode,
		"Review":       review,
		"Markdown":     markdown,
	}
	s.render(w, "review_submitted.html", data)
}

// handleExportReview handles GET /api/review/export — returns markdown export
func (s *Server) handleExportReview(w http.ResponseWriter, r *http.Request) {
	p := parseDiffParams(r)

	if p.RepoPath == "" || p.SourceCommit == "" || p.TargetCommit == "" {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}

	review, err := s.storage.LoadReview(p.RepoPath, p.SourceBranch, p.TargetBranch, p.SourceCommit, p.TargetCommit)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load review: %v", err), http.StatusInternalServerError)
		return
	}

	// Get the diff for code context
	repo, exists, repoErr := s.GetRepository(p.RepoPath)
	var fullDiffText string
	if repoErr == nil && exists {
		fullDiffText, _ = getDiffForMode(repo, p)
	}

	markdown := generateMarkdownExport(review, fullDiffText)

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Write([]byte(markdown))
}

// handleDeletePastReview handles DELETE /api/review/past — deletes a single past review
func (s *Server) handleDeletePastReview(w http.ResponseWriter, r *http.Request) {
	p := parseDiffParams(r)

	pastSourceCommit := r.URL.Query().Get("past_source_commit")
	pastTargetCommit := r.URL.Query().Get("past_target_commit")

	if p.RepoPath == "" || p.SourceBranch == "" || p.TargetBranch == "" || pastSourceCommit == "" || pastTargetCommit == "" {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}

	if err := s.storage.DeleteReviewData(p.RepoPath, p.SourceBranch, p.TargetBranch, pastSourceCommit, pastTargetCommit); err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete past review: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

// handleDeleteAllPastReviews handles DELETE /api/reviews/past — deletes all past reviews for a branch pair
func (s *Server) handleDeleteAllPastReviews(w http.ResponseWriter, r *http.Request) {
	p := parseDiffParams(r)

	if p.RepoPath == "" || p.SourceBranch == "" || p.TargetBranch == "" {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}

	index, err := s.storage.LoadReviewIndex(p.RepoPath, p.SourceBranch, p.TargetBranch)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load review index: %v", err), http.StatusInternalServerError)
		return
	}

	// Delete all entries except the current commit pair (which is the active review)
	for _, entry := range index.Reviews {
		if entry.SourceCommit == p.SourceCommit && entry.TargetCommit == p.TargetCommit {
			continue
		}
		// Best-effort deletion — continue even if one fails
		_ = s.storage.DeleteReviewData(p.RepoPath, p.SourceBranch, p.TargetBranch, entry.SourceCommit, entry.TargetCommit)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

// generateMarkdownExport creates a formatted markdown document from a review with code context
func generateMarkdownExport(review *models.Review, rawDiff string) string {
	var buf bytes.Buffer

	repoName := filepath.Base(review.RepoPath)
	sourceLabel := review.SourceBranch
	if sourceLabel == "" {
		sourceLabel = review.SourceCommit
	}
	targetLabel := review.TargetBranch
	if targetLabel == "" {
		targetLabel = review.TargetCommit
	}

	// Count open comments
	openCount := 0
	for _, c := range review.Comments {
		if c.Status == models.CommentStatusOpen {
			openCount++
		}
	}

	// Header
	buf.WriteString("# Code Review\n\n")
	buf.WriteString(fmt.Sprintf("**Repository**: %s\n", repoName))
	buf.WriteString(fmt.Sprintf("**Comparing**: %s -> %s\n", sourceLabel, targetLabel))
	if len(review.SourceCommit) >= 8 {
		buf.WriteString(fmt.Sprintf("**Source commit**: %s\n", review.SourceCommit[:8]))
	}
	if len(review.TargetCommit) >= 8 {
		buf.WriteString(fmt.Sprintf("**Target commit**: %s\n", review.TargetCommit[:8]))
	}
	buf.WriteString(fmt.Sprintf("**Date**: %s\n", time.Now().UTC().Format(time.RFC3339)))
	buf.WriteString(fmt.Sprintf("**Comments**: %d\n", openCount))
	buf.WriteString("\n---\n\n")

	// Parse diff for code context
	parsedFiles := git.ParseDiff(rawDiff)
	fileMap := make(map[string]models.DiffFile)
	for _, f := range parsedFiles {
		fileMap[f.Path] = f
	}

	// Group open comments by file, then sort by line
	grouped := make(map[string][]models.ReviewComment)
	for _, c := range review.Comments {
		if c.Status != models.CommentStatusOpen {
			continue
		}
		grouped[c.FilePath] = append(grouped[c.FilePath], c)
	}

	// Sort file keys
	fileKeys := make([]string, 0, len(grouped))
	for k := range grouped {
		fileKeys = append(fileKeys, k)
	}
	sort.Strings(fileKeys)

	for _, filePath := range fileKeys {
		comments := grouped[filePath]
		// Sort comments by start line
		sort.Slice(comments, func(i, j int) bool {
			return comments[i].StartLine < comments[j].StartLine
		})

		lang := git.DetectLanguage(filePath)
		buf.WriteString(fmt.Sprintf("## %s\n\n", filePath))

		for _, c := range comments {
			// Line header
			if c.StartLine == c.EndLine {
				buf.WriteString(fmt.Sprintf("### Line %d\n\n", c.StartLine))
			} else {
				buf.WriteString(fmt.Sprintf("### Lines %d-%d\n\n", c.StartLine, c.EndLine))
			}

			// Code context — find surrounding lines from parsed diff
			contextLines := getCodeContext(fileMap, filePath, c.StartLine, c.EndLine, c.Side)
			if len(contextLines) > 0 {
				buf.WriteString(fmt.Sprintf("```%s\n", lang))
				for _, line := range contextLines {
					buf.WriteString(line + "\n")
				}
				buf.WriteString("```\n\n")
			}

			// Comment body as blockquote
			for _, line := range strings.Split(c.Body, "\n") {
				buf.WriteString("> " + line + "\n")
			}
			buf.WriteString("\n---\n\n")
		}
	}

	return buf.String()
}

// getCodeContext extracts the exact lines matching [startLine, endLine] from the parsed diff.
// side controls which line numbers to match: "left" uses left-side (deleted lines),
// "right" uses right-side (added lines), and any other value (including "both") checks both sides.
func getCodeContext(fileMap map[string]models.DiffFile, filePath string, startLine, endLine int, side string) []string {
	df, ok := fileMap[filePath]
	if !ok {
		return nil
	}

	var contextLines []string

	for _, hunk := range df.Sections {
		for i, line := range hunk.Lines {
			leftLine := 0
			if i < len(hunk.LineNumbers.Left) {
				leftLine = hunk.LineNumbers.Left[i]
			}
			rightLine := 0
			if i < len(hunk.LineNumbers.Right) {
				rightLine = hunk.LineNumbers.Right[i]
			}

			// Select the line number to match based on the comment's side
			var match bool
			switch side {
			case "left":
				match = leftLine > 0 && leftLine >= startLine && leftLine <= endLine
			case "right":
				match = rightLine > 0 && rightLine >= startLine && rightLine <= endLine
			default: // "both" or unspecified — check either side
				match = (rightLine > 0 && rightLine >= startLine && rightLine <= endLine) ||
					(leftLine > 0 && leftLine >= startLine && leftLine <= endLine)
			}

			if match {
				// Strip the leading +/-/space prefix for cleaner output
				cleanLine := line
				if len(line) > 0 && (line[0] == '+' || line[0] == '-' || line[0] == ' ') {
					cleanLine = line[1:]
				}
				contextLines = append(contextLines, cleanLine)
			}
		}
	}

	return contextLines
}

// render renders a template with the given data and a 200 OK status
func (s *Server) render(w http.ResponseWriter, templateName string, data interface{}) {
	s.renderWithStatus(w, templateName, data, http.StatusOK)
}

// renderWithStatus renders a template with the given data and HTTP status code.
// It buffers all template output before writing to w, so that headers and status
// code are only sent after successful rendering.
func (s *Server) renderWithStatus(w http.ResponseWriter, templateName string, data interface{}, statusCode int) {
	// First render the content template to a buffer
	var contentBuf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&contentBuf, templateName, data); err != nil {
		log.Printf("Error rendering content template %s: %v", templateName, err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("<html><body><h1>Internal Server Error</h1><p>Failed to render page. Please try again later.</p></body></html>"))
		return
	}

	// Then render the layout with the pre-rendered content into a second buffer
	layoutData := map[string]interface{}{
		"Content":         templateName,
		"ContentData":     data,
		"RenderedContent": template.HTML(contentBuf.String()),
	}

	var layoutBuf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&layoutBuf, "layout.html", layoutData); err != nil {
		log.Printf("Error rendering layout template: %v", err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("<html><body><h1>Internal Server Error</h1><p>Failed to render page layout. Please try again later.</p></body></html>"))
		return
	}

	// Both templates rendered successfully — now write headers and body
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	w.Write(layoutBuf.Bytes())
}

// renderError renders an error page with the given status code and message
func (s *Server) renderError(w http.ResponseWriter, title string, message string, statusCode int) {
	errorData := map[string]interface{}{
		"Title":   title,
		"Message": message,
	}
	s.renderWithStatus(w, "error.html", errorData, statusCode)
}
