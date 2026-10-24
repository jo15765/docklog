#!/usr/bin/env sh

set -e
staged=$(git diff --cached --name-only 2>/dev/null || true)
if [ -z "$staged" ]; then
  exit 0
fi
echo "$staged" | grep -E '\.(db|db-wal|db-shm|db-journal)$|^data/|/\.env$|\.env\.' && {
  echo ""
  echo "Abort: staged files may contain MS SQL passwords, webhooks, or other secrets."
  echo "Unstage them (git reset HEAD -- <file>) and ensure .gitignore covers local data."
  exit 1
}
exit 0
