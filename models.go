package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	leetgptsolver "whisk/leetgptsolver/pkg"

	"cloud.google.com/go/auth/credentials"
	"github.com/anthropics/anthropic-sdk-go"
	anthropic_option "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/cohesion-org/deepseek-go"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
	"google.golang.org/genai"
)

const xaiBaseURL = "https://api.x.ai/v1"

// ModelInfo holds normalized model metadata returned by a provider.
type ModelInfo struct {
	Vendor         string
	ID             string
	IsText         bool
	DisplayName    string
	CreatedAt      time.Time
	Lifecycle      string
	MaxInputTokens int64
	MaxTokens      int64
}

func listModels(args []string, modelVendor string, longFormat, includeAll bool) error {
	vendors, explicit, err := resolveVendorsToList(args, modelVendor)
	if err != nil {
		return err
	}

	ctx := context.Background()
	var allModels []ModelInfo
	attempted := 0
	succeeded := 0

	for _, vendor := range vendors {
		vendorName := leetgptsolver.VendorName(vendor)
		if ok, reason := vendorConfigured(vendor); !ok {
			if explicit {
				return fmt.Errorf("%s: %s", vendorName, reason)
			}
			log.Warn().Msgf("Skipping %s: %s", vendorName, reason)
			continue
		}

		attempted++
		models, err := fetchVendorModels(ctx, vendor, includeAll)
		if err != nil {
			if explicit {
				return fmt.Errorf("failed to list models for %s: %w", vendorName, err)
			}
			log.Err(err).Msgf("Failed to list models for %s", vendorName)
			continue
		}
		succeeded++
		allModels = append(allModels, models...)
	}

	if attempted == 0 {
		return errors.New("no providers are configured in config.production.yaml")
	}
	if succeeded == 0 {
		return errors.New("failed to list models from all configured providers")
	}

	return writeModels(os.Stdout, allModels, len(vendors) > 1, longFormat)
}

func resolveVendorsToList(args []string, modelVendor string) ([]int, bool, error) {
	rawVendors := make([]string, 0, len(args)+1)
	if strings.TrimSpace(modelVendor) != "" {
		rawVendors = append(rawVendors, modelVendor)
	}
	for _, arg := range args {
		if strings.TrimSpace(arg) != "" {
			rawVendors = append(rawVendors, arg)
		}
	}

	if len(rawVendors) == 0 {
		return leetgptsolver.SupportedVendors(), false, nil
	}

	seen := make(map[int]bool, len(rawVendors))
	vendors := make([]int, 0, len(rawVendors))
	for _, raw := range rawVendors {
		v, err := leetgptsolver.ParseModelVendor(raw)
		if err != nil {
			return nil, true, err
		}
		if v == leetgptsolver.MODEL_VENDOR_UNKNOWN {
			return nil, true, fmt.Errorf("unknown model vendor: %s", raw)
		}
		if !seen[v] {
			seen[v] = true
			vendors = append(vendors, v)
		}
	}

	return vendors, true, nil
}

func vendorConfigured(vendor int) (bool, string) {
	switch vendor {
	case leetgptsolver.MODEL_VENDOR_OPENAI:
		if strings.TrimSpace(options.ChatgptApiKey) == "" {
			return false, "chatgpt_api_key is not set"
		}
		return true, ""
	case leetgptsolver.MODEL_VENDOR_GOOGLE:
		if strings.TrimSpace(options.GeminiCredentialsFile) == "" {
			return false, "gemini_credentials_file is not set"
		}
		if strings.TrimSpace(options.GeminiProjectId) == "" {
			return false, "gemini_project_id is not set"
		}
		if strings.TrimSpace(options.GeminiRegion) == "" {
			return false, "gemini_region is not set"
		}
		return true, ""
	case leetgptsolver.MODEL_VENDOR_ANTHROPIC:
		if strings.TrimSpace(options.ClaudeApiKey) == "" {
			return false, "claude_api_key is not set"
		}
		return true, ""
	case leetgptsolver.MODEL_VENDOR_DEEPSEEK:
		if strings.TrimSpace(options.DeepseekApiKey) == "" {
			return false, "deepseek_api_key is not set"
		}
		return true, ""
	case leetgptsolver.MODEL_VENDOR_XAI:
		if strings.TrimSpace(options.XaiApiKey) == "" {
			return false, "xai_api_key is not set"
		}
		return true, ""
	default:
		return false, "unsupported vendor"
	}
}

