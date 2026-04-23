package frontendevents

import (
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
)

type toolSummaryContext struct {
	Status    string
	Name      string
	Kind      string
	Path      string
	Operation string
	Content   string
}

func toolSummary(ctx toolSummaryContext) string {
	if ctx.Path != "" {
		switch {
		case ctx.Kind == "execute" && ctx.Operation != "":
			return fmt.Sprintf("Run %s on %s", ctx.Operation, ctx.Path)
		case ctx.Kind == "execute":
			return fmt.Sprintf("Run command on %s", ctx.Path)
		case ctx.Status == "pending" && ctx.Name == "write_file":
			return "Create " + ctx.Path
		case ctx.Status == runtrace.StatusCompleted && ctx.Name == "write_file":
			return "Created " + ctx.Path
		case ctx.Status == "pending" && ctx.Kind == "edit":
			return "Edit " + ctx.Path
		case ctx.Status == runtrace.StatusCompleted && ctx.Kind == "edit":
			return "Updated " + ctx.Path
		case ctx.Status == "pending" && ctx.Kind == "delete":
			return "Delete " + ctx.Path
		case ctx.Status == runtrace.StatusCompleted && ctx.Kind == "delete":
			return "Deleted " + ctx.Path
		}
	}
	action := map[string]string{"pending": "Start", "running": "Run", runtrace.StatusCompleted: "Completed", runtrace.StatusFailed: "Failed"}[ctx.Status]
	if action == "" {
		action = "Run"
	}
	if ctx.Content != "" {
		return truncateSummary(ctx.Content)
	}
	return fmt.Sprintf("%s %s", action, ctx.Name)
}
