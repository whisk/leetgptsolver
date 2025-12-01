package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/rs/zerolog/log"
)

const SEPARATOR = "\t"

type Result struct {
	Value string
	OrderBy any
}

func list(args []string, whereExpr, orderByExpr, printExpr string, printHeader bool) {
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

	if orderByExpr == "" {
		orderByExpr = ".Question.Data.Question.TitleSlug"
	}

	var orderByQuery *gojq.Query
	orderByQuery, err = gojq.Parse(orderByExpr)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to parse order by query")
		return
	}

	printQuery, err := gojq.Parse(printExpr)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to parse print query")
		return
	}

	if printHeader {
		var s strings.Builder
		queryToHeaderRow(printQuery, &s)
		fmt.Println(s.String())
	}

	result := []Result{}
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

		iterOrderBy := orderByQuery.Run(pStruct)
		var orderBy any
		i = 0
		for {
			i += 1
			v, ok := iterOrderBy.Next()
			if !ok {
				break
			}
			if err, ok := v.(error); ok {
				log.Err(err).Msg("failed to order by")
				continue outerLoop
			}
			log.Trace().Msgf("order by clause %d = %v", i, v)
			orderBy = v
		}

		iterPrint := printQuery.Run(pStruct)
		value := ""
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
			if jsonVal, ok := v.(map[string]any); ok {
				// seems like a json output, print it as a json on a single line
				jsonBytes, err := json.Marshal(jsonVal)
				if err != nil {
					log.Err(err).Msg("failed to marshal json")
					continue outerLoop
				}
				value = string(jsonBytes)
			} else {
				// print the value as is
				value = fmt.Sprintf("%v"+SEPARATOR, v)
			}
		}
		result = append(result, Result{Value: value, OrderBy: orderBy})
	}

	slices.SortFunc(result, func(a, b Result) int {
		switch aVal := a.OrderBy.(type) {
		case int:
			bVal, ok := b.OrderBy.(int)
			if !ok {
				log.Fatal().Msgf("order by values must be of the same type, got int and %T", b.OrderBy)
			}
			return aVal - bVal
		case string:
			bVal, ok := b.OrderBy.(string)
			if !ok {
				log.Fatal().Msgf("order by values must be of the same type, got string and %T", b.OrderBy)
			}
			return strings.Compare(aVal, bVal)
		default:
			log.Fatal().Msgf("unsupported order by type %T", a.OrderBy)
		}

		return 0
	})

	for _, r := range result {
		fmt.Println(r.Value)
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

// convert the "print" query into a format suitable for a header row, allowing for nicely named columns
// e.g., "a,b,c" -> "a b c"
func queryToHeaderRow(e *gojq.Query, s *strings.Builder) {
	if e.Term != nil {
		s.WriteString(e.Term.String())
	} else if e.Right != nil {
		queryToHeaderRow(e.Left, s)
		if e.Op == gojq.OpComma {
			s.WriteString(SEPARATOR)
		} else {
			s.WriteByte(' ')
			s.WriteString(e.Op.String())
			s.WriteByte(' ')
		}
		queryToHeaderRow(e.Right, s)
	}
}
