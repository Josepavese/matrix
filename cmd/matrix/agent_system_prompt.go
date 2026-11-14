package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/spf13/cobra"
)

var (
	agentSystemPromptMode  string
	agentSystemPromptShow  bool
	agentSystemPromptClear bool
)

// agentSystemPromptCmd owns the client-owned instruction body an ACP agent
// accepts on session creation. The body is read from stdin so an instruction
// with quotes and newlines does not have to survive shell splitting, and it is
// stored in the vault as agent configuration rather than passed on every run.
var agentSystemPromptCmd = &cobra.Command{
	Use:   "system-prompt <agent_id>",
	Short: "Manage the client-owned system prompt sent to an ACP agent",
	Long: `Manage the client-owned system prompt sent to an ACP agent.

The text is read from stdin, so it may contain quotes and newlines. The prompt
is delivered only when the agent advertises sessionCapabilities.systemPrompt;
otherwise a run fails closed instead of silently using the agent's default
instructions. --mode override requires the agent to advertise the override mode.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		agentID := args[0]

		ctx, cleanup, err := NewAgentStoreContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		entry, err := agentcfg.LoadEntry(ctx.Store, agentID)
		if err != nil {
			exitf("Error: %v", err)
		}
		endpoint := agentcfg.NormalizeEndpoint(entry.Config)

		if agentSystemPromptShow {
			printSystemPrompt(agentID, entry.Config.SystemPromptMode, entry.Config.SystemPrompt)
			return
		}
		if agentSystemPromptClear {
			entry.Config.SystemPrompt = ""
			entry.Config.SystemPromptMode = ""
			if err := agentcfg.SaveEntry(ctx.Store, agentID, entry); err != nil {
				exitf("Error saving entry: %v", err)
			}
			printSystemPrompt(agentID, "", "")
			return
		}

		if endpoint.Kind != middleware.ProtocolKindACP {
			exitf("Error: agent %s is a %s endpoint; only ACP agents carry client system prompts", agentID, endpoint.Kind)
		}
		if cmd.Flags().Changed("mode") {
			entry.Config.SystemPromptMode = agentSystemPromptMode
		}
		prompt := middleware.SystemPrompt{Mode: entry.Config.SystemPromptMode, Text: readSystemPrompt(cmd.InOrStdin())}
		if err := middleware.ValidateSystemPrompt(prompt); err != nil {
			exitf("Error: %v", err)
		}
		if prompt.IsZero() {
			exitf("Error: system prompt is empty; use --clear to remove the configured prompt")
		}
		entry.Config.SystemPromptMode = prompt.Mode
		entry.Config.SystemPrompt = prompt.Text
		if err := agentcfg.SaveEntry(ctx.Store, agentID, entry); err != nil {
			exitf("Error saving entry: %v", err)
		}
		printSystemPrompt(agentID, entry.Config.SystemPromptMode, entry.Config.SystemPrompt)
	},
}

// readSystemPrompt reads the whole stdin body. One trailing newline is removed
// so an instruction piped from a here-document or echo arrives without the
// shell's formatting newline; any other whitespace is preserved as written.
func readSystemPrompt(reader io.Reader) string {
	body, _ := io.ReadAll(reader)
	return strings.TrimSuffix(string(body), "\n")
}

func printSystemPrompt(agentID, mode, text string) {
	out, err := json.MarshalIndent(map[string]any{
		"agent_id":            agentID,
		"system_prompt_mode":  mode,
		"system_prompt":       text,
		"system_prompt_bytes": len(text),
	}, "", "  ")
	if err != nil {
		exitf("Error: %v", err)
	}
	fmt.Println(string(out))
}

func init() {
	agentSystemPromptCmd.Flags().StringVar(&agentSystemPromptMode, "mode", middleware.SystemPromptModeAppend, "Delivery mode: append (default) or override")
	agentSystemPromptCmd.Flags().BoolVar(&agentSystemPromptShow, "show", false, "Print the configured prompt instead of reading stdin")
	agentSystemPromptCmd.Flags().BoolVar(&agentSystemPromptClear, "clear", false, "Remove the configured prompt")
	agentSystemPromptCmd.MarkFlagsMutuallyExclusive("show", "clear")
	agentCmd.AddCommand(agentSystemPromptCmd)
}
