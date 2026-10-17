package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var version = "dev"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the hibiki version",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "hibiki %s\n", version)
			return nil
		},
	}
}
