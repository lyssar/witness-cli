package templates

import (
	"embed"
	"text/template"
)

//go:embed *.tmpl
var SystemdFs embed.FS

func NewSystemdTemplate() *template.Template {
	tmpl := template.Must(template.New("").ParseFS(SystemdFs, "*.tmpl"))
	return tmpl
}
