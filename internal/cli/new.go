package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nostosx/nostosx-cli/internal/scaffold"
)

func newProjectCommand() *cobra.Command {
	var options scaffold.Options
	var features []string
	create := &cobra.Command{
		Use: "new NAME --module MODULE", Short: "Create a single-module monolith or service monorepo",
		Args: oneArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Name = args[0]
			options.Agent = false
			for _, feature := range features {
				if feature != "agentx" {
					return usageError{fmt.Errorf("unsupported feature %q (use agentx)", feature)}
				}
				options.Agent = true
			}
			dir, err := scaffold.New(cmd.Context(), options)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), dir)
			return err
		},
	}
	create.Flags().StringVar(&options.Module, "module", "", "Go module path (required)")
	create.Flags().StringVar(&options.Layout, "layout", "monolith", "Process topology: monolith or microservices")
	create.Flags().StringVar(&options.Output, "output", "", "New destination directory (default: NAME); parent must exist")
	create.Flags().StringSliceVar(&features, "with", nil, "Optional feature: agentx")
	// The static flag name is registered immediately above.
	if err := create.MarkFlagRequired("module"); err != nil {
		panic(err)
	}
	return create
}
