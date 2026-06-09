#!/bin/bash
# Expunge secrets from git history
# WARNING: This rewrites git history and requires force push

set -euo pipefail

printf '🔐 Git Secret Expungement Tool\n'
printf '==============================\n\n'
printf 'WARNING: This script rewrites git history.\n'
printf 'BACKUP YOUR REPO FIRST!\n\n'

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    printf 'ERROR: required command "%s" not found in PATH.\n' "$1" >&2
    exit 1
  }
}

run_filter_repo() {
  local pattern="$1"
  shift

  require_cmd git-filter-repo
  printf "\nRemoving files matching '%s' from all commits...\n" "$pattern"
  git-filter-repo --force --path-glob "$pattern" --invert-paths --partial
}

run_scan() {
  require_cmd gitleaks
  printf '\nScanning git history for likely secret patterns...\n\n'
  gitleaks detect --source . --log-opts='--all' --no-banner
}

printf '1) Create a backup reference (recommended)\n'
BACKUP_DIR="/tmp/git-backup-$(date +%s)"
cp -r .git "$BACKUP_DIR.git"
printf '   Backup: %s.git\n\n' "$BACKUP_DIR"

printf '2) What secrets need to be removed?\n\n'
printf 'Options:\n'
printf '  1) Remove all *_api_key files\n'
printf '  2) Remove all db_password files\n'
printf '  3) Remove all .env* files\n'
printf '  4) Remove specific file pattern (you specify)\n'
printf '  5) Scan and show what patterns exist (no changes)\n\n'

read -rp 'Choice (1-5): ' choice

case "$choice" in
  5)
    run_scan
    exit 0
    ;;
  1)
    run_filter_repo '*_api_key'
    ;;
  2)
    run_filter_repo '*db_password*'
    ;;
  3)
    run_filter_repo '.env*'
    ;;
  4)
    read -rp "Enter file pattern to remove (e.g., '*_secret'): " pattern
    if [ -z "$pattern" ]; then
      echo 'Pattern cannot be empty'
      exit 1
    fi
    run_filter_repo "$pattern"
    ;;
  *)
    echo 'Invalid choice'
    exit 1
    ;;
esac

printf '\n3) Cleaning up git refs...\n'
git reflog expire --expire=now --all

git gc --prune=now --aggressive

printf '\n4)\n'
printf '   a) Verify changes look correct:\n'
printf '      git log --oneline -20\n'
printf '      git show HEAD\n\n'
printf '   b) If correct, force push to GitHub:\n'
printf '      git push origin --all --force-with-lease\n'
printf '      git push origin --tags --force-with-lease\n\n'
printf '   c) If something went wrong, restore backup:\n'
printf '      rm -rf .git\n'
printf '      cp -r %s.git .git\n\n' "$BACKUP_DIR"
printf '5) Tell your team to:\n'
printf '   - Delete their local clones\n'
printf '   - Re-clone from the cleaned repo\n'
printf '   - Do NOT merge old branches\n'
