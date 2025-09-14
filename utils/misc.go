package utils

import (
	"log/slog"
	"os"
)

const APP_NAME = "skuld-cli"

func CheckErr(err error) {
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
