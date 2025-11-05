package internal

import (
	"os"
	"path/filepath"

	"github.com/lyssar/skuld-cli/templates"
	"github.com/lyssar/skuld-cli/utils"
)

func (observer Observer) Configure() error {
	utils.LogInfo("Configuring observer", "service-name", observer.Name)

	outPath := filepath.Join("", "")
	composeFile, err := os.Create(outPath)

	utils.CheckErr(err)
	defer composeFile.Close()
	tmpl := templates.NewSystemdTemplate()
	tmpl.ExecuteTemplate(composeFile, "docker-compose.tmpl", map[string]any{})
	return nil
}
