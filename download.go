package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http/cookiejar"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/gocolly/colly/v2"
	"github.com/rs/zerolog/log"
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

func download(category string, args []string) {
	files, err := filenamesFromArgs(args)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get files")
		return
	}

	availableSlugs, err := getAvailableSlugs(category)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get questions slugs")
		return
	}
	log.Info().Msgf("got %d question slugs for %s", len(availableSlugs), category)

	// just print slugs
	if options.Slugs {
		printQuestions(availableSlugs)
		return
	}

	slugsToDownload := []QuestionSlug{}
	// download questions
	if len(files) == 0 {
		slugsToDownload = availableSlugs
	} else {
		// filter slugs to download
		slugsMap := map[string]QuestionSlug{}
		for _, qs := range availableSlugs {
			slugsMap[qs.Stat.TitleSlug] = qs
		}

		log.Info().Msgf("searching for %d question(s)...", len(files))
		for _, file := range files {
			// TODO: very awkward, improve
			title := filepath.Base(file)
			title = strings.TrimSuffix(title, filepath.Ext(file))

			if qs, ok := slugsMap[title]; ok {
				slugsToDownload = append(slugsToDownload, qs)
			} else {
				log.Error().Msgf("unknown problem %s, skipping", title)
			}
		}
	}
	downloadQuestions(slugsToDownload)
}

