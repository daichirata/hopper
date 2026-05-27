package cmd

import (
	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:          "hopper",
		Short:        "hopper is a command-line tool to load dummy data into Google Cloud Spanner.",
		SilenceUsage: true,
	}
)

func Execute() error {
	return rootCmd.Execute()
}
