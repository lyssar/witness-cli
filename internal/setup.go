package internal

func (observer Observer) OldConfigure() error {
	// utils.LogInfo("Configuring observer", "service-name", observer.Name)

	// TODO check if .service and .timer for oberser.Name exists, if not
	//      create .service and .timer for observer.Name
	//
	// outPath := filepath.Join("", "")
	// composeFile, err := os.Create(outPath)

	// utils.CheckErr(err)
	// defer composeFile.Close()
	// tmpl := templates.NewSystemdTemplate()
	// tmpl.ExecuteTemplate(composeFile, "docker-compose.tmpl", map[string]any{})
	return nil
}
