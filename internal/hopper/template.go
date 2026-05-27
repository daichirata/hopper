package hopper

import (
	"math/rand"
	"strings"
	"text/template"
)

type Template struct {
	tmpl *template.Template
}

func NewTemplate(text string) (*Template, error) {
	t, err := template.New("col").Delims("{", "}").Parse(text)
	if err != nil {
		return nil, err
	}
	return &Template{tmpl: t}, nil
}

type templateData struct {
	Index int
	rng   *rand.Rand
}

func (d *templateData) Random() string {
	return randString(d.rng, 8)
}

func (t *Template) Render(index int, rng *rand.Rand) (string, error) {
	var sb strings.Builder
	if err := t.tmpl.Execute(&sb, &templateData{Index: index, rng: rng}); err != nil {
		return "", err
	}
	return sb.String(), nil
}
