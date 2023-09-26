package main

import (
	"fmt"
	"net/http"
	"os"

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

	fmt.Println(questions)
}