func getAvailableSlugs(category string) ([]QuestionSlug, error) {
	if !slices.Contains([]string{"all", "algorithms", "database"}, category) {
		return nil, fmt.Errorf("unsupported category: %s", category)
	}

	c := client()
	resp, err := c.Get("https://leetcode.com/api/problems/" + category + "/")
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
				freqBar
				categoryTitle
				sampleTestCase
				exampleTestcases
				topicTags {
					id
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
func downloadQuestions(slugs []QuestionSlug) int {
	// Check if username is not empty before downloading, unless SkipAuthCheck is set
	var err error
	if !options.SkipAuthCheck {
		userStatus, err := LoadUserStatus()
		if err != nil {
			log.Fatal().Err(err).Msg("failed to get user status from leetcode")
			return -1
		}
		log.Debug().Msgf("leetcode username: %s", userStatus.Username)
		if userStatus.Username == "" {
			log.Fatal().Msg("leetcode username is empty. Please ensure you are signed in Firefox before downloading questions, or use -A to allow anonymous download.")
			return -1
		}
	}

	log.Debug().Msgf("queueing %d questions...", len(slugs))

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
	if !options.SkipPaid {
		jar, ok := cookieJar().(*cookiejar.Jar)
		if !ok {
			log.Fatal().Msg("failed to use cookie jar. This is a bug")
			return -1
		}
		c.SetCookieJar(jar)
	}
	err = c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: 2,
		RandomDelay: 15 * time.Second,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to set download limits")
	}

	signalChan := make(chan os.Signal, 1)
	go func() {
		s := <-signalChan
		log.Info().Msgf("got %v, terminating", s)
		exitSignal = s
	}()

	c.OnResponse(func(r *colly.Response) {
		consecutiveErrorsCnt = 0
		if exitSignal != nil {
			log.Info().Msg("terminated by user")
			// we don't like os.Exit, but it seems that colly doesn't have a good way to stop parallel requests
			code, _ := exitSignal.(syscall.Signal)
			os.Exit(int(code))
		}
		log.Debug().Msgf("%s %s %s %d", r.Request.Method, r.Request.URL, r.Ctx.Get("dstFile"), r.StatusCode)
		log.Trace().Msg(string(r.Body))

		var problem Problem
		err := json.Unmarshal(r.Body, &problem.Question)
		if err != nil {
			log.Err(err).Msg("failed to unmarshall question from json")
			return
		}
		problem.Question.DownloadedAt = time.Now()

		dstFile := r.Ctx.Get("dstFile")
		if dstFile == "" {
			log.Error().Msg("no context found")
			return
		}

		// we don't want interruptions while saving the data
		signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
		defer signal.Reset()

		fileAlreadyExists, err := fileExists(dstFile)
		if err != nil {
			log.Fatal().Err(err).Msgf("failed to check if file %s exists", dstFile)
			return
		}

		if options.CreationDate {
			approxCreatedAt, err := LoadFirstUgcContentTime(problem.Question.Data.Question.TitleSlug)
			if err != nil {
				log.Err(err).Msgf("failed to determine approximate creation date for %s", problem.Question.Data.Question.TitleSlug)
			} else {
				log.Info().Msgf("approximate creation date for %s is %s", problem.Question.Data.Question.TitleSlug, approxCreatedAt)
				problem.Question.CreatedAtApprox = time.Date(
					approxCreatedAt.Year(),
					time.Month(approxCreatedAt.Month()),
					approxCreatedAt.Day(),
					0, 0, 0, 0, time.UTC,
				)
			}
		}

		if !options.Overwrite && fileAlreadyExists {
			log.Debug().Msgf("updating %s...", dstFile)
			var existingProblem Problem
			err := existingProblem.ReadProblem(dstFile)
			if err != nil {
				log.Err(err).Msgf("failed to read existing problem from %s", dstFile)
				return
			}
			existingProblem.Question = problem.Question
			err = existingProblem.SaveProblemInto(dstFile)
			if err != nil {
				log.Err(err).Msg("failed to update existing question")
				return
			}
		} else {
			err = problem.SaveProblemInto(dstFile)
			if err != nil {
				log.Err(err).Msg("failed to save downloaded question")
				return
			}
		}
		signal.Reset()

		log.Info().Msgf("Problem %s downloaded successfully", dstFile)
		downloadedCnt += 1
	})
	c.OnError(func(r *colly.Response, e error) {
		log.Error().Err(e).Msgf("failed to fetch question %s", r.Request.Ctx.Get("dstFile"))
		consecutiveErrorsCnt += 1
		errorsCnt += 1
		if consecutiveErrorsCnt >= MAX_CONSECUTIVE_ERRORS {
			log.Error().Msgf("too many errors (%d), aborting...", consecutiveErrorsCnt)
			os.Exit(1)
		}
	})

	for _, qs := range slugs {
		if options.SkipPaid && qs.PaidOnly {
			skippedCnt += 1
			continue
		}
		dstFile := path.Join(options.Dir, qs.Stat.TitleSlug+".json")
		fileAlreadyExists, _ := fileExists(dstFile)
		if fileAlreadyExists {
			alreadyDownloadedCnt += 1
			if options.Force {
				log.Debug().Msgf("file %s already exists, forcefully downloading...", dstFile)
			} else {
				log.Debug().Msgf("file %s already exists", dstFile)
				continue
			}
		}
		queryBytes, err := makeQuestionQuery(qs)
		if err != nil {
			log.Err(err).Msg("failed to make a query")
		}
		ctx := colly.NewContext()
		ctx.Put("dstFile", dstFile)
		hdr := newHeader()
		err = c.Request(
			"POST",
			leetcodeGraphqlUrl.String(),
			bytes.NewBuffer(queryBytes),
			ctx,
			hdr,
		)
		if err != nil {
			log.Err(err).Msgf("failed to create question request for %s", qs.Stat.TitleSlug)
			errorsCnt += 1
			continue
		}
		queuedCnt += 1
	}
	log.Info().Msgf("%d questions already downloaded", alreadyDownloadedCnt)
	log.Info().Msgf("%d requests queued", queuedCnt)
	c.Wait()

	if queuedCnt > 0 {
		log.Info().Msgf("downloaded successfully: %d", downloadedCnt)
		log.Info().Msgf("already downloaded: %d", alreadyDownloadedCnt)
		log.Info().Msgf("skipped: %d", skippedCnt)
		log.Info().Msgf("errors: %d", errorsCnt)
	}
	return downloadedCnt
}
