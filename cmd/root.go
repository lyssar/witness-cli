package cmd

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/fang"
	sysdjournaldslog "github.com/iguanesolutions/go-systemd/v5/journald/slog"
	"github.com/lmittmann/tint"
	"github.com/lyssar/witness-cli/utils"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "witness",
	Short: "Skuld CLI – the future’s watcher for your fleet",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logLevelStr := strings.ToUpper(os.Getenv("LOG_LEVEL"))
		debug, _ := cmd.Flags().GetBool("debug")
		if debug {
			logLevelStr = "DEBUG"
		}
		logLevel := sysdjournaldslog.GetLogLevel(logLevelStr)

		_ = utils.IsRunningAsSysd()
		w := os.Stderr
		switch logLevel {
		case slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError:
			// skip if logLevel is a standard slog level
		default:
			logLevel = slog.LevelError
		}

		slog.SetDefault(slog.New(
			tint.NewHandler(w, &tint.Options{
				Level:      logLevel,
				TimeFormat: time.RFC3339,
			}),
		))
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := fang.Execute(context.Background(), rootCmd); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "Help message for toggle")
}
