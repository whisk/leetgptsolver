package main

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	leetgptsolver "whisk/leetgptsolver/pkg"
	"whisk/leetgptsolver/pkg/throttler"

	"cloud.google.com/go/vertexai/genai"
	"github.com/liushuangls/go-anthropic"
	"github.com/microcosm-cc/bluemonday"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
	"github.com/spf13/viper"
	"google.golang.org/api/option"
)

var (
	errEmptyAnswer = errors.New("empty answer")
)

var promptThrottler throttler.Throttler

func prompt(files []string) {
	promptThrottler = throttler.NewThrottler(2 * time.Second, 30 * time.Second)

	modelName := viper.GetString("model")
	var prompter func(Question, string) (*Solution, error)
	switch leetgptsolver.ModelFamily(modelName) {
	case leetgptsolver.MODEL_FAMILY_GPT:
		prompter = promptChatGPT
	case leetgptsolver.MODEL_FAMILY_GEMINI:
		prompter = promptGemini
	case leetgptsolver.MODEL_FAMILY_CLAUDE:
		prompter = promptClaude
	default:
		log.Error().Msgf("Unknown LLM %s", modelName)
		return
	}

	log.Info().Msgf("Prompting %d solutions...", len(files))
	respCnt := 0
	for i, file := range files {
		log.Info().Msgf("[%d/%d] Prompting %s for solution for problem %s ...", i+1, len(files), modelName, file)

		var problem Problem
		err := problem.ReadProblem(file)
		if err != nil {
			log.Err(err).Msg("Failed to read the problem")
			continue
		}
		if _, ok := problem.Solutions[modelName]; ok && !viper.GetBool("force") {
			log.Info().Msg("Already prompted")
			continue
		}

		for promptThrottler.Wait() {
			var solution *Solution
			solution, err := prompter(problem.Question, modelName)
			if err != nil {
				log.Err(err).Msg("Failed to get a solution")
				if err == errEmptyAnswer {
					promptThrottler.Complete()
					continue
				}
				promptThrottler.Slower()
				continue
			}
			if solution == nil {
				log.Error().Msg("Got nil solution. Probably something bad happened, skipping this problem")
				promptThrottler.Complete()
				continue
			}
			log.Info().Msgf("Got %d line(s) of solution", strings.Count(solution.TypedCode, "\n"))

			problem.Solutions[modelName] = *solution
			problem.Submissions[modelName] = Submission{} // new solutions clears old submissions
			err = problem.SaveProblemInto(file)
			if err != nil {
				log.Err(err).Msg("Failed to save the solution")
				promptThrottler.Again()
				continue
			}
			respCnt += 1
			promptThrottler.Complete()
		}
		if err := promptThrottler.Error(); err != nil {
			log.Err(err).Msgf("throttler error")
		}
	}
	log.Info().Msgf("Got solutions for %d/%d problems", respCnt, len(files))
}

func promptChatGPT(q Question, modelName string) (*Solution, error) {
	client := openai.NewClient(viper.GetString("chatgpt_api_key"))
	lang, prompt, err := makePrompt(q)
	if err != nil {
		return nil, err
	}

	log.Debug().Msgf("Generated prompt:\n%s", prompt)

	seed := int(42)
	t0 := time.Now()
	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: modelName,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			Seed: &seed,
		},
	)
	latency := time.Since(t0)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, errEmptyAnswer
	}
	answer := resp.Choices[0].Message.Content
	log.Debug().Msgf("Got answer:\n%s", answer)
	return &Solution{
		Lang:         lang,
		Prompt:       prompt,
		Answer:       answer,
		TypedCode:    extractCode(answer),
		Model:        resp.Model,
		SolvedAt:     time.Now(),
		Latency:      latency,
		PromptTokens: resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}, nil
}

func promptGemini(q Question, modelName string) (*Solution, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Error().Msgf("recovered: %v", err)
		}
	}()

	projectID := viper.GetString("gemini_project_id")
	region := viper.GetString("gemini_region")

	ctx := context.Background()
	opts := option.WithCredentialsFile(viper.GetString("gemini_credentials_file"))
	client, err := genai.NewClient(ctx, projectID, region, opts)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	lang, prompt, err := makePrompt(q)
	if err != nil {
		return nil, err
	}
	log.Debug().Msgf("Generated prompt:\n%s", prompt)

	gemini := client.GenerativeModel(modelName)
	temp := float32(0.0)
	gemini.GenerationConfig.Temperature = &temp
	chat := gemini.StartChat()
	if chat == nil {
		return nil, errors.New("failed to start a chat")
	}

	t0 := time.Now()
	resp, err := chat.SendMessage(ctx, genai.Text(prompt))
	if err != nil {
		return nil, err
	}
	answer, err := geminiAnswer(resp)
	latency := time.Since(t0)
	if err != nil {
		return nil, err
	}

	log.Debug().Msgf("Got answer:\n%s", answer)
	return &Solution{
		Lang:      lang,
		Prompt:    prompt,
		Answer:    answer,
		TypedCode: extractCode(answer),
		Model:     gemini.Name(),
		SolvedAt:  time.Now(),
		Latency:   latency,
		PromptTokens: int(resp.UsageMetadata.PromptTokenCount),
		OutputTokens: int(resp.UsageMetadata.CandidatesTokenCount),
	}, nil
}

