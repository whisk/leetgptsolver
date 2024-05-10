package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
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
	Path        string `json:"-"`
}

type Question struct {
	// this is what we get from leetcode
	// structure left as is thats why tedious "Question.Data.Question"
	Data struct {
		Question struct {
			FrontendId    string `json:"questionFrontendId"`
			Id            string `json:"questionId"`
			Content       string
			Difficulty    string
			Title         string
			TitleSlug     string
			IsPaidOnly    bool
			Stats         string
			Likes         int
			Dislikes      int
			CategoryTitle string
			TopicTags     []struct {
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
	// parts of question stats
	acRate string

	DownloadedAt time.Time
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

// should we use path field to save to, not a separate argument?
func (p Problem) SaveProblemInto(destPath string) error {
	var jsonBytes bytes.Buffer
	enc := json.NewEncoder(&jsonBytes)
	enc.SetEscapeHTML(false)
	err := enc.Encode(p)
	if err != nil {
		return fmt.Errorf("failed to marshall problem to json: %w", err)
	}
	file, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer file.Close()
	n, err := jsonBytes.WriteTo(file)
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
		return fmt.Errorf("failed to unmarshall problem from json: %w", err)
	}

	var stats map[string]any
	err = json.Unmarshal([]byte(p.Question.Data.Question.Stats), &stats)
	if err != nil {
		log.Err(err).Msg("failed to unmarshall question stats")
	}
	p.Question.acRate, _ = parseAcRate(stats["acRate"])

	p.Path = srcPath
	if p.Solutions == nil {
		p.Solutions = map[string]Solution{}
	}
	if p.Submissions == nil {
		p.Submissions = map[string]Submission{}
	}

	return nil
}

func ProblemTsvHeader(models []string) []byte {
	columns := []string{
		"Id",
		"Title",
		"Url",
		"Path",
		"IsPaidOnly",
		"Difficulty",
		"Likes",
		"Dislikes",
		"ContentFeatures",
		"SnippetFeatures",
		"acRate",
	}
	for _, m := range models {
		columns = append(columns, m+" Solved At", m+" Submitted At", m+" Result")
	}
	var buf bytes.Buffer
	for _, c := range columns {
		buf.WriteString(c)
		buf.WriteString("\t")
	}
	buf.WriteString("\n")
	return buf.Bytes()
}

func (p *Problem) ProblemToTsv(models, languages []string) []byte {
	fields := []string{
		p.Question.Data.Question.FrontendId,
		p.Question.Data.Question.Title,
		p.Url(),
		p.Path,
		fmt.Sprintf("%v", p.Question.Data.Question.IsPaidOnly),
		p.Question.Data.Question.Difficulty,
		fmt.Sprintf("%d", p.Question.Data.Question.Likes),
		fmt.Sprintf("%d", p.Question.Data.Question.Dislikes),
		p.Question.ContentFeatures(),
		p.Question.SnippetFeatures(languages),
		p.Question.AcRate(),
	}
	for _, m := range models {
		if solv, ok := p.Solutions[m]; ok {
			fields = append(fields, humanizeTime(solv.SolvedAt))
		} else {
			fields = append(fields, "")
		}
		if subm, ok := p.Submissions[m]; ok {
			fields = append(fields, humanizeTime(subm.SubmittedAt), subm.CheckResponse.StatusMsg)
		} else {
			fields = append(fields, "", "")
		}
	}

	var buf bytes.Buffer
	for _, f := range fields {
		buf.WriteString(f)
		buf.WriteString("\t")
	}
	buf.WriteString("\n")
	return buf.Bytes()
}

func (p Problem) Url() string {
	return "https://leetcode.com/problems/" + p.Question.Data.Question.TitleSlug + "/"
}

func (q Question) FindSnippet(languages []string) (string, string) {
	selectedSnippet := ""
	selectedLang := ""
outerLoop:
	for _, lang := range languages {
		for _, snippet := range q.Data.Question.CodeSnippets {
			if snippet.LangSlug == lang {
				selectedSnippet = snippet.Code
				selectedLang = lang
				break outerLoop
			}
		}

	}

	return selectedSnippet, selectedLang
}

func (q Question) ContentFeatures() string {
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

func (q Question) AcRate() string {
	res, _ := parseAcRate(q.acRate)
	return res
}
