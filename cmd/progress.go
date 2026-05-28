package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const progressBarWidth = 20

type reporter struct {
	w         io.Writer
	tty       bool
	nameWidth int
}

func newReporter(f *os.File) *reporter {
	return &reporter{w: f, tty: isTerminal(f)}
}

func (r *reporter) prepare(tables []string) {
	for _, t := range tables {
		if len(t) > r.nameWidth {
			r.nameWidth = len(t)
		}
	}
}

func (r *reporter) clear(table string, deleted int64, done bool) {
	if !r.tty {
		if done {
			fmt.Fprintf(r.w, "Cleared %s (%d rows)\n", table, deleted)
		}
		return
	}
	switch {
	case done:
		fmt.Fprintf(r.w, "\rCleared %-*s (%d rows)\033[K\n", r.nameWidth, table, deleted)
	case deleted == 0:
		fmt.Fprintf(r.w, "\rClearing %-*s ...\033[K", r.nameWidth, table)
	default:
		fmt.Fprintf(r.w, "\rClearing %-*s ... %d deleted\033[K", r.nameWidth, table, deleted)
	}
}

func (r *reporter) load(table string, done, total int) {
	if !r.tty {
		if done >= total {
			fmt.Fprintf(r.w, "%s  %d rows\n", table, done)
		}
		return
	}
	ratio := 0.0
	if total > 0 {
		ratio = float64(done) / float64(total)
	}
	fmt.Fprintf(r.w, "\rLoading %-*s [%s] %3.0f%%  (%d/%d)", r.nameWidth, table, progressBar(ratio), ratio*100, done, total)
	if done >= total {
		fmt.Fprintln(r.w)
	}
}

func progressBar(ratio float64) string {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio * progressBarWidth)
	return strings.Repeat("█", filled) + strings.Repeat("░", progressBarWidth-filled)
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
