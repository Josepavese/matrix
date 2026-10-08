package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Josepavese/matrix/internal/logic/admission"
	"github.com/Josepavese/matrix/internal/logic/session"
	"github.com/Josepavese/matrix/internal/logic/workspace"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/oscapacity"
)

func runtimeCapacityLimits(get func(string) (string, error)) func() (admission.Limits, error) {
	return func() (admission.Limits, error) {
		concurrentRaw, err := capacityConfigValue(get, "capacity.max_concurrent")
		if err != nil {
			return admission.Limits{}, err
		}
		concurrent, err := strconv.Atoi(concurrentRaw)
		if err != nil {
			return admission.Limits{}, err
		}
		minimumRaw, err := capacityConfigValue(get, "capacity.min_disk_free_bytes")
		if err != nil {
			return admission.Limits{}, err
		}
		minimum, err := strconv.ParseUint(minimumRaw, 10, 64)
		if err != nil {
			return admission.Limits{}, err
		}
		return admission.ParseLimits(concurrent, minimum)
	}
}

func capacityConfigValue(get func(string) (string, error), name string) (string, error) {
	value, err := get(name)
	if err != nil {
		return "", err
	}
	if value == "" {
		value = "0"
	}
	return value, nil
}

func configureRuntimeCapacity(manager *session.Manager, get func(string) (string, error)) {
	manager.WithAdmission(admission.New(oscapacity.New(), runtimeCapacityLimits(get)))
}

func registerCapacityRuntime(mux *http.ServeMux, manager *session.Manager, storage middleware.Storage, key string) {
	mux.HandleFunc("/_matrix/capacity", func(w http.ResponseWriter, r *http.Request) {
		if !authorizeMatrixRuntimeRequest(w, r, key) {
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		payload := map[string]any{"admission": manager.AdmissionState()}
		if id := r.URL.Query().Get("workspace_id"); id != "" {
			meta, found, err := workspace.LoadMeta(storage, id)
			if err != nil {
				http.Error(w, "workspace lookup failed", http.StatusInternalServerError)
				return
			}
			if !found {
				http.Error(w, "workspace not found", http.StatusNotFound)
				return
			}
			payload["capacity"] = workspace.ObserveCapacity(meta.RootPath, oscapacity.New())
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			slog.Warn("capacity report encoding failed", "event", "capacity_report_encode_failed", "error", err)
		}
	})
}
