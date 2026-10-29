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
			options.Infrastructure = nil
			for _, feature := range features {
				switch feature {
				case "agentx":
					options.Agent = true
				case "mysql", "postgres", "postgresql", "kafka", "nats":
					options.Infrastructure = append(options.Infrastructure, feature)
				default:
					return usageError{fmt.Errorf("unsupported feature %q (use agentx, mysql, postgres, kafka or nats)", feature)}
				}
			}
			dir, err := scaffold.New(cmd.Context(), options)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), dir)
			if err == nil && len(options.Infrastructure) != 0 {
				_, err = fmt.Fprintln(cmd.ErrOrStderr(), "Optional infrastructure added. Run go mod tidy in the generated project before checking or building; supply connection settings before starting.")
			}
			return err
		},
	}
	create.Flags().StringVar(&options.Module, "module", "", "Go module path (required)")
	create.Flags().StringVar(&options.Layout, "layout", "monolith", "Process topology: monolith or microservices")
	create.Flags().StringVar(&options.Output, "output", "", "New destination directory (default: NAME); parent must exist")
	create.Flags().StringVar(&options.Transport, "transport", "http", "Enabled transports: http, grpc or http,grpc")
	create.Flags().StringSliceVar(&features, "with", nil, "Optional features: agentx,mysql,postgres,kafka,nats")
	// The static flag name is registered immediately above.
	if err := create.MarkFlagRequired("module"); err != nil {
		panic(err)
	}
	return create
}
