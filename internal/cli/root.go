// Package cli defines the nostosx command tree. Each invocation owns its flags.
package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// New returns a command suitable for embedding or tests with caller-owned I/O.
func New(version string) *cobra.Command {
	root := &cobra.Command{
		Use: "nostosx", Short: "Generate Go applications with servex and optional agentx",
		Version: version, SilenceUsage: true, SilenceErrors: true,
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err} })
	root.AddCommand(newProjectCommand(), newAddCommand(), newContractCommand(), newDoctorCommand(version))
	return root
}

// Execute is the process boundary; command handlers return errors without exiting.
func Execute(cmd *cobra.Command) int {
	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "nostosx:", err)
		if _, ok := errors.AsType[usageError](err); ok {
			return 2
		}
		return 1
	}
	return 0
}

type usageError struct{ error }

func oneArg(cmd *cobra.Command, args []string) error {
	if err := cobra.ExactArgs(1)(cmd, args); err != nil {
		return usageError{err}
	}
	// Cobra's required-flag check does not call SetFlagErrorFunc.
	if err := cmd.ValidateRequiredFlags(); err != nil {
		return usageError{err}
	}
	return nil
}
