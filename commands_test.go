package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPromptErrorPropagation(t *testing.T) {
	t.Run("empty model returns error", func(t *testing.T) {
		err := prompt([]string{"problem.json"}, "python3", "", "")
		if err == nil {
			t.Fatal("expected error for empty model, got nil")
		}
		if !strings.Contains(err.Error(), "model is not set") {
			t.Errorf("expected 'model is not set', got: %v", err)
		}
	})

	t.Run("unresolvable vendor returns error", func(t *testing.T) {
		err := prompt([]string{"problem.json"}, "python3", "nonexistent-model", "")
		if err == nil {
			t.Fatal("expected error for unresolvable vendor, got nil")
		}
		if !strings.Contains(err.Error(), "failed to resolve vendor") {
			t.Errorf("expected 'failed to resolve vendor', got: %v", err)
		}
		if !strings.Contains(err.Error(), "leetgptsolver models") {
			t.Errorf("expected hint to run 'leetgptsolver models', got: %v", err)
		}
	})

	t.Run("non-text model returns handy unsupported error", func(t *testing.T) {
		err := prompt([]string{"problem.json"}, "python3", "gpt-image-1", "")
		if err == nil {
			t.Fatal("expected error for non-text model, got nil")
		}
		if !strings.Contains(err.Error(), "not a text model") {
			t.Errorf("expected 'not a text model', got: %v", err)
		}
		if !strings.Contains(err.Error(), "leetgptsolver models openai") {
			t.Errorf("expected hint to run 'leetgptsolver models openai', got: %v", err)
		}
	})
}

