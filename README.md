# leetgptsolver

Research to identify the capabilities of LLMs in solving coding interview problems.

## Publications

* [Comparing LLMs' Coding Abilities Across Programming Languages](https://hackernoon.com/comparing-llms-coding-abilities-across-programming-languages)
* [Testing LLMs on Solving Leetcode Problems in 2025](https://hackernoon.com/testing-llms-on-solving-leetcode-problems-in-2025)
* [Testing LLMs on Solving Leetcode Problems](https://hackernoon.com/testing-llms-on-solving-leetcode-problems) (2024)

## Usage

To interact with leetcode.com, please sign up and sign in with your Leetcode account using the Firefox browser.

Copy `config.yaml` to `config.production.yaml` and fill in your API keys / settings, then build or run via Go:

```bash
make build
./leetgptsolver --help
```

## Dataset

The dataset used for this research is available on Hugging Face: https://huggingface.co/datasets/whiskwhite/leetcode-complete

### Updating the dataset

1. **Back up `problems/`**:

   ```bash
   cp -R problems problems.$(date +%Y%m%d).bak
   ```

2. **Download new problems** (requires being signed in to leetcode.com in Firefox):

   ```bash
   go run . download -c all -v
   ```

3. **Refresh metadata/stats for existing problems** (likes, dislikes, acceptance rate, etc., while keeping solutions and submissions intact):

   ```bash
   ./update-problems-metadata.py -n 100        # latest 100 problems
   ./update-problems-metadata.py -r -n 100     # 100 random problems
   ```

4. **Prompt and submit unsolved problems** (optional):

   ```bash
   # Algorithms problems
   ./list-unsolved-problems.sh | go run . prompt -l python3 -m <model> -
   ./list-unsolved-problems.sh | go run . submit -l python3 -m <model> -

   # Database problems (use a SQL language such as mysql or postgresql)
   ./list-unsolved-problems.sh | go run . prompt -l mysql -m <model> -
   ./list-unsolved-problems.sh | go run . submit -l mysql -m <model> -
   ```

   Problems without a matching code snippet for the chosen `-l` language are skipped automatically.

5. **Generate and split the JSONL dataset**:

   ```bash
   ./generate-dataset.py --output-file dataset.jsonl
   ./split-dataset.py dataset.jsonl --output-dir splits
   ```

   This produces `splits/train.jsonl`, `splits/validation.jsonl`, `splits/test.jsonl`, and `splits/unsolved.jsonl`, ready to upload to Hugging Face.

## TODO

- [x] Add support for the database problem category
- [ ] Add support for shell, concurrency and other problem categories
- [x] Make command-line help self-explanatory
- [x] Make logging/output more informative, especially when downloading questions
- [x] Add structure to the config and options
- [x] Add version information
- [x] Add authorization check before downloading problems
- [x] Detect a question creation time
- [ ] Implement a simple custom downloading queue
- [x] Add jq filtering for reporting
- [ ] Implement locks for problem files
- [x] Implement a real rate limiter instead of SimpleThrottler
- [x] Support selecting the programming language

## License

MIT
