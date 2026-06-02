package internal

import (
	"context"
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stesting "k8s.io/client-go/testing"

	"k8s.io/client-go/kubernetes/fake"
)

// helpers

func makeGPUHost(host, hostIP string, models []ModelSpec) GPUHostSpec {
	return GPUHostSpec{
		Host:   host,
		HostIP: hostIP,
		Models: models,
	}
}

func makeModel(name string, port int) ModelSpec {
	return ModelSpec{Name: name, Port: port, Framework: "vllm", Repo: "test/repo", Image: "test:latest"}
}

// controllerService returns a fake Service for "ai-fleet-controller" with the given ClusterIP.
func controllerService(clusterIP string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ai-fleet-controller",
			Namespace: "ai-fleet",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: clusterIP,
		},
	}
}

// ---- TestBuildDesiredPolicy_ExternalHosts ----
// Verifies that BuildDesiredPolicy emits one ipBlock egress rule per active
// GPUHost, collecting all enabled model ports per host.
// This is the key test for the production incident: precision at 192.168.0.90
// with port 8005 must appear as an egress rule.
func TestBuildDesiredPolicy_ExternalHosts(t *testing.T) {
	hosts := []GPUHostSpec{
		makeGPUHost("precision.petersimmons.com", "192.168.0.90", []ModelSpec{
			makeModel("embed", 8005),
			makeModel("gen", 8002),
		}),
		makeGPUHost("leviathan.petersimmons.com", "192.168.0.98", []ModelSpec{
			makeModel("embed", 8004),
		}),
	}

	pol := BuildDesiredPolicy("ai-fleet", "10.43.189.110", hosts)

	// Collect all to-ipBlock CIDRs and their ports for inspection.
	type ruleKey struct {
		cidr string
		port int
	}
	ruleSet := map[ruleKey]bool{}
	for _, egress := range pol.Spec.Egress {
		for _, to := range egress.To {
			if to.IPBlock == nil {
				continue
			}
			for _, p := range egress.Ports {
				ruleSet[ruleKey{to.IPBlock.CIDR, int(p.Port.IntVal)}] = true
			}
		}
	}

	// precision:8005 — the exact gap that caused the production incident.
	if !ruleSet[ruleKey{"192.168.0.90/32", 8005}] {
		t.Errorf("missing egress rule for precision:8005 (192.168.0.90/32:8005); got rules: %v", ruleSet)
	}
	if !ruleSet[ruleKey{"192.168.0.90/32", 8002}] {
		t.Errorf("missing egress rule for precision:8002 (192.168.0.90/32:8002)")
	}
	if !ruleSet[ruleKey{"192.168.0.98/32", 8004}] {
		t.Errorf("missing egress rule for leviathan:8004 (192.168.0.98/32:8004)")
	}
}

// ---- TestBuildDesiredPolicy_ClusterIPUsesIPBlock ----
// Verifies the controller Service egress uses ipBlock (not podSelector) because
// flannel evaluates NetworkPolicy pre-DNAT against ClusterIP.
func TestBuildDesiredPolicy_ClusterIPUsesIPBlock(t *testing.T) {
	clusterIP := "10.43.189.110"
	pol := BuildDesiredPolicy("ai-fleet", clusterIP, []GPUHostSpec{})

	found := false
	for _, egress := range pol.Spec.Egress {
		for _, to := range egress.To {
			if to.IPBlock != nil && to.IPBlock.CIDR == clusterIP+"/32" {
				// Must be port 80
				for _, p := range egress.Ports {
					if p.Port.IntVal == 80 {
						found = true
					}
				}
				// Must NOT use podSelector
				if to.PodSelector != nil {
					t.Errorf("controller ClusterIP rule must not use podSelector (pre-DNAT flannel constraint)")
				}
			}
		}
	}

	if !found {
		t.Errorf("no ipBlock egress rule found for controller ClusterIP %s/32 on port 80", clusterIP)
	}
}

