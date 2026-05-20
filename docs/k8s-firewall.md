# Homelab K8s Egress Firewall

The homelab k8s cluster uses standard `networking.k8s.io/v1` NetworkPolicy resources. Most namespaces have default-deny-all egress with explicit allow rules per destination IP+port.

## Diagnosis order when a pod cannot reach an external host

1. `kubectl get networkpolicy -n <namespace>` — does an allow rule exist for the destination IP and port?
2. If not, that is the cause. Packets are dropped at cluster egress with no log on the destination side.
3. Confirm asymmetry: inbound external→cluster works but outbound cluster→external fails.

## Backend hosts (2026-05-20)

| Host | IP | Notes |
|------|-----|-------|
| leviathan.petersimmons.com | 192.168.0.98 | RX 7900 XT, hosts llamacpp + Infinity |
| precision.petersimmons.com | 192.168.0.90 | W6800 + MI-50, hosts bge-m3 + ollama |
| oblivion.petersimmons.com | 192.168.0.211 | GB10 Grace-Blackwell, hosts vLLM |

## Allow-rule template

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-<app>-<destination>
  namespace: <namespace>
spec:
  podSelector:
    matchLabels:
      app: <app>
  policyTypes: [Egress]
  egress:
  - ports:
    - {port: 53, protocol: TCP}
    - {port: 53, protocol: UDP}
  - ports:
    - {port: <port>, protocol: TCP}
    to:
    - ipBlock: {cidr: <IP>/32}
```

## Reference issue

petersimmons1972/instinct#10 — Olla in ai-fleet could not reach leviathan, no NetworkPolicy existed, packets silently dropped.
