#!/usr/bin/env bash

# List all problems that do NOT have Accepted in .SubmissionsV2 and do NOT have Accepted in .Submissions.
ls problems/*.json | go run . list \
  -w '([(.SubmissionsV2 // {} | .. | objects | .CheckResponse? | select(.status_code == 10 or .status_msg == "Accepted")), (.Submissions // {} | .. | objects | .CheckResponse? | select(.status_code == 10 or .status_msg == "Accepted"))] | length == 0)' \
  -p ".Path" \
  -o ".Question.Data.Question.questionFrontendId|tonumber" \
  --header=false \
  -
