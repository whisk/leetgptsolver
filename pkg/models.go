package leetgptsolver // import "github.com/whisk/leetgptsolver/pkg"

import (
	"strings"

	"github.com/liushuangls/go-anthropic"
	"github.com/sashabaranov/go-openai"
)

const (
	MODEL_FAMILY_UNKNOWN = iota
	MODEL_FAMILY_UNSUPPORTED
	MODEL_FAMILY_GPT
	MODEL_FAMILY_GEMINI
	MODEL_FAMILY_CLAUDE
)

var SupportedModels = []string{
	openai.GPT4Turbo0125,
	"gemini-1.0-pro",
	"gemini-1.5-pro-preview-0409",
	anthropic.ModelClaude3Opus20240229,
}

func ModelFamily(modelName string) int {
	switch {
	case strings.HasPrefix(modelName, "gpt-"):
		return MODEL_FAMILY_GPT
	case strings.HasPrefix(modelName, "gemini-"):
		return MODEL_FAMILY_GEMINI
	case strings.HasPrefix(modelName, "claude-"):
		return MODEL_FAMILY_CLAUDE
	default:
		return MODEL_FAMILY_UNKNOWN
	}
}
