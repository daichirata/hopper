package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/daichirata/hopper/internal/hopper"
)

var (
	scaffoldCmd = &cobra.Command{
		Use:   "scaffold DATABASE",
		Short: "Print a config template generated from the database schema",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			tables, _ := cmd.Flags().GetStringArray("table")

			uri := args[0]
			if hopper.Scheme(uri) != "spanner" {
				return fmt.Errorf("DATABASE must be a spanner:// URI")
			}
			client, err := hopper.NewClient(ctx, uri)
			if err != nil {
				return err
			}
			defer client.Close()
			ddl, err := client.GetDatabaseDDL(ctx)
			if err != nil {
				return err
			}
			schema, err := hopper.ParseSchema(uri, ddl)
			if err != nil {
				return err
			}

			out, err := hopper.Scaffold(schema, tables, hopper.NewGenerator(uint64(time.Now().UnixNano())))
			if err != nil {
				return err
			}
			fmt.Print(out)
			return nil
		},
	}
)

func init() {
	scaffoldCmd.Flags().StringArray("table", nil, "table to include (repeatable; default: all)")

	rootCmd.AddCommand(scaffoldCmd)
}
