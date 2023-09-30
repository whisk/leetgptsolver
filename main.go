package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gocolly/colly"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/thedevsaddam/gojsonq/v2"
)

type QuestionStat struct {
	TitleSlug string `json:"question__title_slug"`
	Title     string `json:"question__title"`
}

type Question struct {
	Difficulty map[string]int `json:"difficulty"`
	Stat       QuestionStat   `json:"stat"`
	PaidOnly   bool           `json:"paid_only"`
}

func main() {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()

	resp, err := http.Get("https://leetcode.com/api/problems/algorithms/")
	if err != nil {
		log.Fatal().Err(err)
	}
	defer resp.Body.Close()

	var questions []Question
	gojsonq.New().Reader(resp.Body).From("stat_status_pairs").Out(&questions)

	fmt.Println(len(questions))

	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 6.1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/41.0.2228.0 Safari/537.36"),
	)
	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("X-Requested-With", "XMLHttpRequest")
		r.Headers.Set("Content-Type", "application/json;charset=UTF-8")
		fmt.Println("Visiting: ", r.URL)
	})
	c.OnResponse(func(r *colly.Response) {
		fmt.Println(".")
		fmt.Println(string(r.Body))
	})
	c.OnError(func(r *colly.Response, e error) {
		fmt.Println("error:", e, r.Request.URL, string(r.Body))
	})
	for _, q := range questions {
		if !q.PaidOnly {
			jsonStr := `{"query":"\n    query questionContent($titleSlug: String!) {\n  question(titleSlug: $titleSlug) {\n    content\n    mysqlSchemas\n    dataSchemas\n  }\n}\n    ","variables":{"titleSlug":"%s"},"operationName":"questionContent"}`
			jsonStr = fmt.Sprintf(jsonStr, q.Stat.TitleSlug)
			c.PostRaw("https://leetcode.com/graphql", []byte(jsonStr))
		}
	}
	c.Wait()
}
