package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nostosx/nostosx-cli/internal/scaffold"
)

func newAddAPICommand() *cobra.Command {
	var dir string
	var options scaffold.APIOptions
	cmd := &cobra.Command{
		Use:   "api NAME --service SERVICE --context CONTEXT [--method METHOD --path PATH]",
		Short: "Add an API with a typed CQRS use case and service wiring",
		Args:  oneArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Name = args[0]
			if err := options.Validate(); err != nil {
				return usageError{err}
			}
			if err := scaffold.AddAPI(cmd.Context(), dir, options); err != nil {
				return err
			}
			next := "Implement the application use case and configure authorization in app.go. The HTTP route and CQRS chain are wired; no contract generation is needed."
			if options.Transport == "grpc" {
				next = "Run task api ACTION=generate and go mod tidy. Implement the application use case and configure authorization; the RPC and CQRS chain are wired."
			}
			if options.Transport == "gateway" {
				next = "Run task api ACTION=generate, go get github.com/grpc-ecosystem/grpc-gateway/v2@v2.29.0 and go mod tidy. Implement the application use case and configure authorization. The RPC and CQRS chain are wired; mount the gateway explicitly."
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Added API %s to %s. %s\n", options.Name, options.Service, next)
			return err
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "Generated project root")
	cmd.Flags().StringVar(&options.Service, "service", "", "Existing service (required)")
	cmd.Flags().StringVar(&options.Context, "context", "", "Existing business context (required; independent of the service)")
	cmd.Flags().StringVar(&options.Transport, "transport", "http", "API style: http (native), grpc or gateway")
	cmd.Flags().StringVar(&options.Kind, "kind", "", "CQRS kind: command or query (HTTP defaults: GET=query, others=command; required for grpc)")
	cmd.Flags().StringVar(&options.Method, "method", "", "HTTP/gateway method: GET, POST, PUT, PATCH or DELETE (requires --path; omit for grpc)")
	cmd.Flags().StringVar(&options.Path, "path", "", "HTTP/gateway path; simple {field_name} parameters become strings (requires --method)")
	for _, flag := range []string{"service", "context"} {
		if err := cmd.MarkFlagRequired(flag); err != nil {
			panic(err)
		}
	}
	return cmd
}
