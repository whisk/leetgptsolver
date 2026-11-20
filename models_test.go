package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
	leetgptsolver "whisk/leetgptsolver/pkg"

	"google.golang.org/genai"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestResolveVendorsToList(t *testing.T) {
	t.Run("no vendor returns all supported vendors", func(t *testing.T) {
		vendors, explicit, err := resolveVendorsToList(nil, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if explicit {
			t.Errorf("expected explicit=false, got true")
		}
		if !reflect.DeepEqual(vendors, leetgptsolver.SupportedVendors()) {
			t.Errorf("got %v, want %v", vendors, leetgptsolver.SupportedVendors())
		}
	})

	t.Run("positional and flag combined and deduplicated", func(t *testing.T) {
		vendors, explicit, err := resolveVendorsToList([]string{"openai", "vertexai", "google"}, "claude")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !explicit {
			t.Errorf("expected explicit=true, got false")
		}
		want := []int{
			leetgptsolver.MODEL_VENDOR_ANTHROPIC,
			leetgptsolver.MODEL_VENDOR_OPENAI,
			leetgptsolver.MODEL_VENDOR_GOOGLE,
		}
		if !reflect.DeepEqual(vendors, want) {
			t.Errorf("got %v, want %v", vendors, want)
		}
	})

	t.Run("unknown vendor returns error", func(t *testing.T) {
		_, _, err := resolveVendorsToList([]string{"nonexistent"}, "")
		if err == nil {
			t.Fatal("expected error for unknown vendor, got nil")
		}
	})
}

func TestNormalizeGoogleModelID(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"publishers/google/models/gemini-2.5-pro", "gemini-2.5-pro"},
		{"projects/p1/locations/us-central1/publishers/google/models/gemini-2.5-flash", "gemini-2.5-flash"},
		{"models/gemini-2.0-flash", "gemini-2.0-flash"},
		{"gemini-2.5-pro", "gemini-2.5-pro"},
	}
	for _, tc := range tests {
		if got := normalizeGoogleModelID(tc.in); got != tc.want {
			t.Errorf("normalizeGoogleModelID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsNonTextModel(t *testing.T) {
	tests := []struct {
		vendor  int
		id      string
		actions []string
		want    bool
	}{
		{leetgptsolver.MODEL_VENDOR_OPENAI, "gpt-5-mini", nil, false},
		{leetgptsolver.MODEL_VENDOR_OPENAI, "o3-mini", nil, false},
		{leetgptsolver.MODEL_VENDOR_OPENAI, "dall-e-3", nil, true},
		{leetgptsolver.MODEL_VENDOR_OPENAI, "gpt-image-1", nil, true},
		{leetgptsolver.MODEL_VENDOR_OPENAI, "whisper-1", nil, true},
		{leetgptsolver.MODEL_VENDOR_OPENAI, "tts-1-hd", nil, true},
		{leetgptsolver.MODEL_VENDOR_OPENAI, "text-embedding-3-large", nil, true},
		{leetgptsolver.MODEL_VENDOR_OPENAI, "omni-moderation-latest", nil, true},
		{leetgptsolver.MODEL_VENDOR_OPENAI, "gpt-4o-realtime-preview", nil, true},
		{leetgptsolver.MODEL_VENDOR_OPENAI, "gpt-4o-audio-preview", nil, true},
		{leetgptsolver.MODEL_VENDOR_OPENAI, "gpt-4o-mini-transcribe", nil, true},
		{leetgptsolver.MODEL_VENDOR_OPENAI, "sora-2", nil, true},

		{leetgptsolver.MODEL_VENDOR_GOOGLE, "gemini-2.5-pro", nil, false},
		{leetgptsolver.MODEL_VENDOR_GOOGLE, "gemini-2.5-flash", []string{"generateContent"}, false},
		{leetgptsolver.MODEL_VENDOR_GOOGLE, "custom-embed", []string{"embedContent"}, true},
		{leetgptsolver.MODEL_VENDOR_GOOGLE, "imagen-3.0-generate-002", nil, true},
		{leetgptsolver.MODEL_VENDOR_GOOGLE, "veo-2.0-generate-001", nil, true},
		{leetgptsolver.MODEL_VENDOR_GOOGLE, "text-embedding-005", nil, true},
		{leetgptsolver.MODEL_VENDOR_GOOGLE, "gemini-embedding-001", nil, true},
		{leetgptsolver.MODEL_VENDOR_GOOGLE, "gemini-2.5-flash-image", nil, true},
		{leetgptsolver.MODEL_VENDOR_GOOGLE, "gemini-2.5-flash-preview-tts", nil, true},
		{leetgptsolver.MODEL_VENDOR_GOOGLE, "gemini-nano-banana-2.1", nil, true},
		{leetgptsolver.MODEL_VENDOR_GOOGLE, "gemini-live-2.5-flash-native-audio", nil, true},

		{leetgptsolver.MODEL_VENDOR_XAI, "grok-3-latest", nil, false},
		{leetgptsolver.MODEL_VENDOR_XAI, "grok-code-fast-1-0825", nil, false},
		{leetgptsolver.MODEL_VENDOR_XAI, "grok-2-image-1212", nil, true},
		{leetgptsolver.MODEL_VENDOR_XAI, "grok-imagine-video", nil, true},

		{leetgptsolver.MODEL_VENDOR_ANTHROPIC, "claude-sonnet-4-5-20250929", nil, false},
		{leetgptsolver.MODEL_VENDOR_DEEPSEEK, "deepseek-reasoner", nil, false},
	}

	for _, tc := range tests {
		got := isNonTextModel(tc.vendor, tc.id, tc.actions)
		if got != tc.want {
			t.Errorf("isNonTextModel(%s, %q, %v) = %v, want %v", leetgptsolver.VendorName(tc.vendor), tc.id, tc.actions, got, tc.want)
		}
	}
}

func TestWriteModels(t *testing.T) {
	sample := []ModelInfo{
		{
			Vendor:         "anthropic",
			ID:             "claude-sonnet-4-5-20250929",
			IsText:         true,
			DisplayName:    "Claude Sonnet 4.5",
			CreatedAt:      time.Date(2025, 9, 29, 0, 0, 0, 0, time.UTC),
			Lifecycle:      "active",
			MaxInputTokens: 200000,
			MaxTokens:      64000,
		},
		{
			Vendor:      "google",
			ID:          "imagen-3.0-generate-002",
			IsText:      false,
			DisplayName: "Imagen 3",
		},
	}

	t.Run("single vendor short format", func(t *testing.T) {
		var buf bytes.Buffer
		if err := writeModels(&buf, sample[:1], false, false); err != nil {
			t.Fatalf("writeModels failed: %v", err)
		}
		want := "claude-sonnet-4-5-20250929\n"
		if buf.String() != want {
			t.Errorf("got %q, want %q", buf.String(), want)
		}
	})

	t.Run("multiple vendors short format", func(t *testing.T) {
		var buf bytes.Buffer
		if err := writeModels(&buf, sample, true, false); err != nil {
			t.Fatalf("writeModels failed: %v", err)
		}
		want := "anthropic\tclaude-sonnet-4-5-20250929\ngoogle\timagen-3.0-generate-002\n"
		if buf.String() != want {
			t.Errorf("got %q, want %q", buf.String(), want)
		}
	})

	t.Run("long format includes header and metadata", func(t *testing.T) {
		var buf bytes.Buffer
		if err := writeModels(&buf, sample, true, true); err != nil {
			t.Fatalf("writeModels failed: %v", err)
		}
		out := buf.String()
		for _, expected := range []string{
			"VENDOR", "ID", "TEXT", "DISPLAY_NAME", "CREATED", "LIFECYCLE", "MAX_INPUT", "MAX_OUTPUT",
			"claude-sonnet-4-5-20250929", "yes", "Claude Sonnet 4.5", "2025-09-29", "active", "200000", "64000",
			"imagen-3.0-generate-002", "no", "Imagen 3",
		} {
			if !strings.Contains(out, expected) {
				t.Errorf("expected output to contain %q, got:\n%s", expected, out)
			}
		}
	})
}

func TestListOpenAICompatibleModels(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/models" {
				t.Errorf("unexpected path: %s", r.URL.Path)
				return jsonResponse(http.StatusNotFound, `{}`), nil
			}
			return jsonResponse(http.StatusOK, `{
				"object": "list",
				"data": [
					{"id": "gpt-5-mini", "object": "model", "created": 1754524800, "owned_by": "system"},
					{"id": "dall-e-3", "object": "model", "created": 1698785189, "owned_by": "system"},
					{"id": "text-embedding-3-large", "object": "model", "created": 1705953180, "owned_by": "system"},
					{"id": "gpt-4o", "object": "model", "created": 1715367049, "owned_by": "system"}
				]
			}`), nil
		}),
	}

	models, err := listOpenAICompatibleModels(context.Background(), leetgptsolver.MODEL_VENDOR_OPENAI, "test-key", "http://openai.test", httpClient, false)
	if err != nil {
		t.Fatalf("listOpenAICompatibleModels failed: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 text models, got %d: %+v", len(models), models)
	}
	if models[0].ID != "gpt-4o" || models[1].ID != "gpt-5-mini" {
		t.Errorf("unexpected sorted IDs: %+v", models)
	}
	if models[0].CreatedAt.IsZero() {
		t.Errorf("expected CreatedAt populated, got %+v", models[0])
	}

	allModels, err := listOpenAICompatibleModels(context.Background(), leetgptsolver.MODEL_VENDOR_OPENAI, "test-key", "http://openai.test", httpClient, true)
	if err != nil {
		t.Fatalf("listOpenAICompatibleModels(all) failed: %v", err)
	}
	if len(allModels) != 4 {
		t.Fatalf("expected all 4 models with --all, got %d: %+v", len(allModels), allModels)
	}
	if allModels[0].ID != "dall-e-3" || allModels[0].IsText {
		t.Errorf("expected dall-e-3 with IsText=false, got %+v", allModels[0])
	}
	if allModels[1].ID != "gpt-4o" || !allModels[1].IsText {
		t.Errorf("expected gpt-4o with IsText=true, got %+v", allModels[1])
	}
}

