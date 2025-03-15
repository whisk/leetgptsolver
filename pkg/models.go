package leetgptsolver // import "github.com/whisk/leetgptsolver/pkg"

import (
	"slices"

	"github.com/liushuangls/go-anthropic"
	"github.com/sashabaranov/go-openai"
)

const (
	MODEL_FAMILY_UNKNOWN = iota
	MODEL_FAMILY_UNSUPPORTED
	MODEL_FAMILY_OPENAI
	MODEL_FAMILY_GOOGLE
	MODEL_FAMILY_ANTHROPIC
)

var OpenAiModels = []string{
	openai.GPT4Turbo0125,
	openai.O120241217,
	openai.O3Mini20250131,
}

var GoogleModels = []string{
	"gemini-1.0-pro",
	"gemini-1.5-pro-preview-0409",
	"gemini-2.0-flash-001",
}

var AnthropicModels = []string{
	anthropic.ModelClaude3Opus20240229,
	"claude-3-7-sonnet-20250219",
}

func ModelFamily(modelName string) int {
	switch {
	case slices.Index(OpenAiModels, modelName) != -1:
		return MODEL_FAMILY_OPENAI
	case slices.Index(GoogleModels, modelName) != -1:
		return MODEL_FAMILY_GOOGLE
	case slices.Index(AnthropicModels, modelName) != -1:
		return MODEL_FAMILY_ANTHROPIC
	default:
		return MODEL_FAMILY_UNKNOWN
	}
}

func SupportedModels() []string {
	return append(append(OpenAiModels[:0:0], OpenAiModels...), append(GoogleModels[:0:0], GoogleModels...)...)
}
