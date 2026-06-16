package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Gamezar/difftypp/internal/models"
)

func TestComputeHunkGaps(t *testing.T) {
	t.Run("gaps above, between, and below hunks", func(t *testing.T) {
		file := models.DiffFile{
			Path: "f.txt",
			Sections: []models.DiffHunk{
				{
					LineNumbers: struct {
						Left  []int `json:"left"`
						Right []int `json:"right"`
					}{
						Left:  []int{10, 11, 12, 13, 14, 15},
						Right: []int{10, 11, 12, 13, 14, 15},
					},
				},
				{
					LineNumbers: struct {
						Left  []int `json:"left"`
						Right []int `json:"right"`
					}{
						Left:  []int{40, 41, 42, 43, 44, 45},
						Right: []int{40, 41, 42, 43, 44, 45},
					},
				},
			},
		}

		gaps, bottom := computeHunkGaps(file)

		if len(gaps) != 2 {
			t.Fatalf("expected 2 gaps, got %d", len(gaps))
		}

		// Gap above first hunk: lines 1-9
		want0 := hunkGap{HasGap: true, RightStart: 1, RightEnd: 9, LeftStart: 1, LeftEnd: 9, Size: 9}
		if gaps[0] != want0 {
			t.Errorf("gap[0] = %+v, want %+v", gaps[0], want0)
		}

		// Gap between hunks: lines 16-39
		want1 := hunkGap{HasGap: true, RightStart: 16, RightEnd: 39, LeftStart: 16, LeftEnd: 39, Size: 24}
		if gaps[1] != want1 {
			t.Errorf("gap[1] = %+v, want %+v", gaps[1], want1)
		}

		// Bottom gap starts after the last hunk
		if !bottom.HasGap || bottom.RightStart != 46 || bottom.LeftStart != 46 {
			t.Errorf("bottom = %+v, want {true 46 46}", bottom)
		}
	})

	t.Run("no gap when first hunk starts at line 1", func(t *testing.T) {
		file := models.DiffFile{
			Sections: []models.DiffHunk{
				{
					LineNumbers: struct {
						Left  []int `json:"left"`
						Right []int `json:"right"`
					}{
						Left:  []int{1, 2, 3},
						Right: []int{1, 2, 3},
					},
				},
			},
		}
		gaps, bottom := computeHunkGaps(file)
		if gaps[0].HasGap {
			t.Errorf("expected no gap above a hunk starting at line 1, got %+v", gaps[0])
		}
		if !bottom.HasGap || bottom.RightStart != 4 {
			t.Errorf("bottom = %+v, want start 4", bottom)
		}
	})

	t.Run("left/right offset preserved across additions", func(t *testing.T) {
		// A first hunk that adds two lines (right advances faster than left),
		// so the gap below it must carry the +2 right/left offset.
		file := models.DiffFile{
			Sections: []models.DiffHunk{
				{
					LineNumbers: struct {
						Left  []int `json:"left"`
						Right []int `json:"right"`
					}{
						// context 5, +new, +new, context 6  -> left ends at 6, right at 8
						Left:  []int{5, 0, 0, 6},
						Right: []int{5, 6, 7, 8},
					},
				},
				{
					LineNumbers: struct {
						Left  []int `json:"left"`
						Right []int `json:"right"`
					}{
						Left:  []int{20, 21},
						Right: []int{22, 23},
					},
				},
			},
		}
		gaps, _ := computeHunkGaps(file)
		// Gap between: right 9..21, left 7..19 — equal length (13), offset +2.
		want := hunkGap{HasGap: true, RightStart: 9, RightEnd: 21, LeftStart: 7, LeftEnd: 19, Size: 13}
		if gaps[1] != want {
			t.Errorf("gap[1] = %+v, want %+v", gaps[1], want)
		}
	})

	t.Run("empty file yields no bottom gap", func(t *testing.T) {
		gaps, bottom := computeHunkGaps(models.DiffFile{})
		if len(gaps) != 0 {
			t.Errorf("expected no gaps, got %d", len(gaps))
		}
		if bottom.HasGap {
			t.Errorf("expected no bottom gap for empty file, got %+v", bottom)
		}
	})
}

