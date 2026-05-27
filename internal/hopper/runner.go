package hopper

import (
	"context"
	"fmt"
	"sort"

	"cloud.google.com/go/spanner"
)

const maxCellsPerCommit = 20000

const maxRowsPerCommit = 1000

type Result struct {
	Table string
	Rows  int
}

type Runner struct {
	schema *Schema
	gen    *Generator
	client *Client
	DryRun bool
	Infer  bool
}

func NewRunner(schema *Schema, gen *Generator, client *Client) *Runner {
	return &Runner{schema: schema, gen: gen, client: client}
}

type genTable struct {
	table     *Table
	columns   map[string]string
	total     int
	parent    *genTable
	fkRefs    []*fkRef
	generated []map[string]any
}

type fkRef struct {
	parent     *genTable
	columns    []string
	refColumns []string
}

func (r *Runner) Run(ctx context.Context, config *Config) ([]Result, error) {
	order, err := r.plan(config)
	if err != nil {
		return nil, err
	}
	if err := r.generate(order); err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(order))
	for _, gt := range order {
		if !r.DryRun && r.client != nil {
			if err := r.insertTable(ctx, gt); err != nil {
				return nil, err
			}
		}
		results = append(results, Result{Table: gt.table.Name, Rows: len(gt.generated)})
	}
	return results, nil
}

func (r *Runner) plan(config *Config) ([]*genTable, error) {
	tables := map[string]*genTable{}
	for _, s := range config.Tables {
		t, ok := r.schema.Table(s.Name)
		if !ok {
			return nil, fmt.Errorf("table %q not found in schema", s.Name)
		}
		for col := range s.Columns {
			if _, ok := t.Column(col); !ok {
				return nil, fmt.Errorf("table %q has no column %q", s.Name, col)
			}
		}
		tables[s.Name] = &genTable{table: t, columns: s.Columns, total: s.Rows}
	}

	queue := sortedTableKeys(tables)
	for len(queue) > 0 {
		t := tables[queue[0]].table
		queue = queue[1:]
		for _, dep := range dependencies(t) {
			if _, exists := tables[dep]; exists {
				continue
			}
			dt, ok := r.schema.Table(dep)
			if !ok {
				return nil, fmt.Errorf("referenced table %q of %q not found in schema", dep, t.Name)
			}
			tables[dep] = &genTable{table: dt, total: 1}
			queue = append(queue, dep)
		}
	}

	for _, gt := range tables {
		if gt.table.Parent != "" {
			gt.parent = tables[gt.table.Parent]
		}
		for _, fk := range gt.table.ForeignKeys {
			if fk.RefTable == gt.table.Name {
				continue
			}
			if p, ok := tables[fk.RefTable]; ok {
				gt.fkRefs = append(gt.fkRefs, &fkRef{
					parent:     p,
					columns:    fk.Columns,
					refColumns: fk.RefColumns,
				})
			}
		}
	}

	return topoSort(tables)
}

func dependencies(t *Table) []string {
	var deps []string
	if t.Parent != "" {
		deps = append(deps, t.Parent)
	}
	for _, fk := range t.ForeignKeys {
		if fk.RefTable != t.Name {
			deps = append(deps, fk.RefTable)
		}
	}
	return deps
}

func (r *Runner) generate(order []*genTable) error {
	for _, gt := range order {
		if gt.total <= 0 {
			return fmt.Errorf("table %q: missing row count (e.g. --table %s=N)", gt.table.Name, gt.table.Name)
		}
		for i := 0; i < gt.total; i++ {
			var parentRow map[string]any
			if gt.parent != nil {
				if len(gt.parent.generated) == 0 {
					return fmt.Errorf("table %q: parent %q produced no rows", gt.table.Name, gt.parent.table.Name)
				}
				parentRow = gt.parent.generated[i%len(gt.parent.generated)]
			}
			row, err := r.generateRow(gt, parentRow, i)
			if err != nil {
				return err
			}
			gt.generated = append(gt.generated, row)
		}
	}
	return nil
}

