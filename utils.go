package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
)

var ErrNonRetriable = errors.New("non-retriable error")

var ErrFatal = errors.New("fatal error")

func NewNonRetriableError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrNonRetriable, err)
}

func NewFatalError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrFatal, err)
}

func humanizeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.DateTime)
}

func fileExists(name string) (bool, error) {
	_, err := os.Stat(name)
	if err == nil {
		// file apparently exists
		return true, nil
	} else {
		// got error, let's see
		if errors.Is(err, os.ErrNotExist) {
			// file not exists, so no actual error here
			return false, nil
		} else {
			// other error
			return false, err
		}
	}
}

// allFilesFromProblemsDir retrieves all JSON files from the problems directory.
func allFilesFromProblemsDir() ([]string, error) {
	fsys := os.DirFS(options.Dir)
	files, err := fs.Glob(fsys, "*.json")
	if err != nil {
		return nil, err
	}

	for i := range files {
		files[i] = path.Join(options.Dir, files[i])
	}
	return files, nil
}

// filenamesFromArgs processes a list of arguments and returns a list of filenames.
// If the first argument is "-", it reads filenames from standard input, ignoring lines
// that are empty or start with a comment character (#).
func filenamesFromArgs(args []string) ([]string, error) {
	if len(args) == 0 || args[0] != "-" {
		return args, nil
	}

	scanner := bufio.NewScanner(os.Stdin)
	files := []string{}
	commentPattern := regexp.MustCompile(`^\s*#`)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if len(line) > 0 && !commentPattern.MatchString(line) {
			files = append(files, line)
		}
	}
	err := scanner.Err()
	if err != nil {
		return nil, err
	}

	return files, nil
}
