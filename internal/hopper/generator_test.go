package hopper

import (
	"math/big"
	"testing"
	"time"

	"cloud.google.com/go/civil"
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
		v, err := g.Pattern(col, "{{ Number 0 10 }}", i, nil, nil)
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
	v, err := g.Pattern(col, "user-{{ Index }}", 7, nil, nil)
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
	v, err := g.Pattern(col, "{{ add Index 1 }}", 0, nil, nil)
	if err != nil {
		t.Fatalf("Pattern: %v", err)
	}
	if v.(int64) != 1 {
		t.Errorf("add Index 1 at index 0 = %v, want 1", v)
	}
}

func TestGeneratorPatternPrintf(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "Id", Type: ColumnType{Base: ast.StringTypeName}}
	v, err := g.Pattern(col, `user-{{ printf "%010d" Index }}`, 7, nil, nil)
	if err != nil {
		t.Fatalf("Pattern: %v", err)
	}
	if v.(string) != "user-0000000007" {
		t.Errorf("Pattern = %q, want user-0000000007", v)
	}
}

func TestGeneratorPatternIndexSelect(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "Category", Type: ColumnType{Base: ast.StringTypeName}}
	pattern := `{{ index (SliceString "a" "b" "c") (mod Index 3) }}`
	want := []string{"a", "b", "c", "a", "b", "c"}
	for i, w := range want {
		v, err := g.Pattern(col, pattern, i, nil, nil)
		if err != nil {
			t.Fatalf("Pattern(%d): %v", i, err)
		}
		if v.(string) != w {
			t.Errorf("index %d: Pattern = %q, want %q", i, v, w)
		}
	}
}

func TestGeneratorPatternConditional(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "Label", Type: ColumnType{Base: ast.StringTypeName}}
	pattern := `{{ if eq (mod Index 2) 0 }}even{{ else }}odd{{ end }}`
	for i, w := range []string{"even", "odd", "even", "odd"} {
		v, err := g.Pattern(col, pattern, i, nil, nil)
		if err != nil {
			t.Fatalf("Pattern(%d): %v", i, err)
		}
		if v.(string) != w {
			t.Errorf("index %d: Pattern = %q, want %q", i, v, w)
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
		if _, err := g.Pattern(col, p, 0, nil, nil); err != nil {
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
	v, err := g.Pattern(col, "{{ Word }}", 0, nil, nil)
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

	pv, err := g.Pattern(col, "P1Y2M3D", 0, nil, nil)
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
	v, err := g.Pattern(col, `{{ Col "FirstName" }}@example.com`, 0, row, nil)
	if err != nil {
		t.Fatalf("Pattern: %v", err)
	}
	if v.(string) != "ada@example.com" {
		t.Errorf("Pattern = %q, want ada@example.com", v)
	}
}

func TestGeneratorPatternNull(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "PushedAt", Type: ColumnType{Base: ast.TimestampTypeName}}
	v, err := g.Pattern(col, "{{ Null }}", 0, nil, nil)
	if err != nil {
		t.Fatalf("Pattern: %v", err)
	}
	if v != nil {
		t.Errorf("Pattern = %v, want nil", v)
	}
}

func TestGeneratorPatternNullConditional(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "PushedAt", Type: ColumnType{Base: ast.Int64TypeName}}
	pattern := `{{ if eq (mod Index 3) 0 }}{{ Null }}{{ else }}{{ Index }}{{ end }}`
	for i := 0; i < 9; i++ {
		v, err := g.Pattern(col, pattern, i, nil, nil)
		if err != nil {
			t.Fatalf("Pattern(%d): %v", i, err)
		}
		if i%3 == 0 {
			if v != nil {
				t.Errorf("index %d: Pattern = %v, want nil", i, v)
			}
			continue
		}
		if v.(int64) != int64(i) {
			t.Errorf("index %d: Pattern = %v, want %d", i, v, i)
		}
	}
}

