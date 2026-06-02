// findroot_internal_test.go — tests for unexported findRoot function.
// Uses package chatgpt (internal) so the unexported function is accessible.
// ParseFile error path is already covered in chatgpt_test.go (external package).
package chatgpt

import (
	"testing"
)

// ---------------------------------------------------------------------------
// findRoot
// ---------------------------------------------------------------------------

func TestFindRoot_SingleNodeNoParent(t *testing.T) {
	mapping := map[string]mapNode{
		"root": {ID: "root", Parent: nil},
	}
	got := findRoot(mapping)
	if got != "root" {
		t.Errorf("findRoot single node = %q, want 'root'", got)
	}
}

func TestFindRoot_MultipleNodes_ReturnsParentlessOne(t *testing.T) {
	parent := "root"
	mapping := map[string]mapNode{
		"root":   {ID: "root", Parent: nil},
		"child1": {ID: "child1", Parent: &parent},
		"child2": {ID: "child2", Parent: &parent},
	}
	got := findRoot(mapping)
	if got != "root" {
		t.Errorf("findRoot multi-node = %q, want 'root'", got)
	}
}

func TestFindRoot_EmptyMapping(t *testing.T) {
	got := findRoot(map[string]mapNode{})
	if got != "" {
		t.Errorf("findRoot empty mapping = %q, want '' (empty string)", got)
	}
}

func TestFindRoot_AllNodesHaveParents(t *testing.T) {
	// Malformed tree: every node has a parent → no root found → empty string.
	p1 := "b"
	p2 := "a"
	mapping := map[string]mapNode{
		"a": {ID: "a", Parent: &p1},
		"b": {ID: "b", Parent: &p2},
	}
	got := findRoot(mapping)
	if got != "" {
		t.Errorf("findRoot no parentless node = %q, want '' (empty string)", got)
	}
}

func TestFindRoot_NilMapping(t *testing.T) {
	// nil map should return empty string without panicking.
	got := findRoot(nil)
	if got != "" {
		t.Errorf("findRoot nil mapping = %q, want ''", got)
	}
}

func TestFindRoot_DeepTree(t *testing.T) {
	// root → node1 → node2 → node3; only root has nil parent.
	n1 := "root"
	n2 := "node1"
	n3 := "node2"
	mapping := map[string]mapNode{
		"root":  {ID: "root", Parent: nil},
		"node1": {ID: "node1", Parent: &n1},
		"node2": {ID: "node2", Parent: &n2},
		"node3": {ID: "node3", Parent: &n3},
	}
	got := findRoot(mapping)
	if got != "root" {
		t.Errorf("findRoot deep tree = %q, want 'root'", got)
	}
}
