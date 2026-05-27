package hopper

import (
	"context"
	"testing"
)

func newDryRunner(t *testing.T) *Runner {
	t.Helper()
	schema, err := ParseSchema("test", testDDL)
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}
	return NewRunner(schema, NewGenerator(1), nil)
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
	if got := len(m["Singers"].generated); got != 1 {
		t.Errorf("Singers rows = %d, want 1 (auto-completed parent)", got)
	}
	if got := len(m["Albums"].generated); got != 300 {
		t.Errorf("Albums rows = %d, want 300", got)
	}

	parentSID := m["Singers"].generated[0]["SingerId"]
	for i, row := range m["Albums"].generated {
		if row["SingerId"] != parentSID {
			t.Fatalf("row %d SingerId = %v, want inherited %v", i, row["SingerId"], parentSID)
		}
	}
}

func TestRunnerRoundRobinDistribution(t *testing.T) {
	r := newDryRunner(t)
	cfg, err := ConfigFromFlags([]string{"Singers=10", "Albums=1000"}, nil)
	if err != nil {
		t.Fatalf("ConfigFromFlags: %v", err)
	}

	m := genTables(t, r, cfg)
	if got := len(m["Singers"].generated); got != 10 {
		t.Errorf("Singers rows = %d, want 10", got)
	}
	if got := len(m["Albums"].generated); got != 1000 {
		t.Errorf("Albums rows = %d, want 1000", got)
	}

	counts := map[any]int{}
	for _, row := range m["Albums"].generated {
		counts[row["SingerId"]]++
	}
	if len(counts) != 10 {
		t.Errorf("distinct parent keys = %d, want 10", len(counts))
	}
	for sid, c := range counts {
		if c != 100 {
			t.Errorf("parent %v has %d children, want 100 (even round-robin)", sid, c)
		}
	}
}

func TestRunnerColumnTemplateAndUniquePK(t *testing.T) {
	r := newDryRunner(t)
	cfg, err := ConfigFromFlags(
		[]string{"Albums=50"},
		[]string{"Albums.MarketingBudget={{ Number 0 3 }}"},
	)
	if err != nil {
		t.Fatalf("ConfigFromFlags: %v", err)
	}

	m := genTables(t, r, cfg)
	rows := m["Albums"].generated
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
	for _, row := range m["Singers"].generated {
		if _, ok := row["FullName"]; ok {
			t.Error("generated column FullName should not be populated")
		}
	}
}

func TestRunnerRunDryReturnsCounts(t *testing.T) {
	r := newDryRunner(t)
	r.DryRun = true
	cfg, _ := ConfigFromFlags([]string{"Singers=3", "Albums=6"}, nil)

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

func TestRunnerForeignKey(t *testing.T) {
	r := newDryRunner(t)
	cfg, err := ConfigFromFlags([]string{"Singers=5", "Concerts=20"}, nil)
	if err != nil {
		t.Fatalf("ConfigFromFlags: %v", err)
	}

	m := genTables(t, r, cfg)
	if got := len(m["Concerts"].generated); got != 20 {
		t.Errorf("Concerts rows = %d, want 20", got)
	}
	if got := len(m["Singers"].generated); got != 5 {
		t.Errorf("Singers rows = %d, want 5", got)
	}

	singerIDs := map[any]bool{}
	for _, row := range m["Singers"].generated {
		singerIDs[row["SingerId"]] = true
	}
	for _, row := range m["Concerts"].generated {
		if !singerIDs[row["SingerId"]] {
			t.Errorf("Concert SingerId %v not found among Singers", row["SingerId"])
		}
	}
}

func TestRunnerForeignKeyAutoComplete(t *testing.T) {
	r := newDryRunner(t)
	cfg, _ := ConfigFromFlags([]string{"Concerts=20"}, nil)

	m := genTables(t, r, cfg)
	if _, ok := m["Singers"]; !ok {
		t.Fatal("Singers should be auto-completed via the foreign key")
	}
	if got := len(m["Singers"].generated); got != 1 {
		t.Errorf("auto-completed Singers rows = %d, want 1", got)
	}

	singerIDs := map[any]bool{}
	for _, row := range m["Singers"].generated {
		singerIDs[row["SingerId"]] = true
	}
	for _, row := range m["Concerts"].generated {
		if !singerIDs[row["SingerId"]] {
			t.Error("Concert references a non-existent Singer")
		}
	}
}

func TestRunnerThreeLevelInterleave(t *testing.T) {
	r := newDryRunner(t)
	cfg, err := ConfigFromFlags([]string{"Singers=3", "Albums=6", "Songs=24"}, nil)
	if err != nil {
		t.Fatalf("ConfigFromFlags: %v", err)
	}

	m := genTables(t, r, cfg)
	if got := len(m["Songs"].generated); got != 24 {
		t.Errorf("Songs rows = %d, want 24", got)
	}

	singerIDs := map[any]bool{}
	for _, row := range m["Singers"].generated {
		singerIDs[row["SingerId"]] = true
	}
	for _, row := range m["Albums"].generated {
		if !singerIDs[row["SingerId"]] {
			t.Error("Album SingerId not found among Singers")
		}
	}

	albumKeys := map[[2]any]bool{}
	for _, row := range m["Albums"].generated {
		albumKeys[[2]any{row["SingerId"], row["AlbumId"]}] = true
	}
	for _, row := range m["Songs"].generated {
		if !albumKeys[[2]any{row["SingerId"], row["AlbumId"]}] {
			t.Error("Song parent (SingerId, AlbumId) not found among Albums")
		}
	}
}

func TestRunnerThreeLevelAutoComplete(t *testing.T) {
	r := newDryRunner(t)
	cfg, _ := ConfigFromFlags([]string{"Songs=24"}, nil)

	m := genTables(t, r, cfg)
	if got := len(m["Singers"].generated); got != 1 {
		t.Errorf("auto-completed Singers = %d, want 1", got)
	}
	if got := len(m["Albums"].generated); got != 1 {
		t.Errorf("auto-completed Albums = %d, want 1", got)
	}
	if got := len(m["Songs"].generated); got != 24 {
		t.Errorf("Songs rows = %d, want 24", got)
	}
}

func TestRunnerNullRate(t *testing.T) {
	r := newDryRunner(t)
	r.NullRate = 1.0
	cfg, _ := ConfigFromFlags([]string{"Singers=10"}, nil)

	m := genTables(t, r, cfg)
	for _, row := range m["Singers"].generated {
		if row["FirstName"] != nil {
			t.Errorf("FirstName = %v, want NULL with null-rate 1.0", row["FirstName"])
		}
		if row["SingerId"] == nil {
			t.Error("SingerId (primary key) must never be NULL")
		}
	}
}

func TestRunnerDryRunSample(t *testing.T) {
	r := newDryRunner(t)
	r.DryRun = true
	cfg, _ := ConfigFromFlags([]string{"Singers=10"}, nil)

	results, err := r.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, res := range results {
		if res.Table == "Singers" {
			if len(res.Sample) != 3 {
				t.Errorf("Singers sample = %d rows, want 3", len(res.Sample))
			}
		}
	}
}
