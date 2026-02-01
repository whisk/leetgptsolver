package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	leetgptsolver "whisk/leetgptsolver/pkg"
	"whisk/leetgptsolver/pkg/throttler"

	"cloud.google.com/go/vertexai/genai"
	"github.com/anthropics/anthropic-sdk-go"
	anthropic_option "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/cohesion-org/deepseek-go"
	"github.com/microcosm-cc/bluemonday"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
	"google.golang.org/api/option"
)

var promptThrottler throttler.Throttler

func prompt(args []string, lang, modelName string) {
	files, err := filenamesFromArgs(args)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get files")
		return
	}

	promptThrottler = throttler.NewSimpleThrottler(1*time.Second, 30*time.Second)

	if modelName == "" {
		log.Error().Msg("Model is not set")
		return
	}
	modelId, modelParams, err := leetgptsolver.ParseModelName(modelName)
	if err != nil {
		log.Err(err).Msg("failed to parse model")
		return
	}

	var prompter func(Question, string, string, string) (*Solution, error)
	switch leetgptsolver.ModelFamily(modelId) {
	case leetgptsolver.MODEL_FAMILY_OPENAI:
		prompter = promptOpenAi
	case leetgptsolver.MODEL_FAMILY_GOOGLE:
		prompter = promptGoogle
	case leetgptsolver.MODEL_FAMILY_ANTHROPIC:
		prompter = promptAnthropic
	case leetgptsolver.MODEL_FAMILY_DEEPSEEK:
		prompter = promptDeepseek
	case leetgptsolver.MODEL_FAMILY_XAI:
		prompter = promptXai
	default:
		log.Error().Msgf("No prompter found for model %s", modelId)
		return
	}

	log.Info().Msgf("Prompting %d solutions...", len(files))
	solvedCnt := 0
	skippedCnt := 0
	errorsCnt := 0
outerLoop:
	for i, file := range files {
		log.Info().Msgf("[%d/%d] Prompting %s for problem %s ...", i+1, len(files), modelName, file)

		var problem Problem
		err := problem.ReadProblem(file)
		if err != nil {
			errorsCnt += 1
			log.Err(err).Msg("Failed to read the problem")
			continue
		}
		if solved, ok := problem.GetSolution(modelName, lang); ok && !options.Force {
			skippedCnt += 1
			log.Info().Msgf("Already solved at %s", solved.SolvedAt.String())
			continue
		}

		var solution *Solution
		maxReties := options.Retries
		i := 0
		promptThrottler.Ready()
		for promptThrottler.Wait() && i < maxReties {
			i += 1
			solution, err = prompter(problem.Question, lang, modelId, modelParams)
			promptThrottler.Touch()
			if err != nil {
				log.Err(err).Msg("Failed to get a solution")
				promptThrottler.Slowdown()
				if _, ok := err.(FatalError); ok {
					log.Error().Msg("Aborting...")
					errorsCnt += 1
					break outerLoop
				}
				if _, ok := err.(NonRetriableError); ok {
					errorsCnt += 1
					continue outerLoop
				}
				// do not retry on this kind of timeout. It usually means the problem takes too much time to solve,
				// and retrying will not help
				if errors.Is(err, context.DeadlineExceeded) {
					errorsCnt += 1
					continue outerLoop
				}
				continue
			}

			break // success
		}

		if solution == nil {
			// did not get a solution after retries
			errorsCnt += 1
			continue
		}

		log.Info().Msgf("Got %d line(s) of code in %0.1f second(s)", strings.Count(solution.TypedCode, "\n"), solution.Latency.Seconds())
		if problem.SolutionsV2 == nil {
			problem.SolutionsV2 = map[string]map[string]Solution{}
		}
		if _, ok := problem.SolutionsV2[modelName]; !ok {
			problem.SolutionsV2[modelName] = map[string]Solution{}
		}
		problem.SolutionsV2[modelName][lang] = *solution
		if problem.SubmissionsV2 == nil {
			problem.SubmissionsV2 = map[string]map[string]Submission{}
		}
		if _, ok := problem.SubmissionsV2[modelName]; !ok {
			problem.SubmissionsV2[modelName] = map[string]Submission{}
		}
		problem.SubmissionsV2[modelName][lang] = Submission{} // new solutions clears old submissions
		err = problem.SaveProblemInto(file)
		if err != nil {
			errorsCnt += 1
			log.Err(err).Msg("Failed to save the solution")
			continue
		}

		solvedCnt += 1
	}
	log.Info().Msgf("Files processed: %d", len(files))
	log.Info().Msgf("Skipped problems: %d", skippedCnt)
	log.Info().Msgf("Problems solved successfully: %d", solvedCnt)
	log.Info().Msgf("Errors: %d", errorsCnt)
}

