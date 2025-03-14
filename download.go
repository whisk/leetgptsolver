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
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gocolly/colly"
	"github.com/rs/zerolog/log"
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

const MAX_CONSECUTIVE_ERRORS = 5

func download(args []string) {
	files, err := filenamesFromArgs(args)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get files")
		return
	}

	questionSlugs, err := getQuestionSlugs()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get questions slugs")
		return
	}
	log.Info().Msgf("Got %d question slugs", len(questionSlugs))

	if viper.GetBool("list") {
		printQuestions(questionSlugs)
	} else if len(files) == 0 {
		downloadQuestions(questionSlugs, viper.GetString("dir"))
		return
	}

	// filter slugs to download
	slugsMap := map[string]QuestionSlug{}
	for _, qs := range questionSlugs {
		slugsMap[qs.Stat.TitleSlug] = qs
	}
	slugsToDownload := []QuestionSlug{}

	log.Info().Msgf("Searching for %d questions...", len(files))
	for _, file := range files {
		// TODO: very awkward, improve
		title := filepath.Base(file)
		title = strings.TrimSuffix(title, filepath.Ext(file))

		if qs, ok := slugsMap[title]; ok {
			slugsToDownload = append(slugsToDownload, qs)
		} else {
			log.Error().Msgf("Unknown problem %s, skipping", title)
		}
	}
	downloadQuestions(slugsToDownload, viper.GetString("dir"))
}

func getQuestionSlugs() ([]QuestionSlug, error) {
	c := client()
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

	return data.StatStatusPairs, nil
}

func printQuestions(slugs []QuestionSlug) {
	for _, slug := range slugs {
		fmt.Printf("%d\t%v\t%s\n", slug.Stat.FrontendId, slug.PaidOnly, slug.Stat.TitleSlug)
	}
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

// for some reason first couple of problems may fail to bypass cloudflare
func downloadQuestions(slugs []QuestionSlug, dstDir string) int {
	log.Debug().Msgf("Queueing %d questions...", len(slugs))

	downloadedCnt := 0
	alreadyDownloadedCnt := 0
	skippedCnt := 0
	queuedCnt := 0
	errorsCnt := 0
	consecutiveErrorsCnt := 0
	var exitSignal os.Signal = nil

	c := colly.NewCollector(
		colly.Async(true),
	)
	c.WithTransport(newTransport())
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: 2,
		RandomDelay: 15 * time.Second,
	})

	signalChan := make(chan os.Signal, 1)
	go func() {
		s := <-signalChan
		log.Info().Msgf("Got %v, terminating", s)
		exitSignal = s
	}()

	c.OnResponse(func(r *colly.Response) {
		consecutiveErrorsCnt = 0
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
		err = problem.SaveProblemInto(dstFile)
		signal.Reset()

		if err != nil {
			log.Err(err).Msg("Failed to download question")
			return
		}
		log.Debug().Msgf("Downloaded %s successfully", dstFile)
		downloadedCnt += 1
	})
	c.OnError(func(r *colly.Response, e error) {
		log.Error().Err(e).Msgf("Failed to fetch question %s", r.Request.Ctx.Get("dstFile"))
		consecutiveErrorsCnt += 1
		errorsCnt += 1
		if consecutiveErrorsCnt >= MAX_CONSECUTIVE_ERRORS {
			log.Error().Msgf("Too many errors (%d), aborting...", consecutiveErrorsCnt)
			os.Exit(1)
		}
	})

	hdr := http.Header{
		"Content-Type": {"application/json"},
	}
	for _, qs := range slugs {
		if qs.PaidOnly {
			skippedCnt += 1
			continue
		}
		dstFile := path.Join(dstDir, qs.Stat.TitleSlug+".json")
		ok, _ := fileExists(dstFile)
		if ok {
			log.Debug().Msgf("file %s already exists", dstFile)
			alreadyDownloadedCnt += 1
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
		queuedCnt += 1
	}
	log.Info().Msgf("%d questions already downloaded", alreadyDownloadedCnt)
	log.Info().Msgf("%d requests queued", queuedCnt)
	c.Wait()

	if queuedCnt > 0 {
		log.Info().Msgf("Downloaded successfully: %d", downloadedCnt)
		log.Info().Msgf("Already downloaded: %d", alreadyDownloadedCnt)
		log.Info().Msgf("Skipped: %d", skippedCnt)
		log.Info().Msgf("Errors: %d", errorsCnt)
	}
	return downloadedCnt
}
