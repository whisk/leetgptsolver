package main

import (
	"github.com/rs/zerolog/log"
)

func fix(files []string) {
	if len(files) == 0 {
		var err error
		files, err = getProblemsFiles()
		if err != nil {
			log.Err(err).Msg("failed to read problems files")
			return
		}
	}

	fixedCnt := 0
	for _, file := range files {
		log.Info().Msgf("Fixing problem %s ...", file)

		var p Problem
		err := readProblem(&p, file)
		if err != nil {
			log.Err(err).Msg("Failed to read the problem")
			continue
		}
		// fixing code go here

		// increase if fixes successfully
		fixedCnt += 1

		err = saveProblem(p, file)
		if err != nil {
			log.Err(err).Msg("Failed to save the problem")
			continue
		}
	}
	log.Info().Msgf("Fixed %d/%d", fixedCnt, len(files))
}