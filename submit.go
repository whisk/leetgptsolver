package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"
)

type InvalidCodeError struct {
	error
}

func NewInvalidCodeError(err error) error {
	return InvalidCodeError{err}
}

var leetcodeLimiter *rate.Limiter

func submit(args []string, lang, modelName string) error {
	if options.DryRun {
		log.Warn().Msg("Running in dry-run mode. No changes will be made to problem files")
	}
	files, err := filenamesFromArgs(args)
	if err != nil {
		return fmt.Errorf("failed to get files: %w", err)
	}

	log.Info().Msgf("Submitting %d solutions...", len(files))
	submittedCnt := 0
	acceptedCnt := 0
	notAcceptedCnt := 0
	skippedCnt := 0
	errorsCnt := 0
	leetcodeLimiter = rate.NewLimiter(rate.Limit(options.SubmitRateLimit), options.SubmitRateBurst)
outerLoop:
	for i, file := range files {
		log.Info().Msgf("[%d/%d] Submitting problem %s ...", i+1, len(files), file)

		var problem Problem
		err := problem.ReadProblem(file)
		if err != nil {
			log.Err(err).Msg("Failed to read problem")
			errorsCnt += 1
			continue
		}

		solv, ok := problem.GetSolution(modelName, lang)
		if !ok {
			log.Warn().Msgf("Model %s has no solution in %s to submit", modelName, lang)
			skippedCnt += 1
			continue
		}
		if solv.TypedCode == "" {
			log.Error().Msgf("Model %s has empty solution", modelName)
			skippedCnt += 1
			continue
		}
		subm, ok := problem.GetSubmission(modelName, lang)
		if !options.Force && (ok && subm.CheckResponse.Finished) {
			log.Info().Msgf("%s's solution is already submitted", modelName)
			skippedCnt += 1
			continue
		}
		log.Info().Msgf("Submitting %s's solution...", modelName)
		submission, err := submitAndCheckSolution(problem.Question, solv)
		if err != nil {
			errorsCnt += 1
			if errors.Is(err, ErrFatal) {
				log.Err(err).Msgf("Aborting...")
				break outerLoop
			}
			log.Err(err).Msgf("Failed to submit or check %s's solution", modelName)
			continue
		}

		log.Info().Msgf("Submission status: %s", submission.CheckResponse.StatusMsg)
		if problem.SubmissionsV2 == nil {
			problem.SubmissionsV2 = map[string]map[string]Submission{}
		}
		if _, ok := problem.SubmissionsV2[modelName]; !ok {
			problem.SubmissionsV2[modelName] = map[string]Submission{}
		}
		problem.SubmissionsV2[modelName][lang] = *submission
		if !options.DryRun {
			err = problem.SaveProblemInto(file)
			if err != nil {
				log.Err(err).Msg("Failed to save the submission result")
				errorsCnt += 1
				continue
			}
		}
		submittedCnt += 1
		if submission.CheckResponse.StatusMsg == "Accepted" {
			acceptedCnt += 1
		} else {
			notAcceptedCnt += 1
		}
	}
	log.Info().Msgf("Files processed: %d", len(files))
	log.Info().Msgf("Skipped problems: %d", skippedCnt)
	log.Info().Msgf("Problems submitted successfully: %d (accepted: %d, not accepted: %d, unknown: %d)", submittedCnt, acceptedCnt, notAcceptedCnt, submittedCnt-acceptedCnt-notAcceptedCnt)
	log.Info().Msgf("Errors: %d", errorsCnt)
	return nil
}

func submitAndCheckSolution(q Question, s Solution) (*Submission, error) {
	typedCode, err := codeToSubmit(s, options.AddMetadataComment)
	if err != nil {
		return nil, err
	}
	subReq := SubmitRequest{
		Lang:       s.Lang,
		QuestionId: q.Data.Question.Id,
		TypedCode:  typedCode,
	}

	submissionId, err := submitCode(SubmitUrl(q), subReq)
	if err != nil {
		var subErr InvalidCodeError
		if errors.As(err, &subErr) {
			// non-retriable submission error, like "Your code is too long"
			return &Submission{
				SubmitRequest: subReq,
				CheckResponse: CheckResponse{
					StatusMsg: subErr.Error(),
					Finished:  true,
				},
				SubmittedAt: time.Now(),
			}, nil
		}

		return nil, err
	}

	checkResponse, err := checkStatus(SubmissionCheckUrl(submissionId))
	if err != nil {
		return nil, err
	}

	return &Submission{
		SubmitRequest: subReq,
		SubmissionId:  submissionId,
		CheckResponse: *checkResponse,
		SubmittedAt:   time.Now(),
	}, nil
}

