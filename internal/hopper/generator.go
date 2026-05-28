package hopper

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"text/template"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/cloudspannerecosystem/memefish/ast"
)

const defaultStringLen = 12

type Generator struct {
	faker *gofakeit.Faker
}

func NewGenerator(seed uint64) *Generator {
	return &Generator{faker: gofakeit.New(seed)}
}

var templateFuncs = template.FuncMap{
	"add": func(a, b int) int { return a + b },
	"sub": func(a, b int) int { return a - b },
	"mul": func(a, b int) int { return a * b },
	"div": func(a, b int) int {
		if b == 0 {
			return 0
		}
		return a / b
	},
	"mod": func(a, b int) int {
		if b == 0 {
			return 0
		}
		return a % b
	},
}

func (g *Generator) Pattern(col *Column, pattern string, index int, row map[string]any) (any, error) {
	funcs := template.FuncMap{
		"Index": func() int { return index },
		"Col": func(name string) any {
			if v := row[name]; v != nil {
				return v
			}
			return ""
		},
	}
	for name, fn := range templateFuncs {
		funcs[name] = fn
	}
	s, err := g.faker.Template(pattern, &gofakeit.TemplateOptions{Funcs: funcs})
	if err != nil {
		return nil, fmt.Errorf("column %s: %w", col.Name, err)
	}
	v, err := g.coerce(col, s)
	if err != nil {
		return nil, fmt.Errorf("column %s: %w", col.Name, err)
	}
	return v, nil
}

func (g *Generator) Unique(col *Column, index int) (any, error) {
	if col.Type.IsArray {
		return g.Default(col)
	}
	switch col.Type.Base {
	case ast.StringTypeName:
		if s, ok := shortUniqueString(col, index); ok {
			return s, nil
		}
		return g.faker.UUID(), nil
	case ast.BytesTypeName:
		if s, ok := shortUniqueString(col, index); ok {
			return []byte(s), nil
		}
		return []byte(g.faker.UUID()), nil
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
	if col.AllowCommitTimestamp {
		return spanner.CommitTimestamp, nil
	}
	if col.Type.IsArray {
		return g.defaultArray(col)
	}
	return g.scalar(col.Type.Base, col.Type.Size)
}

func (g *Generator) scalar(base ast.ScalarTypeName, size int64) (any, error) {
	switch base {
	case ast.StringTypeName:
		return g.faker.LetterN(uint(stringLen(size))), nil
	case ast.BytesTypeName:
		return []byte(g.faker.LetterN(uint(stringLen(size)))), nil
	case ast.Int64TypeName:
		return g.faker.Int64(), nil
	case ast.Float64TypeName:
		return g.faker.Float64(), nil
	case ast.Float32TypeName:
		return g.faker.Float32(), nil
	case ast.BoolTypeName:
		return g.faker.Bool(), nil
	case ast.NumericTypeName:
		return big.NewRat(int64(g.faker.Number(0, 1_000_000)), 100), nil
	case ast.TimestampTypeName:
		return g.faker.Date(), nil
	case ast.DateTypeName:
		return civil.DateOf(g.faker.Date()), nil
	case ast.JSONTypeName:
		return spanner.NullJSON{Value: map[string]any{"v": g.faker.Word()}, Valid: true}, nil
	case ast.IntervalTypeName:
		return spanner.Interval{
			Months: int32(g.faker.Number(0, 24)),
			Days:   int32(g.faker.Number(0, 28)),
			Nanos:  big.NewInt(0),
		}, nil
	default:
		return g.faker.LetterN(uint(stringLen(size))), nil
	}
}

func (g *Generator) defaultArray(col *Column) (any, error) {
	n := 1 + g.faker.IntN(3)
	vals := make([]any, n)
	for i := range vals {
		v, err := g.scalar(col.Type.Base, col.Type.Size)
		if err != nil {
			return nil, err
		}
		vals[i] = v
	}
	return toSlice(col.Type.Base, vals), nil
}

func (g *Generator) coerce(col *Column, s string) (any, error) {
	v, err := coerceScalar(col.Type.Base, s)
	if err != nil {
		return nil, err
	}
	if col.Type.IsArray {
		return toSlice(col.Type.Base, []any{v}), nil
	}
	return v, nil
}

func coerceScalar(base ast.ScalarTypeName, s string) (any, error) {
	switch base {
	case ast.StringTypeName:
		return s, nil
	case ast.BytesTypeName:
		return []byte(s), nil
	case ast.Int64TypeName:
		return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	case ast.Float64TypeName:
		return strconv.ParseFloat(strings.TrimSpace(s), 64)
	case ast.Float32TypeName:
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 32)
		return float32(f), err
	case ast.BoolTypeName:
		return strconv.ParseBool(strings.TrimSpace(s))
	case ast.NumericTypeName:
		r, ok := new(big.Rat).SetString(strings.TrimSpace(s))
		if !ok {
			return nil, fmt.Errorf("cannot parse %q as NUMERIC", s)
		}
		return r, nil
	case ast.IntervalTypeName:
		return spanner.ParseInterval(strings.TrimSpace(s))
	default:
		return s, nil
	}
}

