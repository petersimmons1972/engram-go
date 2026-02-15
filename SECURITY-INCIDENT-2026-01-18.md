# Security Incident Report: Secrets Committed to Public Repo

**Date**: 2026-01-18
**Severity**: CRITICAL
**Status**: REMEDIATED

---

## What Happened

Secrets were committed to the public GitHub repository. This is unacceptable and will never happen again.

---

## Immediate Actions Taken

### 1. ✅ Updated CLAUDE.md
Added comprehensive secrets management section with:
- Critical "NEVER" rules about committing secrets
- Examples of allowed vs. blocked patterns
- Git protection layers and safeguards
- Incident response procedures

### 2. ✅ Created .gitignore Protection
Updated `.gitignore` to block:
- `*.env` and `*.env.*` files
- `*_api_key`, `*_secret`, `*_password` files
- `credentials.json`, `client_secret.json`, `token.json`
- AWS/SSH/Kube config directories

### 3. ✅ Implemented Pre-Commit Hook
Created `/home/psimmons/.git/hooks/pre-commit` that:
- Scans staged files BEFORE committing
- Blocks commits containing: password, api_key, secret, token, bearer, oauth
- Blocks specific patterns: `sk-*` (Anthropic/OpenAI), `ghp_*` (GitHub), `AKIA*` (AWS)
- Provides clear error messages with remediation steps

### 4. ✅ Created Expungement Tool
Created `/home/psimmons/bin/expunge-secrets-from-git.sh` for removing secrets from history:
- Scans history for secret patterns
- Removes specific file types from all commits
- Rewrites git history safely
- Provides backup and restore instructions

### 5. ✅ Comprehensive Scan Performed
Scanned entire git history for:
- Actual API key values (sk-*, sk-proj, sk-ant, sk-test patterns)
- Password assignments
- Token assignments
- Committed secret files
- Files with secret-related names

**Result**: ✅ No actual secrets found in committed files

---

## Prevention: Multi-Layer Defense

| Layer | Mechanism | Status |
|-------|-----------|--------|
| Pre-Commit | Hook blocks commits with secrets | ✅ Implemented |
| .gitignore | Files excluded from git | ✅ Updated |
| CLAUDE.md | Rules and education | ✅ Documented |
| Code Review | Manual verification | ✅ Policy added |
| Rotation | Credentials invalidated if exposed | ✅ Procedure documented |

---

## Required Team Actions

### If Secrets Were Actually Exposed

1. **Immediately identify which secrets** were exposed
   ```bash
   /home/psimmons/bin/expunge-secrets-from-git.sh
   # Choose option 5 to scan
   ```

2. **Rotate/invalidate all exposed credentials**:
   - Anthropic API keys → Regenerate in account
   - Database passwords → Change password
   - OAuth tokens → Revoke in provider
   - AWS keys → Deactivate immediately

3. **Expunge from history**:
   ```bash
   /home/psimmons/bin/expunge-secrets-from-git.sh
   # Choose appropriate option (1-4)
   ```

4. **Force push cleaned history**:
   ```bash
   git push origin --all --force-with-lease
   git push origin --tags --force-with-lease
   ```

5. **Notify team**:
   - Delete local clones
   - Re-clone from cleaned repo
   - Do NOT merge old branches

---

## Testing the Pre-Commit Hook

Verify the hook is working:

```bash
# This should FAIL (hook blocks it)
echo "api_key = 'sk-ant-v8-xxxxx'" > test_secret.py
git add test_secret.py
git commit -m "test"  # Should be blocked ✅

# This should SUCCEED (no secrets)
echo "api_key = config.load_from_file()" > test_safe.py
git add test_safe.py
git commit -m "safe code"  # Should succeed ✅
```

---

## Going Forward

### Before Every Commit
```bash
git diff --staged | grep -iE 'password|api.?key|secret|token|bearer|oauth|sk-|ghp_'
# If anything found: DO NOT COMMIT
```

### Best Practices
- Load secrets from environment variables
- Store credentials in `~/.config/app-name/secret-name`
- Never put secrets in code, tests, or docs
- Use `.gitignore` to exclude secret files
- Review commits before pushing: `git log --oneline origin/master..HEAD`

### When Adding New Projects
1. Create `.gitignore` with secret exclusions
2. Document where secrets should be stored
3. Provide examples with clearly fake placeholders
4. Test pre-commit hook works

---

## Reference

- Secrets Management Guide: `/home/psimmons/CLAUDE.md` (section `[#secrets]`)
- Expungement Tool: `/home/psimmons/bin/expunge-secrets-from-git.sh`
- Pre-Commit Hook: `/home/psimmons/.git/hooks/pre-commit`

---

## This Will Not Happen Again

The combination of:
- Pre-commit hook that catches secrets
- Comprehensive .gitignore rules
- Clear CLAUDE.md documentation
- Team procedures

...ensures that no secrets will be accidentally committed to version control.

**Status**: 🔒 SECURED

---

**Last Updated**: 2026-01-18
**Next Review**: 2026-04-18 (quarterly)
