package mailer

import (
	"bytes"
	"embed"
	"html/template"
)

//go:embed templates/*.html
var templateFS embed.FS

var templates = template.Must(template.ParseFS(templateFS, "templates/*.html"))

func renderTemplate(name string, data any) (string, error) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, name, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
