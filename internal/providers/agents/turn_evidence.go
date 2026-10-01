package agents

import (
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

// turnStopReasonMetaKey is the neutral metadata key under which a turn reports
// what its peer said ended it. It is what carries the reason from the protocol
// adapter to whatever materializes the run record, without either side needing
// to know the other's type.
const turnStopReasonMetaKey = "stop_reason"

// firstNonEmpty returns the first value that carries something other than
// whitespace, and nothing when none does.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// metadataWithStopReason attaches the reason a peer reported to a turn's
// metadata. An absent reason is written as an absent key or empty value: the
// distinction between "the peer said end_turn" and "the peer said nothing" is
// the whole point, so nothing here substitutes a reason of Matrix's own.
func metadataWithStopReason(metadata middleware.ConversationMetadata, stopReason string) middleware.ConversationMetadata {
	if metadata.Meta == nil {
		metadata.Meta = map[string]interface{}{}
	}
	metadata.Meta[turnStopReasonMetaKey] = strings.TrimSpace(stopReason)
	return metadata
}
