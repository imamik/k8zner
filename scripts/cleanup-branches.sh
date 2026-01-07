#!/usr/bin/env bash

# This script removes all local and remote branches except for 'main'.

set -e

# Ensure we are in a git repository
if ! git rev-parse --is-inside-work-tree > /dev/null 2>&1; then
  echo "Error: Not a git repository."
  exit 1
fi

# Switch to main branch
echo "Switching to main branch..."
git checkout main
git pull origin main

# Prune stale remote-tracking branches
echo "Pruning stale remote-tracking branches..."
git fetch --prune

# Delete local branches except main
echo "Deleting local branches..."
git branch | grep -v "main" | xargs -I {} git branch -D {}

# Delete remote branches except main
echo "Deleting remote branches from origin..."
git branch -r | grep origin | grep -v ">" | grep -v "main" | cut -d/ -f2- | xargs -I {} git push origin --delete {}

echo "Cleanup complete. Only 'main' remains."
