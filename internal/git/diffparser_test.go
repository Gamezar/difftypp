package git

import (
	"testing"
)

func TestParseDiffEmpty(t *testing.T) {
	files := ParseDiff("")
	if files != nil {
		t.Errorf("expected nil for empty diff, got %v", files)
	}
}

func TestParseDiffSingleFile(t *testing.T) {
	raw := `diff --git a/main.go b/main.go
index abc1234..def5678 100644
--- a/main.go
+++ b/main.go
@@ -1,5 +1,6 @@
 package main
 
 import "fmt"
+import "os"
 
 func main() {
`

	files := ParseDiff(raw)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	f := files[0]
	if f.Path != "main.go" {
		t.Errorf("expected path 'main.go', got '%s'", f.Path)
	}
	if f.Additions != 1 {
		t.Errorf("expected 1 addition, got %d", f.Additions)
	}
	if f.Deletions != 0 {
		t.Errorf("expected 0 deletions, got %d", f.Deletions)
	}
	if len(f.Sections) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(f.Sections))
	}

	h := f.Sections[0]
	if h.StartLine != 1 {
		t.Errorf("expected hunk start line 1, got %d", h.StartLine)
	}

	// Verify line numbers
	if len(h.Lines) != 6 {
		t.Fatalf("expected 6 lines in hunk, got %d", len(h.Lines))
	}

	// Line 4 is the addition "+import \"os\"" -- left should be 0 (no old), right should be 4
	if h.LineNumbers.Left[3] != 0 {
		t.Errorf("expected left line 0 for addition, got %d", h.LineNumbers.Left[3])
	}
	if h.LineNumbers.Right[3] != 4 {
		t.Errorf("expected right line 4 for addition, got %d", h.LineNumbers.Right[3])
	}
}

func TestParseDiffMultipleFiles(t *testing.T) {
	raw := `diff --git a/file1.go b/file1.go
index abc..def 100644
--- a/file1.go
+++ b/file1.go
@@ -1,3 +1,4 @@
 package main
+// comment
 
 func main() {}
diff --git a/file2.go b/file2.go
index ghi..jkl 100644
--- a/file2.go
+++ b/file2.go
@@ -1,3 +1,2 @@
 package util
-
 func helper() {}
`

	files := ParseDiff(raw)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	if files[0].Path != "file1.go" {
		t.Errorf("expected 'file1.go', got '%s'", files[0].Path)
	}
	if files[0].Additions != 1 || files[0].Deletions != 0 {
		t.Errorf("file1: expected +1/-0, got +%d/-%d", files[0].Additions, files[0].Deletions)
	}

	if files[1].Path != "file2.go" {
		t.Errorf("expected 'file2.go', got '%s'", files[1].Path)
	}
	if files[1].Additions != 0 || files[1].Deletions != 1 {
		t.Errorf("file2: expected +0/-1, got +%d/-%d", files[1].Additions, files[1].Deletions)
	}
}

func TestParseDiffMultipleHunks(t *testing.T) {
	raw := `diff --git a/main.go b/main.go
index abc..def 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
 
 func main() {
@@ -10,3 +11,4 @@
 }
 
+// end
 func helper() {}
`

	files := ParseDiff(raw)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	if len(files[0].Sections) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(files[0].Sections))
	}

	// Second hunk should start at new line 11
	if files[0].Sections[1].StartLine != 11 {
		t.Errorf("expected second hunk start line 11, got %d", files[0].Sections[1].StartLine)
	}
}

func TestParseDiffDeletionLineNumbers(t *testing.T) {
	raw := `diff --git a/main.go b/main.go
index abc..def 100644
--- a/main.go
+++ b/main.go
@@ -1,4 +1,3 @@
 package main
-import "unused"
 
 func main() {
`

	files := ParseDiff(raw)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	h := files[0].Sections[0]

	// Line index 1 is the deletion "-import \"unused\"": left=2, right=0
	if h.LineNumbers.Left[1] != 2 {
		t.Errorf("expected left line 2 for deletion, got %d", h.LineNumbers.Left[1])
	}
	if h.LineNumbers.Right[1] != 0 {
		t.Errorf("expected right line 0 for deletion, got %d", h.LineNumbers.Right[1])
	}

	// Context line after deletion: left=3, right=2
	if h.LineNumbers.Left[2] != 3 {
		t.Errorf("expected left line 3 for context, got %d", h.LineNumbers.Left[2])
	}
	if h.LineNumbers.Right[2] != 2 {
		t.Errorf("expected right line 2 for context, got %d", h.LineNumbers.Right[2])
	}
}

