package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nostosx/nostosx-cli/internal/doctor"
)

func newDoctorCommand(version string) *cobra.Command {
	var dir string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use: "doctor", Short: "Inspect a generated project and trusted tool versions without changing files", Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report := doctor.Run(cmd.Context(), dir, version)
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				if err := encoder.Encode(report); err != nil {
					return err
				}
			} else {
				for _, check := range report.Checks {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s: %s\n", check.Status, check.Name, check.Detail); err != nil {
						return err
					}
					if check.Status == "fail" {
						if _, err := fmt.Fprintln(cmd.OutOrStdout(), "  Fix:", check.Fix); err != nil {
							return err
						}
					}
				}
			}
			if !report.Healthy() {
				return errors.New("doctor found failed checks")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "Generated project root")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Print a JSON report to stdout")
	return cmd
}
