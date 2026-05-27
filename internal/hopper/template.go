package hopper

import (
	"math/rand"
	"strings"
	"text/template"
)

// Template renders a column value template using "{" / "}" delimiters,
// matching the user-facing syntax like "{ .Random }-{ .Index }".
type Template struct {
	tmpl *template.Template
}

// NewTemplate compiles a column value template.
func NewTemplate(text string) (*Template, error) {
	t, err := template.New("col").Delims("{", "}").Parse(text)
	if err != nil {
		return nil, err
	}
	return &Template{tmpl: t}, nil
}

// templateData is the context exposed to templates.
type templateData struct {
	Index int
	rng   *rand.Rand
}

// Random returns a fresh random token each time it is referenced.
func (d *templateData) Random() string {
	return randString(d.rng, 8)
}

// Render evaluates the template for the given row index.
func (t *Template) Render(index int, rng *rand.Rand) (string, error) {
	var sb strings.Builder
	if err := t.tmpl.Execute(&sb, &templateData{Index: index, rng: rng}); err != nil {
		return "", err
	}
	return sb.String(), nil
}
