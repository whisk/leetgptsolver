package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"whisk/leetgptsolver/pkg/throttler"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

var lcThrottler throttler.Throttler

func submit(args []string) {
	files, err := filenamesFromArgs(args)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get files")
		return
	}

	sentCnt := 0
	submittedCnt := 0
	// 2 seconds seems to be minimum acceptable delay for lc
	lcThrottler = throttler.NewThrottler(2*time.Second, 60*time.Second)
	outerLoop: for i, file := range files {
		log.Info().Msgf("[%d/%d] Submitting problem %s...", i+1, len(files), file)

		var problem Problem
		err := problem.ReadProblem(file)
		if err != nil {
			log.Err(err).Msg("Failed to read problem")
			continue
		}
		for modelName, solv := range problem.Solutions {
			if viper.GetString("model") != "" && viper.GetString("model") != modelName {
				continue
			}
			if solv.TypedCode == "" {
				log.Error().Msgf("%s has no solution to submit", modelName)
				continue
			}
			subm, ok := problem.Submissions[modelName]
			if !viper.GetBool("force") && (ok && subm.CheckResponse.Finished) {
				log.Info().Msgf("%s's solution is already submitted", modelName)
				continue
			}

			log.Info().Msgf("Submitting %s's solution...", modelName)
			submission, err := submitAndCheckSolution(problem.Question, solv)
			sentCnt += 1
			if err != nil {
				if _, ok := err.(FatalError); ok {
					log.Err(err).Msgf("Aborting...")
					break outerLoop
				}
				log.Err(err).Msgf("Failed to submit or check %s's solution", modelName)
				continue
			}
			log.Info().Msgf("Submission result: %s", submission.CheckResponse.StatusMsg)
			problem.Submissions[modelName] = *submission
			err = problem.SaveProblemInto(file)
			if err != nil {
				log.Err(err).Msg("Failed to save the submission result")
				continue
			}
		}
		submittedCnt += 1
	}
	log.Info().Msgf("Submitted %d/%d", submittedCnt, len(files))
}

func submitAndCheckSolution(q Question, s Solution) (*Submission, error) {
	subReq := SubmitRequest{
		Lang:       s.Lang,
		QuestionId: q.Data.Question.Id,
		TypedCode:  codeToSubmit(s),
	}

	submissionId, err := submitCode(SubmitUrl(q), subReq)
	if err != nil {
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
		return 0, NewNonRetriableError(fmt.Errorf("failed marshalling GraphQL: %w", err))
	}
	log.Trace().Msgf("Submission request body:\n%s", reqBody.String())
	var respBody []byte
	for lcThrottler.Wait() {
		var code int
		respBody, code, err = makeAuthorizedHttpRequest("POST", url, &reqBody)
		if code == http.StatusBadRequest || code == 499 {
			return 0, NewNonRetriableError(fmt.Errorf("invalid or unauthorized request, see details: %s", string(respBody)))
		}
		if code == http.StatusTooManyRequests || err != nil{
			log.Err(err).Msg("Slowing down...")
			lcThrottler.Slower()
			continue
		}
		log.Trace().Msgf("Submission response body:\n%s", string(respBody))
		lcThrottler.Complete()
	}
	if err != nil {
		return 0, err
	}
	if err = lcThrottler.Error(); err != nil {
		log.Err(err).Msgf("throttler error (this is a bug)")
	}

	var respStruct map[string]uint64
	err = json.Unmarshal(respBody, &respStruct)
	if err != nil {
		return 0, fmt.Errorf("failed unmarshalling submission response: %w", err)
	}
	submissionId := respStruct["submission_id"]
	if submissionId <= 0 {
		return 0, fmt.Errorf("invalid submission id: %d", submissionId)
	}
	log.Debug().Msgf("received submission_id: %d", submissionId)

	return submissionId, nil
}

func checkStatus(url string) (*CheckResponse, error) {
	var checkResp CheckResponse
	for lcThrottler.Wait() {
		log.Trace().Msgf("checking submission status...")
		respBody, code, err := makeAuthorizedHttpRequest("GET", url, bytes.NewReader([]byte{}))
		log.Trace().Msgf("Check response body:\n%s", string(respBody))
		if code == http.StatusBadRequest || code == 499 {
			return &CheckResponse{}, NewNonRetriableError(fmt.Errorf("invalid or unauthorized request, see details: %s", string(respBody)))
		}
		if code == http.StatusTooManyRequests || err != nil {
			log.Err(err).Msg("")
			lcThrottler.Slower()
			continue
		}

		err = json.Unmarshal(respBody, &checkResp)
		if err != nil {
			return nil, fmt.Errorf("failed unmarshalling check response: %w", err)
		}
		if checkResp.Finished {
			lcThrottler.Complete()
		} else {
			lcThrottler.Again()
		}
	}
	if err := lcThrottler.Error(); err != nil {
		log.Err(err).Msgf("throttler error (this is a bug)")
	}
	return &checkResp, nil
}

func codeToSubmit(s Solution) string {
	return "# leetgptsolver submission\n" +
		fmt.Sprintf("# solution generated by model %s at %s \n", s.Model, s.SolvedAt) +
		s.TypedCode
}
