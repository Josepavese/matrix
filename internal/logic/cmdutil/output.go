package cmdutil

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// PrintJSON writes a JSON-encoded payload to the command's output.
func PrintJSON(cmd *cobra.Command, payload any) error {
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	// cmd.Println writes to OutOrStderr, which on a command without an explicit
	// output drains JSON to stderr: `matrix ... > out.json` would produce an
	// empty file. Machine-readable output must go to stdout.
	_, err = fmt.Fprintln(cmd.OutOrStdout(), string(out))
	return err
}