func TestParseDiffNewFile(t *testing.T) {
	raw := `diff --git a/new_file.go b/new_file.go
new file mode 100644
index 0000000..abc1234
--- /dev/null
+++ b/new_file.go
@@ -0,0 +1,3 @@
+package newpkg
+
+func New() {}
`

	files := ParseDiff(raw)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	f := files[0]
	if f.Path != "new_file.go" {
		t.Errorf("expected path 'new_file.go', got '%s'", f.Path)
	}
	if f.Additions != 3 {
		t.Errorf("expected 3 additions, got %d", f.Additions)
	}
	if f.Deletions != 0 {
		t.Errorf("expected 0 deletions, got %d", f.Deletions)
	}
}

func TestParseDiffHunkContext(t *testing.T) {
	raw := `diff --git a/main.go b/main.go
index abc..def 100644
--- a/main.go
+++ b/main.go
@@ -10,3 +10,4 @@ func main() {
 	x := 1
+	y := 2
 	fmt.Println(x)
 }
`

	files := ParseDiff(raw)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	h := files[0].Sections[0]
	if h.Context != "func main() {" {
		t.Errorf("expected context 'func main() {', got '%s'", h.Context)
	}
}

func TestParseDiffNoNewlineAtEnd(t *testing.T) {
	raw := `diff --git a/main.go b/main.go
index abc..def 100644
--- a/main.go
+++ b/main.go
@@ -1,2 +1,2 @@
 package main
-func old()
\ No newline at end of file
+func new()
\ No newline at end of file
`

	files := ParseDiff(raw)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	h := files[0].Sections[0]
	// Should have: context, deletion, no-newline, addition, no-newline = 5 lines
	if len(h.Lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d", len(h.Lines))
	}
}

func TestParseDiffEmptyContextLinesPreserved(t *testing.T) {
	// Regression test for Fix 5.6: empty lines ("") inside hunks represent
	// blank context lines in the source and must be preserved with correct
	// line numbers. Previously they were silently dropped, causing misalignment.
	raw := `diff --git a/main.go b/main.go
index abc..def 100644
--- a/main.go
+++ b/main.go
@@ -1,5 +1,6 @@
 package main

 import "fmt"
+import "os"

 func main() {
`

	files := ParseDiff(raw)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	h := files[0].Sections[0]

	// The hunk should contain 6 lines:
	// 0: " package main"      (context)   left=1, right=1
	// 1: ""                   (empty ctx)  left=2, right=2
	// 2: " import \"fmt\""    (context)   left=3, right=3
	// 3: "+import \"os\""     (addition)  left=0, right=4
	// 4: ""                   (empty ctx)  left=4, right=5
	// 5: " func main() {"    (context)   left=5, right=6
	if len(h.Lines) != 6 {
		t.Fatalf("expected 6 lines in hunk, got %d: %v", len(h.Lines), h.Lines)
	}

	// Line index 1 is an empty context line — must NOT be dropped
	if h.Lines[1] != "" {
		t.Errorf("expected empty context line at index 1, got %q", h.Lines[1])
	}
	if h.LineNumbers.Left[1] != 2 {
		t.Errorf("empty ctx line index 1: expected left=2, got %d", h.LineNumbers.Left[1])
	}
	if h.LineNumbers.Right[1] != 2 {
		t.Errorf("empty ctx line index 1: expected right=2, got %d", h.LineNumbers.Right[1])
	}

	// Line index 4 is the second empty context line (after the addition)
	if h.Lines[4] != "" {
		t.Errorf("expected empty context line at index 4, got %q", h.Lines[4])
	}
	if h.LineNumbers.Left[4] != 4 {
		t.Errorf("empty ctx line index 4: expected left=4, got %d", h.LineNumbers.Left[4])
	}
	if h.LineNumbers.Right[4] != 5 {
		t.Errorf("empty ctx line index 4: expected right=5, got %d", h.LineNumbers.Right[4])
	}

	// Final context line (index 5) should have correct line numbers
	if h.LineNumbers.Left[5] != 5 {
		t.Errorf("context line index 5: expected left=5, got %d", h.LineNumbers.Left[5])
	}
	if h.LineNumbers.Right[5] != 6 {
		t.Errorf("context line index 5: expected right=6, got %d", h.LineNumbers.Right[5])
	}
}

