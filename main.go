// inspired by https://github.com/nikhil-ravi/LeetScrape

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	fs "io/fs"
	"os"
	"path"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const PROBLEMS_DIR = "problems/"
const HTTP_USER_AGENT = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/117.0.0.0 Safari/537.36"

// used only to scrap question content
type QuestionSlug struct {
	Stat struct {
		Id        int    `json:"frontend_question_id"`
		TitleSlug string `json:"question__title_slug"`
	}
	PaidOnly bool `json:"paid_only"`
}

// used for actual content for questions, solutions and submission results
type Problem struct {
	// this is what we get from leetcode
	// structure left as is thats why tedious "Question.Data.Question"
	Question    Question
	Solutions   []Solution
	Submissions []Submission
}

type Question struct {
	Data struct {
		Question struct {
			Id            string `json:"questionFrontendId"`
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
	Lang      string
	Prompt    string
	Answer    string
	TypedCode string
	SolvedAt  time.Time
	Model     string
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
	SubmittedAt   time.Time
	SubmissionId  uint64
	CheckResponse CheckResponse
}

func main() {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
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

	if len(os.Args) == 1 || os.Args[1] == "download" {
		download()
	} else if os.Args[1] == "prompt" {
		prompt(os.Args[2:])
	} else if os.Args[1] == "submit" {
		submit(os.Args[2:])
	} else if os.Args[1] == "fix" {
		fix(os.Args[2:])
	}
}

func saveProblem(p Problem, destFile string) error {
	var jsonBytes bytes.Buffer
	enc := json.NewEncoder(&jsonBytes)
	enc.SetEscapeHTML(false)
	err := enc.Encode(p)
	if err != nil {
		return fmt.Errorf("failed to marshall problem to json: %w", err)
	}
	file, err := os.Create(destFile)
	if err != nil {
		return err
	}
	defer file.Close()
	n, err := jsonBytes.WriteTo(file)
	if err != nil {
		return err
	}
	log.Debug().Msgf("Wrote %d bytes into %s", n, destFile)

	return nil
}

func readProblem(p *Problem, srcFile string) error {
	contents, err := os.ReadFile(srcFile)
	if err != nil {
		return fmt.Errorf("failed to read problem from file: %w", err)
	}
	err = json.Unmarshal(contents, &p)
	if err != nil {
		return fmt.Errorf("failed to unmarshall problem from json: %w", err)
	}
	return nil
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
	fsys := os.DirFS(PROBLEMS_DIR)
	files, err := fs.Glob(fsys, "*.json")
	if err != nil {
		return nil, err
	}

	for i := range files {
		files[i] = path.Join(PROBLEMS_DIR, files[i])
	}
	return files, nil
}
