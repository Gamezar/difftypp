package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Gamezar/difftypp/internal/models"
)

// Storage interface defines methods for persisting and retrieving data
type Storage interface {
	SaveReviewState(state *models.ReviewState, repoPath string) error
	LoadReviewState(repoPath, sourceBranch, targetBranch, sourceCommit, targetCommit string) (*models.ReviewState, error)
	SaveReview(review *models.Review, repoPath string) error
	LoadReview(repoPath, sourceBranch, targetBranch, sourceCommit, targetCommit string) (*models.Review, error)
	SaveRepositories(repos []string) error
	LoadRepositories() ([]string, error)
}

// JSONStorage implements Storage using JSON files
type JSONStorage struct {
	baseStoragePath string
	reposPath       string
}

// NewJSONStorage creates a new JSONStorage instance
func NewJSONStorage() (*JSONStorage, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Ensure .difftypp directory exists
	storageDir := filepath.Join(homeDir, ".difftypp")
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	return &JSONStorage{
		baseStoragePath: storageDir,
		reposPath:       filepath.Join(storageDir, "repositories.json"),
	}, nil
}

// sanitizeRepoPath replaces special characters in a repo path to make it safe for use as a directory name
func sanitizeRepoPath(repoPath string) string {
	safe := strings.ReplaceAll(repoPath, string(os.PathSeparator), "_")
	safe = strings.ReplaceAll(safe, ":", "_")
	return safe
}

// reviewStatePath returns the file path for a review state (pure computation, no side effects)
func (s *JSONStorage) reviewStatePath(repoPath, sourceCommit, targetCommit string) string {
	safeRepoPath := sanitizeRepoPath(repoPath)
	return filepath.Join(s.baseStoragePath, safeRepoPath, sourceCommit, targetCommit, "review-state.json")
}

// reviewPath returns the file path for a review (pure computation, no side effects)
func (s *JSONStorage) reviewPath(repoPath, sourceCommit, targetCommit string) string {
	safeRepoPath := sanitizeRepoPath(repoPath)
	return filepath.Join(s.baseStoragePath, "reviews", safeRepoPath, sourceCommit, targetCommit, "review.json")
}

// ensureDir creates the parent directory of the given file path if it doesn't exist
func ensureDir(filePath string) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	return nil
}

// newEmptyReviewState creates an empty ReviewState with the given parameters
func newEmptyReviewState(sourceBranch, targetBranch, sourceCommit, targetCommit string) *models.ReviewState {
	return &models.ReviewState{
		ReviewedFiles: []models.FileReview{},
		SourceBranch:  sourceBranch,
		TargetBranch:  targetBranch,
		SourceCommit:  sourceCommit,
		TargetCommit:  targetCommit,
	}
}

// newEmptyReview creates an empty Review with the given parameters
func newEmptyReview(repoPath, sourceBranch, targetBranch, sourceCommit, targetCommit string) *models.Review {
	return &models.Review{
		RepoPath:     repoPath,
		SourceBranch: sourceBranch,
		TargetBranch: targetBranch,
		SourceCommit: sourceCommit,
		TargetCommit: targetCommit,
		Comments:     []models.ReviewComment{},
		Status:       models.ReviewStatusDraft,
	}
}

// SaveReviewState saves the review state to a JSON file
func (s *JSONStorage) SaveReviewState(state *models.ReviewState, repoPath string) error {
	if state.SourceCommit == "" || state.TargetCommit == "" {
		return fmt.Errorf("source and target commit hashes are required")
	}

	storagePath := s.reviewStatePath(repoPath, state.SourceCommit, state.TargetCommit)

	if err := ensureDir(storagePath); err != nil {
		return fmt.Errorf("failed to prepare review state directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal review state: %w", err)
	}

	if err := os.WriteFile(storagePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write review state: %w", err)
	}

	return nil
}

// LoadReviewState loads the review state from a JSON file
func (s *JSONStorage) LoadReviewState(repoPath, sourceBranch, targetBranch, sourceCommit, targetCommit string) (*models.ReviewState, error) {
	if sourceCommit == "" || targetCommit == "" {
		return newEmptyReviewState(sourceBranch, targetBranch, sourceCommit, targetCommit), nil
	}

	storagePath := s.reviewStatePath(repoPath, sourceCommit, targetCommit)

	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		return newEmptyReviewState(sourceBranch, targetBranch, sourceCommit, targetCommit), nil
	}

	data, err := os.ReadFile(storagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read review state: %w", err)
	}

	var state models.ReviewState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal review state: %w", err)
	}

	return &state, nil
}

// SaveRepositories saves the repository paths to a JSON file
func (s *JSONStorage) SaveRepositories(repos []string) error {
	data, err := json.MarshalIndent(repos, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal repositories: %w", err)
	}

	if err := os.WriteFile(s.reposPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write repositories: %w", err)
	}

	return nil
}

// LoadRepositories loads the repository paths from a JSON file
func (s *JSONStorage) LoadRepositories() ([]string, error) {
	if _, err := os.Stat(s.reposPath); os.IsNotExist(err) {
		return []string{}, nil
	}

	data, err := os.ReadFile(s.reposPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read repositories: %w", err)
	}

	var repos []string
	if err := json.Unmarshal(data, &repos); err != nil {
		return nil, fmt.Errorf("failed to unmarshal repositories: %w", err)
	}

	return repos, nil
}

// SaveReview saves a review with inline comments to a JSON file
func (s *JSONStorage) SaveReview(review *models.Review, repoPath string) error {
	if review.SourceCommit == "" || review.TargetCommit == "" {
		return fmt.Errorf("source and target commit hashes are required")
	}

	storagePath := s.reviewPath(repoPath, review.SourceCommit, review.TargetCommit)

	if err := ensureDir(storagePath); err != nil {
		return fmt.Errorf("failed to prepare review directory: %w", err)
	}

	data, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal review: %w", err)
	}

	if err := os.WriteFile(storagePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write review: %w", err)
	}

	return nil
}

// LoadReview loads a review with inline comments from a JSON file
func (s *JSONStorage) LoadReview(repoPath, sourceBranch, targetBranch, sourceCommit, targetCommit string) (*models.Review, error) {
	if sourceCommit == "" || targetCommit == "" {
		return newEmptyReview(repoPath, sourceBranch, targetBranch, sourceCommit, targetCommit), nil
	}

	storagePath := s.reviewPath(repoPath, sourceCommit, targetCommit)

	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		return newEmptyReview(repoPath, sourceBranch, targetBranch, sourceCommit, targetCommit), nil
	}

	data, err := os.ReadFile(storagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read review: %w", err)
	}

	var review models.Review
	if err := json.Unmarshal(data, &review); err != nil {
		return nil, fmt.Errorf("failed to unmarshal review: %w", err)
	}

	return &review, nil
}
