package hopper

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the generation plan input, shared by the YAML and CLI front-ends.
type Config struct {
	Tables []*TableSpec
}

// TableSpec describes how many rows to generate for a table and its children.
type TableSpec struct {
	Name          string
	Rows          int // root table: total number of rows
	RowsPerParent int // child table: rows generated per parent row
	Columns       map[string]ColumnRule
	Children      []*TableSpec
}

// ColumnRule overrides how a single column's value is generated.
// Zero value (no template, no range) means "use the type default".
type ColumnRule struct {
	Template string
	Range    *RangeRule
}

// RangeRule generates an integer in the inclusive range [Min, Max].
type RangeRule struct {
	Min int64
	Max int64
}

// Normalize sorts tables and children by name so that YAML- and CLI-built
// configs compare equal and generation is deterministic.
func (c *Config) Normalize() {
	sortSpecs(c.Tables)
}

func sortSpecs(specs []*TableSpec) {
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	for _, s := range specs {
		sortSpecs(s.Children)
	}
}

// ensureTable walks/creates the table tree along path and returns the leaf node.
func (c *Config) ensureTable(path []string) *TableSpec {
	siblings := &c.Tables
	var node *TableSpec
	for _, name := range path {
		node = nil
		for _, t := range *siblings {
			if t.Name == name {
				node = t
				break
			}
		}
		if node == nil {
			node = &TableSpec{Name: name}
			*siblings = append(*siblings, node)
		}
		siblings = &node.Children
	}
	return node
}

// --- YAML front-end ---

type yamlConfig struct {
	Tables map[string]yamlTable `yaml:"tables"`
}

type yamlTable struct {
	Rows          int                   `yaml:"rows"`
	RowsPerParent int                   `yaml:"rows_per_parent"`
	Columns       map[string]yamlColumn `yaml:"columns"`
	Children      map[string]yamlTable  `yaml:"children"`
}

type yamlColumn struct {
	Template string  `yaml:"template"`
	Range    []int64 `yaml:"range"`
}

// ConfigFromYAML parses a YAML configuration into a Config.
func ConfigFromYAML(data []byte) (*Config, error) {
	var yc yamlConfig
	if err := yaml.Unmarshal(data, &yc); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	c := &Config{}
	for name, yt := range yc.Tables {
		ts, err := buildTableSpec(name, yt)
		if err != nil {
			return nil, err
		}
		c.Tables = append(c.Tables, ts)
	}
	c.Normalize()
	return c, nil
}

func buildTableSpec(name string, yt yamlTable) (*TableSpec, error) {
	ts := &TableSpec{Name: name, Rows: yt.Rows, RowsPerParent: yt.RowsPerParent}
	if len(yt.Columns) > 0 {
		ts.Columns = make(map[string]ColumnRule, len(yt.Columns))
		for col, yc := range yt.Columns {
			rule, err := yc.toRule(name, col)
			if err != nil {
				return nil, err
			}
			ts.Columns[col] = rule
		}
	}
	for childName, child := range yt.Children {
		cs, err := buildTableSpec(childName, child)
		if err != nil {
			return nil, err
		}
		ts.Children = append(ts.Children, cs)
	}
	return ts, nil
}

func (yc yamlColumn) toRule(table, col string) (ColumnRule, error) {
	rule := ColumnRule{Template: yc.Template}
	if yc.Range != nil {
		if len(yc.Range) != 2 {
			return ColumnRule{}, fmt.Errorf("%s.%s: range must have exactly 2 elements [min, max]", table, col)
		}
		rule.Range = &RangeRule{Min: yc.Range[0], Max: yc.Range[1]}
	}
	return rule, nil
}

// --- CLI front-end ---

// ConfigFromFlags builds a Config from repeated --table and --set flags.
//
//	tables: "Users=1000", "Users.UserAvatars=100"  (dotted path; depth 1 = total rows, deeper = rows per parent)
//	sets:   "Users.Name=template:{ .Random }-{ .Index }", "Users.ShardId=range:0-10"
func ConfigFromFlags(tables []string, sets []string) (*Config, error) {
	c := &Config{}
	for _, t := range tables {
		path, n, err := parseTableFlag(t)
		if err != nil {
			return nil, err
		}
		node := c.ensureTable(path)
		if len(path) == 1 {
			node.Rows = n
		} else {
			node.RowsPerParent = n
		}
	}
	for _, s := range sets {
		path, col, rule, err := parseSetFlag(s)
		if err != nil {
			return nil, err
		}
		node := c.ensureTable(path)
		if node.Columns == nil {
			node.Columns = map[string]ColumnRule{}
		}
		node.Columns[col] = rule
	}
	c.Normalize()
	return c, nil
}

func parseTableFlag(s string) ([]string, int, error) {
	pathStr, numStr, ok := strings.Cut(s, "=")
	if !ok {
		return nil, 0, fmt.Errorf("invalid --table %q (expected PATH=N)", s)
	}
	n, err := strconv.Atoi(strings.TrimSpace(numStr))
	if err != nil {
		return nil, 0, fmt.Errorf("invalid count in --table %q: %w", s, err)
	}
	path, err := splitPath(pathStr)
	if err != nil {
		return nil, 0, err
	}
	return path, n, nil
}

func parseSetFlag(s string) ([]string, string, ColumnRule, error) {
	key, val, ok := strings.Cut(s, "=")
	if !ok {
		return nil, "", ColumnRule{}, fmt.Errorf("invalid --set %q (expected PATH.Column=RULE)", s)
	}
	rule, err := parseColumnRule(val)
	if err != nil {
		return nil, "", ColumnRule{}, fmt.Errorf("invalid --set %q: %w", s, err)
	}
	segs, err := splitPath(key)
	if err != nil {
		return nil, "", ColumnRule{}, err
	}
	if len(segs) < 2 {
		return nil, "", ColumnRule{}, fmt.Errorf("invalid --set key %q (expected Table.Column or Path.Column)", key)
	}
	return segs[:len(segs)-1], segs[len(segs)-1], rule, nil
}

func parseColumnRule(s string) (ColumnRule, error) {
	kind, rest, ok := strings.Cut(s, ":")
	if !ok {
		return ColumnRule{}, fmt.Errorf("rule %q must be template:... or range:a-b", s)
	}
	switch kind {
	case "template":
		return ColumnRule{Template: rest}, nil
	case "range":
		r, err := parseRange(rest)
		if err != nil {
			return ColumnRule{}, err
		}
		return ColumnRule{Range: r}, nil
	default:
		return ColumnRule{}, fmt.Errorf("unknown rule kind %q (want template or range)", kind)
	}
}

func parseRange(s string) (*RangeRule, error) {
	minStr, maxStr, ok := strings.Cut(s, "-")
	if !ok {
		return nil, fmt.Errorf("invalid range %q (expected a-b)", s)
	}
	min, err := strconv.ParseInt(strings.TrimSpace(minStr), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid range min in %q: %w", s, err)
	}
	max, err := strconv.ParseInt(strings.TrimSpace(maxStr), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid range max in %q: %w", s, err)
	}
	if min > max {
		return nil, fmt.Errorf("invalid range %q: min > max", s)
	}
	return &RangeRule{Min: min, Max: max}, nil
}

func splitPath(s string) ([]string, error) {
	segs := strings.Split(strings.TrimSpace(s), ".")
	for _, seg := range segs {
		if strings.TrimSpace(seg) == "" {
			return nil, fmt.Errorf("invalid path %q (empty segment)", s)
		}
	}
	return segs, nil
}
