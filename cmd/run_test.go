package cmd

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestProgressBar(t *testing.T) {
	cases := []struct {
		ratio  float64
		filled int
	}{
		{0, 0},
		{0.4, 8},
		{0.42, 8},
		{1, 20},
		{-1, 0},
		{2, 20},
	}
	for _, c := range cases {
		bar := progressBar(c.ratio)
		if got := strings.Count(bar, "█"); got != c.filled {
			t.Errorf("progressBar(%v) filled = %d, want %d", c.ratio, got, c.filled)
		}
		if got := utf8.RuneCountInString(bar); got != progressBarWidth {
			t.Errorf("progressBar(%v) width = %d, want %d", c.ratio, got, progressBarWidth)
		}
	}
}

func TestSplitNullRates(t *testing.T) {
	global, cols, err := splitNullRates([]string{"0.3", "Albums.MarketingBudget=0.5", "Songs.Duration=0.1"})
	if err != nil {
		t.Fatalf("splitNullRates: %v", err)
	}
	if global != 0.3 {
		t.Errorf("global = %v, want 0.3", global)
	}
	if len(cols) != 2 {
		t.Errorf("cols = %v, want 2 entries", cols)
	}
}

func TestSplitNullRatesInvalid(t *testing.T) {
	for _, flags := range [][]string{{"2"}, {"x"}, {"-0.1"}} {
		if _, _, err := splitNullRates(flags); err == nil {
			t.Errorf("splitNullRates(%v): want error, got nil", flags)
		}
	}
}

func TestReporterLoadNonTTY(t *testing.T) {
	var buf bytes.Buffer
	r := &reporter{w: &buf, tty: false}
	r.load("Albums", 500, 1000)
	r.load("Albums", 1000, 1000)
	if got := buf.String(); got != "Albums  1000 rows\n" {
		t.Errorf("non-tty load output = %q", got)
	}
}

func TestReporterClearNonTTY(t *testing.T) {
	var buf bytes.Buffer
	r := &reporter{w: &buf, tty: false}
	r.clear("users", 0, false)
	r.clear("users", 1000, false)
	r.clear("users", 1500, true)
	if got := buf.String(); got != "Cleared users (1500 rows)\n" {
		t.Errorf("non-tty clear output = %q", got)
	}
}