func fetchVendorModels(ctx context.Context, vendor int, includeAll bool) ([]ModelInfo, error) {
	switch vendor {
	case leetgptsolver.MODEL_VENDOR_OPENAI:
		return listOpenAICompatibleModels(ctx, vendor, options.ChatgptApiKey, "", nil, includeAll)
	case leetgptsolver.MODEL_VENDOR_XAI:
		return listOpenAICompatibleModels(ctx, vendor, options.XaiApiKey, xaiBaseURL, nil, includeAll)
	case leetgptsolver.MODEL_VENDOR_ANTHROPIC:
		return listAnthropicModels(ctx, options.ClaudeApiKey, "", nil, includeAll)
	case leetgptsolver.MODEL_VENDOR_DEEPSEEK:
		return listDeepseekModels(ctx, options.DeepseekApiKey, "", nil, includeAll)
	case leetgptsolver.MODEL_VENDOR_GOOGLE:
		client, err := newGoogleVertexClient(ctx)
		if err != nil {
			return nil, err
		}
		return listGoogleModels(ctx, client, includeAll)
	default:
		return nil, fmt.Errorf("unsupported vendor: %d", vendor)
	}
}

func newGoogleVertexClient(ctx context.Context) (*genai.Client, error) {
	credJson, err := os.ReadFile(options.GeminiCredentialsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}
	creds, err := credentials.DetectDefault(&credentials.DetectOptions{
		CredentialsJSON: credJson,
		Scopes:          []string{"https://www.googleapis.com/auth/cloud-platform"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load credentials: %w", err)
	}

	cfg := &genai.ClientConfig{
		Project:     options.GeminiProjectId,
		Location:    options.GeminiRegion,
		Backend:     genai.BackendVertexAI,
		Credentials: creds,
	}
	client, err := genai.NewClient(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}
	return client, nil
}

func listOpenAICompatibleModels(ctx context.Context, vendor int, apiKey, baseURL string, httpClient *http.Client, includeAll bool) ([]ModelInfo, error) {
	cfg := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		cfg.BaseURL = baseURL
	}
	if httpClient != nil {
		cfg.HTTPClient = httpClient
	}
	client := openai.NewClientWithConfig(cfg)

	resp, err := client.ListModels(ctx)
	if err != nil {
		return nil, err
	}

	vendorName := leetgptsolver.VendorName(vendor)
	models := make([]ModelInfo, 0, len(resp.Models))
	for _, m := range resp.Models {
		isText := !isNonTextModel(vendor, m.ID, nil)
		if !includeAll && !isText {
			continue
		}
		var createdAt time.Time
		if m.CreatedAt > 0 {
			createdAt = time.Unix(m.CreatedAt, 0).UTC()
		}
		models = append(models, ModelInfo{
			Vendor:    vendorName,
			ID:        m.ID,
			IsText:    isText,
			CreatedAt: createdAt,
		})
	}
	sortModels(models)
	return models, nil
}

func listAnthropicModels(ctx context.Context, apiKey, baseURL string, httpClient *http.Client, includeAll bool) ([]ModelInfo, error) {
	opts := []anthropic_option.RequestOption{
		anthropic_option.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, anthropic_option.WithBaseURL(baseURL))
	}
	if httpClient != nil {
		opts = append(opts, anthropic_option.WithHTTPClient(httpClient))
	}
	client := anthropic.NewClient(opts...)

	params := anthropic.ModelListParams{}
	if includeAll {
		params.Lifecycle = []string{
			string(anthropic.ModelInfoLifecycleActive),
			string(anthropic.ModelInfoLifecycleDeprecated),
			string(anthropic.ModelInfoLifecycleRetired),
		}
	} else {
		params.Lifecycle = []string{
			string(anthropic.ModelInfoLifecycleActive),
		}
	}

	vendorName := leetgptsolver.VendorName(leetgptsolver.MODEL_VENDOR_ANTHROPIC)
	var models []ModelInfo
	pager := client.Models.ListAutoPaging(ctx, params)
	for pager.Next() {
		m := pager.Current()
		if !includeAll && m.Lifecycle != "" && m.Lifecycle != anthropic.ModelInfoLifecycleActive {
			continue
		}
		isText := !isNonTextModel(leetgptsolver.MODEL_VENDOR_ANTHROPIC, m.ID, nil)
		if !includeAll && !isText {
			continue
		}
		models = append(models, ModelInfo{
			Vendor:         vendorName,
			ID:             m.ID,
			IsText:         isText,
			DisplayName:    m.DisplayName,
			CreatedAt:      m.CreatedAt,
			Lifecycle:      string(m.Lifecycle),
			MaxInputTokens: m.MaxInputTokens,
			MaxTokens:      m.MaxTokens,
		})
	}
	if err := pager.Err(); err != nil {
		return nil, err
	}
	sortModels(models)
	return models, nil
}

