package leetgptsolver // import "github.com/whisk/leetgptsolver/pkg"

import (
	"slices"

	deepseek "github.com/cohesion-org/deepseek-go"
	"github.com/liushuangls/go-anthropic"
	"github.com/sashabaranov/go-openai"
)

const (
	MODEL_FAMILY_UNKNOWN = iota
	MODEL_FAMILY_UNSUPPORTED
	MODEL_FAMILY_OPENAI
	MODEL_FAMILY_GOOGLE
	MODEL_FAMILY_ANTHROPIC
	MODEL_FAMILY_DEEPSEEK
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

var DeepseekModels = []string{
	deepseek.DeepSeekChat,
}

var supportedModels []string

func init() {
	supportedModels = append(supportedModels, OpenAiModels...)
	supportedModels = append(supportedModels, GoogleModels...)
	supportedModels = append(supportedModels, AnthropicModels...)
	supportedModels = append(supportedModels, DeepseekModels...)
}

func SupportedModels() []string {
	return supportedModels
}

func ModelFamily(modelName string) int {
	switch {
	case slices.Index(OpenAiModels, modelName) != -1:
		return MODEL_FAMILY_OPENAI
	case slices.Index(GoogleModels, modelName) != -1:
		return MODEL_FAMILY_GOOGLE
	case slices.Index(AnthropicModels, modelName) != -1:
		return MODEL_FAMILY_ANTHROPIC
	case slices.Index(DeepseekModels, modelName) != -1:
		return MODEL_FAMILY_DEEPSEEK
	default:
		return MODEL_FAMILY_UNKNOWN
	}
}
