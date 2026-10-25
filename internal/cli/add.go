package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nostosx/nostosx-cli/internal/scaffold"
)

func newAddCommand() *cobra.Command {
	add := &cobra.Command{Use: "add", Short: "Add contexts, services and Proto APIs to a generated project"}
	for _, kind := range []string{"context", "service"} {
		var dir string
		command := &cobra.Command{
			Use: kind + " NAME", Short: "Add a " + kind + " to a generated project", Args: oneArg,
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := scaffold.Add(cmd.Context(), dir, kind, args[0]); err != nil {
					return err
				}
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "Added %s %s. After adding a service, run make generate and go mod tidy; see README.md for explicit assembly.\n", kind, args[0])
				return err
			},
		}
		command.Flags().StringVar(&dir, "dir", ".", "Generated project root")
		add.AddCommand(command)
	}
	add.AddCommand(newAddAPICommand())
	return add
}
