package main

import (
	"strings"
	"testing"
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
