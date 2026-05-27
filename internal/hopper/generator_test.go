package hopper

import (
	"testing"

	"cloud.google.com/go/spanner"
	"github.com/cloudspannerecosystem/memefish/ast"
)

func newTestGen() *Generator {
	return NewGenerator(1)
}

func TestGeneratorPatternNumber(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "MarketingBudget", Type: ColumnType{Base: ast.Int64TypeName}}
	for i := 0; i < 100; i++ {
		v, err := g.Pattern(col, "{{ Number 0 10 }}", i, nil)
		if err != nil {
			t.Fatalf("Pattern: %v", err)
		}
		n, ok := v.(int64)
		if !ok {
			t.Fatalf("value type = %T, want int64", v)
		}
		if n < 0 || n > 10 {
			t.Fatalf("value %d out of [0,10]", n)
		}
	}
}

func TestGeneratorPatternIndex(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "Name", Type: ColumnType{Base: ast.StringTypeName}}
	v, err := g.Pattern(col, "user-{{ Index }}", 7, nil)
	if err != nil {
		t.Fatalf("Pattern: %v", err)
	}
	if v.(string) != "user-7" {
		t.Errorf("Pattern = %q, want user-7", v)
	}
}

func TestGeneratorPatternArithmetic(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "Id", Type: ColumnType{Base: ast.Int64TypeName}}
	v, err := g.Pattern(col, "{{ add Index 1 }}", 0, nil)
	if err != nil {
		t.Fatalf("Pattern: %v", err)
	}
	if v.(int64) != 1 {
		t.Errorf("add Index 1 at index 0 = %v, want 1", v)
	}
}

func TestGeneratorUnique(t *testing.T) {
	g := newTestGen()

	intPK := &Column{Name: "Id", Type: ColumnType{Base: ast.Int64TypeName}}
	if v, _ := g.Unique(intPK, 0); v.(int64) != 1 {
		t.Errorf("Unique(INT64, 0) = %v, want 1", v)
	}
	if v, _ := g.Unique(intPK, 41); v.(int64) != 42 {
		t.Errorf("Unique(INT64, 41) = %v, want 42", v)
	}

	strPK := &Column{Name: "Id", Type: ColumnType{Base: ast.StringTypeName}}
	v, _ := g.Unique(strPK, 0)
	if s, ok := v.(string); !ok || len(s) != 36 {
		t.Errorf("Unique(STRING) = %v (%T), want uuid string", v, v)
	}
}

func TestGeneratorDefaultTypes(t *testing.T) {
	g := newTestGen()
	cases := []struct {
		base ast.ScalarTypeName
		want string
	}{
		{ast.Int64TypeName, "int64"},
		{ast.BoolTypeName, "bool"},
		{ast.Float64TypeName, "float64"},
	}
	for _, c := range cases {
		col := &Column{Name: "c", Type: ColumnType{Base: c.base}}
		v, err := g.Default(col)
		if err != nil {
			t.Fatalf("Default(%s): %v", c.base, err)
		}
		if got := typeName(v); got != c.want {
			t.Errorf("Default(%s) type = %s, want %s", c.base, got, c.want)
		}
	}
}

func TestGeneratorPatternFunctions(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "c", Type: ColumnType{Base: ast.StringTypeName}}
	patterns := []string{
		`{{ Regex "[A-Z]{5}" }}`,
		`{{ RandomString (SliceString "free" "pro" "ent") }}`,
		`{{ FirstName }}`,
		`{{ UUID }}`,
		`{{ LetterN 8 }}`,
		`{{ FirstName }}-{{ Index }}`,
	}
	for _, p := range patterns {
		if _, err := g.Pattern(col, p, 0, nil); err != nil {
			t.Errorf("Pattern(%q): %v", p, err)
		}
	}
}

func TestGeneratorGuess(t *testing.T) {
	g := newTestGen()
	strCol := func(name string) *Column {
		return &Column{Name: name, Type: ColumnType{Base: ast.StringTypeName}}
	}

	for _, name := range []string{"FirstName", "Email", "first_name"} {
		if v, ok := g.Guess(strCol(name)); !ok || v.(string) == "" {
			t.Errorf("Guess(%q) = %v, %v; want a value", name, v, ok)
		}
	}

	if _, ok := g.Guess(strCol("AlbumTitle")); ok {
		t.Error("Guess(AlbumTitle) should miss (no such gofakeit func)")
	}
	if _, ok := g.Guess(strCol("Number")); ok {
		t.Error("Guess(Number) should miss (param-required func)")
	}
	if _, ok := g.Guess(&Column{Name: "Email", Type: ColumnType{Base: ast.Int64TypeName}}); ok {
		t.Error("Guess on a non-string column should miss")
	}
	if _, ok := g.Guess(&Column{Name: "Email", Type: ColumnType{Base: ast.StringTypeName, Size: 3}}); ok {
		t.Error("Guess should miss when the value exceeds the column size")
	}
}

func TestGeneratorArrayPattern(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "Tags", Type: ColumnType{Base: ast.StringTypeName, IsArray: true}}
	v, err := g.Pattern(col, "{{ Word }}", 0, nil)
	if err != nil {
		t.Fatalf("Pattern: %v", err)
	}
	arr, ok := v.([]string)
	if !ok {
		t.Fatalf("value type = %T, want []string", v)
	}
	if len(arr) != 1 {
		t.Errorf("array len = %d, want 1", len(arr))
	}
}

func TestGeneratorInterval(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "Span", Type: ColumnType{Base: ast.IntervalTypeName}}

	v, err := g.Default(col)
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	if _, ok := v.(spanner.Interval); !ok {
		t.Errorf("Default type = %T, want spanner.Interval", v)
	}

	pv, err := g.Pattern(col, "P1Y2M3D", 0, nil)
	if err != nil {
		t.Fatalf("Pattern: %v", err)
	}
	if _, ok := pv.(spanner.Interval); !ok {
		t.Errorf("Pattern type = %T, want spanner.Interval", pv)
	}
}

func TestGeneratorPatternColRef(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "Email", Type: ColumnType{Base: ast.StringTypeName}}
	row := map[string]any{"FirstName": "ada"}
	v, err := g.Pattern(col, `{{ Col "FirstName" }}@example.com`, 0, row)
	if err != nil {
		t.Fatalf("Pattern: %v", err)
	}
	if v.(string) != "ada@example.com" {
		t.Errorf("Pattern = %q, want ada@example.com", v)
	}
}

func TestGeneratorCommitTimestamp(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "CreatedAt", Type: ColumnType{Base: ast.TimestampTypeName}, AllowCommitTimestamp: true}
	v, err := g.Default(col)
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	if v != spanner.CommitTimestamp {
		t.Errorf("Default = %v, want spanner.CommitTimestamp", v)
	}
}

func typeName(v any) string {
	switch v.(type) {
	case int64:
		return "int64"
	case bool:
		return "bool"
	case float64:
		return "float64"
	default:
		return "other"
	}
}
