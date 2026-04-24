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
		if summary := pathToolSummary(ctx); summary != "" {
			return summary
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

func pathToolSummary(ctx toolSummaryContext) string {
	for _, rule := range pathSummaryRules {
		if rule.match(ctx) {
			return rule.render(ctx)
		}
	}
	return ""
}

type pathSummaryRule struct {
	match  func(toolSummaryContext) bool
	render func(toolSummaryContext) string
}

var pathSummaryRules = []pathSummaryRule{
	{
		match:  func(ctx toolSummaryContext) bool { return ctx.Kind == "execute" && ctx.Operation != "" },
		render: func(ctx toolSummaryContext) string { return fmt.Sprintf("Run %s on %s", ctx.Operation, ctx.Path) },
	},
	{
		match:  func(ctx toolSummaryContext) bool { return ctx.Kind == "execute" },
		render: func(ctx toolSummaryContext) string { return fmt.Sprintf("Run command on %s", ctx.Path) },
	},
	{
		match:  func(ctx toolSummaryContext) bool { return ctx.Status == "pending" && ctx.Name == "write_file" },
		render: func(ctx toolSummaryContext) string { return "Create " + ctx.Path },
	},
	{
		match: func(ctx toolSummaryContext) bool {
			return ctx.Status == runtrace.StatusCompleted && ctx.Name == "write_file"
		},
		render: func(ctx toolSummaryContext) string { return "Created " + ctx.Path },
	},
	{
		match:  func(ctx toolSummaryContext) bool { return ctx.Status == "pending" && ctx.Kind == "edit" },
		render: func(ctx toolSummaryContext) string { return "Edit " + ctx.Path },
	},
	{
		match:  func(ctx toolSummaryContext) bool { return ctx.Status == runtrace.StatusCompleted && ctx.Kind == "edit" },
		render: func(ctx toolSummaryContext) string { return "Updated " + ctx.Path },
	},
	{
		match:  func(ctx toolSummaryContext) bool { return ctx.Status == "pending" && ctx.Kind == "delete" },
		render: func(ctx toolSummaryContext) string { return "Delete " + ctx.Path },
	},
	{
		match: func(ctx toolSummaryContext) bool {
			return ctx.Status == runtrace.StatusCompleted && ctx.Kind == "delete"
		},
		render: func(ctx toolSummaryContext) string { return "Deleted " + ctx.Path },
	},
}