func TestGeneratorPatternNullNotNull(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "PushedAt", Type: ColumnType{Base: ast.TimestampTypeName}, NotNull: true}
	if _, err := g.Pattern(col, "{{ Null }}", 0, nil, nil); err == nil {
		t.Fatal("Pattern on NOT NULL column: want error, got nil")
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

func TestGeneratorUniqueShortString(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "Id", Type: ColumnType{Base: ast.StringTypeName, Size: 8}}
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		v, err := g.Unique(col, i)
		if err != nil {
			t.Fatalf("Unique(STRING(8), %d): %v", i, err)
		}
		s := v.(string)
		if int64(len(s)) > col.Type.Size {
			t.Errorf("Unique(STRING(8), %d) = %q (len %d), exceeds size", i, s, len(s))
		}
		if seen[s] {
			t.Errorf("Unique(STRING(8), %d) = %q, duplicate", i, s)
		}
		seen[s] = true
	}
}

func TestGeneratorUniqueShortBytes(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "Id", Type: ColumnType{Base: ast.BytesTypeName, Size: 4}}
	v, err := g.Unique(col, 0)
	if err != nil {
		t.Fatalf("Unique: %v", err)
	}
	b := v.([]byte)
	if int64(len(b)) > col.Type.Size {
		t.Errorf("Unique(BYTES(4)) len = %d, want <= %d", len(b), col.Type.Size)
	}
}

func TestGeneratorUniqueFullStringStillUUID(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "Id", Type: ColumnType{Base: ast.StringTypeName}} // size unset
	v, err := g.Unique(col, 0)
	if err != nil {
		t.Fatalf("Unique: %v", err)
	}
	if s := v.(string); len(s) != 36 {
		t.Errorf("Unique(STRING) len = %d, want 36 (uuid)", len(s))
	}
}

func TestGeneratorDefaultArrayTypes(t *testing.T) {
	g := newTestGen()
	cases := []struct {
		base ast.ScalarTypeName
		ok   func(any) bool
	}{
		{ast.Int64TypeName, func(v any) bool { _, ok := v.([]int64); return ok }},
		{ast.StringTypeName, func(v any) bool { _, ok := v.([]string); return ok }},
		{ast.BytesTypeName, func(v any) bool { _, ok := v.([][]byte); return ok }},
		{ast.BoolTypeName, func(v any) bool { _, ok := v.([]bool); return ok }},
		{ast.Float64TypeName, func(v any) bool { _, ok := v.([]float64); return ok }},
		{ast.NumericTypeName, func(v any) bool { _, ok := v.([]*big.Rat); return ok }},
		{ast.TimestampTypeName, func(v any) bool { _, ok := v.([]time.Time); return ok }},
		{ast.DateTypeName, func(v any) bool { _, ok := v.([]civil.Date); return ok }},
		{ast.JSONTypeName, func(v any) bool { _, ok := v.([]spanner.NullJSON); return ok }},
		{ast.IntervalTypeName, func(v any) bool { _, ok := v.([]spanner.Interval); return ok }},
	}
	for _, c := range cases {
		col := &Column{Name: "a", Type: ColumnType{Base: c.base, IsArray: true}}
		v, err := g.Default(col)
		if err != nil {
			t.Fatalf("Default(ARRAY<%s>): %v", c.base, err)
		}
		if !c.ok(v) {
			t.Errorf("Default(ARRAY<%s>) type = %T, want strongly-typed slice", c.base, v)
		}
	}
}

func TestGeneratorUniqueShortStringOverflow(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "Id", Type: ColumnType{Base: ast.StringTypeName, Size: 1}}
	// base36(index+1) is single-digit for index 0..34 (i.e. 1..35 -> "1".."z").
	for i := 0; i < 35; i++ {
		if _, err := g.Unique(col, i); err != nil {
			t.Fatalf("Unique(STRING(1), %d) errored unexpectedly: %v", i, err)
		}
	}
	if _, err := g.Unique(col, 35); err == nil {
		t.Error("Unique(STRING(1), 35) should error because base36(36) = \"10\" no longer fits")
	}
}

