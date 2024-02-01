package main

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
	"github.com/spf13/viper"
)

var PREFERRED_LANGUAGES = []string{"python3", "python"}

func prompt(files []string) {
	for _, file := range files {
		log.Info().Msgf("Prompting for solution for problem %s ...", file)

		var problem Problem
		err := readProblem(&problem, file)
		if err != nil {
			log.Err(err).Msg("Failed to read the problem")
			continue
		}
		solution, err := promptChatGPT(problem.Question)
		if err != nil {
			log.Err(err).Msg("Failed to get a solution")
		}
		log.Info().Msgf("Got %d line(s) of solution", strings.Count(solution.TypedCode, "\n"))

		problem.Solution = *solution
		err = saveProblem(problem, file)
		if err != nil {
			log.Err(err).Msg("Failed to save the solution")
			continue
		}
	}
}

func promptChatGPT(q Question) (*Solution, error) {
	client := openai.NewClient(viper.GetString("chatgpt_api_key"))
	lang, prompt, err := makePrompt(q)
	if err != nil {
		return nil, err
	}

	log.Debug().Msgf("Generated prompt:\n%s", prompt)

	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
		},
	)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, errors.New("empty response")
	}
	answer := resp.Choices[0].Message.Content
	log.Debug().Msgf("Got answer:\n%s", answer)
	return &Solution{
		Lang:      lang,
		Prompt:    prompt,
		Answer:    answer,
		TypedCode: extractCode(answer),
		Model:     resp.Model,
		SolvedAt:  time.Now(),
	}, nil
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
