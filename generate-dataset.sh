#!/usr/bin/env bash

# Generates dataset for HF: https://huggingface.co/datasets/whiskwhite/leetcode-complete
# Format: JSON Lines (jsonl)
# Format version: 0.3.1
# Please avoid removing existing fields or changing their types!
p=$(cat <<-'END'
    {
        url: .Question.Url,
        title_slug: .Question.Data.Question.TitleSlug,
        id: .Question.Data.Question.questionId,
        frontend_id: .Question.Data.Question.questionFrontendId,
        title: .Question.Data.Question.Title,
        content: .Question.Data.Question.Content,
        example_test_cases: .Question.Data.Question.ExampleTestcases,
        code_snippets: .Question.Data.Question.CodeSnippets | map({lang: .LangSlug, code: .Code}),
        is_paid_only: .Question.Data.Question.IsPaidOnly,
        difficulty: .Question.Data.Question.Difficulty,
        likes: .Question.Data.Question.Likes,
        dislikes: .Question.Data.Question.Dislikes,
        category: .Question.Data.Question.CategoryTitle,
        topic_tags: .Question.Data.Question.TopicTags | map(.Name),
        total_submissions: .Question.TotalSubmissions,
        total_accepted: .Question.TotalAccepted,
        acceptance_rate: .Question.AcceptanceRate,
        created_at_approx: if .Question.CreatedAtApprox == "0001-01-01T00:00:00Z" then null else .Question.CreatedAtApprox end,
        solutions: ([.Submissions | to_entries[] | select(.value.CheckResponse.status_msg == "Accepted" and .value.SubmittedAt != "0001-01-01T00:00:00Z")] as $accepted | . as $root | $accepted | map({
                lang: .value.SubmitRequest.lang,
                typed_code: .value.SubmitRequest.typed_code,
                prompt: $root.Solutions[.key].Prompt,
                model: $root.Solutions[.key].Model,
                submitted_at: .value.SubmittedAt
            }) | if length > 0 then . else null end)
        }
END
)

ls problems/*.json | go run . list -p "$p" --header=false -
