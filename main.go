// inspired by https://github.com/nikhil-ravi/LeetScrape

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gocolly/colly"
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

// used for actual question content
type Question struct {
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
			// only for premium accounts
			CompanyTagStats string
		}
	}
	DownloadedAt time.Time
}

func main() {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()

	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	err := viper.ReadInConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to read config file")
	}

	questionSlugs, err := getQuestionSlugs()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get questions slugs")
	}

	downloadQuestions(questionSlugs, "downloaded-questions")
}

func getQuestionSlugs() ([]QuestionSlug, error) {
	resp, err := http.Get("https://leetcode.com/api/problems/algorithms/")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	data := struct {
		StatStatusPairs []QuestionSlug `json:"stat_status_pairs"`
	}{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}

	log.Info().Msgf("Got %d question slugs", len(data.StatStatusPairs))
	return data.StatStatusPairs, nil
}

func makeQuestionQuery(q QuestionSlug) ([]byte, error) {
	query := map[string]interface{}{
		"query": `query questionContent($titleSlug: String!) 
		{
			question(titleSlug: $titleSlug) {
				questionFrontendId
				content
				mysqlSchemas
				dataSchemas
				difficulty
				title
				titleSlug			
				isPaidOnly
				stats
				likes
				dislikes
				categoryTitle
				topicTags {
					name
					slug
				}
				companyTagStats
			}
		}`,
		"variables": map[string]string{
			"titleSlug": q.Stat.TitleSlug,
		},
		"operationName": "questionContent",
	}
	queryBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling GraphQL: %w", err)
	}

	return queryBytes, nil
}

func downloadQuestions(slugs []QuestionSlug, dstDir string) int {
	downloadedCnt := 0
	requestsCnt := 0

	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/117.0.0.0 Safari/537.36"),
		colly.Async(true),
	)
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: 2,
		RandomDelay: 60 * time.Second,
	})

	c.OnResponse(func(r *colly.Response) {
		log.Debug().Msgf("%s %s %s %d", r.Request.Method, r.Request.URL, r.Ctx.Get("dstFile"), r.StatusCode)
		log.Trace().Msg(string(r.Body))

		var q Question
		err := json.Unmarshal(r.Body, &q)
		if err != nil {
			log.Error().Err(err).Msg("Failed to unmarshall question from json")
			return
		}

		dstFile := r.Ctx.Get("dstFile")
		if dstFile == "" {
			log.Error().Msg("No context found")
			return
		}
		err = downloadQuestion(q, dstFile)
		if err != nil {
			log.Error().Err(err).Msg("Failed to download question")
			return
		}
		downloadedCnt += 1
	})
	c.OnError(func(r *colly.Response, e error) {
		log.Error().Err(e).Msgf("Failed to fetch question %s", r.Request.Ctx.Get("dstFile"))
	})

	hdr := http.Header{
		"Content-Type": {"application/json"},
	}
	for _, qs := range slugs {
		if qs.PaidOnly {
			continue
		}
		dstFile := dstDir + "/" + fmt.Sprintf("%d-%s.json", qs.Stat.Id, qs.Stat.TitleSlug)
		ok, _ := fileExists(dstFile)
		if ok {
			log.Info().Msgf("file %s already exists", dstFile)
			continue
		}
		queryBytes, err := makeQuestionQuery(qs)
		if err != nil {
			log.Error().Err(err).Msg("Failed to make a query")
		}
		ctx := colly.NewContext()
		ctx.Put("dstFile", dstFile)
		c.Request(
			"POST",
			"https://leetcode.com/graphql",
			bytes.NewBuffer(queryBytes),
			ctx,
			hdr,
		)
		requestsCnt += 1
	}
	log.Debug().Msgf("%d requests queued", requestsCnt)
	c.Wait()

	return downloadedCnt
}

func downloadQuestion(q Question, destFile string) error {
	q.DownloadedAt = time.Now()

	var jsonBytes bytes.Buffer
	enc := json.NewEncoder(&jsonBytes)
	enc.SetEscapeHTML(false)
	err := enc.Encode(q)
	if err != nil {
		return fmt.Errorf("failed to marshall question to json: %w", err)
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
	log.Debug().Msgf("Wrote %d bytes", n)

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
