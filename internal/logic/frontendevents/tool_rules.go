package frontendevents

var officialToolKindAliases = map[string]string{
	"read":        "read",
	"edit":        "edit",
	"delete":      "delete",
	"move":        "move",
	"search":      "search",
	"execute":     "execute",
	"think":       "think",
	"fetch":       "fetch",
	"switch_mode": "switch_mode",
	"other":       "other",
	"write":       "edit",
	"create":      "edit",
	"patch":       "edit",
	"exec":        "execute",
	"shell":       "execute",
	"terminal":    "execute",
}

var toolSubjectByKind = map[string]string{
	"read":        "workspace",
	"edit":        "workspace",
	"delete":      "workspace",
	"move":        "workspace",
	"search":      "workspace",
	"execute":     "process",
	"fetch":       "network",
	"switch_mode": "agent_session",
	"think":       "agent_reasoning",
}
