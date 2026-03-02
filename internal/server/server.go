package server

import (
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
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

//go:embed static
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
		"trimLinePrefix": func(line string) string {
			if len(line) > 0 && (line[0] == '+' || line[0] == '-' || line[0] == ' ') {
				return line[1:]
			}
			return line
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
		// commentsForLine filters review comments for a specific file and line number
		"commentsForLine": func(comments []models.ReviewComment, filePath string, lineNum int, side string) []models.ReviewComment {
			var result []models.ReviewComment
			for _, c := range comments {
				if c.FilePath == filePath && lineNum >= c.StartLine && lineNum <= c.EndLine {
					if c.Side == side || c.Side == "both" || side == "" {
						result = append(result, c)
					}
				}
			}
			return result
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

// GetRepository returns a repository by path
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

	return nil, false, nil
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

	// Static files — wrap with MIME type fixer for embedded assets
	fileServer := http.FileServer(http.FS(staticDir))
	mux.Handle("GET /static/", http.StripPrefix("/static/", mimeFixHandler(fileServer)))

	// API routes
	mux.HandleFunc("POST /api/repository/add", s.handleAddRepository)
	mux.HandleFunc("POST /api/review-state", s.handleReviewState)
	mux.HandleFunc("POST /api/review/comment", s.handleAddComment)
	mux.HandleFunc("DELETE /api/review/comment", s.handleDeleteComment)
	mux.HandleFunc("POST /api/review/comment/resolve", s.handleResolveComment)
	mux.HandleFunc("POST /api/review/submit", s.handleSubmitReview)
	mux.HandleFunc("GET /api/review/export", s.handleExportReview)

	// HTML routes
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
		switch ext {
		case ".css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case ".js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		}
		next.ServeHTTP(w, r)
	})
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
				url.QueryEscape(sourceBranch),
				url.QueryEscape(targetBranch),
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
		commits, err := repo.GetRecentCommits(20)
		if err != nil {
			// Non-fatal: just show empty list
			commits = []git.Commit{}
		}
		data["RecentCommits"] = commits
	}

	s.render(w, "compare.html", data)
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

	// Get the diff based on mode
	var fullDiffText string
	var fullDiffErr error
	var files []map[string]string
	var parsedFiles []models.DiffFile

	fullDiffText, fullDiffErr = getDiffForMode(repo, p)

	if fullDiffErr != nil {
		data["Error"] = fmt.Sprintf("Failed to load diff: %v", fullDiffErr)
	} else if fullDiffText == "" {
		data["NoDiff"] = true
	} else {
		// Parse into structured diff files
		parsedFiles = git.ParseDiff(fullDiffText)
		data["ParsedFiles"] = parsedFiles

		// Extract file paths from parsed diff (for sidebar)
		files = extractFilesFromDiff(parsedFiles, reviewState, p.RepoPath)
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

	if p.FilePath == "" {
		s.render(w, "diff.html", data)
		return
	}

	// If a specific file is requested, find it from the already-parsed full diff.
	// This avoids a redundant git call + re-parse that the old code performed.
	var selectedFile *models.DiffFile
	for i := range parsedFiles {
		if parsedFiles[i].Path == p.FilePath {
			selectedFile = &parsedFiles[i]
			break
		}
	}

	if selectedFile == nil {
		data["Error"] = fmt.Sprintf("File %q not found in diff", p.FilePath)
	} else {
		data["SelectedFile"] = p.FilePath
		data["SelectedFileParsed"] = *selectedFile
		data["SelectedFileLanguage"] = git.DetectLanguage(p.FilePath)

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

	s.render(w, "diff.html", data)
}

// extractFilesFromDiff extracts file paths from a diff output
func extractFilesFromDiff(parsedFiles []models.DiffFile, reviewState *models.ReviewState, repoPath string) []map[string]string {
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

	// Build file list from already-parsed structured diff data
	for _, f := range parsedFiles {
		status, exists := fileStatusMap[f.Path]
		if !exists {
			status = "unreviewed"
		}

		files = append(files, map[string]string{
			"Path":   f.Path,
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

// getCodeContext extracts 3-5 lines of surrounding code context from the parsed diff.
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
				match = leftLine > 0 && leftLine >= startLine-2 && leftLine <= endLine+2
			case "right":
				match = rightLine > 0 && rightLine >= startLine-2 && rightLine <= endLine+2
			default: // "both" or unspecified — check either side
				match = (rightLine > 0 && rightLine >= startLine-2 && rightLine <= endLine+2) ||
					(leftLine > 0 && leftLine >= startLine-2 && leftLine <= endLine+2)
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
