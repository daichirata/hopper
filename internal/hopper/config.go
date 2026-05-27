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
	Columns map[string]string
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
	Rows    int               `yaml:"rows"`
	Columns map[string]string `yaml:"columns"`
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
			ts.Columns = make(map[string]string, len(yt.Columns))
			for col, pattern := range yt.Columns {
				ts.Columns[col] = pattern
			}
		}
		c.Tables = append(c.Tables, ts)
	}
	c.Normalize()
	return c, nil
}

func ConfigFromFlags(tables []string, sets []string) (*Config, error) {
	c := &Config{}
	if err := c.ApplyFlags(tables, sets); err != nil {
		return nil, err
	}
	c.Normalize()
	return c, nil
}

func (c *Config) ApplyFlags(tables []string, sets []string) error {
	for _, t := range tables {
		name, n, err := parseTableFlag(t)
		if err != nil {
			return err
		}
		c.ensureTable(name).Rows = n
	}
	for _, s := range sets {
		table, col, pattern, err := parseSetFlag(s)
		if err != nil {
			return err
		}
		node := c.ensureTable(table)
		if node.Columns == nil {
			node.Columns = map[string]string{}
		}
		node.Columns[col] = pattern
	}
	return nil
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

func parseSetFlag(s string) (string, string, string, error) {
	key, pattern, ok := strings.Cut(s, "=")
	if !ok {
		return "", "", "", fmt.Errorf("invalid --set %q (expected TABLE.COLUMN=PATTERN)", s)
	}
	segs := strings.Split(strings.TrimSpace(key), ".")
	if len(segs) < 2 {
		return "", "", "", fmt.Errorf("invalid --set key %q (expected TABLE.COLUMN)", key)
	}
	table := segs[len(segs)-2]
	col := segs[len(segs)-1]
	if table == "" || col == "" {
		return "", "", "", fmt.Errorf("invalid --set key %q", key)
	}
	return table, col, pattern, nil
}