func promptClaude(q Question, modelName string) (*Solution, error) {
	client := anthropic.NewClient(viper.GetString("claude_api_key"))
	lang, prompt, err := makePrompt(q)
	if err != nil {
		return nil, err
	}
	log.Debug().Msgf("Generated prompt:\n%s", prompt)

	temp := float32(0.0)
	t0 := time.Now()
	resp, err := client.CreateMessages(context.Background(), anthropic.MessagesRequest{
		Model:       modelName,
		Temperature: &temp,
		Messages: []anthropic.Message{
			anthropic.NewUserTextMessage(prompt),
		},
		MaxTokens: 4096,
	})
	latency := time.Since(t0)
	if err != nil {
		var e *anthropic.APIError
		if errors.As(err, &e) {
			log.Err(err).Msgf("Messages error, type: %s, message: %s", e.Type, e.Message)
		} else {
			log.Err(err).Msgf("Messages error: %v\n", err)
		}
		return nil, err
	}

	answer := resp.Content[0].Text

	log.Debug().Msgf("Got answer:\n%s", answer)
	return &Solution{
		Lang:      lang,
		Prompt:    prompt,
		Answer:    answer,
		TypedCode: extractCode(answer),
		Model:     modelName,
		SolvedAt:  time.Now(),
		Latency: latency,
		PromptTokens: resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
	}, nil

}

// very hackish
func geminiAnswer(r *genai.GenerateContentResponse) (string, error) {
	var parts []string
	if len(r.Candidates) == 0 {
		return "", errEmptyAnswer
	}
	if len(r.Candidates[0].Content.Parts) == 0 && r.Candidates[0].FinishReason == genai.FinishReasonRecitation {
		return "", errEmptyAnswer
	}
	buf, err := json.Marshal(r.Candidates[0].Content.Parts)
	if err != nil {
		return "", err
	}
	err = json.Unmarshal(buf, &parts)
	if err != nil {
		return "", err
	}
	return strings.Join(parts, ""), nil
}

func makePrompt(q Question) (string, string, error) {
	selectedSnippet, selectedLang := q.FindSnippet(PREFERRED_LANGUAGES)

	if selectedSnippet == "" {
		return "", "", errors.New("failed to find code snippet")
	}

	question := htmlToPlaintext(q.Data.Question.Content)

	prompt := "Hi, this is a coding interview. I will give you a problem statement with sample test cases and a code snippet. " +
		"I expect you to write the most effective working code using " + selectedLang + " programming language. " +
		"Here is the problem statement: \n" +
		question + "\n\n" +
		"Your code should solve the given problem fully and correctly.\n" +
		"Here is the code snippet, you should expand it with your code: \n" +
		selectedSnippet + "\n\n" +
		"Please do not alter function signature(s) in the code snippet. " +
		"Please output only valid source code which could be run as-is without any fixes, improvements or changes. " +
		"Good luck!"

	return selectedLang, prompt, nil
}

func htmlToPlaintext(s string) string {
	// add newlines where necessary
	s = strings.ReplaceAll(s, "<br>", "<br>\n")
	s = strings.ReplaceAll(s, "<br/>", "<br/>\n")
	s = strings.ReplaceAll(s, "</p>", "</p>\n")

	// handle superscript <sup>...</sup>
	s = regexp.MustCompile(`\<sup\>(.*?)\<\/sup\>`).ReplaceAllString(s, "^$1")

	// strip html tags
	p := bluemonday.StrictPolicy()
	s = p.Sanitize(s)

	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#34;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&amp;", "&")

	// collapse multiple newlines
	s = regexp.MustCompile(`\s+$`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\n+`).ReplaceAllString(s, "\n")

	return s
}

func extractCode(answer string) string {
	re := regexp.MustCompile("(?ms)^```\\w*\\s*$(.+?)^```\\s*$")
	m := re.FindStringSubmatch(answer)
	if m == nil {
		// maybe answer is the code itself?
		return answer
	}
	return m[1]
}
