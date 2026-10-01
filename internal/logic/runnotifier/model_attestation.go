package runnotifier

import (
	"log/slog"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

// OnModelSelection records the provider evidence about the model of this run:
// what Matrix selected, what the provider itself stated, and why an unconfirmed
// selection stayed unconfirmed. The run record and the model.selection event are
// written from the same values so a reader of either sees one story.
func (n *Notifier) OnModelSelection(selection middleware.ModelSelection) {
	if n == nil || n.store == nil {
		return
	}
	run, found, err := n.store.LoadRun(n.runID)
	if err != nil || !found {
		return
	}
	run.ConfiguredModel = selection.ConfiguredModel
	run.EffectiveModel = selection.EffectiveModel
	run.ModelVerification = selection.Verification
	run.ModelFallbackUsed = selection.FallbackUsed
	run.ModelFallbackReason = selection.FallbackReason
	if err := n.store.SaveRun(run); err != nil {
		slog.Warn("failed to record model selection", "error", err, "run_id", n.runID)
		return
	}
	_, _ = n.store.AppendEvent(runtrace.Event{RunID: n.runID, Kind: "model.selection",
		Metadata: modelSelectionMetadata(run, selection)})
}

// modelSelectionMetadata describes one model selection at the three levels a
// consumer must not conflate:
//
//   - requested_model is what the run asked for. It is an input, never a proof.
//   - selected_model is what Matrix applied to the provider session. An applied
//     selection is not a confirmation either: a provider can accept the call
//     and still run something else.
//   - confirmed_model is what the provider itself stated about the session.
//     Only this level can elevate a request to provider_confirmed.
//
// configured_model and effective_model remain in the payload as aliases because
// the exported run record uses those names, and a consumer that already reads
// them must keep working. Neither alias is ever filled from requested_model.
//
// verification_reason and evidence_source travel with the unverified verdict:
// they say why the provider did not confirm the selection and which protocol
// response was inspected, so a provider that never attests is distinguishable
// from a session whose verification was not repeated.
func modelSelectionMetadata(run runtrace.Run, selection middleware.ModelSelection) map[string]interface{} {
	return map[string]interface{}{
		"requested_model":     run.RequestedModel,
		"selected_model":      selection.ConfiguredModel,
		"confirmed_model":     selection.EffectiveModel,
		"configured_model":    selection.ConfiguredModel,
		"effective_model":     selection.EffectiveModel,
		"verification":        selection.Verification,
		"verification_reason": selection.VerificationReason,
		"evidence_source":     selection.EvidenceSource,
		"fallback_used":       selection.FallbackUsed,
		"fallback_reason":     selection.FallbackReason,
	}
}