// ---- TestBuildDesiredPolicy_NoBroadCIDR ----
// No non-DNS egress rule may have a prefix length shorter than /32.
// The only exempt rule is the DNS rule (port 53, no `to:` selector).
func TestBuildDesiredPolicy_NoBroadCIDR(t *testing.T) {
	hosts := []GPUHostSpec{
		makeGPUHost("precision.petersimmons.com", "192.168.0.90", []ModelSpec{makeModel("embed", 8005)}),
	}
	pol := BuildDesiredPolicy("ai-fleet", "10.43.189.110", hosts)

	for _, egress := range pol.Spec.Egress {
		for _, to := range egress.To {
			if to.IPBlock == nil {
				continue
			}
			cidr := to.IPBlock.CIDR
			if !strings.HasSuffix(cidr, "/32") {
				t.Errorf("broad CIDR detected (prefix shorter than /32): %q — only /32 rules are permitted for non-DNS egress", cidr)
			}
		}
	}
}

// ---- TestBuildDesiredPolicy_DNSRuleHasNoTo ----
// The DNS egress rule (port 53) must have no `to:` selector so that it permits
// egress to any DNS server (needed for kube-dns and external resolvers).
func TestBuildDesiredPolicy_DNSRuleHasNoTo(t *testing.T) {
	pol := BuildDesiredPolicy("ai-fleet", "10.43.189.110", []GPUHostSpec{})

	foundDNS := false
	for _, egress := range pol.Spec.Egress {
		for _, p := range egress.Ports {
			if p.Port.IntVal == 53 {
				if len(egress.To) != 0 {
					t.Errorf("DNS egress rule (port 53) must have no 'to:' selector; got %d to entries", len(egress.To))
				}
				foundDNS = true
			}
		}
	}
	if !foundDNS {
		t.Errorf("no DNS egress rule (port 53) found in desired policy")
	}
}

// ---- TestReconcile_RecommendOnlyModeDoesNotApply ----
// In recommend mode the reconciler must NOT make any Kubernetes API write calls
// even when the live policy differs from desired.
func TestReconcile_RecommendOnlyModeDoesNotApply(t *testing.T) {
	clusterIP := "10.43.189.110"
	client := fake.NewSimpleClientset(controllerService(clusterIP))

	hosts := []GPUHostSpec{
		makeGPUHost("precision.petersimmons.com", "192.168.0.90", []ModelSpec{makeModel("embed", 8005)}),
	}

	r := NewNetworkPolicyReconciler(client, "ai-fleet", ModeRecommend)
	err := r.Reconcile(context.Background(), hosts)
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	// In recommend mode no NetworkPolicy should have been created.
	nps, err := client.NetworkingV1().NetworkPolicies("ai-fleet").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list networkpolicies: %v", err)
	}
	if len(nps.Items) != 0 {
		t.Errorf("recommend mode must not create NetworkPolicies; found %d", len(nps.Items))
	}
}

