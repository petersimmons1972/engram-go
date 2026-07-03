package mcp

// Regression tests for #1281: every MCP tool must advertise a real JSON input
// schema (types, required fields) instead of the empty {"properties":{}}
// schema that let clients silently stringify array/number params.
//
// These tests inspect the *mcpgo.Tool* registered on the real mcp-go server
// (srv.mcp.GetTool(name).Tool.InputSchema) rather than re-deriving expected
// values from the handler source, so they catch drift between the schema and
// what registerTools() actually declares.

import (
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"
)

// newSchemaTestServer builds a Server with all tools registered (including
// the Claude-gated ones, since ClaudeEnabled is set) so schema assertions can
// cover the full tool surface.
func newSchemaTestServer(t *testing.T) *Server {
	t.Helper()
	srv := &Server{
		cfg:             Config{ClaudeEnabled: true},
		uploads:         make(map[string]*uploadSession),
		toolAnnotations: make(map[string]mcpgo.ToolAnnotation),
	}
	srv.mcp = server.NewMCPServer("engram-test", "0.0.0",
		server.WithToolCapabilities(true),
		server.WithHooks(&server.Hooks{}),
	)
	srv.registerTools()
	return srv
}

// schemaProps is a small helper that fetches a registered tool's input schema
// properties map, failing the test if the tool was not registered.
func schemaProps(t *testing.T, srv *Server, tool string) map[string]any {
	t.Helper()
	st := srv.mcp.GetTool(tool)
	require.NotNil(t, st, "tool %q must be registered", tool)
	return st.Tool.InputSchema.Properties
}

func schemaRequired(t *testing.T, srv *Server, tool string) []string {
	t.Helper()
	st := srv.mcp.GetTool(tool)
	require.NotNil(t, st, "tool %q must be registered", tool)
	return st.Tool.InputSchema.Required
}

func propType(t *testing.T, props map[string]any, name string) string {
	t.Helper()
	raw, ok := props[name]
	require.True(t, ok, "property %q must be declared", name)
	m, ok := raw.(map[string]any)
	require.True(t, ok, "property %q schema must be a map, got %T", name, raw)
	typ, _ := m["type"].(string)
	return typ
}

func propItemsType(t *testing.T, props map[string]any, name string) string {
	t.Helper()
	raw, ok := props[name]
	require.True(t, ok, "property %q must be declared", name)
	m, ok := raw.(map[string]any)
	require.True(t, ok)
	items, ok := m["items"].(map[string]any)
	require.True(t, ok, "property %q must declare array items", name)
	typ, _ := items["type"].(string)
	return typ
}

func requireContains(t *testing.T, haystack []string, want string) {
	t.Helper()
	for _, s := range haystack {
		if s == want {
			return
		}
	}
	t.Errorf("expected %q in required list %v", want, haystack)
}

// ── #1279 direct regression guard: memory_store "tags" must be a declared array ──

func TestSchema_MemoryStore_TagsIsArrayOfStrings(t *testing.T) {
	srv := newSchemaTestServer(t)
	props := schemaProps(t, srv, "memory_store")
	require.Equal(t, "array", propType(t, props, "tags"))
	require.Equal(t, "string", propItemsType(t, props, "tags"))
	require.Equal(t, "number", propType(t, props, "importance"))
	require.Equal(t, "string", propType(t, props, "content"))
	requireContains(t, schemaRequired(t, srv, "memory_store"), "content")
}

func TestSchema_MemoryStoreBatch_MemoriesIsArrayOfObjects(t *testing.T) {
	srv := newSchemaTestServer(t)
	props := schemaProps(t, srv, "memory_store_batch")
	require.Equal(t, "array", propType(t, props, "memories"))
	requireContains(t, schemaRequired(t, srv, "memory_store_batch"), "memories")
}

// ── #1280 direct regression guard: memory_list "limit"/"offset" must be numbers ──