func TestParseDiffEmptyLineOnlyDiff(t *testing.T) {
	// Test that a diff where blank lines are added/removed is handled correctly
	raw := `diff --git a/main.go b/main.go
index abc..def 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,5 @@
 package main
+
+
 import "fmt"
`

	files := ParseDiff(raw)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	h := files[0].Sections[0]

	// Should have 4 lines:
	// 0: " package main"   context  left=1, right=1
	// 1: "+"               addition left=0, right=2
	// 2: "+"               addition left=0, right=3
	// 3: " import \"fmt\"" context  left=2, right=4
	if len(h.Lines) != 4 {
		t.Fatalf("expected 4 lines in hunk, got %d: %v", len(h.Lines), h.Lines)
	}

	if files[0].Additions != 2 {
		t.Errorf("expected 2 additions, got %d", files[0].Additions)
	}

	// The blank-line additions should be "+"
	if h.Lines[1] != "+" {
		t.Errorf("expected '+' at index 1, got %q", h.Lines[1])
	}
	if h.LineNumbers.Left[1] != 0 {
		t.Errorf("addition index 1: expected left=0, got %d", h.LineNumbers.Left[1])
	}
	if h.LineNumbers.Right[1] != 2 {
		t.Errorf("addition index 1: expected right=2, got %d", h.LineNumbers.Right[1])
	}

	// Final context line
	if h.LineNumbers.Left[3] != 2 {
		t.Errorf("context index 3: expected left=2, got %d", h.LineNumbers.Left[3])
	}
	if h.LineNumbers.Right[3] != 4 {
		t.Errorf("context index 3: expected right=4, got %d", h.LineNumbers.Right[3])
	}
}

func TestParseHunkHeader(t *testing.T) {
	tests := []struct {
		line                                   string
		oldStart, oldCount, newStart, newCount int
		context                                string
	}{
		{"@@ -1,5 +1,6 @@", 1, 5, 1, 6, ""},
		{"@@ -10,3 +11,4 @@ func main() {", 10, 3, 11, 4, "func main() {"},
		{"@@ -0,0 +1,3 @@", 0, 0, 1, 3, ""},
		{"@@ -1 +1,2 @@", 1, 1, 1, 2, ""},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			os, oc, ns, nc, ctx := parseHunkHeader(tt.line)
			if os != tt.oldStart || oc != tt.oldCount || ns != tt.newStart || nc != tt.newCount {
				t.Errorf("parseHunkHeader(%q) = %d,%d,%d,%d; want %d,%d,%d,%d",
					tt.line, os, oc, ns, nc, tt.oldStart, tt.oldCount, tt.newStart, tt.newCount)
			}
			if ctx != tt.context {
				t.Errorf("parseHunkHeader(%q) context = %q, want %q", tt.line, ctx, tt.context)
			}
		})
	}
}

