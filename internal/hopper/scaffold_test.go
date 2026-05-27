package hopper

import (
	"strings"
	"testing"
)

func TestScaffold(t *testing.T) {
	schema, err := ParseSchema("test", testDDL)
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}

	out, err := Scaffold(schema, []string{"Singers"}, NewGenerator(1))
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	if !strings.Contains(out, `FirstName: "{{ FirstName }}"`) {
		t.Errorf("missing FirstName suggestion:\n%s", out)
	}
	if strings.Contains(out, "SingerId") {
		t.Error("primary key SingerId should be omitted")
	}
	if strings.Contains(out, "FullName") {
		t.Error("generated column FullName should be omitted")
	}

	cfg, err := ConfigFromYAML([]byte(out))
	if err != nil {
		t.Fatalf("scaffold output is not valid config: %v", err)
	}
	if len(cfg.Tables) != 1 || cfg.Tables[0].Name != "Singers" {
		t.Errorf("config tables = %+v", cfg.Tables)
	}
}
