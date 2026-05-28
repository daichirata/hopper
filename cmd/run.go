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
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			configPath, _ := cmd.Flags().GetString("config")
			tableFlags, _ := cmd.Flags().GetStringArray("table")
			setFlags, _ := cmd.Flags().GetStringArray("set")
			seed, _ := cmd.Flags().GetInt64("seed")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			noInfer, _ := cmd.Flags().GetBool("no-infer")
			clear, _ := cmd.Flags().GetBool("clear")
			clearBatch, _ := cmd.Flags().GetInt("clear-batch-size")
			nullRate, _ := cmd.Flags().GetFloat64("null-rate")
			verbose, _ := cmd.Flags().GetBool("verbose")

			if nullRate < 0 || nullRate > 1 {
				return fmt.Errorf("--null-rate must be between 0 and 1, got %v", nullRate)
			}
			if clearBatch < 1 {
				return fmt.Errorf("--clear-batch-size must be >= 1, got %d", clearBatch)
			}

			config, err := buildConfig(configPath, tableFlags, setFlags)
			if err != nil {
				return err
			}

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

			if seed == 0 {
				seed = time.Now().UnixNano()
			}
			if verbose {
				fmt.Fprintf(os.Stderr, "Seed: %d\n", seed)
			}
			runner := hopper.NewRunner(schema, hopper.NewGenerator(uint64(seed)), client)
			runner.DryRun = dryRun
			runner.Infer = !noInfer
			runner.Clear = clear
			runner.ClearBatchSize = clearBatch
			runner.NullRate = nullRate

			rep := newReporter(os.Stderr)
			runner.OnStart = rep.prepare
			runner.OnClear = rep.clear
			runner.Progress = rep.load

			results, err := runner.Run(ctx, config)
			if err != nil {
				return err
			}
			printResults(results)
			return nil
		},
	}
)

func buildConfig(configPath string, tableFlags, setFlags []string) (*hopper.Config, error) {
	config := &hopper.Config{}
	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, err
		}
		config, err = hopper.ConfigFromYAML(data)
		if err != nil {
			return nil, err
		}
	}
	if err := config.ApplyFlags(tableFlags, setFlags); err != nil {
		return nil, err
	}
	config.Normalize()
	if len(config.Tables) == 0 {
		return nil, fmt.Errorf("specify --config or at least one --table")
	}
	return config, nil
}

func printResults(results []hopper.Result) {
	for _, res := range results {
		fmt.Printf("%s\t%d rows\n", res.Table, res.Rows)
		for _, row := range res.Sample {
			fmt.Printf("  %v\n", row)
		}
	}
}

func init() {
	runCmd.Flags().StringP("config", "c", "", "path to a YAML config file")
	runCmd.Flags().StringArray("table", nil, "rows to generate as TABLE=N (repeatable); child rows are distributed across parents")
	runCmd.Flags().StringArray("set", nil, "column template as TABLE.COLUMN=TEMPLATE (repeatable); gofakeit template, e.g. '{{ Number 0 100 }}' or '{{ FirstName }}-{{ Index }}'")
	runCmd.Flags().Int64("seed", 0, "random seed (0 = time-based)")
	runCmd.Flags().Bool("dry-run", false, "generate rows but do not insert")
	runCmd.Flags().Bool("no-infer", false, "disable inferring a gofakeit function from unset column names")
	runCmd.Flags().Bool("clear", false, "delete existing rows from each target table before loading")
	runCmd.Flags().Int("clear-batch-size", hopper.DefaultClearBatchSize, "max rows per Delete commit during --clear (auto-halved on too-many-mutations)")
	runCmd.Flags().Float64("null-rate", 0, "probability (0-1) of setting a nullable, unset column to NULL")
	runCmd.Flags().BoolP("verbose", "v", false, "print extra runtime information on stderr (e.g. the seed)")

	rootCmd.AddCommand(runCmd)
}
