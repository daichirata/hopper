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
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("must specify 1 argument (DATABASE)")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			databaseURI := args[0]
			tables, _ := cmd.Flags().GetStringArray("table")

			if hopper.Scheme(databaseURI) != "spanner" {
				return fmt.Errorf("DATABASE must be a spanner:// URI")
			}

			client, err := hopper.NewClient(ctx, databaseURI)
			if err != nil {
				return err
			}
			defer client.Close()

			ddl, err := client.GetDatabaseDDL(ctx)
			if err != nil {
				return err
			}
			schema, err := hopper.ParseSchema(databaseURI, ddl)
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
