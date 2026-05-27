package main

import (
	"os"

	"github.com/daichirata/hopper/cmd"
)

var version string

func main() {
	cmd.Version = version

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
