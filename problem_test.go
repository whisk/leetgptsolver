package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveProblemIntoAtomic(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "two-sum.json")

	var p Problem
	p.Question.Data.Question.Id = "1"
	p.Question.Data.Question.Title = "Two Sum"
	p.Question.Data.Question.TitleSlug = "two-sum"
	p.Question.Data.Question.Stats = `{"totalAccepted": "50", "totalSubmission": "100", "totalAcceptedRaw": 50, "totalSubmissionRaw": 100, "acRate": "50.0%"}`

	// New file write with default permissions and no leftover temp files.
	if err := p.SaveProblemInto(dst); err != nil {
		t.Fatalf("SaveProblemInto failed: %v", err)
	}
	if info, err := os.Stat(dst); err != nil {
		t.Fatalf("stat failed: %v", err)
	} else if info.Mode().Perm() != 0o644 {
		t.Fatalf("expected mode 0644, got %04o", info.Mode().Perm())
	}

	// Overwrite preserves existing file permissions and updates content.
	if err := os.Chmod(dst, 0o600); err != nil {
		t.Fatalf("chmod failed: %v", err)
	}
	p.Question.Data.Question.Title = "Two Sum Updated"
	if err := p.SaveProblemInto(dst); err != nil {
		t.Fatalf("overwrite SaveProblemInto failed: %v", err)
	}
	if info, err := os.Stat(dst); err != nil {
		t.Fatalf("stat failed: %v", err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected preserved mode 0600, got %04o", info.Mode().Perm())
	}
	var got Problem
	if err := got.ReadProblem(dst); err != nil {
		t.Fatalf("ReadProblem failed: %v", err)
	}
	if got.Question.Data.Question.Title != "Two Sum Updated" {
		t.Fatalf("expected updated title, got %q", got.Question.Data.Question.Title)
	}

	// Failed rename (destination is a directory) returns error and cleans up temp file.
	dstDir := filepath.Join(dir, "existing-dir")
	if err := os.Mkdir(dstDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := p.SaveProblemInto(dstDir); err == nil {
		t.Fatal("expected error when destination is a directory, got nil")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected no leftover temp files (2 entries), got %d", len(entries))
	}
}
