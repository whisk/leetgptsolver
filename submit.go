package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
	"whisk/leetgptsolver/pkg/throttler"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type InvalidCodeError struct {
	error
}

func NewInvalidCodeError(err error) error {
	return InvalidCodeError{err}
}

var leetcodeThrottler throttler.Throttler

func submit(args []string, modelName string) {
	files, err := filenamesFromArgs(args)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get files")
		return
	}

	log.Info().Msgf("Submitting %d solutions...", len(files))
	submittedCnt := 0
	skippedCnt := 0
	errorsCnt := 0
	// 2 seconds seems to be minimum acceptable delay for leetcode
	leetcodeThrottler = throttler.NewSimpleThrottler(2*time.Second, 60*time.Second)
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

		solv, ok := problem.Solutions[modelName]
		if !ok {
			log.Warn().Msgf("Model %s has no solution to submit", modelName)
			skippedCnt += 1
			continue
		}
		if solv.TypedCode == "" {
			log.Error().Msgf("Model %s has empty solution", modelName)
			skippedCnt += 1
			continue
		}
		subm, ok := problem.Submissions[modelName]
		if !viper.GetBool("force") && (ok && subm.CheckResponse.Finished) {
			log.Info().Msgf("%s's solution is already submitted", modelName)
			skippedCnt += 1
			continue
		}
		log.Info().Msgf("Submitting %s's solution...", modelName)
		submission, err := submitAndCheckSolution(problem.Question, solv)
		if err != nil {
			errorsCnt += 1
			if _, ok := err.(FatalError); ok {
				log.Err(err).Msgf("Aborting...")
				break outerLoop
			}
			log.Err(err).Msgf("Failed to submit or check %s's solution", modelName)
			continue
		}

		log.Info().Msgf("Submission status: %s", submission.CheckResponse.StatusMsg)
		problem.Submissions[modelName] = *submission
		err = problem.SaveProblemInto(file)
		if err != nil {
			log.Err(err).Msg("Failed to save the submission result")
			errorsCnt += 1
			continue
		}
		submittedCnt += 1
	}
	log.Info().Msgf("Files processed: %d", len(files))
	log.Info().Msgf("Skipped problems: %d", skippedCnt)
	log.Info().Msgf("Problems submitted successfully: %d", submittedCnt)
	log.Info().Msgf("Errors: %d", errorsCnt)
}

func submitAndCheckSolution(q Question, s Solution) (*Submission, error) {
	subReq := SubmitRequest{
		Lang:       s.Lang,
		QuestionId: q.Data.Question.Id,
		TypedCode:  codeToSubmit(s),
	}

	submissionId, err := submitCode(SubmitUrl(q), subReq)
	if err != nil {
		var subErr InvalidCodeError
		if errors.As(err, &subErr) {
			// non-retriable submission error, like "Your code is too long"
			return &Submission{
				SubmitRequest: subReq,
				CheckResponse: CheckResponse{
					StatusMsg:  subErr.Error(),
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
	log.Trace().Msgf("Submission request body:\n%s", reqBody.String())
	var respBody []byte
	maxRetries := viper.GetInt("submit_retries")
	i := 0
	leetcodeThrottler.Ready()
	for leetcodeThrottler.Wait() && i < maxRetries {
		i += 1

		var code int
		respBody, code, err = makeAuthorizedHttpRequest("POST", url, &reqBody)
		leetcodeThrottler.Touch()
		log.Trace().Msgf("submission response body:\n%s", string(respBody))
		if code == http.StatusBadRequest || code == 499 {
			leetcodeThrottler.Slowdown()
			return 0, NewNonRetriableError(fmt.Errorf("invalid or unauthorized request, see details: %s", string(respBody)))
		}
		if code == http.StatusTooManyRequests || err != nil {
			log.Err(err).Msg("Slowing down...")
			leetcodeThrottler.Slowdown()
			continue
		}

		break // success
	}
	if err != nil {
		return 0, err
	}

	var respStruct map[string]any
	decoder := json.NewDecoder(bytes.NewReader(respBody))
	decoder.UseNumber()
	err = decoder.Decode(&respStruct)
	log.Trace().Msgf("submission response struct: %#v", respStruct)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshal submission response: %w", err)
	}
	if errorMsg, ok := respStruct["error"].(string); ok && respStruct["error"] == "Your code is too long. Please reduce your code size and try again." {
		return 0, fmt.Errorf("submission error: %w", NewInvalidCodeError(errors.New(errorMsg)))
	}
	submissionNumber, ok := respStruct["submission_id"].(json.Number);
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
	maxRetries := viper.GetInt("check_retries")
	i := 0
	leetcodeThrottler.Ready()
	for leetcodeThrottler.Wait() && i < maxRetries {
		i += 1
		log.Trace().Msgf("checking submission status (%d/%d)...", i, maxRetries)
		respBody, code, err := makeAuthorizedHttpRequest("GET", url, bytes.NewReader([]byte{}))
		leetcodeThrottler.Touch()
		log.Trace().Msgf("Check response body: %s", string(respBody))
		if code == http.StatusBadRequest || code == 499 {
			return &CheckResponse{}, NewNonRetriableError(fmt.Errorf("invalid or unauthorized request, see details: %s", string(respBody)))
		}
		if code == http.StatusTooManyRequests || err != nil {
			log.Err(err).Msg("")
			leetcodeThrottler.Slowdown()
			continue
		}

		err = json.Unmarshal(respBody, &checkResp)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal check response: %w", err)
		}

		if checkResp.Finished {
			break // success
		}
	}
	if checkResp == nil {
		// did not get a response after retries
		return nil, fmt.Errorf("failed to get check submission status")
	}
	if !checkResp.Finished {
		return nil, fmt.Errorf("submission is not finished")
	}

	return checkResp, nil
}

func codeToSubmit(s Solution) string {
	return "# leetgptsolver submission\n" +
		fmt.Sprintf("# solution generated by model %s at %s \n", s.Model, s.SolvedAt) +
		s.TypedCode
}
