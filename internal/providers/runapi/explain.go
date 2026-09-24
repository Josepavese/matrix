package runapi

import (
	"net/http"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
)

type runExplanation struct {
	RunID         string `json:"run_id"`
	Status        string `json:"status"`
	AgentID       string `json:"agent_id"`
	WorkspaceID   string `json:"workspace_id,omitempty"`
	WorkspacePath string `json:"workspace_path,omitempty"`
	Phase         string `json:"phase,omitempty"`
	FailureCode   string `json:"failure_code,omitempty"`
	PromptReceipt string `json:"prompt_receipt"`
	Cause         string `json:"cause"`
	Uncertain     bool   `json:"uncertain"`
	NextAction    string `json:"next_action"`
	TraceURL      string `json:"trace_url"`
}

func (s *Server) handleRunExplain(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	run, found, err := s.runStore.LoadRun(runID)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	events, err := s.runStore.LoadEvents(runID, 0)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	lang := "en"
	if strings.EqualFold(r.URL.Query().Get("lang"), "it") {
		lang = "it"
	}
	result := explainRun(run, events, lang)
	result.TraceURL = RunResourcePrefixV1 + runID + "/trace"
	writeJSON(w, http.StatusOK, result)
}

func explainRun(run runtrace.Run, events []runtrace.Event, lang string) runExplanation {
	out := runExplanation{RunID: run.ID, Status: run.Status, AgentID: run.AgentID,
		WorkspaceID: run.WorkspaceID, WorkspacePath: run.WorkspacePath, PromptReceipt: "unverified"}
	for _, event := range events {
		if event.Kind == "provider.preflight.failed" {
			out.Phase = event.ProtocolMethod
			if code, ok := event.Metadata["code"].(string); ok {
				out.FailureCode = code
			}
		}
		if event.Kind == "run.failed" && out.FailureCode == "" {
			if code, ok := event.Metadata["failure_code"].(string); ok {
				out.FailureCode = code
			}
		}
		if event.Kind == "agent.message.final" || event.Kind == "run.completed" {
			out.PromptReceipt = "confirmed_by_result"
		}
	}
	choice := explanationFor(lang, run.Status, out.FailureCode)
	out.Cause, out.NextAction, out.Uncertain = choice.cause, choice.action, choice.uncertain
	return out
}

type explanationText struct {
	cause, action string
	uncertain     bool
}

var runExplanations = map[string]map[string]explanationText{
	"it": {
		"completed":                          {"Run completata", "Leggi l'esito o la trace", false},
		"running":                            {"Run ancora in corso", "Attendi la notifica locale; evita un nuovo avvio", false},
		"outcome_unknown":                    {"Daemon interrotto; esito remoto sconosciuto", "Verifica la sessione remota prima di qualunque retry", true},
		"provider_auth_mismatch":             {"Autenticazione provider mancante o errata", "Completa l'autenticazione del provider", false},
		"provider_workspace_rejected":        {"Workspace rifiutato dal provider", "Completa il trust flow del provider per il path richiesto", false},
		"additional_directories_unsupported": {"Directory aggiuntive non supportate", "Usa un provider che le supporta o riduci il workspace richiesto", false},
		"cancelled":                          {"Run interrotta", "Controlla la trace e gli effetti remoti prima di riprovare", true},
		"failed":                             {"Run fallita", "Controlla fase e trace prima di riprovare", true},
	},
	"en": {
		"completed":                          {"Run completed", "Read the outcome or trace", false},
		"running":                            {"Run still active", "Wait for the local notification; avoid starting it again", false},
		"outcome_unknown":                    {"Daemon stopped; remote outcome unknown", "Inspect the remote session before any retry", true},
		"provider_auth_mismatch":             {"Provider authentication missing or invalid", "Complete the provider authentication flow", false},
		"provider_workspace_rejected":        {"Provider rejected workspace", "Complete the provider trust flow for the requested path", false},
		"additional_directories_unsupported": {"Additional directories unsupported", "Use a supporting provider or reduce requested workspace", false},
		"cancelled":                          {"Run interrupted", "Inspect the trace and remote effects before retrying", true},
		"failed":                             {"Run failed", "Inspect phase and trace before retrying", true},
	},
}

func explanationFor(lang, status, failureCode string) explanationText {
	texts, ok := runExplanations[lang]
	if !ok {
		texts = runExplanations["en"]
	}
	if value, ok := texts[status]; ok && status != runtrace.StatusCancelled && status != runtrace.StatusFailed {
		return value
	}
	if value, ok := texts[failureCode]; ok {
		return value
	}
	if value, ok := texts[status]; ok {
		return value
	}
	return texts["failed"]
}
