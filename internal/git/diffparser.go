package git

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/darccio/diffty/internal/models"
)

// ParseDiff parses raw unified diff output into structured DiffFile slices.
// It handles standard git diff output with diff --git headers, @@ hunk headers,
// and +/-/context lines. It computes dual line numbers (old/new) for each line.
func ParseDiff(rawDiff string) []models.DiffFile {
	if rawDiff == "" {
		return nil
	}

	var files []models.DiffFile
	lines := strings.Split(rawDiff, "\n")

	var currentFile *models.DiffFile
	var currentHunk *models.DiffHunk
	var leftLine, rightLine int

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// New file boundary
		if strings.HasPrefix(line, "diff --git ") {
			// Save previous file if any
			if currentFile != nil {
				if currentHunk != nil {
					currentFile.Sections = append(currentFile.Sections, *currentHunk)
					currentHunk = nil
				}
				files = append(files, *currentFile)
			}

			// Parse file path from "diff --git a/path b/path"
			filePath := parseFilePath(line)
			currentFile = &models.DiffFile{
				Path: filePath,
			}
			currentHunk = nil
			continue
		}

		// Skip file metadata lines (index, ---, +++)
		if currentFile != nil && currentHunk == nil {
			if strings.HasPrefix(line, "index ") ||
				strings.HasPrefix(line, "--- ") ||
				strings.HasPrefix(line, "+++ ") ||
				strings.HasPrefix(line, "old mode ") ||
				strings.HasPrefix(line, "new mode ") ||
				strings.HasPrefix(line, "deleted file mode ") ||
				strings.HasPrefix(line, "new file mode ") ||
				strings.HasPrefix(line, "similarity index ") ||
				strings.HasPrefix(line, "rename from ") ||
				strings.HasPrefix(line, "rename to ") ||
				strings.HasPrefix(line, "copy from ") ||
				strings.HasPrefix(line, "copy to ") ||
				strings.HasPrefix(line, "Binary files ") {
				continue
			}
		}

		// Hunk header: @@ -old,count +new,count @@ context
		if strings.HasPrefix(line, "@@") {
			if currentFile == nil {
				continue
			}

			// Save previous hunk
			if currentHunk != nil {
				currentFile.Sections = append(currentFile.Sections, *currentHunk)
			}

			oldStart, oldCount, newStart, newCount, context := parseHunkHeader(line)
			currentHunk = &models.DiffHunk{
				StartLine: newStart,
				LineCount: newCount,
				Context:   context,
			}
			_ = oldCount // used implicitly via leftLine tracking
			_ = newCount
			leftLine = oldStart
			rightLine = newStart
			continue
		}

		// Diff content lines
		if currentHunk == nil || currentFile == nil {
			continue
		}

		if strings.HasPrefix(line, "+") {
			// Addition line
			currentHunk.Lines = append(currentHunk.Lines, line)
			currentHunk.LineNumbers.Left = append(currentHunk.LineNumbers.Left, 0) // no old line
			currentHunk.LineNumbers.Right = append(currentHunk.LineNumbers.Right, rightLine)
			currentFile.Additions++
			rightLine++
		} else if strings.HasPrefix(line, "-") {
			// Deletion line
			currentHunk.Lines = append(currentHunk.Lines, line)
			currentHunk.LineNumbers.Left = append(currentHunk.LineNumbers.Left, leftLine)
			currentHunk.LineNumbers.Right = append(currentHunk.LineNumbers.Right, 0) // no new line
			currentFile.Deletions++
			leftLine++
		} else if strings.HasPrefix(line, " ") || line == "" {
			// Context line — includes lines starting with space and truly empty lines
			// inside hunks (which represent blank lines in the source code).
			// Only skip a trailing empty line at the very end of the input.
			if line == "" && i == len(lines)-1 {
				continue
			}
			currentHunk.Lines = append(currentHunk.Lines, line)
			currentHunk.LineNumbers.Left = append(currentHunk.LineNumbers.Left, leftLine)
			currentHunk.LineNumbers.Right = append(currentHunk.LineNumbers.Right, rightLine)
			leftLine++
			rightLine++
		} else if line == "\\ No newline at end of file" {
			currentHunk.Lines = append(currentHunk.Lines, line)
			currentHunk.LineNumbers.Left = append(currentHunk.LineNumbers.Left, 0)
			currentHunk.LineNumbers.Right = append(currentHunk.LineNumbers.Right, 0)
		}
	}

	// Save last file
	if currentFile != nil {
		if currentHunk != nil {
			currentFile.Sections = append(currentFile.Sections, *currentHunk)
		}
		files = append(files, *currentFile)
	}

	return files
}

// parseFilePath extracts the file path from a "diff --git a/path b/path" line.
// It prefers the b/ path (new file name).
func parseFilePath(diffLine string) string {
	// Format: diff --git a/path/to/file b/path/to/file
	parts := strings.SplitN(diffLine, " b/", 2)
	if len(parts) == 2 {
		return parts[1]
	}

	// Fallback: try to extract from a/ prefix
	trimmed := strings.TrimPrefix(diffLine, "diff --git ")
	parts = strings.SplitN(trimmed, " ", 2)
	if len(parts) >= 1 {
		return strings.TrimPrefix(parts[0], "a/")
	}

	return ""
}

// parseHunkHeader parses @@ -old,count +new,count @@ optional context
// Returns oldStart, oldCount, newStart, newCount, context
func parseHunkHeader(line string) (int, int, int, int, string) {
	// Strip @@ markers
	// Format: @@ -old,count +new,count @@ optional context
	line = strings.TrimPrefix(line, "@@")
	parts := strings.SplitN(line, "@@", 2)
	if len(parts) < 1 {
		return 1, 0, 1, 0, ""
	}

	rangeStr := strings.TrimSpace(parts[0])
	context := ""
	if len(parts) > 1 {
		context = strings.TrimSpace(parts[1])
	}

	// Parse -old,count +new,count
	rangeParts := strings.Fields(rangeStr)
	oldStart, oldCount := 1, 0
	newStart, newCount := 1, 0

	for _, part := range rangeParts {
		if strings.HasPrefix(part, "-") {
			oldStart, oldCount = parseRange(strings.TrimPrefix(part, "-"))
		} else if strings.HasPrefix(part, "+") {
			newStart, newCount = parseRange(strings.TrimPrefix(part, "+"))
		}
	}

	return oldStart, oldCount, newStart, newCount, context
}

// parseRange parses "start,count" or "start" into start and count values
func parseRange(s string) (int, int) {
	parts := strings.SplitN(s, ",", 2)
	start, _ := strconv.Atoi(parts[0])
	count := 1
	if len(parts) == 2 {
		count, _ = strconv.Atoi(parts[1])
	}
	return start, count
}

// DetectLanguage returns the language identifier for syntax highlighting
// based on the file extension. Returns empty string if unknown.
func DetectLanguage(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		return "go"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".py":
		return "python"
	case ".rb":
		return "ruby"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "cpp"
	case ".cs":
		return "csharp"
	case ".css":
		return "css"
	case ".html", ".htm":
		return "html"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".xml":
		return "xml"
	case ".md":
		return "markdown"
	case ".sh", ".bash":
		return "bash"
	case ".sql":
		return "sql"
	case ".toml":
		return "toml"
	case ".dockerfile":
		return "dockerfile"
	default:
		// Check filename for Dockerfile, Makefile, etc.
		base := strings.ToLower(filepath.Base(filePath))
		if base == "dockerfile" {
			return "dockerfile"
		}
		if base == "makefile" {
			return "makefile"
		}
		return ""
	}
}
