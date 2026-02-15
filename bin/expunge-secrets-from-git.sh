#!/bin/bash
# Expunge secrets from git history
# WARNING: This rewrites git history and requires force push

set -e

echo "🔐 Git Secret Expungement Tool"
echo "=============================="
echo ""
echo "WARNING: This script rewrites git history."
echo "BACKUP YOUR REPO FIRST!"
echo ""

# Backup
echo "1️⃣  Creating backup..."
BACKUP_DIR="/tmp/git-backup-$(date +%s)"
cp -r .git "$BACKUP_DIR.git"
echo "   Backup: $BACKUP_DIR.git"
echo ""

# Ask user what to expunge
echo "2️⃣  What secrets need to be removed?"
echo ""
echo "Options:"
echo "  1) Remove all *_api_key files"
echo "  2) Remove all db_password files"
echo "  3) Remove all .env* files"
echo "  4) Remove specific file pattern (you specify)"
echo "  5) Scan and show what patterns exist (no changes)"
echo ""

read -p "Choice (1-5): " choice

case $choice in
    5)
        echo ""
        echo "Scanning git history for secret patterns..."
        echo ""
        git log --all -p | grep -E '(password|api[_-]?key|secret|token|bearer|oauth)' -i | head -50
        echo ""
        exit 0
        ;;
    1)
        echo ""
        echo "Removing *_api_key files from all commits..."
        git filter-branch -f --tree-filter 'find . -name "*_api_key" -type f -delete' -- --all
        ;;
    2)
        echo ""
        echo "Removing db_password files from all commits..."
        git filter-branch -f --tree-filter 'find . -name "*db_password*" -type f -delete' -- --all
        ;;
    3)
        echo ""
        echo "Removing .env* files from all commits..."
        git filter-branch -f --tree-filter 'find . -name ".env*" -type f -delete' -- --all
        ;;
    4)
        read -p "Enter file pattern to remove (e.g., '*_secret'): " pattern
        echo ""
        echo "Removing files matching '$pattern' from all commits..."
        git filter-branch -f --tree-filter "find . -name \"$pattern\" -type f -delete" -- --all
        ;;
    *)
        echo "Invalid choice"
        exit 1
        ;;
esac

echo ""
echo "3️⃣  Cleaning up git refs..."
rm -rf .git/refs/original/
git reflog expire --expire=now --all
git gc --prune=now --aggressive

echo ""
echo "4️⃣  ⚠️  NEXT STEPS:"
echo ""
echo "   a) Verify changes look correct:"
echo "      git log --oneline -20"
echo "      git show HEAD"
echo ""
echo "   b) If correct, force push to GitHub:"
echo "      git push origin --all --force-with-lease"
echo "      git push origin --tags --force-with-lease"
echo ""
echo "   c) If something went wrong, restore backup:"
echo "      rm -rf .git"
echo "      cp -r $BACKUP_DIR.git .git"
echo ""
echo "5️⃣  Tell your team to:"
echo "   - Delete their local clones"
echo "   - Re-clone from the cleaned repo"
echo "   - Do NOT merge old branches"
echo ""