func TestParseFilePath(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"diff --git a/main.go b/main.go", "main.go"},
		{"diff --git a/internal/server/server.go b/internal/server/server.go", "internal/server/server.go"},
		{"diff --git a/path with spaces/file.go b/path with spaces/file.go", "path with spaces/file.go"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := parseFilePath(tt.line)
			if got != tt.want {
				t.Errorf("parseFilePath(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestParseFilePathEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "deeply nested path",
			line: "diff --git a/internal/server/templates/layout.html b/internal/server/templates/layout.html",
			want: "internal/server/templates/layout.html",
		},
		{
			name: "single character filename",
			line: "diff --git a/x b/x",
			want: "x",
		},
		{
			name: "file in root with dotfile",
			line: "diff --git a/.gitignore b/.gitignore",
			want: ".gitignore",
		},
		{
			name: "file with multiple dots",
			line: "diff --git a/file.test.go b/file.test.go",
			want: "file.test.go",
		},
		{
			name: "file with b/ in directory name",
			line: "diff --git a/b/file.go b/b/file.go",
			want: "b/file.go",
		},
		{
			name: "renamed file different a and b paths",
			line: "diff --git a/old_name.go b/new_name.go",
			want: "new_name.go",
		},
		{
			name: "renamed file in different directories",
			line: "diff --git a/old/path.go b/new/path.go",
			want: "new/path.go",
		},
		{
			name: "fallback when no b/ prefix present",
			line: "diff --git a/only_a_path.go",
			want: "only_a_path.go",
		},
		{
			name: "path with special characters in name",
			line: "diff --git a/my-file_v2.go b/my-file_v2.go",
			want: "my-file_v2.go",
		},
		{
			name: "CSS file in static directory",
			line: "diff --git a/internal/server/static/css/style.css b/internal/server/static/css/style.css",
			want: "internal/server/static/css/style.css",
		},
		{
			name: "file with no extension",
			line: "diff --git a/Makefile b/Makefile",
			want: "Makefile",
		},
		{
			name: "empty after diff --git prefix triggers fallback",
			line: "diff --git ",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFilePath(tt.line)
			if got != tt.want {
				t.Errorf("parseFilePath(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestDetectLanguageComplete(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		// Go
		{"main.go", "go"},
		{"internal/server/server.go", "go"},

		// JavaScript / TypeScript
		{"app.js", "javascript"},
		{"component.ts", "typescript"},

		// Python
		{"script.py", "python"},

		// Ruby
		{"app.rb", "ruby"},

		// Rust
		{"lib.rs", "rust"},

		// Java
		{"Main.java", "java"},

		// C family
		{"main.c", "c"},
		{"header.h", "c"},
		{"main.cpp", "cpp"},
		{"module.cc", "cpp"},
		{"lib.cxx", "cpp"},
		{"types.hpp", "cpp"},

		// C#
		{"Program.cs", "csharp"},

		// CSS
		{"style.css", "css"},

		// HTML
		{"index.html", "html"},
		{"page.htm", "html"},

		// JSON
		{"package.json", "json"},

		// YAML
		{"config.yaml", "yaml"},
		{"ci.yml", "yaml"},

		// XML
		{"pom.xml", "xml"},

		// Markdown
		{"README.md", "markdown"},

		// Shell
		{"build.sh", "bash"},
		{"init.bash", "bash"},

		// SQL
		{"schema.sql", "sql"},

		// TOML
		{"Cargo.toml", "toml"},

		// Dockerfile extension
		{"build.dockerfile", "dockerfile"},

		// Special filenames (no extension, handled by basename check)
		{"Dockerfile", "dockerfile"},
		{"path/to/Dockerfile", "dockerfile"},
		{"Makefile", "makefile"},
		{"src/Makefile", "makefile"},

		// Case insensitivity for extensions (strings.ToLower is used)
		{"MAIN.GO", "go"},
		{"App.JS", "javascript"},
		{"Style.CSS", "css"},
		{"README.MD", "markdown"},
		{"CONFIG.YAML", "yaml"},
		{"lib.CPP", "cpp"},
		{"script.PY", "python"},

		// Case insensitivity for special filenames (strings.ToLower on base)
		{"dockerfile", "dockerfile"},
		{"makefile", "makefile"},
		{"DOCKERFILE", "dockerfile"},
		{"MAKEFILE", "makefile"},

		// Files with multiple dots — only last extension matters
		{"file.test.go", "go"},
		{"component.spec.ts", "typescript"},
		{"data.backup.json", "json"},
		{"archive.tar.yml", "yaml"},

		// Hidden files with known extensions
		{".eslintrc.json", "json"},
		{".config.yaml", "yaml"},

		// Hidden files with no recognized extension
		{".gitignore", ""},
		{".env", ""},

		// No extension at all (not a special filename)
		{"LICENSE", ""},
		{"README", ""},

		// Unknown extensions
		{"unknown.xyz", ""},
		{"data.csv", ""},
		{"image.png", ""},
		{"font.woff2", ""},
		{"archive.tar", ""},
		{"doc.pdf", ""},
		{"binary.exe", ""},

		// Empty string
		{"", ""},

		// Path-only with trailing slash edge case
		{"dir/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := DetectLanguage(tt.path)
			if got != tt.want {
				t.Errorf("DetectLanguage(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
