package mcp

// Small helper builders for the MCP tool input schemas declared in
// registerTools() (server.go). Extracted so the (long) per-tool schema list
// stays readable — every helper here is a thin wrapper over the mcpgo
// WithString/WithNumber/WithArray/WithBoolean/WithObject builders.
//
// #1281: prior to this file, every tool was registered with an empty input
// schema ({"properties":{}}), so MCP clients that type-coerce arguments based
// on the advertised schema had no way to know a parameter was an array or a
// number and sent it JSON-encoded as a string. Handlers then silently
// dropped it (see toStringSlice / getInt in tools.go, pre-fix). Every
// property declared below was derived by reading the corresponding handler's
// arg-extraction code, not guessed from docs.

import (
	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// projectProp declares the near-universal optional "project" string param.
func projectProp() mcpgo.ToolOption {
	return mcpgo.WithString("project",
		mcpgo.Description(`Project namespace to scope this call to. Defaults to "default" when omitted.`))
}

// requiredProjectProp is projectProp for the handful of tools that reject an
// empty/absent project (audit + weight tools call getProject(args, "") and
// then explicitly error when the result is empty).
func requiredProjectProp() mcpgo.ToolOption {
	return mcpgo.WithString("project",
		mcpgo.Required(),
		mcpgo.Description("Project namespace."))
}

// tagsProp declares an optional array-of-strings "tags" param.
func tagsProp(desc string) mcpgo.ToolOption {
	return mcpgo.WithArray("tags", mcpgo.WithStringItems(), mcpgo.Description(desc))
}

// stringArrayProp declares an optional array-of-strings param under an
// arbitrary name.
func stringArrayProp(name, desc string) mcpgo.ToolOption {
	return mcpgo.WithArray(name, mcpgo.WithStringItems(), mcpgo.Description(desc))
}

// numberProp declares an optional number param.
func numberProp(name, desc string) mcpgo.ToolOption {
	return mcpgo.WithNumber(name, mcpgo.Description(desc))
}

// requiredNumberProp declares a required number param.
func requiredNumberProp(name, desc string) mcpgo.ToolOption {
	return mcpgo.WithNumber(name, mcpgo.Required(), mcpgo.Description(desc))
}

// boolProp declares an optional boolean param.
func boolProp(name, desc string) mcpgo.ToolOption {
	return mcpgo.WithBoolean(name, mcpgo.Description(desc))
}

// strProp declares an optional string param.
func strProp(name, desc string) mcpgo.ToolOption {
	return mcpgo.WithString(name, mcpgo.Description(desc))
}

// requiredStrProp declares a required string param.
func requiredStrProp(name, desc string) mcpgo.ToolOption {
	return mcpgo.WithString(name, mcpgo.Required(), mcpgo.Description(desc))
}

// enumStrProp declares an optional string param constrained to an enum.
func enumStrProp(name, desc string, values ...string) mcpgo.ToolOption {
	return mcpgo.WithString(name, mcpgo.Description(desc), mcpgo.Enum(values...))
}

// requiredEnumStrProp declares a required string param constrained to an enum.
func requiredEnumStrProp(name, desc string, values ...string) mcpgo.ToolOption {
	return mcpgo.WithString(name, mcpgo.Required(), mcpgo.Description(desc), mcpgo.Enum(values...))
}

// memoryIDProp declares the near-universal required "memory_id" string param.
func memoryIDProp(desc string) mcpgo.ToolOption {
	if desc == "" {
		desc = "ID of the target memory."
	}
	return mcpgo.WithString("memory_id", mcpgo.Required(), mcpgo.Description(desc))
}

// exploreScopeProp declares memory_explore's optional "scope" object param.
// Extracted to a helper (rather than inlined at the call site) so the
// braces of its nested map[string]any literal don't sit inside the
// `if s.cfg.ClaudeEnabled { ... }` block in registerTools() — the doc-count
// regression guard (tool_count_doc_test.go) locates that block with a
// non-greedy brace match and would otherwise stop at the first inner `}`.
func exploreScopeProp() mcpgo.ToolOption {
	return mcpgo.WithObject("scope",
		mcpgo.Description("Restrict recall to memories matching all of these constraints."),
		mcpgo.Properties(map[string]any{
			"tags":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Memory must contain all of these tags."},
			"episode_id": map[string]any{"type": "string"},
			"since":      map[string]any{"type": "string", "description": "RFC3339 lower bound on created_at."},
			"until":      map[string]any{"type": "string", "description": "RFC3339 upper bound on created_at."},
		}),
	)
}

// queryDocumentFilterProp declares memory_query_document's optional "filter"
// object param. See exploreScopeProp for why this is a helper function.
func queryDocumentFilterProp() mcpgo.ToolOption {
	return mcpgo.WithObject("filter",
		mcpgo.Description("Optional pre-filter narrowing which spans are considered."),
		mcpgo.Properties(map[string]any{
			"regex":      map[string]any{"type": "string", "description": "Regex applied to document spans."},
			"substrings": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Spans must contain at least one of these substrings."},
		}),
	)
}
