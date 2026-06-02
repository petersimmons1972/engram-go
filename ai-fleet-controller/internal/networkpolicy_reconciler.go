package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"time"

	networkingv1 "k8s.io/api/networking/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

// ReconcilerMode controls whether the reconciler applies changes to the cluster
// or only logs the diff.
type ReconcilerMode string

const (
	// ModeRecommend (default) computes the desired NetworkPolicy and logs the
	// diff but makes zero Kubernetes API write calls. Safe to run at all times.
	ModeRecommend ReconcilerMode = "recommend"

	// ModeApply writes the desired NetworkPolicy to the cluster. Requires the
	// NETPOL_RECONCILER_MODE=apply env var to be set by the operator.
	ModeApply ReconcilerMode = "apply"

	// managedPolicyName is the canonical name of the NetworkPolicy this
	// reconciler manages. Matches the hand-maintained policy it replaces.
	managedPolicyName = "allow-olla-backends"

	// dialTimeout is the TCP dial timeout used by VerifyEndpointReachability.
	dialTimeout = 5 * time.Second
)

// ReachabilityChecker is a function that checks whether a hostIP:port pair is
// reachable over TCP. Replaceable in tests to avoid real network dials.
type ReachabilityChecker func(hostIP string, port int) error

// NetworkPolicyReconciler reconciles the allow-olla-backends NetworkPolicy as a
// derived artifact of the registered GPUHost endpoint set. It follows the
// OllaConfigReconciler pattern: struct + constructor + Reconcile entry.
type NetworkPolicyReconciler struct {
	client    kubernetes.Interface
	namespace string
	mode      ReconcilerMode

	// skipReachabilityCheck disables TCP dial checks. Set to true in unit tests.
	skipReachabilityCheck bool

	// reachabilityChecker is the function used to verify endpoint reachability.
	// Defaults to tcpDial. Overridable in tests.
	reachabilityChecker ReachabilityChecker
}

// NewNetworkPolicyReconciler creates a new reconciler. mode must be ModeRecommend
// or ModeApply. The caller is responsible for obtaining the kubernetes.Interface
// (use TypedClient() from K8sClient in production).
func NewNetworkPolicyReconciler(client kubernetes.Interface, namespace string, mode ReconcilerMode) *NetworkPolicyReconciler {
	return &NetworkPolicyReconciler{
		client:              client,
		namespace:           namespace,
		mode:                mode,
		reachabilityChecker: tcpDial,
	}
}

// Reconcile is the main entry point. It:
//  1. Fetches the controller ClusterIP dynamically.
//  2. Calls BuildDesiredPolicy to compute the desired state.
//  3. In recommend mode: logs the diff only (no writes).
//  4. In apply mode: creates or patches the NetworkPolicy, then runs
//     VerifyEndpointReachability. On failure, RollbackPolicy restores prior state.
func (r *NetworkPolicyReconciler) Reconcile(ctx context.Context, hosts []GPUHostSpec) error {
	clusterIP, err := r.fetchControllerClusterIP(ctx)
	if err != nil {
		return fmt.Errorf("fetch controller ClusterIP: %w", err)
	}

	desired := BuildDesiredPolicy(r.namespace, clusterIP, hosts)

	switch r.mode {
	case ModeRecommend:
		return r.reconcileRecommend(ctx, desired)
	case ModeApply:
		return r.reconcileApply(ctx, desired, hosts)
	default:
		return fmt.Errorf("unknown reconciler mode: %q", r.mode)
	}
}

// reconcileRecommend computes the diff and logs it; makes no API writes.
func (r *NetworkPolicyReconciler) reconcileRecommend(ctx context.Context, desired *networkingv1.NetworkPolicy) error {
	live, err := r.client.NetworkingV1().NetworkPolicies(r.namespace).Get(ctx, managedPolicyName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		slog.Info("netpol-reconciler recommend: policy absent — would create",
			"policy", managedPolicyName,
			"namespace", r.namespace,
			"egress_rules", len(desired.Spec.Egress),
		)
		return nil
	}
	if err != nil {
		return fmt.Errorf("get live policy: %w", err)
	}

	liveJSON, _ := json.Marshal(live.Spec.Egress)
	desiredJSON, _ := json.Marshal(desired.Spec.Egress)
	if string(liveJSON) == string(desiredJSON) {
		slog.Info("netpol-reconciler recommend: no diff — policy is up to date", "policy", managedPolicyName)
		return nil
	}

	slog.Info("netpol-reconciler recommend: diff detected — would patch",
		"policy", managedPolicyName,
		"live_egress_rules", len(live.Spec.Egress),
		"desired_egress_rules", len(desired.Spec.Egress),
	)
	return nil
}