func TestGeneratorPatternTimestamp(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "CreatedAt", Type: ColumnType{Base: ast.TimestampTypeName}}
	v, err := g.Pattern(col, "2026-05-28T00:00:00Z", 0, nil, nil)
	if err != nil {
		t.Fatalf("Pattern: %v", err)
	}
	if _, ok := v.(time.Time); !ok {
		t.Errorf("Pattern(TIMESTAMP) type = %T, want time.Time", v)
	}
}

func TestGeneratorPatternDate(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "Day", Type: ColumnType{Base: ast.DateTypeName}}
	v, err := g.Pattern(col, "2026-05-28", 0, nil, nil)
	if err != nil {
		t.Fatalf("Pattern: %v", err)
	}
	if _, ok := v.(civil.Date); !ok {
		t.Errorf("Pattern(DATE) type = %T, want civil.Date", v)
	}
}

func TestGeneratorPatternJSON(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "Meta", Type: ColumnType{Base: ast.JSONTypeName}}
	v, err := g.Pattern(col, `{"k":"v"}`, 0, nil, nil)
	if err != nil {
		t.Fatalf("Pattern: %v", err)
	}
	if _, ok := v.(spanner.NullJSON); !ok {
		t.Errorf("Pattern(JSON) type = %T, want spanner.NullJSON", v)
	}
}

func TestGeneratorPatternArrayTimestamp(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "TsTags", Type: ColumnType{Base: ast.TimestampTypeName, IsArray: true}}
	v, err := g.Pattern(col, "2026-05-28T00:00:00Z", 0, nil, nil)
	if err != nil {
		t.Fatalf("Pattern: %v", err)
	}
	if _, ok := v.([]time.Time); !ok {
		t.Errorf("Pattern(ARRAY<TIMESTAMP>) type = %T, want []time.Time", v)
	}
}

func TestGeneratorPatternRef(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "from_user_id", Type: ColumnType{Base: ast.StringTypeName}}
	refs := newRefRegistry()
	refs.add("users", []map[string]any{
		{"user_id": "user-1"},
		{"user_id": "user-2"},
		{"user_id": "user-3"},
	})
	got := map[string]bool{}
	for i := 0; i < 50; i++ {
		v, err := g.Pattern(col, `{{ Ref "users" "user_id" }}`, i, nil, refs)
		if err != nil {
			t.Fatalf("Pattern: %v", err)
		}
		got[v.(string)] = true
	}
	for _, want := range []string{"user-1", "user-2", "user-3"} {
		if !got[want] {
			t.Errorf("Ref never picked %q after 50 attempts (got %v)", want, got)
		}
	}
}

func TestGeneratorPatternRefMissingTable(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "from_user_id", Type: ColumnType{Base: ast.StringTypeName}}
	refs := newRefRegistry()
	_, err := g.Pattern(col, `{{ Ref "users" "user_id" }}`, 0, nil, refs)
	if err == nil {
		t.Fatal("expected error when ref table has no generated rows")
	}
}

