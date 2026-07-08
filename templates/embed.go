package templates

import (
	"embed"
	"io"
	"strings"
	"text/template"

	"github.com/lyssar/witness-cli/utils"
)

//go:embed *.gotmpl manifests/*.gotmpl
var TemplateFs embed.FS

type Renderer struct {
	Tmpl *template.Template
}

func NewRenderer() (*Renderer, error) {
	tmpl, err := template.New("root").
		Funcs(template.FuncMap{
			"encryptSecret": utils.EncryptSecret,
			"ToLower":       strings.ToLower,
			"ToSeconds":     utils.ToSeconds,
		}).
		ParseFS(
			TemplateFs,
			"*.gotmpl",
			"manifests/*.gotmpl",
		)

	if err != nil {
		return nil, err
	}
	return &Renderer{Tmpl: tmpl}, nil
}

func (r *Renderer) Render(name string, data any, w io.Writer) error {
	return r.Tmpl.ExecuteTemplate(w, name, data)
}
