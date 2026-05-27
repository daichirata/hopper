package hopper

import (
	"context"
	"math/rand"
	"testing"
)

func newDryRunner(t *testing.T) *Runner {
	t.Helper()
	schema, err := ParseSchema("test", testDDL)
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}
	gen := NewGenerator(rand.New(rand.NewSource(1)))
	return NewRunner(schema, gen, nil)
}

func genTables(t *testing.T, r *Runner, cfg *Config) map[string]*genTable {
	t.Helper()
	order, err := r.plan(cfg)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if err := r.generate(order); err != nil {
		t.Fatalf("generate: %v", err)
	}
	m := map[string]*genTable{}
	for _, gt := range order {
		m[gt.table.Name] = gt
	}
	return m
}

func TestRunnerAncestorCompletion(t *testing.T) {
	r := newDryRunner(t)
	cfg, err := ConfigFromFlags([]string{"Albums=300"}, nil)
	if err != nil {
		t.Fatalf("ConfigFromFlags: %v", err)
	}

	m := genTables(t, r, cfg)
	if got := len(m["Singers"].rows); got != 1 {
		t.Errorf("Singers rows = %d, want 1 (auto-completed parent)", got)
	}
	if got := len(m["Albums"].rows); got != 300 {
		t.Errorf("Albums rows = %d, want 300", got)
	}

	parentSID := m["Singers"].rows[0]["SingerId"]
	for i, row := range m["Albums"].rows {
		if row["SingerId"] != parentSID {
			t.Fatalf("row %d SingerId = %v, want inherited %v", i, row["SingerId"], parentSID)
		}
	}
}

func TestRunnerPerParentInheritance(t *testing.T) {
	r := newDryRunner(t)
	cfg, err := ConfigFromFlags([]string{"Singers=10", "Singers.Albums=5"}, nil)
	if err != nil {
		t.Fatalf("ConfigFromFlags: %v", err)
	}

	m := genTables(t, r, cfg)
	if got := len(m["Singers"].rows); got != 10 {
		t.Errorf("Singers rows = %d, want 10", got)
	}
	if got := len(m["Albums"].rows); got != 50 {
		t.Errorf("Albums rows = %d, want 50 (10 x 5)", got)
	}

	counts := map[any]int{}
	for _, row := range m["Albums"].rows {
		counts[row["SingerId"]]++
	}
	if len(counts) != 10 {
		t.Errorf("distinct parent keys = %d, want 10", len(counts))
	}
	for sid, c := range counts {
		if c != 5 {
			t.Errorf("parent %v has %d children, want 5", sid, c)
		}
	}
}

func TestRunnerColumnRulesAndUniquePK(t *testing.T) {
	r := newDryRunner(t)
	cfg, err := ConfigFromFlags(
		[]string{"Albums=50"},
		[]string{"Albums.MarketingBudget=range:0-3"},
	)
	if err != nil {
		t.Fatalf("ConfigFromFlags: %v", err)
	}

	m := genTables(t, r, cfg)
	rows := m["Albums"].rows
	if len(rows) != 50 {
		t.Fatalf("Albums rows = %d, want 50", len(rows))
	}

	seen := map[any]bool{}
	for _, row := range rows {
		budget := row["MarketingBudget"].(int64)
		if budget < 0 || budget > 3 {
			t.Errorf("MarketingBudget %d out of [0,3]", budget)
		}
		albumID := row["AlbumId"]
		if seen[albumID] {
			t.Errorf("duplicate PK AlbumId %v", albumID)
		}
		seen[albumID] = true
	}
}

func TestRunnerExcludesGeneratedColumn(t *testing.T) {
	r := newDryRunner(t)
	cfg, _ := ConfigFromFlags([]string{"Singers=5"}, nil)
	m := genTables(t, r, cfg)
	for _, row := range m["Singers"].rows {
		if _, ok := row["FullName"]; ok {
			t.Error("generated column FullName should not be populated")
		}
	}
}

func TestRunnerRunDryReturnsCounts(t *testing.T) {
	r := newDryRunner(t)
	r.DryRun = true
	cfg, _ := ConfigFromFlags([]string{"Singers=3", "Singers.Albums=2"}, nil)

	results, err := r.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := map[string]int{}
	for _, res := range results {
		got[res.Table] = res.Rows
	}
	if got["Singers"] != 3 || got["Albums"] != 6 {
		t.Errorf("counts = %v, want Singers:3 Albums:6", got)
	}
}
