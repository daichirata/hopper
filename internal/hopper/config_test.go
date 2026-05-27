package hopper

import (
	"reflect"
	"testing"
)

func TestConfigYAMLAndFlagsEquivalent(t *testing.T) {
	yamlSrc := []byte(`
tables:
  Singers:
    rows: 10
    columns:
      FirstName: { template: "{ .Random }-{ .Index }" }
  Albums:
    rows: 1000
    columns:
      MarketingBudget: { range: [0, 10] }
`)

	fromYAML, err := ConfigFromYAML(yamlSrc)
	if err != nil {
		t.Fatalf("ConfigFromYAML: %v", err)
	}

	fromFlags, err := ConfigFromFlags(
		[]string{"Singers=10", "Albums=1000"},
		[]string{"Singers.FirstName=template:{ .Random }-{ .Index }", "Albums.MarketingBudget=range:0-10"},
	)
	if err != nil {
		t.Fatalf("ConfigFromFlags: %v", err)
	}

	if !reflect.DeepEqual(fromYAML, fromFlags) {
		t.Errorf("YAML and flags configs differ:\n yaml  = %s\n flags = %s", dumpConfig(fromYAML), dumpConfig(fromFlags))
	}
}

func dumpConfig(c *Config) string {
	var b []byte
	for _, s := range c.Tables {
		b = append(b, []byte(s.Name)...)
		b = append(b, '{')
		for k, v := range s.Columns {
			b = append(b, []byte(k+":"+v.Template)...)
			if v.Range != nil {
				b = append(b, []byte("range")...)
			}
		}
		b = append(b, '}')
	}
	return string(b)
}

func TestParseSetFlagUsesLeafTableName(t *testing.T) {
	for _, key := range []string{"Albums.MarketingBudget", "Singers.Albums.MarketingBudget"} {
		table, col, _, err := parseSetFlag(key + "=range:0-3")
		if err != nil {
			t.Fatalf("parseSetFlag(%q): %v", key, err)
		}
		if table != "Albums" || col != "MarketingBudget" {
			t.Errorf("parseSetFlag(%q) = (%q, %q), want (Albums, MarketingBudget)", key, table, col)
		}
	}
}

func TestParseRange(t *testing.T) {
	r, err := parseRange("0-10")
	if err != nil {
		t.Fatalf("parseRange: %v", err)
	}
	if r.Min != 0 || r.Max != 10 {
		t.Errorf("parseRange(0-10) = %+v", r)
	}
	if _, err := parseRange("10-0"); err == nil {
		t.Error("parseRange(10-0) should error (min > max)")
	}
}