func listDeepseekModels(ctx context.Context, apiKey, baseURL string, httpClient *http.Client, includeAll bool) ([]ModelInfo, error) {
	var opts []deepseek.Option
	if baseURL != "" {
		opts = append(opts, deepseek.WithBaseURL(baseURL))
	}
	if httpClient != nil {
		opts = append(opts, deepseek.WithHTTPClient(httpClient))
	}
	client, err := deepseek.NewClientWithOptions(apiKey, opts...)
	if err != nil {
		return nil, err
	}

	resp, err := deepseek.ListAllModels(client, ctx)
	if err != nil {
		return nil, err
	}

	vendorName := leetgptsolver.VendorName(leetgptsolver.MODEL_VENDOR_DEEPSEEK)
	models := make([]ModelInfo, 0, len(resp.Data))
	for _, m := range resp.Data {
		isText := !isNonTextModel(leetgptsolver.MODEL_VENDOR_DEEPSEEK, m.ID, nil)
		if !includeAll && !isText {
			continue
		}
		models = append(models, ModelInfo{
			Vendor: vendorName,
			ID:     m.ID,
			IsText: isText,
		})
	}
	sortModels(models)
	return models, nil
}

func listGoogleModels(ctx context.Context, client *genai.Client, includeAll bool) ([]ModelInfo, error) {
	vendorName := leetgptsolver.VendorName(leetgptsolver.MODEL_VENDOR_GOOGLE)
	var models []ModelInfo
	for m, err := range client.Models.All(ctx) {
		if err != nil {
			if errors.Is(err, genai.ErrPageDone) {
				break
			}
			return nil, err
		}
		if m == nil {
			continue
		}
		id := normalizeGoogleModelID(m.Name)
		if id == "" {
			continue
		}
		isText := !isNonTextModel(leetgptsolver.MODEL_VENDOR_GOOGLE, id, m.SupportedActions)
		if !includeAll && !isText {
			continue
		}
		models = append(models, ModelInfo{
			Vendor:         vendorName,
			ID:             id,
			IsText:         isText,
			DisplayName:    m.DisplayName,
			MaxInputTokens: int64(m.InputTokenLimit),
			MaxTokens:      int64(m.OutputTokenLimit),
		})
	}
	sortModels(models)
	return models, nil
}

func normalizeGoogleModelID(name string) string {
	name = strings.TrimSpace(name)
	if idx := strings.LastIndex(name, "/models/"); idx != -1 {
		return name[idx+len("/models/"):]
	}
	return strings.TrimPrefix(name, "models/")
}

