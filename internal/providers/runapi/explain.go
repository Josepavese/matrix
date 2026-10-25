package runapi

import (
	"net/http"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/deliverycontract"
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
	// StopReason is what the provider reported ended the turn, as it reported
	// it. "unreported" means it reported nothing, which is not the same as
	// "end_turn": a consumer must not be handed a reason Matrix invented.
	StopReason string `json:"stop_reason,omitempty"`
	// AcceptanceStatus is what the caller's declared delivery contract could be
	// checked against, which is a different question from what the protocol
	// reported: a run can be completed and its delivery incomplete, and reporting
	// only the first is how "completed" came to look like "the work arrived".
	AcceptanceStatus string `json:"acceptance_status"`
	// Delivery carries the individual requirements and their outcomes, so a
	// reader sees which one failed instead of only that something did.
	Delivery      *deliverycontract.Verdict `json:"delivery,omitempty"`
	PromptReceipt string                    `json:"prompt_receipt"`
	Cause         string                    `json:"cause"`
	Uncertain     bool                      `json:"uncertain"`
	NextAction    string                    `json:"next_action"`
	TraceURL      string                    `json:"trace_url"`
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
	out := runExplanation{
		RunID: run.ID, Status: run.Status, AgentID: run.AgentID,
		WorkspaceID: run.WorkspaceID, WorkspacePath: run.WorkspacePath,
		StopReason: run.StopReason, PromptReceipt: "unverified",
	}
	out.AcceptanceStatus, out.Delivery = deliveryExplanation(events)
	// A denied tool permission followed by a turn that delivered no answer is a
	// terminal non-delivery, not an answered prompt. The structural rule is
	// shared with the completion verdict, so explain cannot contradict the run
	// record it is describing.
	deniedNonDelivery := runtrace.DeniedPermissionWithoutAnswer(run.Output, events)
	out.Phase, out.FailureCode = explainFailureCode(events)
	if deniedNonDelivery && (run.Status == runtrace.StatusCompleted || run.Status == runtrace.StatusFailed) {
		// Records written before the completion rule learned this case still say
		// "completed": their trace is the evidence, so explain names the denial
		// instead of repeating the record's optimistic status.
		out.FailureCode = runtrace.FailureCodeProviderToolPermissionDenied
	}
	out.PromptReceipt = explainPromptReceipt(events, deniedNonDelivery)
	choice := explanationFor(lang, run.Status, out.FailureCode)
	out.Cause, out.NextAction, out.Uncertain = choice.cause, choice.action, choice.uncertain
	return out
}

// explainFailureCode is the failure the event stream evidences: the phase and
// code of a typed provider failure, or the failure code a terminal run recorded.
// The scan preserves the stream's own precedence: a later typed provider failure
// overwrites an earlier recorded failure, and a recorded failure only fills an
// empty code.
func explainFailureCode(events []runtrace.Event) (phase, failureCode string) {
	for _, event := range events {
		switch event.Kind {
		case "provider.preflight.failed", "provider.runtime.failed":
			phase = event.ProtocolMethod
			if code, ok := event.Metadata["code"].(string); ok {
				failureCode = code
			}
		case "run.failed":
			if failureCode == "" {
				if code, ok := event.Metadata["failure_code"].(string); ok {
					failureCode = code
				}
			}
		}
	}
	return phase, failureCode
}

// explainPromptReceipt decides whether the run's own result confirms the prompt
// was answered. A denied-permission non-delivery never confirms it, even when an
// older record still carries a terminal "completed" event, and a run with no
// turn evidence never confirms it either: the terminal event says the run
// stopped, not that the peer answered.
func explainPromptReceipt(events []runtrace.Event, deniedNonDelivery bool) string {
	if deniedNonDelivery || !hasTurnEvidence(events) {
		return "unverified"
	}
	for _, event := range events {
		if event.Kind == "agent.message.final" || event.Kind == "run.completed" {
			return "confirmed_by_result"
		}
	}
	return "unverified"
}

// hasTurnEvidence reports whether a run's events carry anything the peer actually
// produced: a message event or a tool call. Operational events — routing, model
// selection, the prompt record, the terminal transition — exist for every run,
// including the ones that produced nothing, so they are not evidence.
//
// The rule reads event kinds only. It never inspects event text, so it cannot
// become a text heuristic, and it names no provider.
func hasTurnEvidence(events []runtrace.Event) bool {
	for _, event := range events {
		if strings.HasPrefix(event.Kind, "agent.message.") || isToolEvent(event.Kind) {
			return true
		}
	}
	return false
}

func isToolEvent(kind string) bool {
	return kind == "tool.call.requested" || kind == "tool.result.received"
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
		"run_no_turn_evidence":               {"Il provider ha chiuso il turno senza output, tool call o messaggio", "Ripeti il run o ispeziona la sessione remota: il turno non ha prodotto nulla di utilizzabile", true},
		runtrace.FailureCodeProviderToolPermissionDenied: {"Il provider ha chiuso il turno dopo un permesso tool negato senza consegnare una risposta", "Il turno è terminale: non riconciliarlo come successo; ispeziona trace e sessione remota prima di un nuovo run", false},
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
		"run_no_turn_evidence":               {"The provider ended the turn with no output, tool call or message", "Retry the run or inspect the remote session: the turn produced nothing usable", true},
		runtrace.FailureCodeProviderToolPermissionDenied: {"The provider ended the turn after a tool permission was denied without delivering an answer", "The turn is terminal: do not reconcile it as a success; inspect the trace and the remote session before starting a new run", false},
	},
}

func explanationFor(lang, status, failureCode string) explanationText {
	texts, ok := runExplanations[lang]
	if !ok {
		texts = runExplanations["en"]
	}
	// A denied-permission non-delivery is terminal even when an older record
	// still carries "completed": the code the trace evidences wins over the
	// status word, because telling the caller "Run completed" is the confusion
	// this explanation exists to remove.
	if failureCode == runtrace.FailureCodeProviderToolPermissionDenied {
		if value, ok := texts[failureCode]; ok {
			return value
		}
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
