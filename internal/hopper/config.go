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
	Name      string
	Rows      int
	Columns   map[string]string
	NullRates map[string]float64
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
	Columns map[string]columnSpec `yaml:"columns"`
}

type columnSpec struct {
	Template string
	NullRate *float64
}

func (cs *columnSpec) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		return node.Decode(&cs.Template)
	}
	var raw struct {
		Template string   `yaml:"template"`
		NullRate *float64 `yaml:"null_rate"`
	}
	if err := node.Decode(&raw); err != nil {
		return err
	}
	cs.Template = raw.Template
	cs.NullRate = raw.NullRate
	return nil
}

func ConfigFromYAML(data []byte) (*Config, error) {
	var yc yamlConfig
	if err := yaml.Unmarshal(data, &yc); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	c := &Config{}
	for name, yt := range yc.Tables {
		ts := &TableSpec{Name: name, Rows: yt.Rows}
		for col, spec := range yt.Columns {
			if spec.Template != "" {
				if ts.Columns == nil {
					ts.Columns = map[string]string{}
				}
				ts.Columns[col] = spec.Template
			}
			if spec.NullRate != nil {
				if *spec.NullRate < 0 || *spec.NullRate > 1 {
					return nil, fmt.Errorf("table %q column %q: null_rate must be between 0 and 1, got %v", name, col, *spec.NullRate)
				}
				if ts.NullRates == nil {
					ts.NullRates = map[string]float64{}
				}
				ts.NullRates[col] = *spec.NullRate
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

func (c *Config) ApplyNullRates(specs []string) error {
	for _, s := range specs {
		table, col, rate, err := parseNullRateFlag(s)
		if err != nil {
			return err
		}
		node := c.ensureTable(table)
		if node.NullRates == nil {
			node.NullRates = map[string]float64{}
		}
		node.NullRates[col] = rate
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
	if n <= 0 {
		return "", 0, fmt.Errorf("invalid count in --table %q: must be > 0", s)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", 0, fmt.Errorf("invalid --table %q (empty table name)", s)
	}
	return name, n, nil
}

func parseColumnKey(flag, key string) (string, string, error) {
	segs := strings.Split(strings.TrimSpace(key), ".")
	if len(segs) < 2 {
		return "", "", fmt.Errorf("invalid --%s key %q (expected TABLE.COLUMN)", flag, key)
	}
	table := segs[len(segs)-2]
	col := segs[len(segs)-1]
	if table == "" || col == "" {
		return "", "", fmt.Errorf("invalid --%s key %q", flag, key)
	}
	return table, col, nil
}

func parseSetFlag(s string) (string, string, string, error) {
	key, pattern, ok := strings.Cut(s, "=")
	if !ok {
		return "", "", "", fmt.Errorf("invalid --set %q (expected TABLE.COLUMN=PATTERN)", s)
	}
	table, col, err := parseColumnKey("set", key)
	if err != nil {
		return "", "", "", err
	}
	return table, col, pattern, nil
}

func parseNullRateFlag(s string) (string, string, float64, error) {
	key, rateStr, ok := strings.Cut(s, "=")
	if !ok {
		return "", "", 0, fmt.Errorf("invalid --null-rate %q (expected TABLE.COLUMN=RATE)", s)
	}
	table, col, err := parseColumnKey("null-rate", key)
	if err != nil {
		return "", "", 0, err
	}
	rate, err := strconv.ParseFloat(strings.TrimSpace(rateStr), 64)
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid rate in --null-rate %q: %w", s, err)
	}
	if rate < 0 || rate > 1 {
		return "", "", 0, fmt.Errorf("invalid rate in --null-rate %q: must be between 0 and 1", s)
	}
	return table, col, rate, nil
}
