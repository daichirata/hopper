package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
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
			truncate, _ := cmd.Flags().GetBool("truncate")
			nullRate, _ := cmd.Flags().GetFloat64("null-rate")

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
			runner.Truncate = truncate
			runner.NullRate = nullRate
			tty := isTerminal(os.Stderr)
			runner.Progress = func(table string, done, total int) {
				if !tty {
					if done >= total {
						fmt.Fprintf(os.Stderr, "%s  %d rows\n", table, done)
					}
					return
				}
				ratio := 0.0
				if total > 0 {
					ratio = float64(done) / float64(total)
				}
				fmt.Fprintf(os.Stderr, "\rLoading %-16s [%s] %3.0f%%  (%d/%d)", table, progressBar(ratio), ratio*100, done, total)
				if done >= total {
					fmt.Fprintln(os.Stderr)
				}
			}

			results, err := runner.Run(ctx, config)
			if err != nil {
				return err
			}
			for _, res := range results {
				fmt.Printf("%s\t%d rows\n", res.Table, res.Rows)
				for _, row := range res.Sample {
					fmt.Printf("  %v\n", row)
				}
			}
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

const progressBarWidth = 20

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

func init() {
	runCmd.Flags().StringP("config", "c", "", "path to a YAML config file")
	runCmd.Flags().StringArray("table", nil, "rows to generate as TABLE=N (repeatable); child rows are distributed across parents")
	runCmd.Flags().StringArray("set", nil, "column template as TABLE.COLUMN=TEMPLATE (repeatable); gofakeit template, e.g. '{{ Number 0 100 }}' or '{{ FirstName }}-{{ Index }}'")
	runCmd.Flags().Int64("seed", 0, "random seed (0 = time-based)")
	runCmd.Flags().Bool("dry-run", false, "generate rows but do not insert")
	runCmd.Flags().Bool("no-infer", false, "disable inferring a gofakeit function from unset column names")
	runCmd.Flags().Bool("truncate", false, "delete existing rows from each target table before loading")
	runCmd.Flags().Float64("null-rate", 0, "probability (0-1) of setting a nullable, unset column to NULL")

	rootCmd.AddCommand(runCmd)
}
