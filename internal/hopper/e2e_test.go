//go:build e2e

package hopper

import (
	"context"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	dbadmin "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	instadmin "cloud.google.com/go/spanner/admin/instance/apiv1"
	"cloud.google.com/go/spanner/admin/instance/apiv1/instancepb"
)

func TestE2EEmulator(t *testing.T) {
	if os.Getenv("SPANNER_EMULATOR_HOST") == "" {
		t.Skip("SPANNER_EMULATOR_HOST not set; skipping emulator E2E")
	}
	ctx := context.Background()

	const (
		project  = "test-project"
		instance = "test-instance"
		dbName   = "test-db"
	)
	instPath := "projects/" + project + "/instances/" + instance
	dbPath := instPath + "/databases/" + dbName
	uri := "spanner://" + dbPath

	ia, err := instadmin.NewInstanceAdminClient(ctx)
	if err != nil {
		t.Fatalf("instance admin client: %v", err)
	}
	defer ia.Close()
	if iop, err := ia.CreateInstance(ctx, &instancepb.CreateInstanceRequest{
		Parent:     "projects/" + project,
		InstanceId: instance,
		Instance: &instancepb.Instance{
			Config:      "projects/" + project + "/instanceConfigs/emulator-config",
			DisplayName: instance,
			NodeCount:   1,
		},
	}); err == nil {
		_, _ = iop.Wait(ctx)
	}

	da, err := dbadmin.NewDatabaseAdminClient(ctx)
	if err != nil {
		t.Fatalf("database admin client: %v", err)
	}
	defer da.Close()

	var stmts []string
	for _, s := range strings.Split(testDDL, ";") {
		if strings.TrimSpace(s) == "" {
			continue
		}
		stmts = append(stmts, s)
	}
	dop, err := da.CreateDatabase(ctx, &databasepb.CreateDatabaseRequest{
		Parent:          instPath,
		CreateStatement: "CREATE DATABASE `" + dbName + "`",
		ExtraStatements: stmts,
	})
	if err != nil {
		t.Fatalf("create database: %v", err)
	}
	if _, err := dop.Wait(ctx); err != nil {
		t.Fatalf("create database wait: %v", err)
	}

	hc, err := NewClient(ctx, uri)
	if err != nil {
		t.Fatalf("hopper client: %v", err)
	}
	defer hc.Close()

	ddl, err := hc.GetDatabaseDDL(ctx)
	if err != nil {
		t.Fatalf("GetDatabaseDDL: %v", err)
	}
	schema, err := ParseSchema(uri, ddl)
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}
	gen := NewGenerator(1)
	runner := NewRunner(schema, gen, hc)
	runner.Infer = true

	cfg, err := ConfigFromFlags([]string{"Singers=10", "Albums=30"}, nil)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := runner.Run(ctx, cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}

	sc, err := spanner.NewClient(ctx, dbPath)
	if err != nil {
		t.Fatalf("spanner client: %v", err)
	}
	defer sc.Close()

	if got := queryCount(ctx, t, sc, "SELECT COUNT(*) FROM Singers"); got != 10 {
		t.Errorf("Singers count = %d, want 10", got)
	}
	if got := queryCount(ctx, t, sc, "SELECT COUNT(*) FROM Albums"); got != 30 {
		t.Errorf("Albums count = %d, want 30", got)
	}
	if got := queryCount(ctx, t, sc,
		"SELECT COUNT(*) FROM Albums a WHERE NOT EXISTS (SELECT 1 FROM Singers s WHERE s.SingerId = a.SingerId)"); got != 0 {
		t.Errorf("orphan Albums = %d, want 0", got)
	}
}

func queryCount(ctx context.Context, t *testing.T, sc *spanner.Client, sql string) int64 {
	t.Helper()
	iter := sc.Single().Query(ctx, spanner.Statement{SQL: sql})
	defer iter.Stop()
	row, err := iter.Next()
	if err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
	var c int64
	if err := row.Columns(&c); err != nil {
		t.Fatalf("columns: %v", err)
	}
	return c
}