func TestSchema_MemoryList_LimitOffsetAreNumbers(t *testing.T) {
	srv := newSchemaTestServer(t)
	props := schemaProps(t, srv, "memory_list")
	require.Equal(t, "number", propType(t, props, "limit"))
	require.Equal(t, "number", propType(t, props, "offset"))
	require.Equal(t, "array", propType(t, props, "tags"))
	require.Equal(t, "string", propItemsType(t, props, "tags"))
}

// ── Representative coverage across the rest of the tool surface ────────────

func TestSchema_MemoryRecall_TypesAndRequired(t *testing.T) {
	srv := newSchemaTestServer(t)
	props := schemaProps(t, srv, "memory_recall")
	require.Equal(t, "string", propType(t, props, "query"))
	require.Equal(t, "number", propType(t, props, "top_k"))
	require.Equal(t, "number", propType(t, props, "limit"))
	require.Equal(t, "boolean", propType(t, props, "include_conflicts"))
	require.Equal(t, "array", propType(t, props, "projects"))
	requireContains(t, schemaRequired(t, srv, "memory_recall"), "query")
}

func TestSchema_MemoryCorrect_TagsImportancePatternConfidence(t *testing.T) {
	srv := newSchemaTestServer(t)
	props := schemaProps(t, srv, "memory_correct")
	require.Equal(t, "array", propType(t, props, "tags"))
	require.Equal(t, "number", propType(t, props, "importance"))
	require.Equal(t, "number", propType(t, props, "pattern_confidence"))
	requireContains(t, schemaRequired(t, srv, "memory_correct"), "memory_id")
}

func TestSchema_MemoryConnect_StrengthIsNumber(t *testing.T) {
	srv := newSchemaTestServer(t)
	props := schemaProps(t, srv, "memory_connect")
	require.Equal(t, "number", propType(t, props, "strength"))
	req := schemaRequired(t, srv, "memory_connect")
	requireContains(t, req, "source_id")
	requireContains(t, req, "target_id")
}

func TestSchema_MemoryFeedback_MemoryIDsIsArray(t *testing.T) {
	srv := newSchemaTestServer(t)
	props := schemaProps(t, srv, "memory_feedback")
	require.Equal(t, "array", propType(t, props, "memory_ids"))
	require.Equal(t, "string", propItemsType(t, props, "memory_ids"))
}

func TestSchema_MemoryAggregate_LimitIsNumberAndByRequired(t *testing.T) {
	srv := newSchemaTestServer(t)
	props := schemaProps(t, srv, "memory_aggregate")
	require.Equal(t, "number", propType(t, props, "limit"))
	requireContains(t, schemaRequired(t, srv, "memory_aggregate"), "by")
}

func TestSchema_MemorySleep_NumericAndBooleanParams(t *testing.T) {
	srv := newSchemaTestServer(t)
	props := schemaProps(t, srv, "memory_sleep")
	require.Equal(t, "number", propType(t, props, "min_similarity"))
	require.Equal(t, "number", propType(t, props, "limit"))
	require.Equal(t, "boolean", propType(t, props, "llm_contradiction_detection"))
	require.Equal(t, "number", propType(t, props, "llm_max_calls"))
	require.Equal(t, "boolean", propType(t, props, "auto_supersede"))
}

func TestSchema_MemoryMigrateEmbedder_BooleanGuards(t *testing.T) {
	srv := newSchemaTestServer(t)
	props := schemaProps(t, srv, "memory_migrate_embedder")
	require.Equal(t, "boolean", propType(t, props, "force"))
	require.Equal(t, "boolean", propType(t, props, "dry_run"))
	require.Equal(t, "boolean", propType(t, props, "confirm"))
	requireContains(t, schemaRequired(t, srv, "memory_migrate_embedder"), "new_model")
}

func TestSchema_GetConstraints_LimitIsNumber(t *testing.T) {
	srv := newSchemaTestServer(t)
	props := schemaProps(t, srv, "get_constraints")
	require.Equal(t, "number", propType(t, props, "limit"))
	require.Equal(t, "number", propType(t, props, "stale_after_days"))
}

