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

var options struct {
	// options below usually set by command line flags, but can also be set in config file
	Force         bool
	Verbose       int
	Dir           string
	DryRun        bool `mapstructure:"dry_run"`
	Slugs         bool
	CreationDate  bool `mapstructure:"creation_date"`
	SkipPaid      bool `mapstructure:"skip_paid"`
	SkipAuthCheck bool `mapstructure:"skip_auth_check"`
	Overwrite     bool
	      string
	Model         string
	Retries       int
	CheckRetries  int `mapstructure:"check_retries"`
	SubmitRetries int `mapstructure:"submit_retries"`

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
			viper.BindPFlag("skip_auth_check", cmd.Flags().Lookup("skip_auth_check"))
			viper.BindPFlag("creation_date", cmd.Flags().Lookup("creation_date"))
			viper.BindPFlag("overwrite", cmd.Flags().Lookup("overwrite"))
			viper.Unmarshal(&options)
			download(args)
		},
	}
	cmdDownload.Flags().BoolP("slugs", "s", false, "list available problem slugs without downloading")
	cmdDownload.Flags().BoolP("skip_paid", "P", false, "skip paid problems")
	cmdDownload.Flags().BoolP("skip_auth_check", "A", false, "allow anonymous download (disable username check)")
	cmdDownload.Flags().BoolP("creation_date", "C", true, "determine approximate creation date for each problem based on user-generated content")
	cmdDownload.Flags().BoolP("overwrite", "o", false, "overwrite existing problems")

	cmdList := &cobra.Command{
		Use:   "list",
		Short: "List problems info using jq",
		Run: func(cmd *cobra.Command, args []string) {
			list(args, cmd.Flag("where").Value.String(), cmd.Flag("print").Value.String(), cmd.Flag("header").Value.String() == "true")
		},
	}
	cmdList.Flags().StringP("where", "w", "", "filter problems by where clause (using jq)")
	cmdList.Flags().StringP("print", "p", ".", "print fields (using jq)")
	cmdList.Flags().BoolP("header", "H", true, "print header row")

	cmdPrompt := &cobra.Command{
		Use:   "prompt",
		Short: "Prompt for a solution",
		Run: func(cmd *cobra.Command, args []string) {
			viper.BindPFlag("language", cmd.Flags().Lookup("language"))
			viper.BindPFlag("retries", cmd.Flags().Lookup("retries"))
			viper.Unmarshal(&options)
			prompt(args, cmd.Flag("language").Value.String(), cmd.Flag("model").Value.String())
		},
	}
	cmdPrompt.PersistentFlags().StringP("language", "l", "python3", "programming language")
	cmdPrompt.PersistentFlags().StringP("model", "m", "", "large  model family name to use")
	cmdPrompt.PersistentFlags().IntP("retries", "r", 2, "number of retries")

	cmdSubmit := &cobra.Command{
		Use:   "submit",
		Short: "Submit a solution",
		Run: func(cmd *cobra.Command, args []string) {
			viper.BindPFlag("language", cmd.Flags().Lookup("language"))
			viper.BindPFlag("submit_retries", cmd.Flags().Lookup("submit_retries"))
			viper.BindPFlag("check_retries", cmd.Flags().Lookup("check_retries"))
			viper.Unmarshal(&options)
			submit(args, cmd.Flag("language").Value.String(), cmd.Flag("model").Value.String())
		},
	}
	cmdSubmit.PersistentFlags().StringP("language", "l", "python3", "programming language")
	cmdSubmit.Flags().Int("submit_retries", 2, "number of retries")
	cmdSubmit.Flags().Int("check_retries", 5, "number of retries")
	cmdSubmit.PersistentFlags().StringP("model", "m", "", "large  model family name to use")

	cmdFix := &cobra.Command{
		Use:   "fix",
		Short: "Fix problems",
		Run: func(cmd *cobra.Command, args []string) {
			fix(args)
		},
	}

	rootCmd.AddCommand(cmdDownload, cmdList, cmdPrompt, cmdSubmit, cmdFix)

	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}
