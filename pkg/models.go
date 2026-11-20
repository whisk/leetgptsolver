package leetgptsolver // import "github.com/whisk/leetgptsolver/pkg"

import (
	"encoding/json"
	"fmt"
	"strings"
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

func SupportedVendors() []int {
	return []int{
		MODEL_VENDOR_OPENAI,
		MODEL_VENDOR_GOOGLE,
		MODEL_VENDOR_ANTHROPIC,
		MODEL_VENDOR_DEEPSEEK,
		MODEL_VENDOR_XAI,
	}
}

func VendorName(vendor int) string {
	switch vendor {
	case MODEL_VENDOR_OPENAI:
		return "openai"
	case MODEL_VENDOR_GOOGLE:
		return "google"
	case MODEL_VENDOR_ANTHROPIC:
		return "anthropic"
	case MODEL_VENDOR_DEEPSEEK:
		return "deepseek"
	case MODEL_VENDOR_XAI:
		return "xai"
	default:
		return "unknown"
	}
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
