// inspired by https://github.com/nikhil-ravi/LeetScrape

package main

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var PREFERRED_LANGUAGES = []string{"python3", "python"}

var options struct {
	// options below usually set by command line flags, but can also be set in config file
	Force         bool
	Verbose       int
	Dir           string
	DryRun        bool `mapstructure:"dry_run"`
	Slugs         bool
	SkipPaid      bool `mapstructure:"skip_paid"`
	Model         string
	Retries       int
	CheckRetries  int `mapstructure:"check_retries"`
	SubmitRetries int `mapstructure:"submit_retries"`
	Output        string

	// options below usually set in config file
	ChatgptApiKey         string `mapstructure:"chatgpt_api_key"`
	GeminiProjectId       string `mapstructure:"gemini_project_id"`
	GeminiRegion          string `mapstructure:"gemini_region"`
	GeminiCredentialsFile string `mapstructure:"gemini_credentials_file"`
	ClaudeApiKey          string `mapstructure:"claude_api_key"`
	DeepseekApiKey        string `mapstructure:"deepseek_api_key"`
	XaiApiKey             string `mapstructure:"xai_api_key"`

	PromptTemplate string `mapstructure:"prompt_template"`
}

func initConfig() {
	viper.AddConfigPath(".")
	viper.SetConfigName("config.production")
	viper.SetConfigType("yaml")

	err := viper.ReadInConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to read config file")
	}

	err = viper.Unmarshal(&options)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to unmarshal flags. This is a bug")
	}
}

func initVerbosity() {
	if options.Verbose >= 2 {
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	} else if options.Verbose >= 1 {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

func main() {
	cobra.OnInitialize(initConfig, initVerbosity)
	consoleWriter := zerolog.NewConsoleWriter()
	consoleWriter.TimeFormat = time.DateTime
	consoleWriter.Out = os.Stderr
	log.Logger = zerolog.New(consoleWriter).With().Timestamp().Logger()

	rootCmd := &cobra.Command{Use: "leetgptsolver", CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true}}
	rootCmd.PersistentFlags().BoolP("force", "f", false, "be forceful: download already downloaded, submit already submitted etc.")
	rootCmd.PersistentFlags().StringP("dir", "D", "problems", "")
	rootCmd.PersistentFlags().BoolP("dry_run", "d", false, "do not make any changes to problem files")
	rootCmd.PersistentFlags().CountP("verbose", "v", "increase verbosity level. Use -v for troubleshooting, -vv for advanced debugging")
	err := viper.BindPFlags(rootCmd.PersistentFlags())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to bind flags. This is a bug")
	}

	cmdDownload := &cobra.Command{
		Use:   "download",
		Short: "Download problems from leetcode",
		Run: func(cmd *cobra.Command, args []string) {
			viper.BindPFlag("slugs", cmd.Flags().Lookup("slugs"))
			viper.BindPFlag("skip_paid", cmd.Flags().Lookup("skip_paid"))
			viper.Unmarshal(&options)
			download(args)
		},
	}
	cmdDownload.Flags().BoolP("slugs", "s", false, "list available problem slugs without downloading")
	cmdDownload.Flags().BoolP("skip_paid", "P", false, "skip paid problems")

	cmdList := &cobra.Command{
		Use:   "list",
		Short: "List problems info using jq",
		Run: func(cmd *cobra.Command, args []string) {
			list(args, cmd.Flag("where").Value.String(), cmd.Flag("print").Value.String())
		},
	}
	cmdList.Flags().StringP("where", "w", "", "filter problems by where clause (using jq)")
	cmdList.Flags().StringP("print", "p", ".", "print fields (using jq)")

	cmdPrompt := &cobra.Command{
		Use:   "prompt",
		Short: "Prompt for a solution",
		Run: func(cmd *cobra.Command, args []string) {
			viper.BindPFlag("retries", cmd.Flags().Lookup("retries"))
			viper.Unmarshal(&options)
			prompt(args, cmd.Flag("model").Value.String())
		},
	}
	cmdPrompt.PersistentFlags().StringP("model", "m", "", "large language model family name to use")
	cmdPrompt.PersistentFlags().IntP("retries", "r", 2, "number of retries")

	cmdSubmit := &cobra.Command{
		Use:   "submit",
		Short: "Submit a solution",
		Run: func(cmd *cobra.Command, args []string) {
			viper.BindPFlag("submit_retries", cmd.Flags().Lookup("submit_retries"))
			viper.BindPFlag("check_retries", cmd.Flags().Lookup("check_retries"))
			viper.Unmarshal(&options)
			submit(args, cmd.Flag("model").Value.String())
		},
	}
	cmdSubmit.Flags().Int("submit_retries", 2, "number of retries")
	cmdSubmit.Flags().Int("check_retries", 5, "number of retries")
	cmdSubmit.PersistentFlags().StringP("model", "m", "", "large language model family name to use")

	cmdReport := &cobra.Command{
		Use:   "report",
		Short: "Generate a report",
		Run: func(cmd *cobra.Command, args []string) {
			viper.BindPFlag("output", cmd.Flags().Lookup("output"))
			viper.Unmarshal(&options)
			report(args)
		},
	}
	cmdReport.Flags().StringP("output", "o", "report.tsv", "")

	cmdFix := &cobra.Command{
		Use:   "fix",
		Short: "Fix problems",
		Run: func(cmd *cobra.Command, args []string) {
			fix(args)
		},
	}

	rootCmd.AddCommand(cmdDownload, cmdList, cmdPrompt, cmdSubmit, cmdReport, cmdFix)

	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}
