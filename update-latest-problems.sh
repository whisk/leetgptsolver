#!/usr/bin/env bash

# Update question data and stats for the last 100 problems (sorted by frontend ID).
ls problems/*.json | go run . list -p ".Filename" -o ".Question.Data.Question.questionFrontendId|tonumber" --header=false - \
| tail -n 100 \
| go run . download -v -u --detect_approx_creation_date=false -
