// inspired by https://github.com/nikhil-ravi/LeetScrape

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gocolly/colly"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/thedevsaddam/gojsonq/v2"
)

// used only to scrap question content
type QuestionSlug struct {
	Stat struct {
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

	var slugs []QuestionSlug
	gojsonq.New().Reader(resp.Body).From("stat_status_pairs").Out(&slugs)

	log.Info().Msgf("Got %d question slugs", len(slugs))
	return slugs, nil
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

func downloadQuestions(slugs []QuestionSlug, destDir string) int {
	downloadedCnt := 0
	requestsCnt := 0

	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/117.0.0.0 Safari/537.36"),
		colly.Async(true),
	)
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: 2,
		RandomDelay: 5 * time.Second,
	})

	c.OnResponse(func(r *colly.Response) {
		log.Debug().Msgf("%s %s %d", r.Request.Method, r.Request.URL, r.StatusCode)
		log.Trace().Msg(string(r.Body))

		var q Question
		err := json.Unmarshal(r.Body, &q)
		if err != nil {
			log.Error().Err(err).Msg("Failed to unmarshall question from json")
			return
		}

		err = downloadQuestion(q, destDir)
		if err != nil {
			log.Error().Err(err).Msg("Failed to download question")
			return
		}
		downloadedCnt += 1
	})
	c.OnError(func(r *colly.Response, e error) {
		log.Error().Err(e).Msgf("Failed to fetch URL %s", r.Request.URL)
	})

	hdr := http.Header{
		"Content-Type": {"application/json"},
	}
	for _, qs := range slugs {
		if qs.PaidOnly {
			continue
		}
		queryBytes, err := makeQuestionQuery(qs)
		if err != nil {
			log.Error().Err(err).Msg("Failed to make a query")
		}
		c.Request(
			"POST",
			"https://leetcode.com/graphql",
			bytes.NewBuffer(queryBytes),
			&colly.Context{},
			hdr,
		)
		requestsCnt += 1
	}
	log.Debug().Msgf("%d requests queued", requestsCnt)
	c.Wait()

	return downloadedCnt
}

func downloadQuestion(q Question, destDir string) error {
	q.DownloadedAt = time.Now()

	var jsonBytes bytes.Buffer
	enc := json.NewEncoder(&jsonBytes)
	enc.SetEscapeHTML(false)
	err := enc.Encode(q)
	if err != nil {
		return fmt.Errorf("Failed to marshall question to json: %w", err)
	}
	filename := q.Data.Question.Id + "-" + q.Data.Question.TitleSlug + ".json"
	file, err := os.Create(destDir + "/" + filename)
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
