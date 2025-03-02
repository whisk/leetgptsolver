package main

import (
	"github.com/rs/zerolog/log"
)

func fix(args []string) {
	files, err := filenamesFromArgs(args)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get files")
		return
	}

	if len(files) == 0 {
		var err error
		files, err = allFilesFromProblemsDir()
		if err != nil {
			log.Err(err).Msg("failed to read problems files")
			return
		}
	}

	fixedCnt := 0
	for i, file := range files {
		log.Info().Msgf("[%d/%d] Fixing problem %s ...", i+1, len(files), file)

		var p Problem
		err := p.ReadProblem(file)
		if err != nil {
			log.Err(err).Msg("Failed to read the problem")
			continue
		}
		// fixing code go here

		err = p.SaveProblemInto(file)
		if err != nil {
			log.Err(err).Msg("Failed to save the problem")
			continue
		}
		// increase if fixed and saved successfully
		fixedCnt += 1
	}
	log.Info().Msgf("Fixed %d/%d", fixedCnt, len(files))
}
