package main

import (
	"bufio"
	"os"
	leetgptsolver "whisk/leetgptsolver/pkg"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func report(args []string) {
	files, err := filenamesFromArgs(args)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get files")
		return
	}

	reportFilename := viper.GetString("output")
	if len(files) == 0 {
		var err error
		files, err = allFilesFromProblemsDir()
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

	models := leetgptsolver.SupportedModels()
	_, _ = w.Write(ProblemTsvHeader(models))
	reportedCnt := 0
	for _, file := range files {
		var p Problem
		err := p.ReadProblem(file)
		if err != nil {
			log.Err(err).Msg("Failed to read the problem")
			continue
		}
		_, err = w.Write(p.ProblemToTsv(models, PREFERRED_LANGUAGES))
		if err != nil {
			log.Err(err).Msg("Failed to write to the report file")
			continue
		}
		reportedCnt += 1
	}

	log.Info().Msgf("Reported %d/%d", reportedCnt, len(files))
}
