package main

import (
	"bufio"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func report(files []string) {
	reportFilename := viper.GetString("output")
	if len(files) == 0 {
		var err error
		files, err = getProblemsFiles()
		if err != nil {
			log.Err(err).Msg("Failed to read problems files")
			return
		}
	}

	log.Info().Msgf("Generating %s for %d problem(s)...", reportFilename, len(files))

	f, err := os.Create(reportFilename)
	if err != nil {
		log.Err(err).Msg("failed to create report file")
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	models := []string{GPT4, Gemini10Pro, Claude} // very dirty and temporary
	_, _ = w.Write(problemTsvHeader(models))
	reportedCnt := 0
	for _, file := range files {
		var p Problem
		err := readProblem(&p, file)
		if err != nil {
			log.Err(err).Msg("Failed to read the problem")
			continue
		}
		_, err = w.Write(problemToTsv(p, models))
		if err != nil {
			log.Err(err).Msg("Failed to write to the report file")
			continue
		}
		reportedCnt += 1
	}

	log.Info().Msgf("Reported %d/%d", reportedCnt, len(files))
}
