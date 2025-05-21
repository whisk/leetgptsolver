package leetgptsolver // import "github.com/whisk/leetgptsolver/pkg"

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	deepseek "github.com/cohesion-org/deepseek-go"
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
	"gemini-2.5-pro-exp-03-25",
}

var AnthropicModels = []string{
	anthropic.ModelClaude_3_Opus_20240229,  // Training data cut-off: Aug 2023
	anthropic.ModelClaude3_7Sonnet20250219, // Training data cut-off: Nov 2024 (knowledge cut-off date is the end of October 2024)
}

var DeepseekModels = []string{
	deepseek.DeepSeekChat, // points to DeepSeek-V3 as of Mar 2025, most likely the version 2024/12/26 (https://api-docs.deepseek.com/news/news1226)
	deepseek.DeepSeekReasoner, // points to DeepSeek-R1 as of Mar 2025, most likely the version 2025/01/20 (https://api-docs.deepseek.com/news/news250120)
}

var XaiModels = []string{
	"grok-2-1212", // The grok-2-1212 models have a knowledge cutoff date of July 17, 2024.
	"grok-3-latest", // The grok 3 model family have a knowledge cutoff date of November 17, 2024 (made available on April 18, 2025).
	"grok-3-mini-latest",

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
