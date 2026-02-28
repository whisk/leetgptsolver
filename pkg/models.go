package leetgptsolver // import "github.com/whisk/leetgptsolver/pkg"

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	deepseek "github.com/cohesion-org/deepseek-go"
	"github.com/sashabaranov/go-openai"
)

const (
	MODEL_VENDOR_UNKNOWN = iota
	MODEL_VENDOR_UNSUPPORTED
	MODEL_VENDOR_OPENAI
	MODEL_VENDOR_GOOGLE
	MODEL_VENDOR_ANTHROPIC
	MODEL_VENDOR_DEEPSEEK
	MODEL_VENDOR_XAI
)

var OpenAiModels = []string{
	openai.GPT4Turbo0125,           // Knowledge Cutoff: Dec 01, 2023
	openai.O120241217,              // Knowledge Cutoff: Oct 01, 2023
	openai.O3Mini20250131,          // Knowledge Cutoff: Oct 01, 2023
	openai.GPT4Dot5Preview20250227, // Knowledge Cutoff: Oct 01, 2023
	openai.GPT5Mini,                // Knowledge Cutoff: May 31, 2024
}

var GoogleModels = []string{
	"gemini-1.0-pro",
	"gemini-1.5-pro-preview-0409",
	"gemini-2.0-flash-001",
	"gemini-2.0-pro-exp-02-05",
	"gemini-2.5-pro-exp-03-25",
	"gemini-2.5-pro",
	"gemini-2.5-flash",
	"gemini-3-flash-preview",
}

var AnthropicModels = []string{
	string(anthropic.ModelClaude_3_Opus_20240229),   // Training data cut-off: Aug 2023
	string(anthropic.ModelClaude3_7Sonnet20250219),  // Training data cut-off: Nov 2024 (knowledge cut-off date is the end of October 2024)
	string(anthropic.ModelClaudeSonnet4_5_20250929), // Training data cut-off: Jul 2025
}

var DeepseekModels = []string{
	deepseek.DeepSeekChat,     // points to DeepSeek-V3 as of Mar 2025, most likely the version 2024/12/26 (https://api-docs.deepseek.com/news/news1226)
	deepseek.DeepSeekReasoner, // points to DeepSeek-R1 as of Mar 2025, most likely the version 2025/01/20 (https://api-docs.deepseek.com/news/news250120)
}

var XaiModels = []string{
	"grok-2-1212",   // The grok-2-1212 models have a knowledge cutoff date of July 17, 2024.
	"grok-3-latest", // The grok 3 and 4 model families have a knowledge cutoff date of November, 2024
	"grok-3-mini-latest",
	"grok-code-fast-1-0825",
}

var supportedModels []string

func init() {
	supportedModels = append(supportedModels, OpenAiModels...)
	supportedModels = append(supportedModels, GoogleModels...)
	supportedModels = append(supportedModels, AnthropicModels...)
	supportedModels = append(supportedModels, DeepseekModels...)
	supportedModels = append(supportedModels, XaiModels...)
}

func SupportedModels() []string {
	return supportedModels
}

func GuessModelVendor(modelName string) int {
	modelName = strings.ToLower(strings.TrimSpace(modelName))

	switch {
	case strings.HasPrefix(modelName, "gpt"),
		strings.HasPrefix(modelName, "chatgpt"),
		strings.HasPrefix(modelName, "o1"),
		strings.HasPrefix(modelName, "o3"),
		strings.HasPrefix(modelName, "o4"),
		strings.HasPrefix(modelName, "o5"):
		return MODEL_VENDOR_OPENAI
	case strings.HasPrefix(modelName, "gemini"):
		return MODEL_VENDOR_GOOGLE
	case strings.HasPrefix(modelName, "claude"):
		return MODEL_VENDOR_ANTHROPIC
	case strings.HasPrefix(modelName, "deepseek"):
		return MODEL_VENDOR_DEEPSEEK
	case strings.HasPrefix(modelName, "grok"),
		strings.HasPrefix(modelName, "xai"):
		return MODEL_VENDOR_XAI
	default:
		return MODEL_VENDOR_UNKNOWN
	}
}

func ParseModelVendor(modelVendor string) (int, error) {
	modelVendor = strings.ToLower(strings.TrimSpace(modelVendor))
	if modelVendor == "" {
		return MODEL_VENDOR_UNKNOWN, nil
	}

	switch modelVendor {
	case "openai":
		return MODEL_VENDOR_OPENAI, nil
	case "google", "vertexai", "gemini":
		return MODEL_VENDOR_GOOGLE, nil
	case "anthropic", "claude":
		return MODEL_VENDOR_ANTHROPIC, nil
	case "deepseek":
		return MODEL_VENDOR_DEEPSEEK, nil
	case "xai", "grok":
		return MODEL_VENDOR_XAI, nil
	default:
		return MODEL_VENDOR_UNKNOWN, fmt.Errorf("unknown model vendor: %s", modelVendor)
	}
}

func ResolveModelVendor(modelName, modelVendor string) (int, error) {
	vendorType, err := ParseModelVendor(modelVendor)
	if err != nil {
		return MODEL_VENDOR_UNKNOWN, err
	}
	if vendorType != MODEL_VENDOR_UNKNOWN {
		return vendorType, nil
	}

	modelVendorType := GuessModelVendor(modelName)
	if modelVendorType == MODEL_VENDOR_UNKNOWN {
		return MODEL_VENDOR_UNKNOWN, fmt.Errorf("failed to guess vendor")
	}

	return modelVendorType, nil
}

// very quick and dirty support for model parameters
func ParseModelName(modelName string) (string, string, error) {
	modelParts := strings.SplitN(modelName, "@", 2)
	if len(modelParts) == 1 {
		return modelParts[0], "", nil
	}

	params := make(map[string]any)
	err := json.Unmarshal([]byte(modelParts[1]), &params)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse model parameters: %w", err)
	}
	bytes, _ := json.Marshal(params)
	if string(bytes) != modelParts[1] {
		return "", "", fmt.Errorf("params are not in a canonical form. Expected: %s", string(bytes))
	}

	return modelParts[0], modelParts[1], nil
}