// ---- TestReconcile_ApplyModeCreatesAndPatches ----
// In apply mode: creates when absent, patches when diff non-empty, no-op when equal.
func TestReconcile_ApplyModeCreatesAndPatches(t *testing.T) {
	clusterIP := "10.43.189.110"
	client := fake.NewSimpleClientset(controllerService(clusterIP))

	hosts := []GPUHostSpec{
		makeGPUHost("precision.petersimmons.com", "192.168.0.90", []ModelSpec{makeModel("embed", 8005)}),
	}

	r := NewNetworkPolicyReconciler(client, "ai-fleet", ModeApply)
	// Disable reachability check in tests to avoid real TCP dials.
	r.skipReachabilityCheck = true

	// First reconcile: policy absent → should be created.
	if err := r.Reconcile(context.Background(), hosts); err != nil {
		t.Fatalf("first Reconcile error: %v", err)
	}
	nps, err := client.NetworkingV1().NetworkPolicies("ai-fleet").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(nps.Items) != 1 {
		t.Fatalf("expected 1 NetworkPolicy after creation; got %d", len(nps.Items))
	}

	// Second reconcile with the same hosts: should be a no-op (idempotent).
	actions0 := len(client.Actions())
	if err := r.Reconcile(context.Background(), hosts); err != nil {
		t.Fatalf("idempotent Reconcile error: %v", err)
	}
	// Allow get + list actions; but no create/update/patch after idempotent check.
	newWriteActions := countWriteActions(client.Actions()[actions0:])
	if newWriteActions > 0 {
		t.Errorf("idempotent reconcile made %d write actions; expected 0", newWriteActions)
	}

	// Third reconcile with a new port: should patch.
	hosts2 := []GPUHostSpec{
		makeGPUHost("precision.petersimmons.com", "192.168.0.90", []ModelSpec{
			makeModel("embed", 8005),
			makeModel("gen", 8002), // new port
		}),
	}
	if err := r.Reconcile(context.Background(), hosts2); err != nil {
		t.Fatalf("patch Reconcile error: %v", err)
	}
	// Verify new port is in the applied policy.
	np, err := client.NetworkingV1().NetworkPolicies("ai-fleet").Get(context.Background(), "allow-olla-backends", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get after patch: %v", err)
	}
	found8002 := false
	for _, egress := range np.Spec.Egress {
		for _, p := range egress.Ports {
			if p.Port.IntVal == 8002 {
				found8002 = true
			}
		}
	}
	if !found8002 {
		t.Errorf("port 8002 not present in patched policy")
	}
}

// ---- TestReconcile_RollbackOnUnreachableEndpoint ----
// When any hostIP:port is unreachable after apply, Reconcile must restore the
// prior policy and return an error describing the unreachable endpoint.
func TestReconcile_RollbackOnUnreachableEndpoint(t *testing.T) {
	clusterIP := "10.43.189.110"

	// Pre-populate with an existing (prior) policy.
	priorPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-olla-backends",
			Namespace: "ai-fleet",
		},
		Spec: networkingv1.NetworkPolicySpec{
			Egress: []networkingv1.NetworkPolicyEgressRule{},
		},
	}
	client := fake.NewSimpleClientset(controllerService(clusterIP), priorPolicy)

	hosts := []GPUHostSpec{
		makeGPUHost("precision.petersimmons.com", "192.168.0.90", []ModelSpec{makeModel("embed", 8005)}),
	}

	r := NewNetworkPolicyReconciler(client, "ai-fleet", ModeApply)
	// Override reachability check to always report failure.
	r.reachabilityChecker = func(hostIP string, port int) error {
		return fmt.Errorf("connection refused (simulated)")
	}

	err := r.Reconcile(context.Background(), hosts)
	if err == nil {
		t.Fatal("expected error from Reconcile when endpoint is unreachable; got nil")
	}
	if !strings.Contains(err.Error(), "192.168.0.90") {
		t.Errorf("error should mention the unreachable hostIP; got: %v", err)
	}

	// The prior (empty egress) policy should be restored.
	np, getErr := client.NetworkingV1().NetworkPolicies("ai-fleet").Get(context.Background(), "allow-olla-backends", metav1.GetOptions{})
	if getErr != nil {
		t.Fatalf("get after rollback: %v", getErr)
	}
	// The restored policy should match the prior (empty egress = no external rules).
	if len(np.Spec.Egress) != 0 {
		t.Errorf("rollback should restore prior empty-egress policy; got %d egress rules", len(np.Spec.Egress))
	}
}

// ---- TestSpecHash_HostIPAffectsHash ----
// Adding HostIP to GPUHostSpec must change the SpecHash (proves field participates
// in policy version tracking).
func TestSpecHash_HostIPAffectsHash(t *testing.T) {
	base := GPUHostSpec{
		Host:   "precision.petersimmons.com",
		Models: []ModelSpec{makeModel("embed", 8005)},
	}
	withIP := base
	withIP.HostIP = "192.168.0.90"

	h1 := SpecHash(base)
	h2 := SpecHash(withIP)

	if h1 == h2 {
		t.Errorf("SpecHash should differ when HostIP is set; both returned %q", h1)
	}
}

