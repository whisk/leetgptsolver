# AGENTS.md — leetgptsolver

Research tool that benchmarks LLMs on LeetCode problems and produces the Hugging Face dataset [`whiskwhite/leetcode-complete`](https://huggingface.co/datasets/whiskwhite/leetcode-complete).

Single source of truth for AI coding agents (`CLAUDE.md` and `.github/copilot-instructions.md` only point here).

## Ground rules

1. **Ask before running anything that costs money, touches external services, or modifies problem data:**
   - `go run . prompt …` (paid LLM API calls)
   - `go run . submit …` (submits under the user's LeetCode account)
   - `go run . download …` and `./update-problems-metadata.py` (scrapes leetcode.com and rewrites `problems/*.json`)
   - any command or script that edits `problems/*.json` (including `go run . fix`)
   - Do **not** edit `problems/*.json` manually unless explicitly asked. Read-only commands (`list`, `go build`, `go test`, `generate-dataset.py` to stdout or a new file, `split-dataset.py`) are fine.
2. **Back up before bulk edits to `problems/`:** `cp -R problems problems.$(date +%Y%m%d).bak`. `problems/*.json` is gitignored and cannot be recovered by git.
3. **Git:** commit only when asked; never push. Use a descriptive subject line plus a bullet-point body explaining *what* changed and *why*. Don't stage unrelated untracked files.
4. **Secrets:** never read, print, or commit `config.production.yaml` or `.env*` (`config.yaml` is only a template).
5. **Helper scripts:** write new ones in Python (`#!/usr/bin/env python3`, `argparse`, executable). Use bash only for trivial one-pipeline wrappers like `list-unsolved-problems.sh`.

## Build & test

```bash
make build      # builds ./leetgptsolver with version ldflags
go build ./...
go test ./...
```

- Module path is `whisk/leetgptsolver` (internal import: `whisk/leetgptsolver/pkg`). See `go.mod` for the Go version.
- Without a cgo toolchain, use `CGO_ENABLED=0`.
- `submit_test.go` binds `127.0.0.1` (`httptest`); failures with `connect: operation not permitted` in a network-restricted sandbox are environmental.
- Tests don't need `config.production.yaml` (loaded only when a Cobra command runs).
- Dependency updates: `go get -u ./... && go mod tidy`, then build and test. **Keep `github.com/gobwas/glob` at `v0.2.3`** — `v1.x` breaks `gocolly/colly/v2 v2.3.0` (`undefined: glob.Glob`).

## Architecture & gotchas

Single Go binary (Cobra CLI in `main.go`; all code in `main` except `pkg/models.go`). Data flow: `download` → `prompt` → `submit` → `list` / `generate-dataset.py`.

| Command | Files | Purpose |
|---|---|---|
| `download` | `download.go`, `lc.go` | Fetch problems via LeetCode GraphQL; write/update `problems/<slug>.json` |
| `prompt` | `prompt.go` | Send a problem to an LLM; store the solution in the JSON file |
| `submit` | `submit.go` | Submit a stored solution to LeetCode; store the result |
| `list` | `list.go` | Filter/sort/print problem files with jq (`gojq`) |
| `fix` | `fix.go` | Scaffold for one-off data migrations |

- **Stdin file lists:** pass `-` as the first arg to read paths from stdin (blank lines, `#` comments, and surrounding whitespace—including the trailing tab `list` emits on scalar values—are trimmed).
- **`-d/--dry_run` only works in `submit`** — `download` and `prompt` ignore it.
- **`download -c` defaults to `algorithms`.** Pass `-c all` when inputs may include `Database` problems, or they get skipped as `unknown problem`.
- **Auth & config:** LeetCode requests read cookies from the **Firefox default profile only** (`kooky`; Chrome isn't supported). `download` checks for a signed-in user unless `-A` is passed. Runtime config is read from `config.production.yaml` in CWD (fatals if missing); `prompt_template` must contain `{language}`, `{question}`, and `{snippet}`.
- **LLM routing:** vendors are guessed by model-name prefix in `pkg/models.go` (override with `--model_vendor`). `prompt` accepts arbitrary model names—don't update the model lists in `pkg/models.go` unless asked. Google uses **Vertex AI** (service-account JSON); xAI uses the OpenAI-compatible client. Optional params (`model@{json}`) must be canonical JSON (`json.Marshal`, sorted keys, no spaces) and become part of the map key. A fresh `prompt` resets that `[model][lang]` submission to `Submission{}`.
- **Error types (`utils.go`):** `ErrFatal` aborts the run; `ErrNonRetriable` skips retries for one file and continues.

## Problem JSON data model

One file per problem (`problems/<titleSlug>.json`). Keys are Go PascalCase except LeetCode-native fields (`questionFrontendId`, `questionId`, `SubmitRequest`, `CheckResponse`).

- **Always load through `ReadProblem` (`problem.go`) or `go run . list`** — don't parse raw JSON for derived data. `ReadProblem`:
  - recalculates `AcRate`, `TotalSubmissions`, `TotalAccepted`, `AcceptanceRate`, `ContentFeatures`, `CodeSnippetFeatures`, `Url`, `Path`, `Filename` (these may be stale or zero on disk, e.g. after `download -u`).
  - migrates legacy `Solutions`/`Submissions` into **`SolutionsV2`/`SubmissionsV2`** (`[model][lang]`). New code must use the V2 maps.
  - backfills top-level `Problem.CreatedAtApprox`/`DownloadedAt` from the obsolete `Question.*` fields (still the only copy on disk in many older files). Use the top-level fields in new code.
- **Metadata updates (`download -u` → `saveOrUpdateProblem`) must never drop solutions, submissions, or `CreatedAtApprox`.** Guarded by `TestSaveOrUpdateProblem` in `commands_test.go`.
- Zero time (`"0001-01-01T00:00:00Z"`) means unset. "Accepted" means `CheckResponse.status_msg == "Accepted"` (`status_code == 10`). Pre-2024-03-19 submissions have a zero `SubmittedAt` even when accepted.

## Scripts & experiments

- `generate-dataset.py`: `problems/*.json` → JSONL via `go run . list`. **Format 0.3.1 — never remove fields or change types.** Intentionally excludes accepted submissions with a zero `SubmittedAt` (resubmit instead); problems with none get `solutions: null`.
- `split-dataset.py`: JSONL → `train`/`validation`/`test`/`unsolved` (`solutions == null` → `unsolved`; else deterministic `md5(id + "leetgptsolver-v1") % 100` — don't change the salt).
- `update-problems-metadata.py`: refresh metadata/stats for `-n` latest or `-r` random problems (`download -c all -u --detect_approx_creation_date=false`). Needs approval + backup.
- `list-unsolved-problems.sh`: paths of problems with no accepted submission.
- `experiments/`: historical problem sets (`set-*.txt`) and publication reports (`report-*.tsv/txt`); don't rewrite existing files.

## Conventions

- Go: `gofmt`, wrap errors with `fmt.Errorf("…: %w", err)`, log with `zerolog` (to stderr), and return errors from command functions instead of `os.Exit`/`log.Fatal`.
- Keep new logic testable in isolation (`httptest`, `t.TempDir()`); never hit live LeetCode or LLM APIs in tests.
