package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nostosx/nostosx-cli/internal/scaffold"
)

func newAPIScaffoldCommand() *cobra.Command {
	var dir, service string
	cmd := &cobra.Command{
		Use: "scaffold --service SERVICE", Short: "Fill missing unary RPC adapters from local Proto contracts, offline",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			created, err := scaffold.ScaffoldRPC(cmd.Context(), dir, service)
			if err != nil {
				return err
			}
			for _, name := range created {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Created", name); err != nil {
					return err
				}
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%d RPC adapters added. Existing implementations preserved. Run task api ACTION=generate and go mod tidy; implement authorization and application calls.\n", len(created))
			return err
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "Generated project root")
	cmd.Flags().StringVar(&service, "service", "", "Existing v1 service (required)")
	if err := cmd.MarkFlagRequired("service"); err != nil {
		panic(err)
	}
	return cmd
}

func noArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return usageError{err}
	}
	if err := cmd.ValidateRequiredFlags(); err != nil {
		return usageError{err}
	}
	return nil
}
