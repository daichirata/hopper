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
	return NewRunner(schema, gen, nil) // nil client => generation only
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

// Child specified alone -> parent auto-completed with 1 row; child rows inherit
// the parent's primary key.
func TestRunnerAncestorCompletion(t *testing.T) {
	r := newDryRunner(t)
	cfg, err := ConfigFromFlags([]string{"UserAvatars=300"}, nil)
	if err != nil {
		t.Fatalf("ConfigFromFlags: %v", err)
	}

	m := genTables(t, r, cfg)
	if got := len(m["Users"].rows); got != 1 {
		t.Errorf("Users rows = %d, want 1 (auto-completed parent)", got)
	}
	if got := len(m["UserAvatars"].rows); got != 300 {
		t.Errorf("UserAvatars rows = %d, want 300", got)
	}

	parentUID := m["Users"].rows[0]["UserId"]
	for i, row := range m["UserAvatars"].rows {
		if row["UserId"] != parentUID {
			t.Fatalf("row %d UserId = %v, want inherited %v", i, row["UserId"], parentUID)
		}
	}
}

// rows_per_parent: each parent gets N children, each inheriting its own parent key.
func TestRunnerPerParentInheritance(t *testing.T) {
	r := newDryRunner(t)
	cfg, err := ConfigFromFlags([]string{"Users=10", "Users.UserAvatars=5"}, nil)
	if err != nil {
		t.Fatalf("ConfigFromFlags: %v", err)
	}

	m := genTables(t, r, cfg)
	if got := len(m["Users"].rows); got != 10 {
		t.Errorf("Users rows = %d, want 10", got)
	}
	if got := len(m["UserAvatars"].rows); got != 50 {
		t.Errorf("UserAvatars rows = %d, want 50 (10 x 5)", got)
	}

	// Group children by inherited UserId and check counts.
	counts := map[any]int{}
	for _, row := range m["UserAvatars"].rows {
		counts[row["UserId"]]++
	}
	if len(counts) != 10 {
		t.Errorf("distinct parent keys = %d, want 10", len(counts))
	}
	for uid, c := range counts {
		if c != 5 {
			t.Errorf("parent %v has %d children, want 5", uid, c)
		}
	}
}

// Column rules: range stays in bounds; PK is unique across rows.
func TestRunnerColumnRulesAndUniquePK(t *testing.T) {
	r := newDryRunner(t)
	cfg, err := ConfigFromFlags(
		[]string{"Users=50"},
		[]string{"Users.ShardId=range:0-3"},
	)
	if err != nil {
		t.Fatalf("ConfigFromFlags: %v", err)
	}

	m := genTables(t, r, cfg)
	rows := m["Users"].rows
	if len(rows) != 50 {
		t.Fatalf("Users rows = %d, want 50", len(rows))
	}

	seen := map[any]bool{}
	for _, row := range rows {
		shard := row["ShardId"].(int64)
		if shard < 0 || shard > 3 {
			t.Errorf("ShardId %d out of [0,3]", shard)
		}
		uid := row["UserId"]
		if seen[uid] {
			t.Errorf("duplicate PK UserId %v", uid)
		}
		seen[uid] = true
		if _, ok := row["FullName"]; ok {
			t.Error("generated column FullName should not be populated")
		}
	}
}

func TestRunnerRunDryReturnsCounts(t *testing.T) {
	r := newDryRunner(t)
	r.DryRun = true
	cfg, _ := ConfigFromFlags([]string{"Users=3", "Users.UserAvatars=2"}, nil)

	results, err := r.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := map[string]int{}
	for _, res := range results {
		got[res.Table] = res.Rows
	}
	if got["Users"] != 3 || got["UserAvatars"] != 6 {
		t.Errorf("counts = %v, want Users:3 UserAvatars:6", got)
	}
}
