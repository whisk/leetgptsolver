// inspired by https://github.com/nikhil-ravi/LeetScrape

package main

import (
	"errors"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

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
	Question struct {
		Data struct {
			Question struct {
				Id            string `json:"questionFrontendId"`
				Content       string
				Difficulty    string
				Title         string
				TitleSlug     string
				IsPaidOnly    bool `json:"isPaidOnly"`
				Stats         string
				Likes         int
				Dislikes      int
				CategoryTitle string
				TopicTags     []struct {
					Name string
					Slug string
				}
				CodeSnippets []struct {
					Lang string
					LangSlug string
					Code string
				}
				// only for premium accounts
				CompanyTagStats string
			}
		}
	}
	DownloadedAt time.Time
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
	} else if os.Args[1] == "solve" {
		solve()
	}
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
