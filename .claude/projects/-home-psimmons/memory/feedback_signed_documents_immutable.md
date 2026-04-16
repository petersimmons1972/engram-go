---
name: Signed Documents Are Immutable
description: Cryptographically signed published documents must never be altered after publication, even for fixes
type: feedback
Category: feedback
---

Once a document is cryptographically signed (Ed25519) and published to www.petersimmons.com, it must NEVER be edited again. This is absolute.

**Why:** Signed documents are historically relevant artifacts. The signature is a permanent attestation tied to specific content bytes at a specific moment. Editing the document — even for typos or improvements — invalidates the signature, breaks the historical record, and violates the trust contract with anyone who has saved or referenced the original.

**How to apply:**
- Never edit a signed published HTML in `briefings/`, `/projects/www/k8s/locked-shields/`, or any signed artifact location.
- If a fix is needed, create a NEW document with a versioned filename (e.g., `red-team-prediction-v2.html` or `red-team-prediction-2026-04-08.html`) and sign that.
- The original signed document continues to exist alongside the new one.
- In Locked Shields specifically: `red-team-prediction.html` published 2026-04-07 with content SHA `b22f3ce90c01b12f8a1f4adb8ca1e9477a36329e06ad49481a61cf3c450bd335` is immutable. Future predictions are new files.
- Apply the same rule to any other signed deliverable: signed Substack illustrations, signed reports, signed analysis documents.

**The test:** Before editing any HTML/PDF/SVG file in a published location, check whether a `.sig` sidecar exists. If it does, do not edit. Create a new versioned file instead.