func TestGeneratorPatternRefDistinct(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "from_user_id", Type: ColumnType{Base: ast.StringTypeName}}
	refs := newRefRegistry()
	users := []map[string]any{
		{"user_id": "user-1"},
		{"user_id": "user-2"},
		{"user_id": "user-3"},
		{"user_id": "user-4"},
	}
	refs.add("users", users)

	// receiver = user-1, ask for 3 distinct senders (should be user-2/3/4 in some order, never user-1)
	row := map[string]any{"user_id": "user-1"}
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		v, err := g.Pattern(col, `{{ RefDistinct "users" "user_id" "user_id" }}`, i, row, refs)
		if err != nil {
			t.Fatalf("Pattern (i=%d): %v", i, err)
		}
		s := v.(string)
		if s == "user-1" {
			t.Errorf("RefDistinct picked self-reference user-1 at i=%d", i)
		}
		if seen[s] {
			t.Errorf("RefDistinct picked duplicate %q at i=%d (seen=%v)", s, i, seen)
		}
		seen[s] = true
	}
	if len(seen) != 3 {
		t.Errorf("expected 3 distinct picks, got %d: %v", len(seen), seen)
	}

	// 4th pick should exhaust (only self-ref left)
	if _, err := g.Pattern(col, `{{ RefDistinct "users" "user_id" "user_id" }}`, 3, row, refs); err == nil {
		t.Error("expected exhaustion error on 4th distinct pick")
	}

	// switching to a different receiver scope starts fresh
	row2 := map[string]any{"user_id": "user-2"}
	v, err := g.Pattern(col, `{{ RefDistinct "users" "user_id" "user_id" }}`, 0, row2, refs)
	if err != nil {
		t.Fatalf("Pattern with new scope: %v", err)
	}
	if v.(string) == "user-2" {
		t.Errorf("RefDistinct picked self-reference user-2 in new scope")
	}
}

func TestGeneratorPatternRefMissingColumn(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "x", Type: ColumnType{Base: ast.StringTypeName}}
	refs := newRefRegistry()
	refs.add("users", []map[string]any{{"user_id": "u1"}})
	if _, err := g.Pattern(col, `{{ Ref "users" "missing" }}`, 0, nil, refs); err == nil {
		t.Fatal("expected error when referenced column is missing")
	}
}

func TestGeneratorPatternRefDistinctMissingColumn(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "x", Type: ColumnType{Base: ast.StringTypeName}}
	refs := newRefRegistry()
	refs.add("users", []map[string]any{{"user_id": "u1"}})
	row := map[string]any{"scope": "s1"}
	if _, err := g.Pattern(col, `{{ RefDistinct "users" "missing" "scope" }}`, 0, row, refs); err == nil {
		t.Fatal("expected error when referenced column is missing in RefDistinct")
	}
}

// Same scope value with two different columns must not share the used set:
// exhausting the "id" pool must not block picks from the "name" pool.
func TestGeneratorPatternRefDistinctPerColumnUsedSet(t *testing.T) {
	g := newTestGen()
	col := &Column{Name: "x", Type: ColumnType{Base: ast.StringTypeName}}
	refs := newRefRegistry()
	refs.add("users", []map[string]any{
		{"id": "u1", "name": "Alice"},
		{"id": "u2", "name": "Bob"},
	})
	row := map[string]any{"scope": "s1"}

	seenIds := map[string]bool{}
	for i := 0; i < 2; i++ {
		v, err := g.Pattern(col, `{{ RefDistinct "users" "id" "scope" }}`, i, row, refs)
		if err != nil {
			t.Fatalf("id pick %d: %v", i, err)
		}
		seenIds[v.(string)] = true
	}
	if len(seenIds) != 2 {
		t.Errorf("expected both ids picked, got %v", seenIds)
	}
	// "id" pool is now exhausted under scope=s1; this would fail.
	if _, err := g.Pattern(col, `{{ RefDistinct "users" "id" "scope" }}`, 2, row, refs); err == nil {
		t.Fatal("expected id pool exhaustion under scope=s1")
	}

	// But picks from the "name" column under the same scope must still work,
	// because the used set is keyed per (table, column, scope, scopeVal).
	seenNames := map[string]bool{}
	for i := 0; i < 2; i++ {
		v, err := g.Pattern(col, `{{ RefDistinct "users" "name" "scope" }}`, i, row, refs)
		if err != nil {
			t.Fatalf("name pick %d (should not share used set with id): %v", i, err)
		}
		seenNames[v.(string)] = true
	}
	if len(seenNames) != 2 {
		t.Errorf("expected both names picked, got %v", seenNames)
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
