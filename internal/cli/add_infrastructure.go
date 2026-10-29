package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nostosx/nostosx-cli/internal/scaffold"
)

func newAddInfrastructureCommand() *cobra.Command {
	var dir string
	command := &cobra.Command{
		Use:       "infrastructure NAME",
		Short:     "Add mysql, postgres, kafka or nats construction and pinned dependencies",
		Args:      oneArg,
		ValidArgs: []string{"mysql", "postgres", "kafka", "nats"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := scaffold.AddInfrastructure(cmd.Context(), dir, args[0]); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "Added infrastructure. Merge its config.example.toml into your configuration, construct it explicitly in app.go and pass it before transport hooks. Run go mod tidy; existing configuration and assembly were preserved.")
			return err
		},
	}
	command.Flags().StringVar(&dir, "dir", ".", "Generated project root")
	return command
}
