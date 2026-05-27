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
      FirstName: "{{ FirstName }}-{{ Index }}"
  Albums:
    rows: 1000
    columns:
      MarketingBudget: "{{ Number 0 1000000 }}"
`)

	fromYAML, err := ConfigFromYAML(yamlSrc)
	if err != nil {
		t.Fatalf("ConfigFromYAML: %v", err)
	}

	fromFlags, err := ConfigFromFlags(
		[]string{"Singers=10", "Albums=1000"},
		[]string{"Singers.FirstName={{ FirstName }}-{{ Index }}", "Albums.MarketingBudget={{ Number 0 1000000 }}"},
	)
	if err != nil {
		t.Fatalf("ConfigFromFlags: %v", err)
	}

	if !reflect.DeepEqual(fromYAML, fromFlags) {
		t.Errorf("YAML and flags configs differ:\n yaml  = %+v\n flags = %+v", fromYAML.Tables, fromFlags.Tables)
	}
}

func TestConfigApplyFlagsOverridesYAML(t *testing.T) {
	c, err := ConfigFromYAML([]byte(`
tables:
  Singers:
    rows: 10
    columns:
      FirstName: "{{ FirstName }}"
`))
	if err != nil {
		t.Fatalf("ConfigFromYAML: %v", err)
	}
	if err := c.ApplyFlags([]string{"Singers=50"}, []string{"Singers.LastName={{ LastName }}"}); err != nil {
		t.Fatalf("ApplyFlags: %v", err)
	}

	var singers *TableSpec
	for _, ts := range c.Tables {
		if ts.Name == "Singers" {
			singers = ts
		}
	}
	if singers == nil {
		t.Fatal("Singers not found")
	}
	if singers.Rows != 50 {
		t.Errorf("Rows = %d, want 50 (overridden by flag)", singers.Rows)
	}
	if singers.Columns["FirstName"] == "" {
		t.Error("FirstName from YAML was lost")
	}
	if singers.Columns["LastName"] == "" {
		t.Error("LastName from flag is missing")
	}
}

func TestParseSetFlagLeafTable(t *testing.T) {
	for _, key := range []string{"Albums.MarketingBudget", "Singers.Albums.MarketingBudget"} {
		table, col, pattern, err := parseSetFlag(key + "={{ Number 0 3 }}")
		if err != nil {
			t.Fatalf("parseSetFlag(%q): %v", key, err)
		}
		if table != "Albums" || col != "MarketingBudget" {
			t.Errorf("parseSetFlag(%q) = (%q, %q), want (Albums, MarketingBudget)", key, table, col)
		}
		if pattern != "{{ Number 0 3 }}" {
			t.Errorf("parseSetFlag(%q) pattern = %q", key, pattern)
		}
	}
}