// reconcileApply creates or patches the NetworkPolicy and verifies reachability.
func (r *NetworkPolicyReconciler) reconcileApply(ctx context.Context, desired *networkingv1.NetworkPolicy, hosts []GPUHostSpec) error {
	live, err := r.client.NetworkingV1().NetworkPolicies(r.namespace).Get(ctx, managedPolicyName, metav1.GetOptions{})

	if errors.IsNotFound(err) {
		// Policy absent: create it.
		if _, createErr := r.client.NetworkingV1().NetworkPolicies(r.namespace).Create(ctx, desired, metav1.CreateOptions{}); createErr != nil {
			return fmt.Errorf("create NetworkPolicy: %w", createErr)
		}
		slog.Info("netpol-reconciler apply: created policy", "policy", managedPolicyName)
		return r.verifyAndRollback(ctx, nil, desired, hosts)
	}
	if err != nil {
		return fmt.Errorf("get live policy: %w", err)
	}

	// Policy exists: check for diff.
	liveJSON, _ := json.Marshal(live.Spec.Egress)
	desiredJSON, _ := json.Marshal(desired.Spec.Egress)
	if string(liveJSON) == string(desiredJSON) {
		slog.Info("netpol-reconciler apply: no diff — skipping patch", "policy", managedPolicyName)
		return nil
	}

	// Snapshot prior state for rollback.
	prior := live.DeepCopy()

	// Patch: replace spec.
	updated := live.DeepCopy()
	updated.Spec = desired.Spec
	if _, updateErr := r.client.NetworkingV1().NetworkPolicies(r.namespace).Update(ctx, updated, metav1.UpdateOptions{}); updateErr != nil {
		return fmt.Errorf("patch NetworkPolicy: %w", updateErr)
	}
	slog.Info("netpol-reconciler apply: patched policy",
		"policy", managedPolicyName,
		"live_egress_rules", len(live.Spec.Egress),
		"new_egress_rules", len(desired.Spec.Egress),
	)
	return r.verifyAndRollback(ctx, prior, desired, hosts)
}

// verifyAndRollback runs reachability checks after an apply. On any failure it
// calls RollbackPolicy and returns a descriptive error.
func (r *NetworkPolicyReconciler) verifyAndRollback(ctx context.Context, prior *networkingv1.NetworkPolicy, desired *networkingv1.NetworkPolicy, hosts []GPUHostSpec) error {
	if r.skipReachabilityCheck {
		return nil
	}
	if err := r.VerifyEndpointReachability(hosts); err != nil {
		slog.Error("netpol-reconciler: endpoint unreachable after apply — rolling back",
			"policy", managedPolicyName,
			"err", err,
		)
		if rollbackErr := r.RollbackPolicy(ctx, prior); rollbackErr != nil {
			return fmt.Errorf("endpoint unreachable (%w) AND rollback failed: %v", err, rollbackErr)
		}
		return fmt.Errorf("rolled back NetworkPolicy after unreachable endpoint: %w", err)
	}
	return nil
}

// VerifyEndpointReachability TCP-dials each hostIP:port from the GPUHost list
// (5 s timeout). Returns the first unreachable endpoint as an error.
func (r *NetworkPolicyReconciler) VerifyEndpointReachability(hosts []GPUHostSpec) error {
	for _, h := range hosts {
		if h.HostIP == "" {
			continue
		}
		for _, m := range h.Models {
			if err := r.reachabilityChecker(h.HostIP, m.Port); err != nil {
				return fmt.Errorf("endpoint %s:%d unreachable: %w", h.HostIP, m.Port, err)
			}
		}
	}
	return nil
}

// RollbackPolicy restores a prior NetworkPolicy. If prior is nil (no prior
// existed), the policy is deleted.
func (r *NetworkPolicyReconciler) RollbackPolicy(ctx context.Context, prior *networkingv1.NetworkPolicy) error {
	if prior == nil {
		// No prior policy: delete the one we just created.
		err := r.client.NetworkingV1().NetworkPolicies(r.namespace).Delete(ctx, managedPolicyName, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("delete policy during rollback: %w", err)
		}
		return nil
	}

	// Restore prior state via update.
	live, err := r.client.NetworkingV1().NetworkPolicies(r.namespace).Get(ctx, managedPolicyName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get policy for rollback: %w", err)
	}
	live.Spec = prior.Spec
	if _, err := r.client.NetworkingV1().NetworkPolicies(r.namespace).Update(ctx, live, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("restore prior policy: %w", err)
	}
	slog.Info("netpol-reconciler: rolled back to prior policy", "policy", managedPolicyName)
	return nil
}

