package hopper

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Tables []*TableSpec
}

type TableSpec struct {
	Name    string
	Rows    int
	Columns map[string]ColumnRule
}

type ColumnRule struct {
	Template string
	Range    *RangeRule
}

type RangeRule struct {
	Min int64
	Max int64
}

func (c *Config) Normalize() {
	sort.Slice(c.Tables, func(i, j int) bool { return c.Tables[i].Name < c.Tables[j].Name })
}

func (c *Config) ensureTable(name string) *TableSpec {
	for _, t := range c.Tables {
		if t.Name == name {
			return t
		}
	}
	ts := &TableSpec{Name: name}
	c.Tables = append(c.Tables, ts)
	return ts
}

type yamlConfig struct {
	Tables map[string]yamlTable `yaml:"tables"`
}

type yamlTable struct {
	Rows    int                   `yaml:"rows"`
	Columns map[string]yamlColumn `yaml:"columns"`
}

type yamlColumn struct {
	Template string  `yaml:"template"`
	Range    []int64 `yaml:"range"`
}

func ConfigFromYAML(data []byte) (*Config, error) {
	var yc yamlConfig
	if err := yaml.Unmarshal(data, &yc); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	c := &Config{}
	for name, yt := range yc.Tables {
		ts := &TableSpec{Name: name, Rows: yt.Rows}
		if len(yt.Columns) > 0 {
			ts.Columns = make(map[string]ColumnRule, len(yt.Columns))
			for col, ycol := range yt.Columns {
				rule, err := ycol.toRule(name, col)
				if err != nil {
					return nil, err
				}
				ts.Columns[col] = rule
			}
		}
		c.Tables = append(c.Tables, ts)
	}
	c.Normalize()
	return c, nil
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

func ConfigFromFlags(tables []string, sets []string) (*Config, error) {
	c := &Config{}
	for _, t := range tables {
		name, n, err := parseTableFlag(t)
		if err != nil {
			return nil, err
		}
		c.ensureTable(name).Rows = n
	}
	for _, s := range sets {
		table, col, rule, err := parseSetFlag(s)
		if err != nil {
			return nil, err
		}
		node := c.ensureTable(table)
		if node.Columns == nil {
			node.Columns = map[string]ColumnRule{}
		}
		node.Columns[col] = rule
	}
	c.Normalize()
	return c, nil
}

func parseTableFlag(s string) (string, int, error) {
	name, numStr, ok := strings.Cut(s, "=")
	if !ok {
		return "", 0, fmt.Errorf("invalid --table %q (expected TABLE=N)", s)
	}
	n, err := strconv.Atoi(strings.TrimSpace(numStr))
	if err != nil {
		return "", 0, fmt.Errorf("invalid count in --table %q: %w", s, err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", 0, fmt.Errorf("invalid --table %q (empty table name)", s)
	}
	return name, n, nil
}

func parseSetFlag(s string) (string, string, ColumnRule, error) {
	key, val, ok := strings.Cut(s, "=")
	if !ok {
		return "", "", ColumnRule{}, fmt.Errorf("invalid --set %q (expected TABLE.COLUMN=RULE)", s)
	}
	rule, err := parseColumnRule(val)
	if err != nil {
		return "", "", ColumnRule{}, fmt.Errorf("invalid --set %q: %w", s, err)
	}
	segs := strings.Split(strings.TrimSpace(key), ".")
	if len(segs) < 2 {
		return "", "", ColumnRule{}, fmt.Errorf("invalid --set key %q (expected TABLE.COLUMN)", key)
	}
	table := segs[len(segs)-2]
	col := segs[len(segs)-1]
	if table == "" || col == "" {
		return "", "", ColumnRule{}, fmt.Errorf("invalid --set key %q", key)
	}
	return table, col, rule, nil
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
