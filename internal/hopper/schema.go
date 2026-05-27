package hopper

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
)

// Schema is the parsed structure of a Spanner database.
type Schema struct {
	Tables []*Table
	byName map[string]*Table
}

// Table returns the table by name.
func (s *Schema) Table(name string) (*Table, bool) {
	t, ok := s.byName[name]
	return t, ok
}

// Table is a single Spanner table.
type Table struct {
	Name        string
	Columns     []*Column
	PrimaryKeys []string // primary key column names, in key order
	Parent      string   // INTERLEAVE IN PARENT table name, "" if none
}

// Column returns the column by name.
func (t *Table) Column(name string) (*Column, bool) {
	for _, c := range t.Columns {
		if c.Name == name {
			return c, true
		}
	}
	return nil, false
}

// IsPrimaryKey reports whether the named column is part of the primary key.
func (t *Table) IsPrimaryKey(name string) bool {
	for _, pk := range t.PrimaryKeys {
		if pk == name {
			return true
		}
	}
	return false
}

// Column is a single column definition.
type Column struct {
	Name      string
	Type      ColumnType
	NotNull   bool
	Generated bool // generated/identity/auto_increment column; excluded from inserts
}

// ColumnType describes a column's Spanner type.
type ColumnType struct {
	Base    ast.ScalarTypeName // e.g. ast.Int64TypeName, ast.StringTypeName
	IsArray bool
	Size    int64 // STRING(N)/BYTES(N) length; 0 when MAX or not applicable
}

// ParseSchema parses a Spanner DDL string into a Schema.
func ParseSchema(uri, ddl string) (*Schema, error) {
	// Normalize into ";"-terminated statements, mirroring hammer's ParseDDL.
	var lines []string
	for _, line := range strings.Split(ddl, ";") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line+";")
	}

	stmts, err := memefish.ParseDDLs(uri, strings.Join(lines, ""))
	if err != nil {
		return nil, fmt.Errorf("failed to parse ddl: %w", err)
	}

	s := &Schema{byName: map[string]*Table{}}
	for _, stmt := range stmts {
		ct, ok := stmt.(*ast.CreateTable)
		if !ok {
			continue
		}
		t := &Table{Name: pathName(ct.Name)}
		for _, cd := range ct.Columns {
			// Skip hidden columns (Hidden is an invalid pos when not hidden).
			if !cd.Hidden.Invalid() {
				continue
			}
			t.Columns = append(t.Columns, &Column{
				Name:      cd.Name.Name,
				Type:      schemaType(cd.Type),
				NotNull:   cd.NotNull,
				Generated: isGenerated(cd),
			})
		}
		t.PrimaryKeys = primaryKeys(ct)
		if ct.Cluster != nil {
			t.Parent = pathName(ct.Cluster.TableName)
		}
		s.Tables = append(s.Tables, t)
		s.byName[t.Name] = t
	}
	return s, nil
}

// pathName joins a dot-chained identifier path (e.g. schema-qualified names).
func pathName(p *ast.Path) string {
	if p == nil || len(p.Idents) == 0 {
		return ""
	}
	names := make([]string, len(p.Idents))
	for i, id := range p.Idents {
		names[i] = id.Name
	}
	return strings.Join(names, ".")
}

func schemaType(t ast.SchemaType) ColumnType {
	switch tt := t.(type) {
	case *ast.ScalarSchemaType:
		return ColumnType{Base: tt.Name}
	case *ast.SizedSchemaType:
		ct := ColumnType{Base: tt.Name}
		if !tt.Max {
			if iv, ok := tt.Size.(*ast.IntLiteral); ok {
				if n, err := strconv.ParseInt(iv.Value, 0, 64); err == nil {
					ct.Size = n
				}
			}
		}
		return ct
	case *ast.ArraySchemaType:
		ct := schemaType(tt.Item)
		ct.IsArray = true
		return ct
	}
	return ColumnType{Base: ast.StringTypeName}
}

func isGenerated(cd *ast.ColumnDef) bool {
	switch cd.DefaultSemantics.(type) {
	case *ast.GeneratedColumnExpr, *ast.IdentityColumn, *ast.AutoIncrement:
		return true
	}
	return false
}

func primaryKeys(ct *ast.CreateTable) []string {
	if len(ct.PrimaryKeys) > 0 {
		keys := make([]string, len(ct.PrimaryKeys))
		for i, k := range ct.PrimaryKeys {
			keys[i] = k.Name.Name
		}
		return keys
	}
	// Fall back to column-level PRIMARY KEY.
	var keys []string
	for _, cd := range ct.Columns {
		if cd.PrimaryKey {
			keys = append(keys, cd.Name.Name)
		}
	}
	return keys
}
