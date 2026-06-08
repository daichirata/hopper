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

func TestConfigApplyNullRates(t *testing.T) {
	c := &Config{}
	if err := c.ApplyNullRates([]string{"Albums.MarketingBudget=0.33"}); err != nil {
		t.Fatalf("ApplyNullRates: %v", err)
	}
	if got := c.Tables[0].NullRates["MarketingBudget"]; got != 0.33 {
		t.Errorf("NullRates[MarketingBudget] = %v, want 0.33", got)
	}
}

func TestConfigApplyNullRatesInvalid(t *testing.T) {
	for _, spec := range []string{"Albums.MarketingBudget=2", "Albums.MarketingBudget=x", "MarketingBudget=0.5", "Albums.MarketingBudget"} {
		c := &Config{}
		if err := c.ApplyNullRates([]string{spec}); err == nil {
			t.Errorf("ApplyNullRates(%q): want error, got nil", spec)
		}
	}
}

func TestConfigColumnSpecFromYAML(t *testing.T) {
	c, err := ConfigFromYAML([]byte(`
tables:
  Albums:
    rows: 100
    columns:
      AlbumTitle: "{{ Sentence 3 }}"
      MarketingBudget:
        template: "{{ Number 0 1000000 }}"
        null_rate: 0.3
      ReleaseDate:
        null_rate: 0.5
`))
	if err != nil {
		t.Fatalf("ConfigFromYAML: %v", err)
	}
	ts := c.Tables[0]
	if got := ts.Columns["AlbumTitle"]; got != "{{ Sentence 3 }}" {
		t.Errorf("Columns[AlbumTitle] = %q, want template", got)
	}
	if got := ts.Columns["MarketingBudget"]; got != "{{ Number 0 1000000 }}" {
		t.Errorf("Columns[MarketingBudget] = %q, want template", got)
	}
	if got := ts.NullRates["MarketingBudget"]; got != 0.3 {
		t.Errorf("NullRates[MarketingBudget] = %v, want 0.3", got)
	}
	if got := ts.NullRates["ReleaseDate"]; got != 0.5 {
		t.Errorf("NullRates[ReleaseDate] = %v, want 0.5", got)
	}
	if _, ok := ts.Columns["ReleaseDate"]; ok {
		t.Error("ReleaseDate should have no template")
	}
}

func TestConfigColumnSpecInvalidNullRate(t *testing.T) {
	_, err := ConfigFromYAML([]byte(`
tables:
  Albums:
    rows: 100
    columns:
      MarketingBudget:
        null_rate: 2
`))
	if err == nil {
		t.Fatal("ConfigFromYAML with null_rate 2: want error, got nil")
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
