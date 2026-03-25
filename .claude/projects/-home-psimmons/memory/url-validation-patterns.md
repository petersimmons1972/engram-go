---
name: url-validation-patterns
description: Browser spoofing for URL validation — principle and code pointer
type: reference
Category: reference
---
# URL Validation Patterns

**Principle:** Mimic Windows 11 + Chrome (current stable) with full Sec-Fetch headers. Update Chrome version monthly via https://chromereleases.googleblog.com/

**403 = alive but protected.** Only 404, 410, 5xx, and connection failures are "dead."

**Implementation:** Grep for `User-Agent` in the active codebase — SIB repo is archived.

**Impact:** ~72% → ~92% citation validation score (Feb 2026).
