package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// used for actual content for questions, solutions and submission results
type Problem struct {
	Question    Question
	Solutions   map[string]Solution
	Submissions map[string]Submission
	// metadata
	Path     string `json:"-"`
	Filename string `json:"-"`
}

type Question struct {
	// this is what we get from leetcode
	// structure left as is thats why tedious "Question.Data.Question"
	Data struct {
		Question struct {
			FrontendId       string `json:"questionFrontendId"`
			Id               string `json:"questionId"`
			Content          string
			SampleTestCase   string
			ExampleTestcases string
			Difficulty       string
			Title            string
			TitleSlug        string
			IsPaidOnly       bool
			Stats            string
			Likes            int
			Dislikes         int
			FreqBar          float64
			CategoryTitle    string
			TopicTags        []struct {
				Id   string
				Name string
				Slug string
			}
			CodeSnippets []struct {
				Lang     string
				LangSlug string
				Code     string
			}
			// only for premium accounts
			CompanyTagStats string
		}
	}
	// metadata. It is always recalculated on read
	DownloadedAt        time.Time
	// string as parsed from stats
	AcRate              string
	TotalSubmissions    int
	TotalAccepted       int
	// calculated from total submissions and accepted
	AcceptanceRate	    float64
	ContentFeatures     string
	CodeSnippetFeatures map[string]string
	Url                 string
}

type Solution struct {
	Lang         string
	Prompt       string
	Answer       string
	TypedCode    string
	Model        string
	Latency      time.Duration
	SolvedAt     time.Time
	PromptTokens int
	OutputTokens int
}

// this we submit to leetcode
type SubmitRequest struct {
	Lang       string `json:"lang"`
	QuestionId string `json:"question_id"`
	TypedCode  string `json:"typed_code"`
}

// this we get from leetcode
type CheckResponse struct {
	StatusCode int32  `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
	Finished   bool
	State      string
}

type Submission struct {
	SubmitRequest SubmitRequest
	SubmissionId  uint64
	CheckResponse CheckResponse
	SubmittedAt   time.Time
}

func (p Problem) MarshalJSON() ([]byte, error) {
	var jsonBytes bytes.Buffer
	enc := json.NewEncoder(&jsonBytes)
	enc.SetEscapeHTML(false)

	// we need to use alias to avoid infinite recursion
	type ProblemAlias Problem
	err := enc.Encode(ProblemAlias(p))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal problem to json: %w", err)
	}
	return jsonBytes.Bytes(), nil
}

// should we use path field to save to, not a separate argument?
func (p Problem) SaveProblemInto(destPath string) error {
	jsonBytes, err := p.MarshalJSON()
	if err != nil {
		return err
	}
	file, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer file.Close()
	n, err := file.Write(jsonBytes)
	if err != nil {
		return err
	}
	log.Debug().Msgf("Wrote %d bytes into %s", n, destPath)

	return nil
}

func (p *Problem) ReadProblem(srcPath string) error {
	contents, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read problem from file: %w", err)
	}
	err = json.Unmarshal(contents, &p)
	if err != nil {
		return fmt.Errorf("failed to unmarshal problem from json: %w", err)
	}

	// enrich problem with metadata
	err = scanAcRate(p.Question.Data.Question.Stats, &p.Question)
	if err != nil {
		return fmt.Errorf("failed to scan acRate: %w", err)
	}

	p.Question.ContentFeatures = p.Question.parseContentFeatures()
	p.Question.CodeSnippetFeatures = map[string]string{}
	for _, lang := range p.Question.Data.Question.CodeSnippets {
		p.Question.CodeSnippetFeatures[lang.LangSlug] = p.Question.SnippetFeatures([]string{lang.LangSlug})
	}
	p.Question.Url = p.Url()

	p.Path = srcPath
	p.Filename = filepath.Base(srcPath)
	if p.Solutions == nil {
		p.Solutions = map[string]Solution{}
	}
	if p.Submissions == nil {
		p.Submissions = map[string]Submission{}
	}

	return nil
}

func (p Problem) Url() string {
	return "https://leetcode.com/problems/" + p.Question.Data.Question.TitleSlug + "/"
}

func (q Question) FindSnippet(languageSlugs []string) (string, string) {
	selectedSnippet := ""
	selectedLang := ""
outerLoop:
	for _, langSlug := range languageSlugs {
		for _, snippet := range q.Data.Question.CodeSnippets {
			if snippet.LangSlug == langSlug {
				selectedSnippet = snippet.Code
				selectedLang = langSlug
				break outerLoop
			}
		}

	}

	return selectedSnippet, selectedLang
}

func (q Question) parseContentFeatures() string {
	var features []string
	// has links
	if regexp.MustCompile(`<a\s+`).MatchString(q.Data.Question.Content) {
		features = append(features, "a")
	}
	// has images
	if regexp.MustCompile(`<img\s+`).MatchString(q.Data.Question.Content) {
		features = append(features, "img")
	}

	return strings.Join(features, ",")
}

func (q Question) SnippetFeatures(languages []string) string {
	snippet, _ := q.FindSnippet(languages)

	// remove multiline python comments
	snippet = regexp.MustCompile(`(?ms)""".+"""`).ReplaceAllString(snippet, "")

	// more than 1 function in the code snippet to implement
	m := regexp.MustCompile(`(?m)^\s*def\s+`).FindAllString(snippet, -1)
	if len(m) > 1 {
		return "multi"
	}

	return ""
}

func scanAcRate(statsStr string, q *Question) error {
	var stats map[string]any
	err := json.Unmarshal([]byte(statsStr), &stats)
	if err != nil {
		return fmt.Errorf("failed to unmarshal question stats: %w", err)
	}

	acRate, ok := stats["acRate"].(string)
	if !ok {
		return errors.New("acRate is not a string")
	}
	q.AcRate = strings.TrimSuffix(acRate, "%")

	// for a unknown reason totalSubmissionRaw and totalAcceptedRaw are float64, not int
	totalSubmissions, ok := stats["totalSubmissionRaw"].(float64)
	if !ok {
		return errors.New("totalSubmissionRaw is not an string")
	}
	q.TotalSubmissions = int(totalSubmissions)
	totalAccepted, ok := stats["totalAcceptedRaw"].(float64)
	if !ok {
		return errors.New("totalAcceptedRaw is not an string")
	}
	q.TotalAccepted = int(totalAccepted)
	if totalSubmissions > 0 {
		q.AcceptanceRate = totalAccepted / totalSubmissions
	}

	return nil
}