func promptOpenAi(q Question, lang, modelName, params string) (*Solution, error) {
	client := openai.NewClient(options.ChatgptApiKey)
	lang, prompt, err := generatePrompt(q, lang)
	if err != nil {
		return nil, NewFatalError(fmt.Errorf("failed to make prompt: %w", err))
	}
	log.Debug().Msgf("Generated %d line(s) of code prompt", strings.Count(prompt, "\n"))
	log.Trace().Msgf("Generated prompt:\n%s", prompt)

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
		return nil, NewNonRetriableError(errors.New("no choices in response"))
	}
	answer := resp.Choices[0].Message.Content
	log.Trace().Msgf("Got answer:\n%s", answer)
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

func promptDeepseek(q Question, lang, modelName, params string) (*Solution, error) {
	client := deepseek.NewClient(options.DeepseekApiKey)
	lang, prompt, err := generatePrompt(q, lang)
	if err != nil {
		return nil, NewFatalError(fmt.Errorf("failed to make prompt: %w", err))
	}
	log.Debug().Msgf("Generated %d line(s) of code prompt", strings.Count(prompt, "\n"))
	log.Trace().Msgf("Generated prompt:\n%s", prompt)

	t0 := time.Now()
	client.Timeout = 15 * time.Minute
	resp, err := client.CreateChatCompletion(
		context.Background(),
		&deepseek.ChatCompletionRequest{
			Model: modelName,
			Messages: []deepseek.ChatCompletionMessage{
				{
					Role:    deepseek.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			Temperature: 0.0,
		},
	)
	latency := time.Since(t0)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, NewNonRetriableError(errors.New("no choices in response"))
	}
	answer := resp.Choices[0].Message.Content
	log.Trace().Msgf("Got answer:\n%s", answer)
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

// very dirty
func promptXai(q Question, lang, modelName, params string) (*Solution, error) {
	config := openai.DefaultConfig(options.XaiApiKey)
	config.BaseURL = "https://api.x.ai/v1"
	client := openai.NewClientWithConfig(config)

	lang, prompt, err := generatePrompt(q, lang)
	if err != nil {
		return nil, NewFatalError(fmt.Errorf("failed to make prompt: %w", err))
	}
	log.Debug().Msgf("Generated %d line(s) of code prompt", strings.Count(prompt, "\n"))
	log.Trace().Msgf("Generated prompt:\n%s", prompt)

	var customParams struct {
		ReasoningEffort string `json:"reasoning_effort"`
	}
	if params != "" {
		err = json.Unmarshal([]byte(params), &customParams)
		if err != nil {
			return nil, NewFatalError(fmt.Errorf("failed to parse custom params: %w", err))
		}
		log.Debug().Msgf("using custom params: %+v", customParams)
	}

	seed := int(42)
	completionRequest := openai.ChatCompletionRequest{
		Model: modelName,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		Seed: &seed,
	}
	if customParams.ReasoningEffort != "" {
		completionRequest.ReasoningEffort = customParams.ReasoningEffort
	}

	t0 := time.Now()
	resp, err := client.CreateChatCompletion(
		context.Background(),
		completionRequest,
	)
	latency := time.Since(t0)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, NewNonRetriableError(errors.New("no choices in response"))
	}
	answer := resp.Choices[0].Message.Content
	log.Trace().Msgf("Got answer:\n%s", answer)
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

func promptGoogle(q Question, lang, modelName, params string) (*Solution, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Error().Msgf("recovered: %v", err)
		}
	}()

	lang, prompt, err := generatePrompt(q, lang)
	if err != nil {
		return nil, NewFatalError(fmt.Errorf("failed to make prompt: %w", err))
	}
	log.Debug().Msgf("Generated %d line(s) of code prompt", strings.Count(prompt, "\n"))
	log.Trace().Msgf("Generated prompt:\n%s", prompt)

	projectID := options.GeminiProjectId
	region := options.GeminiRegion

	ctx := context.Background()
	opts := option.WithCredentialsFile(options.GeminiCredentialsFile)
	client, err := genai.NewClient(ctx, projectID, region, opts)
	if err != nil {
		return nil, NewFatalError(fmt.Errorf("failed to create a client: %w", err))
	}
	defer client.Close()

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
		return nil, fmt.Errorf("failed to send a message: %w", err)
	}
	answer, err := geminiAnswer(resp)
	latency := time.Since(t0)
	if err != nil {
		return nil, err
	}

	log.Trace().Msgf("Got answer:\n%s", answer)
	return &Solution{
		Lang:         lang,
		Prompt:       prompt,
		Answer:       answer,
		TypedCode:    extractCode(answer),
		Model:        gemini.Name(),
		SolvedAt:     time.Now(),
		Latency:      latency,
		PromptTokens: int(resp.UsageMetadata.PromptTokenCount),
		OutputTokens: int(resp.UsageMetadata.CandidatesTokenCount),
	}, nil
}

