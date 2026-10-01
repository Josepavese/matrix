package runaction

import "github.com/Josepavese/matrix/internal/middleware"

// OnModelSelection keeps the wrapped notifier's optional model-attestation
// capability reachable through the delivery-proof decorator. The decorator adds
// delivery metadata to thought updates; it must not decide which evidence the
// run trace can receive, because the ACP adapter publishes model attestation
// only through this interface.
func (n *attachProofNotifier) OnModelSelection(selection middleware.ModelSelection) {
	if n.base == nil {
		return
	}
	if forwarder, ok := n.base.(middleware.ModelSelectionNotifier); ok {
		forwarder.OnModelSelection(selection)
	}
}