func toSlice(base ast.ScalarTypeName, vals []any) any {
	switch base {
	case ast.StringTypeName:
		out := make([]string, len(vals))
		for i, v := range vals {
			out[i] = v.(string)
		}
		return out
	case ast.BytesTypeName:
		out := make([][]byte, len(vals))
		for i, v := range vals {
			out[i] = v.([]byte)
		}
		return out
	case ast.Int64TypeName:
		out := make([]int64, len(vals))
		for i, v := range vals {
			out[i] = v.(int64)
		}
		return out
	case ast.Float64TypeName:
		out := make([]float64, len(vals))
		for i, v := range vals {
			out[i] = v.(float64)
		}
		return out
	case ast.Float32TypeName:
		out := make([]float32, len(vals))
		for i, v := range vals {
			out[i] = v.(float32)
		}
		return out
	case ast.BoolTypeName:
		out := make([]bool, len(vals))
		for i, v := range vals {
			out[i] = v.(bool)
		}
		return out
	case ast.NumericTypeName:
		out := make([]*big.Rat, len(vals))
		for i, v := range vals {
			out[i] = v.(*big.Rat)
		}
		return out
	case ast.TimestampTypeName:
		out := make([]time.Time, len(vals))
		for i, v := range vals {
			out[i] = v.(time.Time)
		}
		return out
	case ast.DateTypeName:
		out := make([]civil.Date, len(vals))
		for i, v := range vals {
			out[i] = v.(civil.Date)
		}
		return out
	case ast.JSONTypeName:
		out := make([]spanner.NullJSON, len(vals))
		for i, v := range vals {
			out[i] = v.(spanner.NullJSON)
		}
		return out
	case ast.IntervalTypeName:
		out := make([]spanner.Interval, len(vals))
		for i, v := range vals {
			out[i] = v.(spanner.Interval)
		}
		return out
	default:
		return vals
	}
}

func (g *Generator) Float() float64 {
	return g.faker.Float64()
}

func (g *Generator) Intn(n int) int {
	if n <= 0 {
		return 0
	}
	return g.faker.IntN(n)
}

func (g *Generator) Guess(col *Column) (any, bool) {
	if col.Type.IsArray {
		return nil, false
	}
	if col.Type.Base != ast.StringTypeName && col.Type.Base != ast.BytesTypeName {
		return nil, false
	}
	info := gofakeit.GetFuncLookup(normalizeColumnName(col.Name))
	if info == nil || len(info.Params) > 0 {
		return nil, false
	}
	v, err := info.Generate(g.faker, &gofakeit.MapParams{}, info)
	if err != nil {
		return nil, false
	}
	s, ok := v.(string)
	if !ok {
		return nil, false
	}
	if col.Type.Size > 0 && int64(len(s)) > col.Type.Size {
		return nil, false
	}
	if col.Type.Base == ast.BytesTypeName {
		return []byte(s), true
	}
	return s, true
}

func normalizeColumnName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", ""))
}

func (g *Generator) Suggest(col *Column) (string, bool) {
	if col.Type.IsArray {
		return "", false
	}
	if col.Type.Base != ast.StringTypeName && col.Type.Base != ast.BytesTypeName {
		return "", false
	}
	if gofakeit.GetFuncLookup(normalizeColumnName(col.Name)) == nil {
		return "", false
	}
	tmpl := "{{ " + pascalCase(col.Name) + " }}"
	if _, err := g.faker.Template(tmpl, &gofakeit.TemplateOptions{}); err != nil {
		return "", false
	}
	return tmpl, true
}

func pascalCase(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '_' })
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

func stringLen(size int64) int {
	if size > 0 && size < defaultStringLen {
		return int(size)
	}
	return defaultStringLen
}

func shortUniqueString(col *Column, index int) (string, bool) {
	if col.Type.Size <= 0 || col.Type.Size >= 36 {
		return "", false
	}
	s := strconv.FormatInt(int64(index)+1, 36)
	if int64(len(s)) > col.Type.Size {
		return "", false
	}
	return s, true
}
