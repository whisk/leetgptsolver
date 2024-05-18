// inspired by https://github.com/nikhil-ravi/LeetScrape

package main

import (
	"bufio"
	"errors"
	fs "io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var PREFERRED_LANGUAGES = []string{"python3", "python"}

func init() {
	// prompt
	flag.StringP("model", "m", "", "large language model family name to use")

	// general
	flag.BoolP("force", "f", false, "be forceful: download already downloaded, submit already submitted etc.")
	flag.StringP("output", "o", "report.tsv", "")
	flag.BoolP("help", "h", false, "show this help")
	flag.StringP("dir", "d", "problems", "")
	flag.BoolP("list", "l", false, "print list of problems, but do not download")
	flag.CountP("verbose", "v", "increase verbosity level. Use -v for troubleshooting, -vv for advanced debugging")

	err := viper.BindPFlags(flag.CommandLine)
	if err != nil {
		os.Stderr.WriteString("failed to bind flags: " + err.Error())
	}
	flag.Parse()
}

func main() {
	if viper.GetBool("help") || flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}

	if viper.GetInt("verbose") >= 2 {
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	} else if viper.GetInt("verbose") >= 1 {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
	consoleWriter := zerolog.NewConsoleWriter()
	consoleWriter.TimeFormat = time.DateTime
	log.Logger = zerolog.New(consoleWriter).With().Timestamp().Logger()

	viper.AddConfigPath(".")
	viper.SetConfigName("config.production")
	viper.SetConfigType("yaml")

	err := viper.ReadInConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to read config file")
	}

	command := flag.Args()[0]
	fileNames := flag.Args()[1:]

	fileNames, err = getActualFiles(fileNames)
	if err != nil {
		log.Err(err).Msg("Failed to get the files list")
		return
	}

	if command == "download" {
		download(fileNames)
	} else if command == "prompt" {
		prompt(fileNames)
	} else if command == "submit" {
		submit(fileNames)
	} else if command == "report" {
		report(fileNames)
	} else if command == "fix" {
		fix(fileNames)
	} else {
		log.Error().Msgf("unknown command %s", command)
	}
}

func getProblemsFiles() ([]string, error) {
	fsys := os.DirFS(viper.GetString("dir"))
	files, err := fs.Glob(fsys, "*.json")
	if err != nil {
		return nil, err
	}

	for i := range files {
		files[i] = path.Join(viper.GetString("dir"), files[i])
	}
	return files, nil
}

func getActualFiles(files []string) ([]string, error) {
	if len(files) == 0 {
		return []string{}, errors.New("no files given")
	} else if files[0] != "-" {
		return files, nil
	}

	scanner := bufio.NewScanner(os.Stdin)
	files = []string{}
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
