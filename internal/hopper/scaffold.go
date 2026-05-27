package hopper

import (
	"fmt"
	"strings"
)

func Scaffold(schema *Schema, tables []string, gen *Generator) (string, error) {
	targets, err := scaffoldTargets(schema, tables)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("tables:\n")
	for _, t := range targets {
		fmt.Fprintf(&b, "  %s:\n", t.Name)
		b.WriteString("    rows: 100\n")

		var cols []*Column
		for _, c := range t.Columns {
			if c.Generated || t.IsPrimaryKey(c.Name) {
				continue
			}
			cols = append(cols, c)
		}
		if len(cols) == 0 {
			continue
		}

		b.WriteString("    columns:\n")
		for _, c := range cols {
			if tmpl, ok := gen.Suggest(c); ok {
				fmt.Fprintf(&b, "      %s: %q\n", c.Name, tmpl)
			} else {
				fmt.Fprintf(&b, "      # %s:\n", c.Name)
			}
		}
	}
	return b.String(), nil
}

func scaffoldTargets(schema *Schema, names []string) ([]*Table, error) {
	if len(names) == 0 {
		return schema.Tables, nil
	}
	out := make([]*Table, 0, len(names))
	for _, n := range names {
		t, ok := schema.Table(n)
		if !ok {
			return nil, fmt.Errorf("table %q not found in schema", n)
		}
		out = append(out, t)
	}
	return out, nil
}