// isNonTextModel returns true only when a model can be confidently identified as non-text
// (e.g., image, video, audio/speech/TTS, embedding, or moderation models).
func isNonTextModel(vendor int, id string, supportedActions []string) bool {
	if len(supportedActions) > 0 {
		hasGenerateContent := false
		for _, action := range supportedActions {
			if strings.EqualFold(action, "generateContent") {
				hasGenerateContent = true
				break
			}
		}
		if !hasGenerateContent {
			return true
		}
	}

	lower := strings.ToLower(strings.TrimSpace(id))
	switch vendor {
	case leetgptsolver.MODEL_VENDOR_OPENAI:
		prefixes := []string{
			"dall-e",
			"gpt-image",
			"chatgpt-image",
			"sora",
			"whisper",
			"tts-",
			"text-embedding",
			"omni-moderation",
			"text-moderation",
		}
		for _, p := range prefixes {
			if strings.HasPrefix(lower, p) {
				return true
			}
		}
		substrings := []string{
			"-embedding",
			"-tts",
			"-transcribe",
			"-audio",
			"-realtime",
			"-image-",
		}
		for _, sub := range substrings {
			if strings.Contains(lower, sub) {
				return true
			}
		}
	case leetgptsolver.MODEL_VENDOR_GOOGLE:
		prefixes := []string{
			"imagen",
			"veo",
			"chirp",
			"lyria",
			"textembedding",
			"text-embedding",
			"multimodalembedding",
			"gemini-embedding",
			"embedding-",
			"embeddinggemma",
			"virtual-try-on",
			"imagegeneration",
			"aqa",
		}
		for _, p := range prefixes {
			if strings.HasPrefix(lower, p) {
				return true
			}
		}
		substrings := []string{
			"-tts",
			"-audio",
			"-live-",
			"-image-generation",
			"-image",
			"-embedding",
			"nano-banana",
		}
		for _, sub := range substrings {
			if strings.Contains(lower, sub) {
				return true
			}
		}
	case leetgptsolver.MODEL_VENDOR_XAI:
		prefixes := []string{
			"grok-2-image",
			"grok-imagine",
		}
		for _, p := range prefixes {
			if strings.HasPrefix(lower, p) {
				return true
			}
		}
		substrings := []string{
			"-image",
			"-video",
			"-embedding",
		}
		for _, sub := range substrings {
			if strings.Contains(lower, sub) {
				return true
			}
		}
	default:
		substrings := []string{
			"-embedding",
			"-image",
			"-video",
			"-tts",
			"-audio",
		}
		for _, sub := range substrings {
			if strings.Contains(lower, sub) {
				return true
			}
		}
	}

	return false
}

func sortModels(models []ModelInfo) {
	slices.SortFunc(models, func(a, b ModelInfo) int {
		return strings.Compare(a.ID, b.ID)
	})
}

func writeModels(w io.Writer, models []ModelInfo, showVendor, longFormat bool) error {
	if !longFormat {
		for _, m := range models {
			var err error
			if showVendor {
				_, err = fmt.Fprintf(w, "%s\t%s\n", m.Vendor, m.ID)
			} else {
				_, err = fmt.Fprintf(w, "%s\n", m.ID)
			}
			if err != nil {
				return err
			}
		}
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "VENDOR\tID\tTEXT\tDISPLAY_NAME\tCREATED\tLIFECYCLE\tMAX_INPUT\tMAX_OUTPUT"); err != nil {
		return err
	}
	for _, m := range models {
		text := "no"
		if m.IsText {
			text = "yes"
		}
		created := "-"
		if !m.CreatedAt.IsZero() {
			created = m.CreatedAt.UTC().Format("2006-01-02")
		}
		maxIn := "-"
		if m.MaxInputTokens > 0 {
			maxIn = strconv.FormatInt(m.MaxInputTokens, 10)
		}
		maxOut := "-"
		if m.MaxTokens > 0 {
			maxOut = strconv.FormatInt(m.MaxTokens, 10)
		}
		if _, err := fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			dashIfEmpty(m.Vendor),
			dashIfEmpty(m.ID),
			text,
			dashIfEmpty(m.DisplayName),
			created,
			dashIfEmpty(m.Lifecycle),
			maxIn,
			maxOut,
		); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func dashIfEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
