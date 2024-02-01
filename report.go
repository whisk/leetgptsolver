package main

import (
	"bufio"
	"os"

	"github.com/rs/zerolog/log"
)

func report(files []string) {
	reportFilename := "report.tsv"
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

	_, _ = w.Write(problemTsvHeader())
	reportedCnt := 0
	for _, file := range files {
		var p Problem
		err := readProblem(&p, file)
		if err != nil {
			log.Err(err).Msg("Failed to read the problem")
			continue
		}
		_, err = w.Write(problemToTsv(p))
		if err != nil {
			log.Err(err).Msg("Failed to write to the report file")
			continue
		}
		reportedCnt += 1
	}

	log.Info().Msgf("Reported %d/%d", reportedCnt, len(files))
}