func TestListAnthropicModels(t *testing.T) {
	var seenLifecycle []string
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			seenLifecycle = r.URL.Query()["lifecycle[]"]
			if r.URL.Query().Get("after_id") == "" {
				return jsonResponse(http.StatusOK, `{
					"data": [
						{
							"id": "claude-sonnet-4-5-20250929",
							"type": "model",
							"display_name": "Claude Sonnet 4.5",
							"created_at": "2025-09-29T00:00:00Z",
							"lifecycle": "active",
							"max_input_tokens": 200000,
							"max_tokens": 64000
						}
					],
					"has_more": true,
					"first_id": "claude-sonnet-4-5-20250929",
					"last_id": "claude-sonnet-4-5-20250929"
				}`), nil
			}
			return jsonResponse(http.StatusOK, `{
				"data": [
					{
						"id": "claude-3-opus-20240229",
						"type": "model",
						"display_name": "Claude 3 Opus",
						"created_at": "2024-02-29T00:00:00Z",
						"lifecycle": "retired",
						"max_input_tokens": 200000,
						"max_tokens": 4096
					}
				],
				"has_more": false,
				"first_id": "claude-3-opus-20240229",
				"last_id": "claude-3-opus-20240229"
			}`), nil
		}),
	}

	t.Run("active only by default", func(t *testing.T) {
		models, err := listAnthropicModels(context.Background(), "test-key", "http://anthropic.test", httpClient, false)
		if err != nil {
			t.Fatalf("listAnthropicModels failed: %v", err)
		}
		if !reflect.DeepEqual(seenLifecycle, []string{"active"}) {
			t.Errorf("expected lifecycle query [active], got %v", seenLifecycle)
		}
		if len(models) != 1 || models[0].ID != "claude-sonnet-4-5-20250929" {
			t.Fatalf("expected only active model, got %+v", models)
		}
		if models[0].DisplayName != "Claude Sonnet 4.5" || models[0].MaxInputTokens != 200000 || models[0].MaxTokens != 64000 {
			t.Errorf("unexpected metadata: %+v", models[0])
		}
	})

	t.Run("all includes retired", func(t *testing.T) {
		models, err := listAnthropicModels(context.Background(), "test-key", "http://anthropic.test", httpClient, true)
		if err != nil {
			t.Fatalf("listAnthropicModels(all) failed: %v", err)
		}
		if len(models) != 2 {
			t.Fatalf("expected 2 models with --all, got %d: %+v", len(models), models)
		}
		if models[0].ID != "claude-3-opus-20240229" || models[1].ID != "claude-sonnet-4-5-20250929" {
			t.Errorf("unexpected sorted models: %+v", models)
		}
	})
}