func (r *Runner) generateRow(gt *genTable, parentRow map[string]any, index int) (map[string]any, error) {
	fkValues := map[string]any{}
	for _, fk := range gt.fkRefs {
		if len(fk.parent.generated) == 0 {
			continue
		}
		prow := fk.parent.generated[r.gen.Intn(len(fk.parent.generated))]
		for i, c := range fk.columns {
			if i < len(fk.refColumns) {
				fkValues[c] = prow[fk.refColumns[i]]
			}
		}
	}

	row := make(map[string]any, len(gt.table.Columns))
	for _, col := range gt.table.Columns {
		if col.Generated {
			continue
		}
		if parentRow != nil && gt.parent != nil && gt.parent.table.IsPrimaryKey(col.Name) {
			if v, ok := parentRow[col.Name]; ok {
				row[col.Name] = v
				continue
			}
		}
		if v, ok := fkValues[col.Name]; ok {
			row[col.Name] = v
			continue
		}
		if pattern, ok := gt.columns[col.Name]; ok {
			v, err := r.gen.Pattern(col, pattern, index)
			if err != nil {
				return nil, err
			}
			row[col.Name] = v
			continue
		}
		if gt.table.IsPrimaryKey(col.Name) {
			v, err := r.gen.Unique(col, index)
			if err != nil {
				return nil, err
			}
			row[col.Name] = v
			continue
		}
		if r.Infer {
			if v, ok := r.gen.Guess(col); ok {
				row[col.Name] = v
				continue
			}
		}
		v, err := r.gen.Default(col)
		if err != nil {
			return nil, err
		}
		row[col.Name] = v
	}
	return row, nil
}

func (r *Runner) insertTable(ctx context.Context, gt *genTable) error {
	if len(gt.generated) == 0 {
		return nil
	}
	cols := orderedColumns(gt.table)
	batchRows := maxCellsPerCommit / max(1, len(cols))
	batchRows = min(max(batchRows, 1), maxRowsPerCommit)

	ms := make([]*spanner.Mutation, 0, batchRows)
	flush := func() error {
		if len(ms) == 0 {
			return nil
		}
		if err := r.client.Apply(ctx, ms); err != nil {
			return fmt.Errorf("insert into %s: %w", gt.table.Name, err)
		}
		ms = ms[:0]
		return nil
	}
	for _, row := range gt.generated {
		vals := make([]any, len(cols))
		for i, c := range cols {
			vals[i] = row[c]
		}
		ms = append(ms, spanner.InsertOrUpdate(gt.table.Name, cols, vals))
		if len(ms) >= batchRows {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

func orderedColumns(t *Table) []string {
	cols := make([]string, 0, len(t.Columns))
	for _, c := range t.Columns {
		if c.Generated {
			continue
		}
		cols = append(cols, c.Name)
	}
	return cols
}

func topoSort(tables map[string]*genTable) ([]*genTable, error) {
	order := make([]*genTable, 0, len(tables))
	added := make(map[string]bool, len(tables))
	for len(order) < len(tables) {
		progress := false
		for _, name := range sortedTableKeys(tables) {
			if added[name] {
				continue
			}
			if !ready(tables[name], added) {
				continue
			}
			order = append(order, tables[name])
			added[name] = true
			progress = true
		}
		if !progress {
			return nil, fmt.Errorf("cycle detected in table dependencies")
		}
	}
	return order, nil
}

func ready(gt *genTable, added map[string]bool) bool {
	if gt.parent != nil && !added[gt.parent.table.Name] {
		return false
	}
	for _, fk := range gt.fkRefs {
		if !added[fk.parent.table.Name] {
			return false
		}
	}
	return true
}

func sortedTableKeys(tables map[string]*genTable) []string {
	keys := make([]string, 0, len(tables))
	for k := range tables {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
