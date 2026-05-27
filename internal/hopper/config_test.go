package hopper

import (
	"reflect"
	"testing"
)

func TestConfigYAMLAndFlagsEquivalent(t *testing.T) {
	yamlSrc := []byte(`
tables:
  Users:
    rows: 10
    columns:
      Name: { template: "{ .Random }-{ .Index }" }
      ShardId: { range: [0, 10] }
    children:
      UserAvatars:
        rows_per_parent: 5
`)

	fromYAML, err := ConfigFromYAML(yamlSrc)
	if err != nil {
		t.Fatalf("ConfigFromYAML: %v", err)
	}

	fromFlags, err := ConfigFromFlags(
		[]string{"Users=10", "Users.UserAvatars=5"},
		[]string{"Users.Name=template:{ .Random }-{ .Index }", "Users.ShardId=range:0-10"},
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
	var walk func(specs []*TableSpec, depth string)
	walk = func(specs []*TableSpec, depth string) {
		for _, s := range specs {
			b = append(b, []byte(depth+s.Name)...)
			b = append(b, []byte("{")...)
			for k, v := range s.Columns {
				b = append(b, []byte(k+":"+v.Template)...)
				if v.Range != nil {
					b = append(b, []byte("range")...)
				}
			}
			b = append(b, []byte("}")...)
			walk(s.Children, depth+"  ")
		}
	}
	walk(c.Tables, "")
	return string(b)
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
