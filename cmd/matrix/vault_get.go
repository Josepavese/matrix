package main

import (
	"encoding/json"
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/channelcfg"
	"github.com/spf13/cobra"
)

var vaultGetReveal bool
var vaultGetField string
var vaultGetMaxBytes int

var vaultGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get a value from the Vault",
	Long: `Get a string value from the Vault.

This getter is string-oriented: it prints the text a string key holds. Records
Matrix writes as typed objects — a run record is one — are not strings, and the
command says so instead of failing a parse. For those, list the fields and their
types with --help, read one with --field <name>, or use the typed getters:
` + "`matrix vault result <run id>`" + ` for the diagnostic terminal outcome and
` + "`matrix vault summary <run id>`" + ` for the terminal summary alone.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runVaultGet(cmd, args[0]); err != nil {
			exitVaultReadError(err)
		}
	},
}

func init() {
	vaultGetCmd.Flags().BoolVar(&vaultGetReveal, "reveal", false, "print the raw value without redaction")
	vaultGetCmd.Flags().StringVar(&vaultGetField, "field", "", "print one field of a typed record instead of the whole value")
	vaultGetCmd.Flags().IntVar(&vaultGetMaxBytes, "max-bytes", defaultVaultGetMaxBytes, "refuse a field value larger than this many bytes")
	vaultGetCmd.SetHelpFunc(helpVaultGet)
	vaultCmd.AddCommand(vaultGetCmd)
}

// runVaultGet is the getter without the process exit, so its answers — the codes
// included — are testable.
func runVaultGet(cmd *cobra.Command, key string) error {
	blob, err := readVaultRecord(key)
	if err != nil {
		return err
	}
	if vaultGetField != "" {
		return printVaultRecordField(cmd, key, blob)
	}
	var value string
	if err := json.Unmarshal(blob, &value); err != nil {
		return typedRecordError(key, err)
	}
	if value == "" {
		return errVaultNotFound
	}
	if !vaultGetReveal && channelcfg.IsSecretKey(key) {
		cmd.Println(channelcfg.RedactSecret(value))
		return nil
	}
	cmd.Println(value)
	return nil
}

func printVaultRecordField(cmd *cobra.Command, key string, blob []byte) error {
	value, err := vaultRecordFieldValue(blob, key, vaultGetField)
	if err != nil {
		return err
	}
	bounded, err := vaultBoundedValue(vaultFieldTooLargeCode, fmt.Sprintf("field %q of %s", vaultGetField, key), value, vaultGetMaxBytes)
	if err != nil {
		return err
	}
	cmd.Println(bounded)
	return nil
}

// typedRecordError answers the mistake this getter used to report as a failure of
// the vault: the value is a record, the getter is a string getter, and what the
// caller needs next is the field list, not a page of unmarshal prose.
func typedRecordError(key string, parseErr error) error {
	message := fmt.Sprintf("ERR_VAULT_TYPED_RECORD %s holds a typed record, not a string", key)
	if _, known := vaultSchemaFor(key); known {
		message += fmt.Sprintf("; run `matrix vault get %s --help` for its fields, or read one with --field <name>", key)
	}
	return fmt.Errorf("%s (%w)", message, parseErr)
}

// helpVaultGet answers `--help` with the schema of the record the caller named,
// before any parse of that record can fail: knowing the fields is what turns the
// next call into a field access instead of another guess.
func helpVaultGet(cmd *cobra.Command, args []string) {
	for _, arg := range args {
		if _, known := vaultSchemaFor(arg); known {
			cmd.Println(vaultRecordSchemaText(arg))
			return
		}
	}
	// The command replaced its own help function, so the default one is restored
	// for this call: asking it for help would come straight back here.
	cmd.SetHelpFunc(nil)
	defer cmd.SetHelpFunc(helpVaultGet)
	_ = cmd.Help()
}
