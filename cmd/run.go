package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/daichirata/hopper/internal/hopper"
)

var (
	runCmd = &cobra.Command{
		Use:   "run DATABASE",
		Short: "Generate and load dummy data into Spanner",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("must specify 1 argument (DATABASE)")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			databaseURI := args[0]

			configPath, _ := cmd.Flags().GetString("config")
			tableFlags, _ := cmd.Flags().GetStringArray("table")
			setFlags, _ := cmd.Flags().GetStringArray("set")
			seed, _ := cmd.Flags().GetInt64("seed")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			noInfer, _ := cmd.Flags().GetBool("no-infer")

			if hopper.Scheme(databaseURI) != "spanner" {
				return fmt.Errorf("DATABASE must be a spanner:// URI")
			}

			config, err := buildConfig(configPath, tableFlags, setFlags)
			if err != nil {
				return err
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

			if seed == 0 {
				seed = time.Now().UnixNano()
			}
			gen := hopper.NewGenerator(uint64(seed))

			runner := hopper.NewRunner(schema, gen, client)
			runner.DryRun = dryRun
			runner.Infer = !noInfer

			results, err := runner.Run(ctx, config)
			if err != nil {
				return err
			}
			for _, res := range results {
				fmt.Printf("%s\t%d rows\n", res.Table, res.Rows)
			}
			return nil
		},
	}
)

func buildConfig(configPath string, tableFlags, setFlags []string) (*hopper.Config, error) {
	switch {
	case configPath != "":
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, err
		}
		return hopper.ConfigFromYAML(data)
	case len(tableFlags) > 0 || len(setFlags) > 0:
		return hopper.ConfigFromFlags(tableFlags, setFlags)
	default:
		return nil, fmt.Errorf("specify --config or at least one --table")
	}
}

func init() {
	runCmd.Flags().StringP("config", "c", "", "path to a YAML config file")
	runCmd.Flags().StringArray("table", nil, "rows to generate as TABLE=N (repeatable); child rows are distributed across parents")
	runCmd.Flags().StringArray("set", nil, "column template as TABLE.COLUMN=TEMPLATE (repeatable); gofakeit template, e.g. '{{ Number 0 100 }}' or '{{ FirstName }}-{{ Index }}'")
	runCmd.Flags().Int64("seed", 0, "random seed (0 = time-based)")
	runCmd.Flags().Bool("dry-run", false, "generate rows but do not insert")
	runCmd.Flags().Bool("no-infer", false, "disable inferring a gofakeit function from unset column names")

	rootCmd.AddCommand(runCmd)
}
