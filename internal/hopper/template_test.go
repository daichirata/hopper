package hopper

import (
	"math/rand"
	"strings"
	"testing"
)

func TestTemplateRender(t *testing.T) {
	tpl, err := NewTemplate("{ .Random }-{ .Index }")
	if err != nil {
		t.Fatalf("NewTemplate: %v", err)
	}
	rng := rand.New(rand.NewSource(1))

	out, err := tpl.Render(7, rng)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.HasSuffix(out, "-7") {
		t.Errorf("Render = %q, want suffix -7", out)
	}
	prefix := strings.TrimSuffix(out, "-7")
	if len(prefix) != 8 {
		t.Errorf("random prefix = %q (len %d), want len 8", prefix, len(prefix))
	}

	a, _ := tpl.Render(0, rng)
	b, _ := tpl.Render(0, rng)
	if a == b {
		t.Errorf("expected distinct random values, got %q twice", a)
	}
}
