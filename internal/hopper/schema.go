package hopper

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
)

type Schema struct {
	Tables []*Table
	byName map[string]*Table
}

func (s *Schema) Table(name string) (*Table, bool) {
	t, ok := s.byName[name]
	return t, ok
}

type Table struct {
	Name          string
	Columns       []*Column
	PrimaryKeys   []string
	Parent        string
	ForeignKeys   []*ForeignKey
	uniqueColumns map[string]bool
}

type ForeignKey struct {
	Columns    []string
	RefTable   string
	RefColumns []string
}

func (t *Table) Column(name string) (*Column, bool) {
	for _, c := range t.Columns {
		if c.Name == name {
			return c, true
		}
	}
	return nil, false
}

func (t *Table) IsPrimaryKey(name string) bool {
	for _, pk := range t.PrimaryKeys {
		if pk == name {
			return true
		}
	}
	return false
}

func (t *Table) IsUnique(name string) bool {
	return t.uniqueColumns[name]
}

type Column struct {
	Name      string
	Type      ColumnType
	NotNull   bool
	Generated bool
}

type ColumnType struct {
	Base    ast.ScalarTypeName
	IsArray bool
	Size    int64
}

func ParseSchema(uri, ddl string) (*Schema, error) {
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
		for _, tc := range ct.TableConstraints {
			if fk, ok := tc.Constraint.(*ast.ForeignKey); ok {
				t.ForeignKeys = append(t.ForeignKeys, foreignKey(fk))
			}
		}
		s.Tables = append(s.Tables, t)
		s.byName[t.Name] = t
	}
	for _, stmt := range stmts {
		switch v := stmt.(type) {
		case *ast.AlterTable:
			add, ok := v.TableAlteration.(*ast.AddTableConstraint)
			if !ok {
				continue
			}
			fk, ok := add.TableConstraint.Constraint.(*ast.ForeignKey)
			if !ok {
				continue
			}
			if t, ok := s.byName[pathName(v.Name)]; ok {
				t.ForeignKeys = append(t.ForeignKeys, foreignKey(fk))
			}
		case *ast.CreateIndex:
			if !v.Unique {
				continue
			}
			t, ok := s.byName[pathName(v.TableName)]
			if !ok {
				continue
			}
			if t.uniqueColumns == nil {
				t.uniqueColumns = map[string]bool{}
			}
			for _, k := range v.Keys {
				t.uniqueColumns[k.Name.Name] = true
			}
		}
	}
	return s, nil
}

func foreignKey(fk *ast.ForeignKey) *ForeignKey {
	return &ForeignKey{
		Columns:    identNames(fk.Columns),
		RefTable:   pathName(fk.ReferenceTable),
		RefColumns: identNames(fk.ReferenceColumns),
	}
}

func identNames(ids []*ast.Ident) []string {
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = id.Name
	}
	return names
}

func pathName(p *ast.Path) string {
	if p == nil || len(p.Idents) == 0 {
		return ""
	}
	return strings.Join(identNames(p.Idents), ".")
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
	var keys []string
	for _, cd := range ct.Columns {
		if cd.PrimaryKey {
			keys = append(keys, cd.Name.Name)
		}
	}
	return keys
}
