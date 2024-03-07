package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/gocolly/colly"
	"github.com/rs/zerolog/log"
)

const MAX_CONSECUTIVE_ERRORS = 5

func download() {
	questionSlugs, err := getQuestionSlugs()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get questions slugs")
	}

	downloadQuestions(questionSlugs, PROBLEMS_DIR)
}

func getQuestionSlugs() ([]QuestionSlug, error) {
	c := lcClient()
	resp, err := c.Get("https://leetcode.com/api/problems/algorithms/")
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
				questionId
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
				codeSnippets {
					lang
					langSlug
					code
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
		return nil, fmt.Errorf("failed marshalling GraphQL: %w", err)
	}

	return queryBytes, nil
}

// for some reason first couple of problems would fail to bypass cloudflare
func downloadQuestions(slugs []QuestionSlug, dstDir string) int {
	downloadedCnt := 0
	requestsCnt := 0
	consecutiveErrors := 0
	var exitSignal os.Signal = nil

	c := colly.NewCollector(
		colly.Async(true),
	)
	c.WithTransport(lcTransport())
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: 2,
		RandomDelay: 10 * time.Second,
	})

	signalChan := make(chan os.Signal, 1)
	go func() {
		s := <-signalChan
		log.Info().Msgf("Got %v, terminating...", s)
		exitSignal = s
	}()

	c.OnResponse(func(r *colly.Response) {
		consecutiveErrors = 0
		if exitSignal != nil {
			log.Info().Msg("Terminated by user")
			// we don't like os.Exit, but it seems that colly doesn't have a good way to stop parallel requests
			code, _ := exitSignal.(syscall.Signal)
			os.Exit(int(code))
		}
		log.Debug().Msgf("%s %s %s %d", r.Request.Method, r.Request.URL, r.Ctx.Get("dstFile"), r.StatusCode)
		log.Trace().Msg(string(r.Body))

		var problem Problem
		problem.Question.DownloadedAt = time.Now()
		err := json.Unmarshal(r.Body, &problem.Question)
		if err != nil {
			log.Err(err).Msg("Failed to unmarshall question from json")
			return
		}

		dstFile := r.Ctx.Get("dstFile")
		if dstFile == "" {
			log.Error().Msg("No context found")
			return
		}

		// we don't want interruptions while saving the data
		signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
		err = saveProblemInto(problem, dstFile)
		signal.Reset()

		if err != nil {
			log.Err(err).Msg("Failed to download question")
			return
		}
		downloadedCnt += 1
	})
	c.OnError(func(r *colly.Response, e error) {
		log.Error().Err(e).Msgf("Failed to fetch question %s", r.Request.Ctx.Get("dstFile"))
		consecutiveErrors += 1
		if consecutiveErrors >= MAX_CONSECUTIVE_ERRORS {
			log.Error().Msgf("Too many errors (%d), aborting...", consecutiveErrors)
			os.Exit(1)
		}
	})

	hdr := http.Header{
		"Content-Type": {"application/json"},
	}
	for _, qs := range slugs {
		if qs.PaidOnly {
			continue
		}
		dstFile := path.Join(dstDir, fmt.Sprintf("%d-%s.json", qs.Stat.FrontendId, qs.Stat.TitleSlug))
		ok, _ := fileExists(dstFile)
		if ok {
			log.Info().Msgf("file %s already exists", dstFile)
			continue
		}
		queryBytes, err := makeQuestionQuery(qs)
		if err != nil {
			log.Err(err).Msg("Failed to make a query")
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
