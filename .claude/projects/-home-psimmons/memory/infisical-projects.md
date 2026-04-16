---
name: Infisical Project IDs
description: Project IDs for infisical CLI -- required for --projectId flag
type: reference
originSessionId: 4618d425-182d-44d5-a54e-a28ed6df76c7
---
## Homelab Project

- **Project ID**: `f49c5b01-4bd1-4883-afbd-51c1fef53a2f`
- **Slug**: `homelab-jz5w`
- **Default env**: `prod`
- **Wrapper**: `infisical-homelab` (pre-configured, no flags needed)

## Usage

```bash
# Store a secret
infisical-homelab secrets set MY_KEY=myvalue

# Read a secret
infisical-homelab secrets get MY_KEY

# List all secrets
infisical-homelab secrets
```

Or directly:
```bash
infisical secrets set KEY=value --projectId=f49c5b01-4bd1-4883-afbd-51c1fef53a2f --env=prod
```
