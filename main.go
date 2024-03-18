// inspired by https://github.com/nikhil-ravi/LeetScrape

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	fs "io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// used only to scrap question content
type QuestionSlug struct {
	Stat struct {
		FrontendId int    `json:"frontend_question_id"`
		TitleSlug  string `json:"question__title_slug"`
	}
	PaidOnly bool `json:"paid_only"`
}

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

func init() {
	// prompt
	flag.StringP("model", "m", "", "large language model family name to use")

	// general
	flag.BoolP("force", "f", false, "be forceful: download already downloaded, submit already submitted etc.")
	flag.BoolP("help", "h", false, "show this help")
	flag.StringP("dir", "d", "problems", "")
	flag.BoolP("list", "l", false, "print list of problems, but do not download")
	flag.Bool("v", false, "be verbose")
	flag.Bool("vv", false, "be very verbose (implies -v)")

	err := viper.BindPFlags(flag.CommandLine)
	if err != nil {
		os.Stderr.WriteString("failed to bind flags: " + err.Error())
	}
	flag.Parse()
}

func main() {
	if viper.GetBool("help") || flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}

	if viper.GetBool("v") {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else if viper.GetBool("vv") {
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
	consoleWriter := zerolog.NewConsoleWriter()
	consoleWriter.TimeFormat = time.DateTime
	log.Logger = zerolog.New(consoleWriter).With().Timestamp().Logger()

	viper.AddConfigPath(".")
	viper.SetConfigName("config.production")
	viper.SetConfigType("yaml")

	err := viper.ReadInConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to read config file")
	}

	command := flag.Args()[0]
	fileNames := flag.Args()[1:]

	fileNames, err = getActualFiles(fileNames)
	if err != nil {
		log.Err(err).Msg("Failed to get the files list")
		return
	}

	if command == "download" {
		download(fileNames)
	} else if command == "prompt" {
		prompt(fileNames)
	} else if command == "submit" {
		submit(fileNames)
	} else if command == "report" {
		report(fileNames)
	} else if command == "fix" {
		fix(fileNames)
	}
}

// should we use path field to save to, not a separate argument?
func saveProblemInto(p Problem, destPath string) error {
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

func readProblem(p *Problem, srcPath string) error {
	contents, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read problem from file: %w", err)
	}
	err = json.Unmarshal(contents, &p)
	if err != nil {
		return fmt.Errorf("failed to unmarshall problem from json: %w", err)
	}
	p.Path = srcPath
	if p.Solutions == nil {
		p.Solutions = map[string]Solution{}
	}
	if p.Submissions == nil {
		p.Submissions = map[string]Submission{}
	}

	return nil
}

func problemTsvHeader() []byte {
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
		"Model",
		"SolvedAt",
		"StatusMsg",
		"SubmittedAt",
	}
	var buf bytes.Buffer
	for _, c := range columns {
		buf.WriteString(c)
		buf.WriteString("\t")
	}
	buf.WriteString("\n")
	return buf.Bytes()
}

func problemToTsv(p Problem) []byte {
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
		p.Question.SnippetFeatures(),
		p.Solutions[GPT4].Model,
		humanizeTime(p.Solutions[GPT4].SolvedAt),
		p.Submissions[GPT4].CheckResponse.StatusMsg,
		humanizeTime(p.Submissions[GPT4].SubmittedAt),
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
	for _, lang := range PREFERRED_LANGUAGES {
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

func (q Question) SnippetFeatures() string {
	snippet, _ := q.FindSnippet(PREFERRED_LANGUAGES)

	// remove multiline python comments
	snippet = regexp.MustCompile(`(?ms)""".+"""`).ReplaceAllString(snippet, "")

	// more than 1 function in the code snippet to implement
	m := regexp.MustCompile(`(?m)^\s*def\s+`).FindAllString(snippet, -1)
	if len(m) > 1 {
		return "multi"
	}

	return ""
}

func fileExists(name string) (bool, error) {
	_, err := os.Stat(name)
	if err == nil {
		// file apparently exists
		return true, nil
	} else {
		// got error, let's see
		if errors.Is(err, os.ErrNotExist) {
			// file not exists, so no actual error here
			return false, nil
		} else {
			// other error
			return false, err
		}
	}
}

func getProblemsFiles() ([]string, error) {
	fsys := os.DirFS(viper.GetString("dir"))
	files, err := fs.Glob(fsys, "*.json")
	if err != nil {
		return nil, err
	}

	for i := range files {
		files[i] = path.Join(viper.GetString("dir"), files[i])
	}
	return files, nil
}

func humanizeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.DateTime)
}

func getActualFiles(files []string) ([]string, error) {
	if len(files) == 0 || files[0] != "-" {
		return files, nil
	}

	scanner := bufio.NewScanner(os.Stdin)
	files = []string{}
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if len(line) > 0 {
			files = append(files, line)
		}
	}
	err := scanner.Err()
	if err != nil {
		return nil, err
	}

	return files, nil
}
