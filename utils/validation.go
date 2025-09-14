package utils

import (
	"io"
	"log/slog"
	"os"
	"os/exec"
	"regexp"

	sysd "github.com/iguanesolutions/go-systemd/v5"
)

const pattern = `^\d+[kKmMgG]?$`

func IsValidMemory(input string) bool {
	inputMatched, _ := regexp.MatchString(pattern, input)
	return inputMatched
}

func FileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !os.IsNotExist(err)
}

func FolderExists(folder string) bool {
	info, err := os.Stat(folder)
	return err == nil && info.IsDir()
}

func DirIsEmpty(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	return err == io.EOF
}

func BinaryExists(name string) (bool, string) {
	slog.Debug("checking for binary", "name", name)
	path, err := exec.LookPath(name)
	if err != nil {
		slog.Debug("binary not found", "name", name, "error", err.Error())
		return false, ""
	}
	slog.Debug("binary found", "name", name, "path", path)
	return true, path
}

func IsRunningAsSysd() bool {
	_, sysdStarted := sysd.GetInvocationID()
	return sysdStarted
}
