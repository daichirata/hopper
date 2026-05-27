package cmd

import (
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
