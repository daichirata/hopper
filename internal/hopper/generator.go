package hopper

import (
	"fmt"
	"math/big"
	"math/rand"
	"strconv"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/google/uuid"
)

const defaultStringLen = 12

type Generator struct {
	rng       *rand.Rand
	templates map[string]*Template
}

func NewGenerator(rng *rand.Rand) *Generator {
	return &Generator{rng: rng, templates: map[string]*Template{}}
}

func (g *Generator) FromRule(col *Column, rule ColumnRule, index int) (any, error) {
	switch {
	case rule.Template != "":
		t, err := g.template(rule.Template)
		if err != nil {
			return nil, fmt.Errorf("column %s: %w", col.Name, err)
		}
		s, err := t.Render(index, g.rng)
		if err != nil {
			return nil, fmt.Errorf("column %s: template: %w", col.Name, err)
		}
		v, err := g.coerceString(col, s)
		if err != nil {
			return nil, fmt.Errorf("column %s: %w", col.Name, err)
		}
		return v, nil
	case rule.Range != nil:
		v := rule.Range.Min
		if rule.Range.Max > rule.Range.Min {
			v += g.rng.Int63n(rule.Range.Max - rule.Range.Min + 1)
		}
		out, err := g.coerceInt(col, v)
		if err != nil {
			return nil, fmt.Errorf("column %s: %w", col.Name, err)
		}
		return out, nil
	default:
		return g.Default(col)
	}
}

func (g *Generator) template(text string) (*Template, error) {
	if t, ok := g.templates[text]; ok {
		return t, nil
	}
	t, err := NewTemplate(text)
	if err != nil {
		return nil, err
	}
	g.templates[text] = t
	return t, nil
}

func (g *Generator) Unique(col *Column, index int) (any, error) {
	if col.Type.IsArray {
		return g.Default(col)
	}
	switch col.Type.Base {
	case ast.StringTypeName:
		return uuid.NewString(), nil
	case ast.BytesTypeName:
		return []byte(uuid.NewString()), nil
	case ast.Int64TypeName:
		return int64(index) + 1, nil
	case ast.Float64TypeName:
		return float64(index) + 1, nil
	case ast.Float32TypeName:
		return float32(index) + 1, nil
	case ast.NumericTypeName:
		return big.NewRat(int64(index)+1, 1), nil
	case ast.TimestampTypeName:
		return time.Now().Add(time.Duration(index) * time.Microsecond), nil
	case ast.DateTypeName:
		return civil.DateOf(time.Now().AddDate(0, 0, index)), nil
	case ast.BoolTypeName:
		return index%2 == 0, nil
	default:
		return g.Default(col)
	}
}

func (g *Generator) Default(col *Column) (any, error) {
	if col.Type.IsArray {
		return g.defaultArray(col)
	}
	return g.scalar(col.Type.Base, col.Type.Size)
}

func (g *Generator) scalar(base ast.ScalarTypeName, size int64) (any, error) {
	switch base {
	case ast.StringTypeName:
		return randString(g.rng, stringLen(size)), nil
	case ast.BytesTypeName:
		return randBytes(g.rng, stringLen(size)), nil
	case ast.Int64TypeName:
		return g.rng.Int63(), nil
	case ast.Float64TypeName:
		return g.rng.Float64(), nil
	case ast.Float32TypeName:
		return float32(g.rng.Float64()), nil
	case ast.BoolTypeName:
		return g.rng.Intn(2) == 0, nil
	case ast.NumericTypeName:
		return big.NewRat(g.rng.Int63n(1_000_000), 100), nil
	case ast.TimestampTypeName:
		return time.Now().Add(-time.Duration(g.rng.Intn(86400)) * time.Second), nil
	case ast.DateTypeName:
		return civil.DateOf(time.Now().AddDate(0, 0, -g.rng.Intn(365))), nil
	case ast.JSONTypeName:
		return spanner.NullJSON{Value: map[string]any{"v": randString(g.rng, 6)}, Valid: true}, nil
	default:
		return randString(g.rng, stringLen(size)), nil
	}
}

func (g *Generator) defaultArray(col *Column) (any, error) {
	n := 1 + g.rng.Intn(3)
	switch col.Type.Base {
	case ast.StringTypeName:
		out := make([]string, n)
		for i := range out {
			out[i] = randString(g.rng, stringLen(col.Type.Size))
		}
		return out, nil
	case ast.Int64TypeName:
		out := make([]int64, n)
		for i := range out {
			out[i] = g.rng.Int63()
		}
		return out, nil
	case ast.Float64TypeName:
		out := make([]float64, n)
		for i := range out {
			out[i] = g.rng.Float64()
		}
		return out, nil
	case ast.BoolTypeName:
		out := make([]bool, n)
		for i := range out {
			out[i] = g.rng.Intn(2) == 0
		}
		return out, nil
	default:
		v, err := g.scalar(col.Type.Base, col.Type.Size)
		if err != nil {
			return nil, err
		}
		return []any{v}, nil
	}
}

func (g *Generator) coerceString(col *Column, s string) (any, error) {
	if col.Type.IsArray {
		return nil, fmt.Errorf("template not supported for ARRAY column")
	}
	switch col.Type.Base {
	case ast.StringTypeName:
		return s, nil
	case ast.BytesTypeName:
		return []byte(s), nil
	case ast.Int64TypeName:
		return strconv.ParseInt(s, 10, 64)
	case ast.Float64TypeName:
		return strconv.ParseFloat(s, 64)
	case ast.Float32TypeName:
		f, err := strconv.ParseFloat(s, 32)
		return float32(f), err
	case ast.BoolTypeName:
		return strconv.ParseBool(s)
	default:
		return s, nil
	}
}

func (g *Generator) coerceInt(col *Column, v int64) (any, error) {
	if col.Type.IsArray {
		return nil, fmt.Errorf("range not supported for ARRAY column")
	}
	switch col.Type.Base {
	case ast.Int64TypeName:
		return v, nil
	case ast.Float64TypeName:
		return float64(v), nil
	case ast.Float32TypeName:
		return float32(v), nil
	case ast.StringTypeName:
		return strconv.FormatInt(v, 10), nil
	case ast.NumericTypeName:
		return big.NewRat(v, 1), nil
	default:
		return v, nil
	}
}

func stringLen(size int64) int {
	if size > 0 && size < defaultStringLen {
		return int(size)
	}
	return defaultStringLen
}

const alphanum = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randString(rng *rand.Rand, n int) string {
	if n <= 0 {
		n = 1
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = alphanum[rng.Intn(len(alphanum))]
	}
	return string(b)
}

func randBytes(rng *rand.Rand, n int) []byte {
	if n <= 0 {
		n = 1
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(rng.Intn(256))
	}
	return b
}
