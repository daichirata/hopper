package hopper

import (
	"math/rand"
	"testing"

	"github.com/cloudspannerecosystem/memefish/ast"
)

func newTestGen() *Generator {
	return NewGenerator(rand.New(rand.NewSource(1)))
}

func TestGeneratorRange(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "MarketingBudget", Type: ColumnType{Base: ast.Int64TypeName}}
	for i := 0; i < 200; i++ {
		v, err := g.FromRule(col, ColumnRule{Range: &RangeRule{Min: 0, Max: 10}}, i)
		if err != nil {
			t.Fatalf("FromRule: %v", err)
		}
		n, ok := v.(int64)
		if !ok {
			t.Fatalf("range value type = %T, want int64", v)
		}
		if n < 0 || n > 10 {
			t.Fatalf("range value %d out of [0,10]", n)
		}
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