func submitCode(url string, subReq SubmitRequest) (uint64, error) {
	var reqBody bytes.Buffer
	// use encoder, not standard json.Marshal() because we don't need to escape "<", ">" etc. in the source code
	encoder := json.NewEncoder(&reqBody)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(subReq)
	if err != nil {
		return 0, NewNonRetriableError(fmt.Errorf("failed marshaling GraphQL: %w", err))
	}
	reqBodyBytes := reqBody.Bytes()
	var respBody []byte
	var lastErr error
	maxRetries := options.SubmitRetries
	i := 0
	for i < maxRetries {
		i += 1
		if leetcodeLimiter != nil {
			if err := leetcodeLimiter.Wait(context.Background()); err != nil {
				return 0, err
			}
		}

		var code int
		respBody, code, err = makeAuthorizedHttpRequest("POST", url, bytes.NewReader(reqBodyBytes))
		if code == http.StatusBadRequest || code == http.StatusForbidden || code == http.StatusTooManyRequests {
			err_message := string(respBody)
			if len(err_message) > 80 {
				err_message = err_message[:80] + "..."
			}
			return 0, NewNonRetriableError(fmt.Errorf("%w. See response for details: %s", err, err_message))
		}
		lastErr = err
		if err == nil {
			break // success
		}

		log.Err(err).Msg("Retrying...")
	}
	if lastErr != nil {
		return 0, lastErr
	}

	var respStruct map[string]any
	decoder := json.NewDecoder(bytes.NewReader(respBody))
	decoder.UseNumber()
	err = decoder.Decode(&respStruct)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshal submission response: %w", err)
	}
	if errVal, exists := respStruct["error"]; exists && errVal != nil {
		if errorMsg, ok := errVal.(string); ok && errorMsg != "" {
			if errorMsg == "Your code is too long. Please reduce your code size and try again." {
				return 0, fmt.Errorf("submission error: %w", NewInvalidCodeError(errors.New(errorMsg)))
			}
			return 0, fmt.Errorf("submission error: %s", errorMsg)
		}
		return 0, fmt.Errorf("submission error: %v", errVal)
	}
	submissionNumber, ok := respStruct["submission_id"].(json.Number)
	if !ok {
		return 0, fmt.Errorf("submission_id is not a number: %v", respStruct["submission_id"])
	}
	submissionId, err := submissionNumber.Int64()
	if err != nil {
		return 0, fmt.Errorf("invalid submission id: %w", err)
	}
	if submissionId <= 0 {
		return 0, fmt.Errorf("invalid submission id: %d", submissionId)
	}
	log.Debug().Msgf("received submission_id: %d", submissionId)

	return uint64(submissionId), nil
}

func checkStatus(url string) (*CheckResponse, error) {
	var checkResp *CheckResponse
	var lastErr error
	maxRetries := options.CheckRetries
	i := 0
	for i < maxRetries {
		i += 1
		if leetcodeLimiter != nil {
			if err := leetcodeLimiter.Wait(context.Background()); err != nil {
				return nil, err
			}
		}
		log.Trace().Msgf("checking submission status (%d/%d)...", i, maxRetries)
		respBody, code, err := makeAuthorizedHttpRequest("GET", url, bytes.NewReader([]byte{}))
		if code == http.StatusBadRequest || code == http.StatusForbidden || code == 499 {
			err_message := string(respBody)
			if len(err_message) > 80 {
				err_message = err_message[:80] + "..."
			}
			return nil, NewNonRetriableError(fmt.Errorf("invalid or unauthorized request, see response: %s", err_message))
		}
		if code == http.StatusTooManyRequests || err != nil {
			lastErr = err
			log.Err(err).Msg("Retrying...")
			continue
		}

		var current CheckResponse
		err = json.Unmarshal(respBody, &current)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal check response: %w", err)
		}

		checkResp = &current
		lastErr = nil

		if current.Finished {
			return &current, nil // success
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("check status failed: %w", lastErr)
	}
	if checkResp == nil {
		// did not get a response after retries
		return nil, fmt.Errorf("failed to get check submission status")
	}
	return nil, fmt.Errorf("submission is not finished after %d retries", maxRetries)
}

func codeToSubmit(s Solution, addMetadataComment bool) (string, error) {
	code := s.TypedCode
	if s.Lang == "rust" {
		// some models generate "pub struct Solution;" which is excessive and causes compilation error
		code = strings.ReplaceAll(code, "pub struct Solution;\n", "")
		code = strings.ReplaceAll(code, "struct Solution;\n", "")
	}

	if !addMetadataComment {
		return code, nil
	}

	commentPrefix := ""
	switch s.Lang {
	case "python", "python3", "ruby", "elixir", "bash":
		commentPrefix = "#"
	case "java", "csharp", "cpp", "javascript", "typescript", "swift", "go", "rust", "php":
		commentPrefix = "//"
	case "mysql", "mssql", "postgresql", "oraclesql":
		commentPrefix = "--"
	}

	if commentPrefix == "" {
		return "", fmt.Errorf("unsupported language for metadata comment: %s", s.Lang)
	}

	comment := commentPrefix + " leetgptsolver submission\n" +
		fmt.Sprintf(commentPrefix+" solution generated by model %s at %s \n", s.Model, s.SolvedAt)
	return comment + code, nil
}
