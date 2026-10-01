package main

import (
	"encoding/json"
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/cmdutil"
	"github.com/Josepavese/matrix/internal/logic/runtimeendpoints"
	"github.com/Josepavese/matrix/internal/providers/osfs"
	"github.com/spf13/cobra"
)

var runtimeEndpointsJSON bool

var runtimeEndpointsCmd = &cobra.Command{
	Use:   "endpoints",
	Short: "Show the runtime endpoints a client must use",
	Long: `Show how to reach this Matrix runtime.

The addresses come from what the daemon published (the runtime broker
descriptor) and from the installation's own configuration, each labelled with
its source. The runtime log is never parsed: a log records what happened once,
while the descriptor records what is bound now.

Nothing secret is printed. A surface that authenticates names the credential it
expects and, for the broker, the file the token lives in.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		if err := runRuntimeEndpoints(cmd); err != nil {
			exitf("Error: %v", err)
		}
	},
}

var runtimeCmd = &cobra.Command{
	Use:   "runtime",
	Short: "Inspect how clients reach this Matrix runtime",
}

func runRuntimeEndpoints(cmd *cobra.Command) error {
	home, err := resolveActiveHome()
	if err != nil {
		return err
	}
	manager, cleanup, err := cmdutil.OpenReadOnlyConfigManager(DefaultVaultPath)
	if err != nil {
		return fmt.Errorf("read runtime configuration: %w", err)
	}
	defer cleanup()

	surfaces, notes := runtimeendpoints.Discover(runtimeendpoints.Input{
		Home:           home,
		ConfiguredRPC:  manager.GetWithDefault("jsonrpc_addr", ""),
		ConfiguredHTTP: manager.GetWithDefault("matrix_http_addr", ""),
		RPCKey:         manager.GetWithDefault("daemon_api_key", ""),
		HTTPKey:        manager.GetWithDefault("matrix_api_key", ""),
	}, osfs.NewFSProvider())

	if runtimeEndpointsJSON {
		encoded, encodeErr := json.MarshalIndent(map[string]any{"surfaces": surfaces, "notes": notes}, "", "  ")
		if encodeErr != nil {
			return encodeErr
		}
		cmd.Println(string(encoded))
		return nil
	}
	for _, surface := range surfaces {
		cmd.Printf("%s transport=%s address=%s source=%s auth=%s exposure=%s\n",
			surface.Kind, surface.Transport, surface.Address, surface.Source, surface.Auth, surface.Exposure)
		if surface.TokenFile != "" {
			cmd.Printf("  token_file=%s\n", surface.TokenFile)
		}
		if surface.Warning != "" {
			cmd.Printf("  warning=%s\n", surface.Warning)
		}
	}
	for _, note := range notes {
		cmd.Printf("note=%s\n", note)
	}
	return nil
}

func init() {
	runtimeEndpointsCmd.Flags().BoolVar(&runtimeEndpointsJSON, "json", false, "print the machine-readable surface list")
	runtimeCmd.AddCommand(runtimeEndpointsCmd)
	rootCmd.AddCommand(runtimeCmd)
}
