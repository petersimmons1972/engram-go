---
Category: feedback
name: K8s deployment — discover existing infrastructure
description: Don't ask user to manually set up secrets/storage when K8s infrastructure already exists
type: feedback
---

**Rule:** Before asking the user to manually create K8s secrets or infrastructure, check what already exists in the cluster.

**Why:** User has existing K8s infrastructure (external-secrets, Infisical integration, ClusterSecretStores) that can be leveraged. Asking them to manually create secrets wastes their time when the automation is already in place.

**How to apply:**
1. When deploying to K8s, first check: `kubectl get secrets`, `kubectl get externalsecrets`, `kubectl get clustersecretstores`
2. Look for existing Infisical integration patterns (e.g., `infisical-clearwatch-*`, `infisical-job-search-*`)
3. Create ExternalSecret manifests that reference existing ClusterSecretStore names (not hardcoded ones)
4. Test image availability locally before asking user for help (`docker image ls`)
5. Troubleshoot K8s deployment issues proactively (volume mounts, imagePullPolicy, etc.)

**Example:** For CISO tracker, discovered existing Infisical integration → created ExternalSecret to auto-sync token from Infisical → deployment worked without manual secret creation.
