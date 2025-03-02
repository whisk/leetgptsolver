package main

import (
	"fmt"

	"github.com/rs/zerolog/log"
)

func show(args []string) {
	files, err := filenamesFromArgs(args)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get files")
		return
	}

	for _, file := range files {
		var problem Problem
		err := problem.ReadProblem(file)
		if err != nil {
			log.Err(err).Msg("Failed to read the problem")
			continue
		}

		fmt.Printf("%s\t%s\t%v\t%s\t%s\n",
			problem.Question.Data.Question.FrontendId,
			problem.Question.Data.Question.Id,
			problem.Question.Data.Question.IsPaidOnly,
			problem.Question.Data.Question.Title,
			problem.Question.Data.Question.Difficulty,
		)
	}
}