// fetchControllerClusterIP fetches the ClusterIP of the ai-fleet-controller
// Service dynamically. Never hardcodes the IP (ClusterIPs change on re-deploy).
func (r *NetworkPolicyReconciler) fetchControllerClusterIP(ctx context.Context) (string, error) {
	svc, err := r.client.CoreV1().Services(r.namespace).Get(ctx, "ai-fleet-controller", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get ai-fleet-controller service: %w", err)
	}
	if svc.Spec.ClusterIP == "" || svc.Spec.ClusterIP == "None" {
		return "", fmt.Errorf("ai-fleet-controller service has no ClusterIP")
	}
	return svc.Spec.ClusterIP, nil
}

// BuildDesiredPolicy is a pure function: given a namespace, controller ClusterIP,
// and a set of GPUHostSpecs, it returns the desired NetworkPolicy object.
//
// Rules emitted:
//  1. DNS egress: port 53 (TCP+UDP), no `to:` selector — allows any DNS server.
//  2. Per GPUHost with HostIP set: one ipBlock /32 egress rule per model port.
//     All ports for a single host are collected into one egress rule (one `to:` block).
//  3. Controller ClusterIP: ipBlock /32 on port 80 (pre-DNAT flannel constraint —
//     NOT podSelector).
//
// The function never emits a non-DNS rule with prefix length shorter than /32.
func BuildDesiredPolicy(namespace, controllerClusterIP string, hosts []GPUHostSpec) *networkingv1.NetworkPolicy {
	tcp := corev1.ProtocolTCP
	udp := corev1.ProtocolUDP

	var egress []networkingv1.NetworkPolicyEgressRule

	// Rule 1: DNS (port 53, no to: — any destination).
	port53 := intstr.FromInt32(53)
	egress = append(egress, networkingv1.NetworkPolicyEgressRule{
		Ports: []networkingv1.NetworkPolicyPort{
			{Protocol: &tcp, Port: &port53},
			{Protocol: &udp, Port: &port53},
		},
		// No To: — DNS must be reachable to any server.
	})

	// Rule 2: Per-host egress. One rule per host, all ports in a single `to:` block.
	for _, h := range hosts {
		if h.HostIP == "" {
			slog.Warn("netpol-reconciler: skipping GPUHost with no HostIP", "host", h.Host)
			continue
		}
		if len(h.Models) == 0 {
			continue
		}

		var ports []networkingv1.NetworkPolicyPort
		for _, m := range h.Models {
			p := intstr.FromInt32(int32(m.Port))
			ports = append(ports, networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
				Port:     &p,
			})
		}

		cidr := h.HostIP + "/32"
		egress = append(egress, networkingv1.NetworkPolicyEgressRule{
			Ports: ports,
			To: []networkingv1.NetworkPolicyPeer{
				{
					IPBlock: &networkingv1.IPBlock{CIDR: cidr},
				},
			},
		})
	}

	// Rule 3: Controller ClusterIP via ipBlock /32 on port 80 (NOT podSelector —
	// flannel evaluates pre-DNAT so a podSelector would never match the ClusterIP).
	if controllerClusterIP != "" {
		port80 := intstr.FromInt32(80)
		controllerCIDR := controllerClusterIP + "/32"
		egress = append(egress, networkingv1.NetworkPolicyEgressRule{
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: &tcp, Port: &port80},
			},
			To: []networkingv1.NetworkPolicyPeer{
				{
					IPBlock: &networkingv1.IPBlock{CIDR: controllerCIDR},
				},
			},
		})
	}

	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedPolicyName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":        "olla",
				"managed-by": "ai-fleet-controller",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			// Scoped to olla pods only.
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "olla"},
			},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress:      egress,
		},
	}
}

// tcpDial is the default ReachabilityChecker: opens a TCP connection to hostIP:port.
func tcpDial(hostIP string, port int) error {
	addr := net.JoinHostPort(hostIP, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}
