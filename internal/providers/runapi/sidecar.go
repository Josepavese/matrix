package runapi

import (
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/logic/sidecartrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

func (s *Server) appendSidecarEvents(run runtrace.Run, capsules []middleware.SidecarCapsule) {
	for _, event := range sidecartrace.Events(run, capsules) {
		_, _ = s.runStore.AppendEvent(event)
	}
}
