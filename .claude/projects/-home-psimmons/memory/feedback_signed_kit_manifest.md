---
name: Signed Kit Manifest Pattern
description: When you sign published documents, also sign a manifest of the code that implements them — supply chain discipline matching content discipline
type: feedback
Category: pattern
---

When a project publishes cryptographically signed documents (Ed25519, wax seal, etc.), extend the same discipline to the code that implements the content. Sign a single manifest file containing SHA-256 of every file in the kit, using the same key. Ship a verify script so operators can check integrity before running anything.

**Why:** A document that says "see Bluesteel module X for the countermeasure code" is trustworthy only if the reader can verify that module X on their machine is the same module X the document was written against. Signing only the document leaves a supply-chain gap: an attacker who can tamper with the repo can change what module X does while leaving the signed document intact. Manifest signing closes that gap with one signature covering all files. One key, one verification path, uniform discipline.

**How to apply:**
- Build a `sign_kit.py` script that walks the kit directory, collects SHA-256 for each included file, writes a plain-text `manifest.txt`, and signs it with the same Ed25519 key used for document signing.
- Include: source modules, configs, docs, SIEM queries, operational tools, README.
- Exclude: transcripts/runtime output, user-supplied binaries (Sysmon.exe), the manifest itself, __pycache__, .git.
- Manifest format: one line per file as `<sha256>  <relative_path>`. Comments begin with `#`. Ordering deterministic (sort by path) so manifests are reproducible.
- Write a `verify-kit.ps1` (or equivalent) that:
  1. Re-reads the manifest and signature
  2. Recomputes manifest SHA-256 and compares to signature payload
  3. Verifies Ed25519 signature via `openssl pkeyutl -verify -rawin` (or native if available)
  4. Re-hashes each file listed in the manifest and compares to recorded hash
  5. Exits non-zero on any mismatch
- Operators run verify-kit before any Apply/destructive operation.
- Fingerprint published once on the project landing page so one key covers everything.
- Reference implementation: `tools/signing/sign_bluesteel.py` + `tools/bluesteel/manifest.txt` + `tools/bluesteel/verify-bluesteel.ps1` in locked-shields. 25 files covered. Openssl fallback handles PowerShell versions without native Ed25519.

**When to re-sign:**
- After any code change to any included file
- After adding new files to the include_globs
- After a commit that changes the kit semantic meaning
- Automate this in CI if the kit is stable; manual for low-volume projects

**Do not:**
- Sign individual files separately — one signature for the manifest is simpler and cheaper to verify
- Include the manifest file itself in the manifest (circular)
- Include runtime artifacts (transcripts, logs) — they change per-run
- Use a different key for code vs documents — same key, same fingerprint, same trust chain
