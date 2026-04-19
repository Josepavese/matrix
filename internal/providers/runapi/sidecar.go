package runapi

import (
	"github.com/jose/matrix-v2/internal/logic/runtrace"
	"github.com/jose/matrix-v2/internal/logic/sidecartrace"
	"github.com/jose/matrix-v2/internal/middleware"
)

func (s *Server) appendSidecarEvents(run runtrace.Run, capsules []middleware.SidecarCapsule) {
	for _, event := range sidecartrace.Events(run, capsules) {
		_, _ = s.runStore.AppendEvent(event)
	}
}
