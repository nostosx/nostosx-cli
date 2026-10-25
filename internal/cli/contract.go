package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nostosx/nostosx-cli/internal/contract"
)

func newContractCommand() *cobra.Command {
	api := &cobra.Command{Use: "api", Short: "Scaffold, build and verify transport contracts"}
	api.AddCommand(newAPIScaffoldCommand())
	for _, operation := range []struct{ name, summary string }{
		{"generate", "Generate Go, gRPC, HTTP gateway and OpenAPI output"},
		{"lint", "Lint Proto contracts with Buf"},
		{"check", "Verify generated contracts without changing published output"},
		{"breaking", "Check compatibility against an explicit Buf input"},
	} {
		action := operation.name
		var dir, against string
		command := &cobra.Command{
			Use: action, Short: operation.summary,
			Args: func(cmd *cobra.Command, args []string) error {
				if err := cobra.NoArgs(cmd, args); err != nil {
					return usageError{err}
				}
				if err := cmd.ValidateRequiredFlags(); err != nil {
					return usageError{err}
				}
				return nil
			},
			RunE: func(cmd *cobra.Command, _ []string) error {
				if action == "breaking" && strings.TrimSpace(against) == "" {
					return usageError{fmt.Errorf("--against requires a non-empty baseline")}
				}
				return contract.Run(cmd.Context(), dir, action, against, cmd.OutOrStdout(), cmd.ErrOrStderr())
			},
		}
		command.Flags().StringVar(&dir, "dir", ".", "Generated project root (trusted Go/Buf configuration)")
		if action == "breaking" {
			command.Flags().StringVar(&against, "against", "", "Explicit Buf baseline, for example .git#ref=main (required)")
			if err := command.MarkFlagRequired("against"); err != nil {
				panic(err)
			}
		}
		api.AddCommand(command)
	}
	return api
}
