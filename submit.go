package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

func submit(files []string) {
	for _, file := range files {
		var problem Problem
		err := readProblem(&problem, file)
		if err != nil {
			log.Err(err).Msg("Failed to read problem")
			continue
		}
		if len(problem.Solutions) < 1 {
			log.Error().Msg("No solution to submit")
			continue
		}
		submission, err := submitAndCheckSolution(problem.Question, problem.Solutions[0])
		if err != nil {
			log.Err(err).Msg("Failed to submit or check the solution")
			continue
		}
		log.Info().Msgf("Submission result: %s", submission.CheckResponse.StatusMsg)
		problem.Submissions = []Submission{*submission}
		err = saveProblem(problem, file)
		if err != nil {
			log.Err(err).Msg("Failed to save the submission")
			continue
		}
	}
}

func submitAndCheckSolution(q Question, s Solution) (*Submission, error) {

	subReq := SubmitRequest{
		Lang:       s.Lang,
		QuestionId: q.Data.Question.Id,
		TypedCode:  getTypedCode(s),
	}

	url := "https://leetcode.com/problems/" + q.Data.Question.TitleSlug + "/submit/"
	submissionId, err := submitCode(url, subReq)
	if err != nil {
		return nil, err
	}
	url = fmt.Sprintf("https://leetcode.com/submissions/detail/%d/check/", submissionId)

	checkResponse, err := checkStatus(url, submissionId, 30*time.Second)
	if err != nil {
		return nil, err
	}

	return &Submission{
		SubmitRequest: subReq,
		SubmissionId:  submissionId,
		CheckResponse: *checkResponse,
	}, nil
}

func submitCode(url string, subReq SubmitRequest) (uint64, error) {
	reqBody, err := json.Marshal(subReq)
	if err != nil {
		return 0, fmt.Errorf("failed marshalling GraphQL: %w", err)
	}
	log.Trace().Msgf("Submission request body:\n%s", string(reqBody))
	respBody, err := makeAuthorizedHttpRequest("POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return 0, err
	}
	log.Trace().Msgf("Submission response body:\n%s", string(respBody))

	var respStruct map[string]uint64
	err = json.Unmarshal(respBody, &respStruct)
	if err != nil {
		return 0, fmt.Errorf("failed unmarshalling submission response: %w", err)
	}
	submissionId := respStruct["submission_id"]
	if submissionId <= 0 {
		return 0, fmt.Errorf("invalid submission id")
	}
	log.Debug().Msgf("Got submission_id %d", submissionId)

	return submissionId, nil
}

func checkStatus(url string, submissionId uint64, maxWaitTime time.Duration) (*CheckResponse, error) {
	var t time.Duration = 0
	var d time.Duration = 1 * time.Second
	for t < maxWaitTime {
		respBody, err := makeAuthorizedHttpRequest("GET", url, bytes.NewReader([]byte{}))
		if err != nil {
			return nil, err
		}
		log.Trace().Msgf("Check response body:\n%s", string(respBody))

		var checkResp CheckResponse
		err = json.Unmarshal(respBody, &checkResp)
		if err != nil {
			return nil, fmt.Errorf("failed unmarshalling check response: %w", err)
		}
		if checkResp.Finished {
			return &checkResp, nil
		}
		time.Sleep(d)
		t += d
		d = min(d*2, maxWaitTime-t)
	}
	return nil, errors.New("server have not checked the solution in a timely manner")
}

func getTypedCode(s Solution) string {
	return "# leetgptsolver submission\n" +
		fmt.Sprintf("# solution generated by model %s at %s \n", s.Model, s.SolvedAt) +
		s.TypedCode
}