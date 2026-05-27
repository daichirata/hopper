package cmd

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/daichirata/hopper/internal/hopper"
)

var (
	runExample = `
* Load 1000 rows into Users (PK auto-generated, other columns type-default)
  hopper run spanner://projects/p/instances/i/databases/d --table 'Users=1000'

* Interleaved children: 100 UserAvatars per User, with column rules
  hopper run spanner://projects/p/instances/i/databases/d \
    --table 'Users=10' --table 'Users.UserAvatars=100' \
    --set 'Users.Name=template:{ .Random }-{ .Index }' --set 'Users.ShardId=range:0-10'

* Child only (parent is auto-completed)
  hopper run spanner://projects/p/instances/i/databases/d --table 'UserAvatars=300'

* From a config file
  hopper run spanner://projects/p/instances/i/databases/d --config hopper.yaml`

	runCmd = &cobra.Command{
		Use:     "run DATABASE",
		Short:   "Generate and load dummy data into Spanner",
		Example: runExample,
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
			gen := hopper.NewGenerator(rand.New(rand.NewSource(seed)))

			runner := hopper.NewRunner(schema, gen, client)
			runner.DryRun = dryRun

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
	runCmd.Flags().StringP("config", "c", "", "path to YAML config file")
	runCmd.Flags().StringArray("table", nil, "table rows as PATH=N (e.g. 'Users=1000' or 'Users.UserAvatars=100')")
	runCmd.Flags().StringArray("set", nil, "column rule as PATH.Column=RULE (e.g. 'Users.Name=template:{ .Random }-{ .Index }')")
	runCmd.Flags().Int64("seed", 0, "random seed (0 = time-based)")
	runCmd.Flags().Bool("dry-run", false, "generate rows but do not insert")

	rootCmd.AddCommand(runCmd)
}
