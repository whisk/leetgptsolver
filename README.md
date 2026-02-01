# leetgptsolver

Research to identify the capabilities of LLMs in solving coding interview problems.

## Publications

* [Testing LLMs on Solving Leetcode Problems in 2025
](https://hackernoon.com/testing-llms-on-solving-leetcode-problems-in-2025)
* [Testing LLMs on Solving Leetcode Problems](https://hackernoon.com/testing-llms-on-solving-leetcode-problems) (2024)

## Usage

To interact with leetcode.com, please sign up and sign in with your Leetcode account using the Firefox browser.

## Dataset

The dataset used for this research is available on Hugging Face: https://huggingface.co/datasets/whiskwhite/leetcode-complete

## TODO

- [x] Add support for the database problem category
- [ ] Add support for shell, concurrency and other problem categories
- [x] Make command-line help self-explanatory
- [x] Make logging/output more informative, especially when downloading questions
- [x] Add structure to the config and options
- [ ] Add version information
- [x] Add authorization check before downloading problems
- [x] Detect a question creation time
- [ ] Implement a simple custom downloading queue
- [x] Add jq filtering for reporting
- [ ] Implement locks for problem files
- [ ] Implement a real rate limiter instead of SimpleThrottler
- [x] Support selecting the programming language

## License

MIT
