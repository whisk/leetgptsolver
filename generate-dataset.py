#!/usr/bin/env python3
import sys
import argparse
import subprocess
from pathlib import Path

# Generates dataset for HF: https://huggingface.co/datasets/whiskwhite/leetcode-complete
# Format: JSON Lines (jsonl)
# Format version: 0.3.1
# Please avoid removing existing fields or changing their types!
JQ_FILTER = '''
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
        created_at_approx: if .Question.CreatedAtApprox == "0001-01-01T00:00:00Z" then
            if .CreatedAtApprox == "0001-01-01T00:00:00Z" then null else .CreatedAtApprox end
        else
            .Question.CreatedAtApprox
        end,
        solutions: (
                . as $root
                | [
                        if (.SubmissionsV2? != null) then
                            (.SubmissionsV2 | to_entries[] as $modelEntry
                            | $modelEntry.value | to_entries[] as $langEntry
                            | select($langEntry.value.CheckResponse.status_msg == "Accepted" and $langEntry.value.SubmittedAt != "0001-01-01T00:00:00Z")
                            | {
                                    lang: $langEntry.value.SubmitRequest.lang,
                                    typed_code: $langEntry.value.SubmitRequest.typed_code,
                                    prompt: $root.SolutionsV2[$modelEntry.key][$langEntry.key].Prompt,
                                    model: $root.SolutionsV2[$modelEntry.key][$langEntry.key].Model,
                                    submitted_at: $langEntry.value.SubmittedAt
                                })
                        else
                            (.Submissions | to_entries[]
                            | select(.value.CheckResponse.status_msg == "Accepted" and .value.SubmittedAt != "0001-01-01T00:00:00Z")
                            | {
                                    lang: .value.SubmitRequest.lang,
                                    typed_code: .value.SubmitRequest.typed_code,
                                    prompt: $root.Solutions[.key].Prompt,
                                    model: $root.Solutions[.key].Model,
                                    submitted_at: .value.SubmittedAt
                                })
                        end
                    ]
                | if length > 0 then . else null end
            )
        }
'''

def main():
    parser = argparse.ArgumentParser(description="Generate dataset using existing go tool.")
    parser.add_argument("--problems-dir", default="problems", help="Directory containing problem JSON files")
    parser.add_argument("--output-file", help="Output JSONL file (default: stdout)")
    args = parser.parse_args()

    problems_dir = Path(args.problems_dir)

    # Get list of files
    if not problems_dir.is_dir():
        sys.exit(f"Error: {problems_dir} is not a directory")

    # Glob files and sort them to ensure deterministic order (like ls)
    files = sorted(str(p) for p in problems_dir.glob("*.json"))
    if not files:
        sys.exit(f"Error: No json files found in {problems_dir}")

    # Prepare command: go run . list -p "$p" --header=false -
    cmd = ["go", "run", ".", "list", "-p", JQ_FILTER, "--header=false", "-"]

    # Setup output
    stdout_target = sys.stdout
    if args.output_file:
        stdout_target = open(args.output_file, "w")

    try:
        subprocess.run(
            cmd,
            input="\n".join(files),
            stdout=stdout_target,
            stderr=sys.stderr, # Pass stderr through to see potential go build errors
            text=True, # Enable text mode for stdin strings
            check=True # Raise CalledProcessError on non-zero return code
        )

    except subprocess.CalledProcessError as e:
        sys.exit(e.returncode)
    except Exception as e:
        sys.exit(f"Execution failed: {e}")
    finally:
        if args.output_file and stdout_target != sys.stdout:
            stdout_target.close()

if __name__ == "__main__":
    main()
