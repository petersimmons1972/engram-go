#!/usr/bin/env bash
# ansible-drift-notify.sh — run ansible --check, alert on drift
set -euo pipefail

PLAYBOOK="${ANSIBLE_DRIFT_PLAYBOOK:-${HOME}/projects/homelab-config/ansible/playbooks/registry-hosts.yaml}"
LOG_DIR="${HOME}/.local/state/ansible"
LOG_FILE="${LOG_DIR}/drift-$(date -u +%Y%m%d).log"
REPO="petersimmons1972/homelab-config"

mkdir -p "$LOG_DIR"

# Run ansible in check mode, capture output and exit code
DIFF_OUTPUT=$(ansible-playbook --check --diff "$PLAYBOOK" 2>&1) || STATUS=$?
STATUS=${STATUS:-0}

printf '[%s] exit=%d\n%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$STATUS" "$DIFF_OUTPUT" >> "$LOG_FILE"

case $STATUS in
  0) echo "No drift detected." ;;
  2)
    # Drift found — post GitHub issue
    BODY="## Ansible drift detected\n\nPlaybook: \`${PLAYBOOK}\`\nTime: $(date -u)\n\n\`\`\`\n${DIFF_OUTPUT}\n\`\`\`"
    gh issue create --repo "$REPO" \
      --title "ops: ansible drift detected $(date -u +%Y-%m-%d)" \
      --body "$BODY" \
      --label "severity/serious" 2>>"$LOG_FILE" || true
    echo "Drift detected — GitHub issue filed."
    exit 2
    ;;
  *)
    echo "ansible-playbook error (exit $STATUS) — check $LOG_FILE"
    exit "$STATUS"
    ;;
esac
