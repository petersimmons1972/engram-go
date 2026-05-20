package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var gpuHostGVR = schema.GroupVersionResource{
	Group:    "ai.petersimmons.com",
	Version:  "v1",
	Resource: "gpuhosts",
}

type K8sClient struct {
	dyn       dynamic.Interface
	namespace string
}

func NewK8sClient() (*K8sClient, error) {
	ns := os.Getenv("NAMESPACE")
	if ns == "" {
		ns = "ai-fleet"
	}

	// In-cluster config first, fall back to kubeconfig.
	cfg, err := rest.InClusterConfig()
	if err != nil {
		cfg, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		if err != nil {
			return nil, fmt.Errorf("build k8s config: %w", err)
		}
	}

	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("dynamic client: %w", err)
	}
	return &K8sClient{dyn: dyn, namespace: ns}, nil
}

// GetPolicy fetches the GPUHost whose spec.host matches hostname and returns a Policy.
func (c *K8sClient) GetPolicy(ctx context.Context, hostname string) (*Policy, error) {
	list, err := c.dyn.Resource(gpuHostGVR).Namespace(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list gpuhosts: %w", err)
	}

	for _, item := range list.Items {
		specRaw, ok := item.Object["spec"]
		if !ok {
			continue
		}
		b, _ := json.Marshal(specRaw)
		var spec GPUHostSpec
		if err := json.Unmarshal(b, &spec); err != nil {
			continue
		}
		if spec.Host == hostname {
			return &Policy{
				Hostname:      hostname,
				Spec:          spec,
				PolicyVersion: SpecHash(spec),
			}, nil
		}
	}
	return nil, nil // not found — caller returns 404
}

// ListPolicies returns all GPUHost policies in the namespace.
func (c *K8sClient) ListPolicies(ctx context.Context) ([]Policy, error) {
	list, err := c.dyn.Resource(gpuHostGVR).Namespace(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list gpuhosts: %w", err)
	}

	var out []Policy
	for _, item := range list.Items {
		specRaw, ok := item.Object["spec"]
		if !ok {
			continue
		}
		b, _ := json.Marshal(specRaw)
		var spec GPUHostSpec
		if err := json.Unmarshal(b, &spec); err != nil {
			continue
		}
		out = append(out, Policy{
			Hostname:      spec.Host,
			Spec:          spec,
			PolicyVersion: SpecHash(spec),
		})
	}
	return out, nil
}
