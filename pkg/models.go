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
	MODEL_FAMILY_XAI
)

var OpenAiModels = []string{
	openai.GPT4Turbo0125,           // Knowledge Cutoff: Dec 01, 2023
	openai.O120241217,              // Knowledge Cutoff: Oct 01, 2023
	openai.O3Mini20250131,          // Knowledge Cutoff: Oct 01, 2023
	openai.GPT4Dot5Preview20250227, // Knowledge Cutoff: Oct 01, 2023
}

var GoogleModels = []string{
	"gemini-1.0-pro",
	"gemini-1.5-pro-preview-0409",
	"gemini-2.0-flash-001",
	"gemini-2.0-pro-exp-02-05",
}

var AnthropicModels = []string{
	anthropic.ModelClaude3Opus20240229, // Training data cut-off: Aug 2023
	"claude-3-7-sonnet-20250219",       // Training data cut-off: Nov 2024 (knowledge cut-off date is the end of October 2024)
}

var DeepseekModels = []string{
	deepseek.DeepSeekChat, // points to DeepSeek-V3 as of Mar 2025
}

var XaiModels = []string{
	"grok-2-1212", // The grok-2-1212 models have a knowledge cutoff date of July 17, 2024.
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
	case slices.Index(XaiModels, modelName) != -1:
		return MODEL_FAMILY_XAI
	default:
		return MODEL_FAMILY_UNKNOWN
	}
}
