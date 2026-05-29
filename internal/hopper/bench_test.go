package hopper

import (
	"context"
	"fmt"
	"testing"
)

// Run with:
//
//	go test -bench=. -benchmem ./internal/hopper/
//
// For memory profiling:
//
//	go test -bench=BenchmarkRunnerGenerate -benchmem \
//	  -memprofile=mem.prof ./internal/hopper/
//	go tool pprof -top mem.prof

const benchDDL = `
CREATE TABLE Users (
  UserId    STRING(36) NOT NULL,
  Name      STRING(MAX) NOT NULL,
  Email     STRING(MAX) NOT NULL,
  Status    STRING(MAX) NOT NULL,
  ShardKey  INT64 NOT NULL,
  Bio       STRING(MAX),
  Tags      ARRAY<STRING(MAX)>,
  CreatedAt TIMESTAMP NOT NULL,
  UpdatedAt TIMESTAMP NOT NULL,
) PRIMARY KEY (UserId);
`

// BenchmarkRunnerGenerate measures the cost of generating rows for a typical
// wide-ish table (UUID PK, gofakeit-inferable name/email columns, a free-text
// column, an ARRAY column, and timestamps) at several scales.
//
// Reported B/op and allocs/op include everything Generator.Pattern, Unique,
// Default, and the runner accumulate while building rows in memory. No Spanner
// I/O is performed (DryRun = true).
func BenchmarkRunnerGenerate(b *testing.B) {
	schema, err := ParseSchema("bench", benchDDL)
	if err != nil {
		b.Fatal(err)
	}
	for _, n := range []int{1_000, 10_000, 100_000} {
		b.Run(fmt.Sprintf("rows=%d", n), func(b *testing.B) {
			cfg, err := ConfigFromFlags([]string{fmt.Sprintf("Users=%d", n)}, nil)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				gen := NewGenerator(1)
				runner := NewRunner(schema, gen, nil)
				runner.DryRun = true
				if _, err := runner.Run(context.Background(), cfg); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkRunnerGenerateLarge is the same shape at 1M rows. Kept separate so
// the common runs stay quick; opt in explicitly with:
//
//	go test -bench=BenchmarkRunnerGenerateLarge -benchmem ./internal/hopper/
func BenchmarkRunnerGenerateLarge(b *testing.B) {
	schema, err := ParseSchema("bench", benchDDL)
	if err != nil {
		b.Fatal(err)
	}
	cfg, err := ConfigFromFlags([]string{"Users=1000000"}, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gen := NewGenerator(1)
		runner := NewRunner(schema, gen, nil)
		runner.DryRun = true
		if _, err := runner.Run(context.Background(), cfg); err != nil {
			b.Fatal(err)
		}
	}
}
