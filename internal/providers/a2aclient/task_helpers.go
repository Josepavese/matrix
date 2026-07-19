package a2aclient

import (
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

func isA2ASessionNotFound(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "task not found")
}

func a2aTaskTitle(task *a2a.Task) string {
	if task == nil {
		return ""
	}
	if raw, ok := task.Metadata["title"].(string); ok {
		return strings.TrimSpace(raw)
	}
	if task.Status.Message != nil {
		return strings.TrimSpace(a2aPartsText(task.Status.Message.Parts))
	}
	return ""
}

func a2aTaskUpdatedAt(task *a2a.Task) string {
	if task == nil || task.Status.Timestamp == nil {
		return ""
	}
	return task.Status.Timestamp.UTC().Format(time.RFC3339)
}

func a2aTaskGoneError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "task not found") || strings.Contains(msg, "failed to load a task")
}
