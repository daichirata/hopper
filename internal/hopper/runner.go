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
}

func NewRunner(schema *Schema, gen *Generator, client *Client) *Runner {
	return &Runner{schema: schema, gen: gen, client: client}
}

type genTable struct {
	table     *Table
	columns   map[string]ColumnRule
	total     int
	parent    *genTable
	generated []map[string]any
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

	for _, name := range sortedTableKeys(tables) {
		cur := tables[name].table
		for cur.Parent != "" {
			p, ok := r.schema.Table(cur.Parent)
			if !ok {
				return nil, fmt.Errorf("interleave parent %q of %q not found in schema", cur.Parent, cur.Name)
			}
			if _, exists := tables[p.Name]; !exists {
				tables[p.Name] = &genTable{table: p, total: 1}
			}
			cur = p
		}
	}

	for _, gt := range tables {
		if gt.table.Parent != "" {
			gt.parent = tables[gt.table.Parent]
		}
	}

	return topoSort(tables)
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
		if rule, ok := gt.columns[col.Name]; ok {
			v, err := r.gen.FromRule(col, rule, index)
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
			gt := tables[name]
			if gt.parent == nil || added[gt.parent.table.Name] {
				order = append(order, gt)
				added[name] = true
				progress = true
			}
		}
		if !progress {
			return nil, fmt.Errorf("cycle detected in interleave hierarchy")
		}
	}
	return order, nil
}

func sortedTableKeys(tables map[string]*genTable) []string {
	keys := make([]string, 0, len(tables))
	for k := range tables {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
