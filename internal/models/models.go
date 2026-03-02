package models

// FileReview represents the review state of a file
type FileReview struct {
	Repo  string            `json:"repo"`
	Path  string            `json:"path"`
	Lines map[string]string `json:"lines"` // line number or range -> state (approved, skipped, rejected)
}

// ReviewState represents the overall review state
type ReviewState struct {
	ReviewedFiles []FileReview `json:"reviewed_files"`
	SourceBranch  string       `json:"source_branch"`
	TargetBranch  string       `json:"target_branch"`
	SourceCommit  string       `json:"source_commit"`
	TargetCommit  string       `json:"target_commit"`
	DiffMode      string       `json:"diff_mode,omitempty"`
}

// LineState constants
const (
	StateApproved = "approved"
	StateRejected = "rejected"
	StateSkipped  = "skipped"
)

// DiffMode constants define the type of diff comparison
const (
	ModeBranches = "branches"
	ModeCommits  = "commits"
	ModeStaged   = "staged"
	ModeUnstaged = "unstaged"
)

// DiffFile represents a file diff
type DiffFile struct {
	Path      string     `json:"path"`
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
	Sections  []DiffHunk `json:"sections"`
}

// DiffHunk represents a section of a diff
type DiffHunk struct {
	StartLine   int      `json:"start_line"`
	LineCount   int      `json:"line_count"`
	Context     string   `json:"context"`
	Lines       []string `json:"lines"`
	LineNumbers struct {
		Left  []int `json:"left"`
		Right []int `json:"right"`
	} `json:"line_numbers"`
}

// ReviewComment represents a single inline comment on a diff
type ReviewComment struct {
	ID         string `json:"id"`                    // UUID, generated server-side
	FilePath   string `json:"file_path"`             // e.g. "internal/server/server.go"
	StartLine  int    `json:"start_line"`            // First line number (in diff output)
	EndLine    int    `json:"end_line"`              // Last line (same as start for single-line)
	Side       string `json:"side"`                  // "left" (old) or "right" (new) or "both"
	Body       string `json:"body"`                  // Comment text (plain text or markdown)
	Status     string `json:"status"`                // "open", "resolved"
	CreatedAt  string `json:"created_at"`            // ISO 8601 timestamp
	ResolvedAt string `json:"resolved_at,omitempty"` // ISO 8601 timestamp
}

// Review represents a batch of comments for a diff comparison
type Review struct {
	ID           string          `json:"id"`
	RepoPath     string          `json:"repo_path"`
	SourceBranch string          `json:"source_branch"`
	TargetBranch string          `json:"target_branch"`
	SourceCommit string          `json:"source_commit"`
	TargetCommit string          `json:"target_commit"`
	DiffMode     string          `json:"diff_mode"`
	Comments     []ReviewComment `json:"comments"`
	Status       string          `json:"status"` // "draft", "submitted"
	CreatedAt    string          `json:"created_at"`
	SubmittedAt  string          `json:"submitted_at,omitempty"`
}

// Comment status constants
const (
	CommentStatusOpen     = "open"
	CommentStatusResolved = "resolved"
)

// Review status constants
const (
	ReviewStatusDraft     = "draft"
	ReviewStatusSubmitted = "submitted"
)