func TestSchema_VerifyBeforeActing_ProposedActionRequired(t *testing.T) {
	srv := newSchemaTestServer(t)
	requireContains(t, schemaRequired(t, srv, "verify_before_acting"), "proposed_action")
}

func TestSchema_MemoryEpisodeEnd_RequiresEpisodeID(t *testing.T) {
	srv := newSchemaTestServer(t)
	requireContains(t, schemaRequired(t, srv, "memory_episode_end"), "episode_id")
}

func TestSchema_MemoryDeleteProject_ConfirmAndProjectRequired(t *testing.T) {
	srv := newSchemaTestServer(t)
	req := schemaRequired(t, srv, "memory_delete_project")
	requireContains(t, req, "project")
	requireContains(t, req, "confirm")
}

func TestSchema_MemoryExplore_ScopeIsObject(t *testing.T) {
	srv := newSchemaTestServer(t)
	props := schemaProps(t, srv, "memory_explore")
	require.Equal(t, "object", propType(t, props, "scope"))
	require.Equal(t, "number", propType(t, props, "max_iterations"))
	require.Equal(t, "number", propType(t, props, "confidence_threshold"))
}

func TestSchema_MemoryQueryDocument_FilterIsObject(t *testing.T) {
	srv := newSchemaTestServer(t)
	props := schemaProps(t, srv, "memory_query_document")
	require.Equal(t, "object", propType(t, props, "filter"))
	requireContains(t, schemaRequired(t, srv, "memory_query_document"), "question")
}

func TestSchema_MemoryIngestDocumentStream_ActionEnumAndPart(t *testing.T) {
	srv := newSchemaTestServer(t)
	props := schemaProps(t, srv, "memory_ingest_document_stream")
	require.Equal(t, "number", propType(t, props, "part"))
	require.Equal(t, "string", propType(t, props, "action"))
}

// ── Blanket coverage: no tool ships with an empty schema unless documented ──

// noArgTools lists tools whose handlers genuinely read no MCP arguments (or
// only cfg). Any registered tool NOT in this list must declare at least one
// property — an empty schema for anything else means a new tool was added
// without its parameters being declared, exactly the #1281 failure mode.
func noArgTools() map[string]bool {
	return map[string]bool{
		"memory_status_ping": true,
		"memory_models":      true,
	}
}

func TestSchema_NoToolShipsWithEmptySchemaUnlessDocumented(t *testing.T) {
	srv := newSchemaTestServer(t)
	allow := noArgTools()

	// Union of every name we know should be registered: the main tool slice
	// plus the Claude-gated ones. readOnlyToolNames/hiddenToolNames double as
	// a convenient enumeration of "tools that exist" for this sweep, unioned
	// with a few mutating tools not in either set.
	names := map[string]bool{}
	for n := range readOnlyToolNames() {
		names[n] = true
	}
	for n := range hiddenToolNames() {
		names[n] = true
	}
	extra := []string{
		"memory_store", "memory_store_document", "memory_store_batch",
		"memory_recall", "memory_fetch", "memory_list", "memory_history",
		"memory_connect", "memory_correct", "memory_forget", "memory_feedback",
		"memory_adopt", "memory_quick_store", "memory_query",
		"get_constraints", "check_constraints", "verify_before_acting",
		"memory_reason", "memory_explore", "memory_query_document", "memory_ask",
		"memory_ingest_document_stream", "memory_episode_start", "memory_episode_end",
		"memory_audit_add_query", "memory_audit_deactivate_query", "memory_audit_run",
		"memory_projects", "memory_status",
	}
	for _, n := range extra {
		names[n] = true
	}

	for name := range names {
		if allow[name] {
			continue
		}
		st := srv.mcp.GetTool(name)
		require.NotNilf(t, st, "tool %q must be registered", name)
		require.NotEmptyf(t, st.Tool.InputSchema.Properties,
			"tool %q was registered with an empty input schema — this is the exact #1281 bug shape", name)
	}
}