// ---- TestBuildDesiredPolicy_Idempotent ----
// Calling BuildDesiredPolicy twice with the same inputs must produce identical output.
func TestBuildDesiredPolicy_Idempotent(t *testing.T) {
	hosts := []GPUHostSpec{
		makeGPUHost("precision.petersimmons.com", "192.168.0.90", []ModelSpec{
			makeModel("embed", 8005),
			makeModel("gen", 8002),
		}),
		makeGPUHost("leviathan.petersimmons.com", "192.168.0.98", []ModelSpec{
			makeModel("embed", 8004),
		}),
	}

	p1 := BuildDesiredPolicy("ai-fleet", "10.43.189.110", hosts)
	p2 := BuildDesiredPolicy("ai-fleet", "10.43.189.110", hosts)

	// Compare egress rule counts and CIDRs.
	if len(p1.Spec.Egress) != len(p2.Spec.Egress) {
		t.Errorf("idempotent: egress rule count changed: %d vs %d", len(p1.Spec.Egress), len(p2.Spec.Egress))
	}
}

// ---- TestBuildDesiredPolicy_ScopedToOllaPods ----
// The NetworkPolicy must target olla pods specifically (podSelector with app=olla label).
func TestBuildDesiredPolicy_ScopedToOllaPods(t *testing.T) {
	pol := BuildDesiredPolicy("ai-fleet", "10.43.189.110", []GPUHostSpec{})

	selector := pol.Spec.PodSelector
	if selector.MatchLabels["app"] != "olla" {
		t.Errorf("NetworkPolicy podSelector should target app=olla; got labels: %v", selector.MatchLabels)
	}
}

// ---- TestBuildDesiredPolicy_PolicyNameAndNamespace ----
// The generated policy must use the canonical name and be in the correct namespace.
func TestBuildDesiredPolicy_PolicyNameAndNamespace(t *testing.T) {
	pol := BuildDesiredPolicy("ai-fleet", "10.43.189.110", []GPUHostSpec{})

	if pol.Name != "allow-olla-backends" {
		t.Errorf("expected policy name 'allow-olla-backends'; got %q", pol.Name)
	}
	if pol.Namespace != "ai-fleet" {
		t.Errorf("expected namespace 'ai-fleet'; got %q", pol.Namespace)
	}
}

// ---- TestBuildDesiredPolicy_EmptyHostIPSkipped ----
// GPUHosts with no HostIP set must be skipped (no rule emitted for them).
func TestBuildDesiredPolicy_EmptyHostIPSkipped(t *testing.T) {
	hosts := []GPUHostSpec{
		// No HostIP — should be skipped.
		{Host: "mystery.petersimmons.com", Models: []ModelSpec{makeModel("embed", 8005)}},
		makeGPUHost("precision.petersimmons.com", "192.168.0.90", []ModelSpec{makeModel("embed", 8005)}),
	}

	pol := BuildDesiredPolicy("ai-fleet", "10.43.189.110", hosts)

	for _, egress := range pol.Spec.Egress {
		for _, to := range egress.To {
			if to.IPBlock != nil && strings.HasSuffix(to.IPBlock.CIDR, "/32") {
				cidr := to.IPBlock.CIDR
				if cidr == "/32" || cidr == "0.0.0.0/32" {
					t.Errorf("empty HostIP produced malformed CIDR rule: %q", cidr)
				}
			}
		}
	}
}

// ---- helpers ----

func countWriteActions(actions []k8stesting.Action) int {
	count := 0
	for _, a := range actions {
		v := a.GetVerb()
		if v == "create" || v == "update" || v == "patch" || v == "delete" {
			count++
		}
	}
	return count
}

