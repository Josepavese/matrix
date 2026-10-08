package main

import (
	"net/http"

	"github.com/Josepavese/matrix/internal/logic/agentmgr"
	"github.com/Josepavese/matrix/internal/logic/semanticfs"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/oscapacity"
)

func registerSemanticRuntime(mux *http.ServeMux, registry *agentmgr.Registry, storage middleware.Storage, key string) {
	view := semanticfs.New(semanticfs.Source{Storage: storage, Agents: semanticAgentViews(registry), Capacity: oscapacity.New()})
	handler := http.StripPrefix("/_matrix/fs/", http.FileServer(http.FS(view)))
	mux.HandleFunc("/_matrix/fs/", func(w http.ResponseWriter, r *http.Request) {
		if !authorizeMatrixRuntimeRequest(w, r, key) {
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		handler.ServeHTTP(w, r)
	})
}