func TestListErrorPropagation(t *testing.T) {
	t.Run("invalid where expression returns error instead of exit", func(t *testing.T) {
		err := list([]string{}, "this is not valid jq syntax !!!", "", ".", false)
		if err == nil {
			t.Fatal("expected error for invalid jq expression, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse where query") {
			t.Errorf("expected 'failed to parse where query', got: %v", err)
		}
	})

	t.Run("invalid print expression returns error instead of exit", func(t *testing.T) {
		err := list([]string{}, "true", "", "invalid jq {{{", false)
		if err == nil {
			t.Fatal("expected error for invalid print query, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse print query") {
			t.Errorf("expected 'failed to parse print query', got: %v", err)
		}
	})
}

func TestDownloadErrorPropagation(t *testing.T) {
	t.Run("unsupported category returns error instead of exit", func(t *testing.T) {
		err := download("unsupported_category", []string{})
		if err == nil {
			t.Fatal("expected error for unsupported category, got nil")
		}
		if !strings.Contains(err.Error(), "unsupported category") {
			t.Errorf("expected 'unsupported category', got: %v", err)
		}
	})
}

func TestSaveOrUpdateProblem(t *testing.T) {
	initialDownloadedAt := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	initialCreatedAt := time.Date(2025, 5, 20, 0, 0, 0, 0, time.UTC)
	solvedAt := time.Date(2025, 6, 2, 10, 0, 0, 0, time.UTC)
	submittedAt := time.Date(2025, 6, 2, 10, 5, 0, 0, time.UTC)

	sol := Solution{
		Lang:         "python3",
		Prompt:       "Solve Two Sum",
		Answer:       "```python\nclass Solution:\n    pass\n```",
		TypedCode:    "class Solution:\n    pass\n",
		Model:        "gpt-4o",
		Latency:      2 * time.Second,
		SolvedAt:     solvedAt,
		PromptTokens: 120,
		OutputTokens: 45,
	}
	subm := Submission{
		SubmitRequest: SubmitRequest{
			Lang:       "python3",
			QuestionId: "1",
			TypedCode:  "class Solution:\n    pass\n",
		},
		SubmissionId: 123456789,
		CheckResponse: CheckResponse{
			StatusCode: 10,
			StatusMsg:  "Accepted",
			Finished:   true,
			State:      "SUCCESS",
		},
		SubmittedAt: submittedAt,
	}

	makeInitialProblem := func() Problem {
		var p Problem
		p.Question.Data.Question.FrontendId = "1"
		p.Question.Data.Question.Id = "1"
		p.Question.Data.Question.Title = "Two Sum"
		p.Question.Data.Question.TitleSlug = "two-sum"
		p.Question.Data.Question.Difficulty = "Easy"
		p.Question.Data.Question.Content = "<p>Original content</p>"
		p.Question.Data.Question.Likes = 100
		p.Question.Data.Question.Dislikes = 10
		p.Question.Data.Question.Stats = `{"totalAccepted": "50", "totalSubmission": "100", "totalAcceptedRaw": 50, "totalSubmissionRaw": 100, "acRate": "50.0%"}`
		p.DownloadedAt = initialDownloadedAt
		p.CreatedAtApprox = initialCreatedAt
		p.Question.DownloadedAt = initialDownloadedAt
		p.Question.CreatedAtApprox = initialCreatedAt
		p.Solutions = map[string]Solution{"gpt-4o": sol}
		p.SolutionsV2 = map[string]map[string]Solution{"gpt-4o": {"python3": sol}}
		p.Submissions = map[string]Submission{"gpt-4o": subm}
		p.SubmissionsV2 = map[string]map[string]Submission{"gpt-4o": {"python3": subm}}
		return p
	}

	t.Run("updates question metadata and keeps prompts, submissions, and CreatedAtApprox intact", func(t *testing.T) {
		tmpDir := t.TempDir()
		dstFile := filepath.Join(tmpDir, "two-sum.json")

		initial := makeInitialProblem()
		if err := initial.SaveProblemInto(dstFile); err != nil {
			t.Fatalf("failed to save initial problem: %v", err)
		}

		updatedDownloadedAt := time.Date(2026, 2, 1, 15, 30, 0, 0, time.UTC)
		var downloaded Problem
		downloaded.Question.Data.Question.FrontendId = "1"
		downloaded.Question.Data.Question.Id = "1"
		downloaded.Question.Data.Question.Title = "Two Sum"
		downloaded.Question.Data.Question.TitleSlug = "two-sum"
		downloaded.Question.Data.Question.Difficulty = "Easy"
		downloaded.Question.Data.Question.Content = "<p>Updated content</p>"
		downloaded.Question.Data.Question.Likes = 250
		downloaded.Question.Data.Question.Dislikes = 25
		downloaded.Question.Data.Question.Stats = `{"totalAccepted": "750", "totalSubmission": "1000", "totalAcceptedRaw": 750, "totalSubmissionRaw": 1000, "acRate": "75.0%"}`
		downloaded.DownloadedAt = updatedDownloadedAt
		downloaded.Question.DownloadedAt = updatedDownloadedAt
		// CreatedAtApprox left zero to match --detect_approx_creation_date=false in update-problems-metadata.py

		if err := saveOrUpdateProblem(dstFile, downloaded, true); err != nil {
			t.Fatalf("saveOrUpdateProblem failed: %v", err)
		}

		var got Problem
		if err := got.ReadProblem(dstFile); err != nil {
			t.Fatalf("failed to read updated problem: %v", err)
		}

		if got.Question.Data.Question.Likes != 250 || got.Question.Data.Question.Dislikes != 25 {
			t.Errorf("expected Likes=250, Dislikes=25, got Likes=%d, Dislikes=%d", got.Question.Data.Question.Likes, got.Question.Data.Question.Dislikes)
		}
		if got.Question.Data.Question.Content != "<p>Updated content</p>" {
			t.Errorf("expected updated content, got %q", got.Question.Data.Question.Content)
		}
		if got.Question.AcRate != "75.0" || got.Question.TotalAccepted != 750 || got.Question.TotalSubmissions != 1000 || got.Question.AcceptanceRate != 0.75 {
			t.Errorf("expected updated stats (75.0, 750/1000, 0.75), got acRate=%q accepted=%d total=%d rate=%v",
				got.Question.AcRate, got.Question.TotalAccepted, got.Question.TotalSubmissions, got.Question.AcceptanceRate)
		}
		if !got.DownloadedAt.Equal(updatedDownloadedAt) {
			t.Errorf("expected DownloadedAt=%v, got %v", updatedDownloadedAt, got.DownloadedAt)
		}
		if !got.CreatedAtApprox.Equal(initialCreatedAt) {
			t.Errorf("expected CreatedAtApprox=%v to be preserved when downloaded value is zero, got %v", initialCreatedAt, got.CreatedAtApprox)
		}
		if !reflect.DeepEqual(got.Solutions, initial.Solutions) {
			t.Errorf("Solutions changed:\ngot:  %+v\nwant: %+v", got.Solutions, initial.Solutions)
		}
		if !reflect.DeepEqual(got.SolutionsV2, initial.SolutionsV2) {
			t.Errorf("SolutionsV2 changed:\ngot:  %+v\nwant: %+v", got.SolutionsV2, initial.SolutionsV2)
		}
		if !reflect.DeepEqual(got.Submissions, initial.Submissions) {
			t.Errorf("Submissions changed:\ngot:  %+v\nwant: %+v", got.Submissions, initial.Submissions)
		}
		if !reflect.DeepEqual(got.SubmissionsV2, initial.SubmissionsV2) {
			t.Errorf("SubmissionsV2 changed:\ngot:  %+v\nwant: %+v", got.SubmissionsV2, initial.SubmissionsV2)
		}
	})

	t.Run("preserves legacy Question.CreatedAtApprox when top-level CreatedAtApprox is zero", func(t *testing.T) {
		tmpDir := t.TempDir()
		dstFile := filepath.Join(tmpDir, "two-sum.json")

		initial := makeInitialProblem()
		initial.CreatedAtApprox = time.Time{}
		initial.DownloadedAt = time.Time{}
		if err := initial.SaveProblemInto(dstFile); err != nil {
			t.Fatalf("failed to save initial problem: %v", err)
		}

		var downloaded Problem
		downloaded.Question.Data.Question.FrontendId = "1"
		downloaded.Question.Data.Question.Id = "1"
		downloaded.Question.Data.Question.Title = "Two Sum"
		downloaded.Question.Data.Question.TitleSlug = "two-sum"
		downloaded.Question.Data.Question.Difficulty = "Easy"
		downloaded.Question.Data.Question.Stats = `{"totalAccepted": "750", "totalSubmission": "1000", "totalAcceptedRaw": 750, "totalSubmissionRaw": 1000, "acRate": "75.0%"}`
		downloaded.DownloadedAt = time.Date(2026, 2, 1, 15, 30, 0, 0, time.UTC)
		downloaded.Question.DownloadedAt = downloaded.DownloadedAt

		if err := saveOrUpdateProblem(dstFile, downloaded, true); err != nil {
			t.Fatalf("saveOrUpdateProblem failed: %v", err)
		}

		var got Problem
		if err := got.ReadProblem(dstFile); err != nil {
			t.Fatalf("failed to read updated problem: %v", err)
		}

		if !got.CreatedAtApprox.Equal(initialCreatedAt) {
			t.Errorf("expected CreatedAtApprox=%v migrated from legacy Question.CreatedAtApprox, got %v", initialCreatedAt, got.CreatedAtApprox)
		}
	})
}