func promptAnthropic(q Question, lang, modelName, params string) (*Solution, error) {
	client := anthropic.NewClient(anthropic_option.WithAPIKey(options.ClaudeApiKey))
	lang, prompt, err := generatePrompt(q, lang)
	if err != nil {
		return nil, NewFatalError(fmt.Errorf("failed to make prompt: %w", err))
	}
	log.Debug().Msgf("Generated %d line(s) of code prompt", strings.Count(prompt, "\n"))
	log.Trace().Msgf("Generated prompt:\n%s", prompt)

	var customParams struct {
		MaxTokens int `json:"max_tokens"`
		Thinking  struct {
			Type         string `json:"type"`
			BudgetTokens int    `json:"budget_tokens"`
		} `json:"thinking"`
	}
	if params != "" {
		err = json.Unmarshal([]byte(params), &customParams)
		if err != nil {
			return nil, NewFatalError(fmt.Errorf("failed to parse custom params: %w", err))
		}
		log.Debug().Msgf("using custom params: %+v", customParams)
	}

	messageParams := anthropic.MessageNewParams{
		Model:       anthropic.Model(modelName),
		Temperature: anthropic.Float(0.0),
		Messages:    []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))},
		MaxTokens:   4096,
	}
	if customParams.MaxTokens > 0 {
		messageParams.MaxTokens = int64(customParams.MaxTokens)
	}
	if customParams.Thinking.Type == "enabled" {
		budgetTokens := int64(0)
		if customParams.Thinking.BudgetTokens > 0 {
			budgetTokens = int64(customParams.Thinking.BudgetTokens)
		}
		messageParams.Thinking = anthropic.ThinkingConfigParamOfEnabled(budgetTokens)
		messageParams.Temperature = anthropic.Float(1.0)
	}

	t0 := time.Now()
	resp, err := client.Messages.New(context.Background(), messageParams)
	latency := time.Since(t0)
	if err != nil {
		return nil, fmt.Errorf("failed to send a message: %w", err)
	}

	log.Trace().Msgf("Got response:\n%+v", resp.Content)
	answer := ""
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			answer += block.Text + "\n"
		case "thinking":
			// Skip thinking blocks for the final answer
			continue
		}
	}

	return &Solution{
		Lang:         lang,
		Prompt:       prompt,
		Answer:       answer,
		TypedCode:    extractCode(answer),
		Model:        modelName,
		SolvedAt:     time.Now(),
		Latency:      latency,
		PromptTokens: int(resp.Usage.InputTokens),
		OutputTokens: int(resp.Usage.OutputTokens),
	}, nil

}

// very hackish
func geminiAnswer(r *genai.GenerateContentResponse) (string, error) {
	var parts []string
	if len(r.Candidates) == 0 {
		return "", NewNonRetriableError(errors.New("no candidates in response"))
	}
	if len(r.Candidates[0].Content.Parts) == 0 && r.Candidates[0].FinishReason == genai.FinishReasonRecitation {
		return "", NewNonRetriableError(errors.New("got FinishReasonRecitation in response"))
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

func generatePrompt(q Question, lang string) (string, string, error) {
	prompt := options.PromptTemplate
	if prompt == "" {
		return "", "", errors.New("prompt_template is not set")
	}

	selectedLang := lang
	selectedSnippet := q.FindSnippet(selectedLang)
	if selectedSnippet == "" {
		return "", "", fmt.Errorf("failed to find code snippet for %s", selectedLang)
	}
	question := htmlToPlaintext(q.Data.Question.Content)
	if replaceInplace(&prompt, "{language}", selectedLang) == 0 {
		return "", "", errors.New("no {language} in prompt_template")
	}
	if replaceInplace(&prompt, "{question}", question) == 0 {
		return "", "", errors.New("no {question} in prompt_template")
	}
	if replaceInplace(&prompt, "{snippet}", selectedSnippet) == 0 {
		return "", "", errors.New("no {snippet} in prompt_template")
	}

	return selectedLang, prompt, nil
}

func replaceInplace(s *string, old, new string) int {
	cnt := strings.Count(*s, old)
	*s = strings.ReplaceAll(*s, old, new)
	return cnt
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
