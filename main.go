// inspired by https://github.com/nikhil-ravi/LeetScrape

package main

import (
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var PREFERRED_LANGUAGES = []string{"python3", "python"}

func initConfig() {
	viper.AddConfigPath(".")
	viper.SetConfigName("config.production")
	viper.SetConfigType("yaml")

	err := viper.ReadInConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to read config file")
	}
}

func initVerbosity() {
	if viper.GetInt("verbose") >= 2 {
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	} else if viper.GetInt("verbose") >= 1 {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

func main() {
	cobra.OnInitialize(initConfig, initVerbosity)
	consoleWriter := zerolog.NewConsoleWriter()
	consoleWriter.TimeFormat = time.DateTime
	log.Logger = zerolog.New(consoleWriter).With().Timestamp().Logger()

	rootCmd := &cobra.Command{Use: "leetgptsolver", CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true}}
	rootCmd.PersistentFlags().BoolP("force", "f", false, "be forceful: download already downloaded, submit already submitted etc.")
	rootCmd.PersistentFlags().StringP("dir", "d", "problems", "")
	rootCmd.PersistentFlags().CountP("verbose", "v", "increase verbosity level. Use -v for troubleshooting, -vv for advanced debugging")
	viper.BindPFlags(rootCmd.PersistentFlags())

	cmdDownload := &cobra.Command{
		Use:   "download",
		Short: "Download problems from leetcode",
		Run: func(cmd *cobra.Command, args []string) {
			download(args)
		},
	}
	cmdDownload.Flags().BoolP("slugs", "s", false, "list available problem slugs without downloading")
	viper.BindPFlags(cmdDownload.Flags())

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
			viper.BindPFlag("model", cmd.PersistentFlags().Lookup("model"))
			prompt(args)
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
