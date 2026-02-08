#!/usr/bin/env bash

# This script creates a new release by:
# 1. Updating CHANGELOG.md with the new version
# 2. Creating a git tag
# 3. Pushing the tag (which triggers GitHub Actions to build and release)

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Ensure we are in a git repository
if ! git rev-parse --is-inside-work-tree > /dev/null 2>&1; then
  echo -e "${RED}Error: Not a git repository.${NC}"
  exit 1
fi

# Ensure we are on main branch
CURRENT_BRANCH=$(git branch --show-current)
if [ "$CURRENT_BRANCH" != "main" ]; then
  echo -e "${RED}Error: Must be on main branch to release. Currently on: $CURRENT_BRANCH${NC}"
  exit 1
fi

# Ensure working directory is clean
if ! git diff-index --quiet HEAD --; then
  echo -e "${RED}Error: Working directory is not clean. Commit or stash changes first.${NC}"
  exit 1
fi

# Get current version
CURRENT_VERSION=$(git tag --sort=-v:refname | head -1)
echo -e "${GREEN}Current version: ${CURRENT_VERSION}${NC}"

# Parse version components
if [[ $CURRENT_VERSION =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
  MAJOR="${BASH_REMATCH[1]}"
  MINOR="${BASH_REMATCH[2]}"
  PATCH="${BASH_REMATCH[3]}"
else
  echo -e "${RED}Error: Could not parse current version: $CURRENT_VERSION${NC}"
  exit 1
fi

# Determine new version based on argument
case "${1:-minor}" in
  major)
    NEW_VERSION="v$((MAJOR + 1)).0.0"
    ;;
  minor)
    NEW_VERSION="v${MAJOR}.$((MINOR + 1)).0"
    ;;
  patch)
    NEW_VERSION="v${MAJOR}.${MINOR}.$((PATCH + 1))"
    ;;
  *)
    # Allow explicit version
    if [[ $1 =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      NEW_VERSION="$1"
    else
      echo -e "${RED}Usage: $0 [major|minor|patch|vX.Y.Z]${NC}"
      echo "  major - Bump major version (breaking changes)"
      echo "  minor - Bump minor version (new features)"
      echo "  patch - Bump patch version (bug fixes)"
      echo "  vX.Y.Z - Set explicit version"
      exit 1
    fi
    ;;
esac

echo -e "${GREEN}New version: ${NEW_VERSION}${NC}"

# Check if tag already exists
if git tag | grep -q "^${NEW_VERSION}$"; then
  echo -e "${RED}Error: Tag ${NEW_VERSION} already exists.${NC}"
  exit 1
fi

# Get today's date
TODAY=$(date +%Y-%m-%d)

# Update CHANGELOG.md - replace [Unreleased] with new version
echo -e "${YELLOW}Updating CHANGELOG.md...${NC}"
sed -i "s/## \[Unreleased\]/## [Unreleased]\n\n## [${NEW_VERSION#v}] - ${TODAY}/" CHANGELOG.md

# Commit the changelog update
git add CHANGELOG.md
git commit -m "chore: release ${NEW_VERSION}"

# Create annotated tag
echo -e "${YELLOW}Creating tag ${NEW_VERSION}...${NC}"
git tag -a "${NEW_VERSION}" -m "Release ${NEW_VERSION}"

# Push commit and tag
echo -e "${YELLOW}Pushing to origin...${NC}"
git push origin main
git push origin "${NEW_VERSION}"

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Release ${NEW_VERSION} created successfully!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo "GitHub Actions will now build and publish the release."
echo "Check progress at: https://github.com/imamik/k8zner/actions"
echo ""
echo "Release page will be at: https://github.com/imamik/k8zner/releases/tag/${NEW_VERSION}"
