package frontendevents

import "strings"

// Tool classification answers one question: which structural action did this
// event describe? The evidence has a strict order. A protocol kind is the
// provider's own statement and wins outright. A tool identity that names the
// action ("read_file", "write_file") is the next strongest: it is declared
// identity, not text inference. Free-form words are the last resort, and they
// are never read from a tool result body, because a result can quote any page
// or proof text and a quotation must not become a write claim in the trace.

// declaredToolName is the identity the provider declared for the tool, in the
// order the protocol carries it. Content-derived fallback names are not part of
// this set: only a declared name can decide the kind.
func declaredToolName(metadata map[string]interface{}) string {
	return FirstNonEmpty(
		StringValue(metadata, "tool_name"),
		StringValue(metadata, "name"),
		StringValue(metadata, "title"),
	)
}

// classifyToolKind returns the kind, its evidence class, its confidence and
// whether the kind came from a declared tool identity. The identity flag lets
// the summary renderer keep past-tense write text for an identified action and
// refuse it for a word guess.
func classifyToolKind(metadata map[string]interface{}, declaredName, name, content string) (string, string, string, bool) {
	for _, key := range []string{"tool_kind", "acp_tool_kind", "kind"} {
		if kind := NormalizeOfficialToolKind(StringValue(metadata, key)); kind != "" {
			return kind, "protocol_metadata", "high", false
		}
	}
	if kind := nestedACPToolKind(metadata); kind != "" {
		return kind, "protocol_metadata", "high", false
	}
	if kind := kindFromToolIdentity(declaredName); kind != "" {
		return kind, "heuristic_fallback", "low", true
	}
	if isToolResultUpdate(metadata) {
		// A result body is output, not a call. Without a protocol kind or a
		// declared identity the action stays explicitly unknown; the caller may
		// still correlate it with the call the result belongs to.
		return "other", "unknown", "low", false
	}
	kind, source, confidence := fallbackToolKind(name, content)
	return kind, source, confidence, false
}

// toolKindByIdentity maps the canonical tool vocabulary to the action it names.
// These are the names NormalizeToolName produces from a declared tool title, so
// the kind and the displayed tool name cannot disagree.
var toolKindByIdentity = map[string]string{
	"read_file":  "read",
	"list_files": "read",
	"search":     "search",
	"write_file": "edit",
	"edit_file":  "edit",
	"shell":      "execute",
}

func kindFromToolIdentity(declaredName string) string {
	declaredName = strings.TrimSpace(declaredName)
	if declaredName == "" {
		return ""
	}
	return toolKindByIdentity[NormalizeToolName(declaredName)]
}

// isToolResultUpdate reports whether the event describes the result of a tool
// call rather than the call itself. Client-handled tool signals carry their own
// protocol kind, so the result-body restriction only affects updates whose
// identity the provider did not state.
func isToolResultUpdate(metadata map[string]interface{}) bool {
	return SourceUpdateType(metadata, "") == "tool_call_update"
}

// InheritToolIdentity lets a result update without an identity of its own use
// the identity of the call it correlates with. Correlation is protocol
// evidence (same tool_call_id window), not text inference, so a result body can
// never invent a delete or an edit by mentioning the words.
func InheritToolIdentity(result, call ToolEvent) ToolEvent {
	if result.ClassificationSource != "unknown" || !identityWorthInheriting(call) {
		return result
	}
	result.Name = call.Name
	result.Kind = call.Kind
	result.SemanticKind = call.SemanticKind
	result.Effect = call.Effect
	result.SubjectKind = call.SubjectKind
	result.ClassificationSource = call.ClassificationSource
	result.ClassificationConfidence = call.ClassificationConfidence
	result.KindFromIdentity = call.KindFromIdentity
	return result
}

func identityWorthInheriting(call ToolEvent) bool {
	if call.Kind == "" || call.Kind == "other" || call.ClassificationSource == "" {
		return false
	}
	return call.ClassificationSource != "unknown"
}

func NormalizeToolKind(name, content string) string {
	kind, _, _ := fallbackToolKind(name, content)
	return kind
}

func NormalizeToolKindFromMetadata(metadata map[string]interface{}, name, content string) (string, string, string) {
	kind, source, confidence, _ := classifyToolKind(metadata, name, name, content)
	return kind, source, confidence
}

func NormalizeOfficialToolKind(raw string) string {
	return officialToolKindAliases[strings.ToLower(strings.TrimSpace(raw))]
}

func fallbackToolKind(name, content string) (string, string, string) {
	value := strings.ToLower(name + " " + content)
	for _, rule := range toolKindRules {
		if containsAny(value, rule.matches) {
			return rule.kind, "heuristic_fallback", "low"
		}
	}
	return "other", "unknown", "low"
}

func nestedACPToolKind(metadata map[string]interface{}) string {
	rawACP, ok := metadata["acp"].(map[string]interface{})
	if !ok {
		return ""
	}
	for _, key := range []string{"tool_kind", "kind"} {
		if value, ok := rawACP[key].(string); ok {
			if kind := NormalizeOfficialToolKind(value); kind != "" {
				return kind
			}
		}
	}
	return ""
}

type toolKindRule struct {
	kind    string
	matches []string
}

var toolKindRules = []toolKindRule{
	{kind: "edit", matches: []string{"write", "create", "edit", "patch"}},
	{kind: "delete", matches: []string{"delete", "remove"}},
	{kind: "move", matches: []string{"move", "rename"}},
	{kind: "search", matches: []string{"search", "grep"}},
	{kind: "read", matches: []string{"read", "list"}},
	{kind: "execute", matches: []string{"shell", "terminal", "exec", "bash"}},
	{kind: "fetch", matches: []string{"fetch", "http"}},
	{kind: "think", matches: []string{"think", "reason"}},
	{kind: "switch_mode", matches: []string{"switch_mode", "set_mode"}},
}
