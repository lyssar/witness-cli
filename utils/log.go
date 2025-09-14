package utils

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

func structurize(msg any) string {
	transformedMsg, _ := json.MarshalIndent(msg, "", "  ")

	return string(transformedMsg)
}

func DebugStruct(msg any, args ...any) {
	slog.Debug(structurize(msg), args...)
}

func LogSuccess(msg string, args ...any) {
	slog.Info(fmt.Sprintf("\u2705 %s", msg), args...)
}

func LogInfo(msg string, args ...any) {
	slog.Info(fmt.Sprintf("\U0001F6C8 %s", msg), args...)
}