// setupContextRepo builds a repo whose file has several lines so context can be
// expanded, and returns the repo path plus the commit hash holding the content.
func setupContextRepo(t *testing.T) (string, string) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "diffty-context-test")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", tempDir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "--local", "commit.gpgsign", "false")
	run("config", "--local", "user.email", "test@example.com")
	run("config", "--local", "user.name", "Test")

	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	run("add", "test.txt")
	run("commit", "-m", "initial")

	cmd := exec.Command("git", "-C", tempDir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	hash := string(out)
	hash = hash[:len(hash)-1] // trim newline

	return tempDir, hash
}

func TestHandleFileContext(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoPath, commit := setupContextRepo(t)

	mock := &MockStorage{repositories: []string{repoPath}}
	origFS := getTemplateDir
	getTemplateDir = func() fs.FS { return baseTestTemplates() }
	t.Cleanup(func() { getTemplateDir = origFS })

	srv, err := New(mock)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	doRequest := func(start, end string) (*http.Response, struct {
		Start int      `json:"start"`
		Lines []string `json:"lines"`
		EOF   bool     `json:"eof"`
	}) {
		q := url.Values{}
		q.Set("repo", repoPath)
		q.Set("source_commit", commit)
		q.Set("mode", models.ModeBranches)
		q.Set("file", "test.txt")
		q.Set("start", start)
		q.Set("end", end)
		req := httptest.NewRequest("GET", "/api/file-context?"+q.Encode(), nil)
		w := httptest.NewRecorder()
		srv.handleFileContext(w, req)
		resp := w.Result()
		var body struct {
			Start int      `json:"start"`
			Lines []string `json:"lines"`
			EOF   bool     `json:"eof"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return resp, body
	}

	t.Run("returns requested range", func(t *testing.T) {
		resp, body := doRequest("2", "4")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		want := []string{"line2", "line3", "line4"}
		if len(body.Lines) != 3 || body.Lines[0] != want[0] || body.Lines[2] != want[2] {
			t.Errorf("lines = %v, want %v", body.Lines, want)
		}
		if body.EOF {
			t.Errorf("expected eof=false for lines 2-4")
		}
	})

	t.Run("clamps past end of file and reports eof", func(t *testing.T) {
		resp, body := doRequest("4", "20")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		if len(body.Lines) != 2 || body.Lines[0] != "line4" || body.Lines[1] != "line5" {
			t.Errorf("lines = %v, want [line4 line5]", body.Lines)
		}
		if !body.EOF {
			t.Errorf("expected eof=true when range exceeds file length")
		}
	})

	t.Run("missing params is a 400", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/file-context?repo="+url.QueryEscape(repoPath), nil)
		w := httptest.NewRecorder()
		srv.handleFileContext(w, req)
		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Result().StatusCode)
		}
	})

	t.Run("branch mode without source_commit is a 400", func(t *testing.T) {
		q := url.Values{}
		q.Set("repo", repoPath)
		q.Set("mode", models.ModeBranches)
		q.Set("file", "test.txt")
		q.Set("start", "1")
		q.Set("end", "2")
		req := httptest.NewRequest("GET", "/api/file-context?"+q.Encode(), nil)
		w := httptest.NewRecorder()
		srv.handleFileContext(w, req)
		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 when source_commit is missing, got %d", w.Result().StatusCode)
		}
	})
}

func TestSplitFileLines(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", []string{}},
		{"a\nb\nc\n", []string{"a", "b", "c"}},
		{"a\nb\nc", []string{"a", "b", "c"}},
		{"\n", []string{""}},
	}
	for _, c := range cases {
		got := splitFileLines(c.in)
		if len(got) != len(c.want) {
			t.Errorf("splitFileLines(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitFileLines(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}
