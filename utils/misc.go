package utils

import (
	"fmt"
	"log/slog"
	"os"
	"time"
)

const APP_NAME = "skuld-cli"

func CheckErr(err error) {
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func ToSeconds(duration time.Duration) string {
	durationInSecondes := (duration + time.Second - 1) / time.Second
	return fmt.Sprintf("%ds", durationInSecondes)
}