func TestListDeepseekModels(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{
				"object": "list",
				"data": [
					{"id": "deepseek-reasoner", "object": "model", "owned_by": "deepseek"},
					{"id": "deepseek-chat", "object": "model", "owned_by": "deepseek"}
				]
			}`), nil
		}),
	}

	models, err := listDeepseekModels(context.Background(), "test-key", "http://deepseek.test/", httpClient, false)
	if err != nil {
		t.Fatalf("listDeepseekModels failed: %v", err)
	}
	if len(models) != 2 || models[0].ID != "deepseek-chat" || models[1].ID != "deepseek-reasoner" {
		t.Fatalf("unexpected deepseek models: %+v", models)
	}
}

func TestListGoogleModels(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			var payload map[string]any
			if r.URL.Query().Get("pageToken") == "" {
				payload = map[string]any{
					"publisherModels": []map[string]any{
						{"name": "publishers/google/models/gemini-2.5-pro", "displayName": "Gemini 2.5 Pro"},
						{"name": "publishers/google/models/imagen-3.0-generate-002", "displayName": "Imagen 3"},
					},
					"nextPageToken": "page2",
				}
			} else {
				payload = map[string]any{
					"publisherModels": []map[string]any{
						{"name": "publishers/google/models/gemini-2.5-flash", "displayName": "Gemini 2.5 Flash"},
						{"name": "publishers/google/models/text-embedding-005", "displayName": "Text Embedding"},
					},
				}
			}
			b, err := json.Marshal(payload)
			if err != nil {
				return nil, err
			}
			return jsonResponse(http.StatusOK, string(b)), nil
		}),
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend:     genai.BackendVertexAI,
		APIKey:      "fake-vertex-key",
		HTTPClient:  httpClient,
		HTTPOptions: genai.HTTPOptions{BaseURL: "http://vertex.test"},
	})
	if err != nil {
		t.Fatalf("failed to create genai client: %v", err)
	}

	models, err := listGoogleModels(ctx, client, false)
	if err != nil {
		t.Fatalf("listGoogleModels failed: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 text models, got %d: %+v", len(models), models)
	}
	if models[0].ID != "gemini-2.5-flash" || models[1].ID != "gemini-2.5-pro" {
		t.Errorf("unexpected sorted google models: %+v", models)
	}
	if models[0].DisplayName != "Gemini 2.5 Flash" {
		t.Errorf("expected display name preserved, got %+v", models[0])
	}

	allModels, err := listGoogleModels(ctx, client, true)
	if err != nil {
		t.Fatalf("listGoogleModels(all) failed: %v", err)
	}
	if len(allModels) != 4 {
		t.Fatalf("expected all 4 models with --all, got %d: %+v", len(allModels), allModels)
	}
	if allModels[0].ID != "gemini-2.5-flash" || !allModels[0].IsText {
		t.Errorf("expected gemini-2.5-flash with IsText=true, got %+v", allModels[0])
	}
	if allModels[2].ID != "imagen-3.0-generate-002" || allModels[2].IsText {
		t.Errorf("expected imagen-3.0-generate-002 with IsText=false, got %+v", allModels[2])
	}
}
