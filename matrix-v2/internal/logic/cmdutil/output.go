package cmdutil

import (
	"encoding/json"

	"github.com/spf13/cobra"
)

func PrintJSON(cmd *cobra.Command, payload any) error {
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	cmd.Println(string(out))
	return nil
}
