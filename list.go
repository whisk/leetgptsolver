package main

import (
	"encoding/json"
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/rs/zerolog/log"
)

func list(args []string, whereExpr string, printExpr string) {
	files, err := filenamesFromArgs(args)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get files")
		return
	}

	if whereExpr == "" {
		whereExpr = "true"
	}

	var whereQuery *gojq.Query
	whereQuery, err = gojq.Parse(whereExpr)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to parse where query")
		return
	}

	printQuery, err := gojq.Parse(printExpr)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to parse print query")
		return
	}

outerLoop:
	for _, file := range files {
		var problem Problem
		err := problem.ReadProblem(file)
		if err != nil {
			log.Err(err).Msg("failed to read the problem")
			continue
		}

		pStruct, err := problemToMap(problem)
		if err != nil {
			log.Err(err).Msg("failed to convert problem to map (this is a bug)")
			break outerLoop
		}

		match := false
		iterWhere := whereQuery.Run(pStruct)
		i := 0
		for {
			i += 1
			v, ok := iterWhere.Next()
			if !ok {
				break
			}
			if err, ok := v.(error); ok {
				log.Err(err).Msg("failed to match")
				continue outerLoop
			}
			if _, ok := v.(bool); !ok {
				log.Error().Msgf("where expression must return boolean, got %T", v)
				continue outerLoop
			} else {
				log.Trace().Msgf("where clause %d = %v", i, v)
				match = v.(bool)
			}
		}

		if !match {
			continue
		}
		iterPrint := printQuery.Run(pStruct)
		for {
			v, ok := iterPrint.Next()
			if !ok {
				break
			}
			if err, ok := v.(error); ok {
				if err, ok := err.(*gojq.HaltError); ok && err.Value() == nil {
					break outerLoop
				}
				log.Err(err).Msg("failed to print")
				continue outerLoop
			}
			fmt.Printf("%v\t", v)
		}
		fmt.Println()
	}
}

// convert Problem struct to map[string]any for gojq
func problemToMap(p Problem) (map[string]any, error) {
	jsonBytes, err := p.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal the problem: %w", err)
	}
	var pMap map[string]any
	err = json.Unmarshal(jsonBytes, &pMap)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal the problem back: %w", err)
	}
	pMap["Path"] = p.Path
	pMap["Filename"] = p.Filename
	return pMap, nil
}
